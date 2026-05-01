# Changelog
## v2026.05.01.1 — 2026-05-01

### feat(config): accept proxctl-style hosts map in linux.yaml (#48)

`hosts:` in linux.yaml now accepts both shapes:

1. **Legacy selector array** (existing): `hosts: [{selector: {role: [db]}, spec: {...}}]` → populates `Linux.Hosts []HostSpec`
2. **proxctl-style map** (NEW): `hosts: {dbx01: {packages: {install: [...]}}}` → populates `Linux.HostsByName map[string]Spec`

Custom `Linux.UnmarshalYAML` dispatches on the YAML node kind (sequence vs
mapping) and decodes into the appropriate field. Both fields are
consulted at runtime; only one is non-empty for a given manifest.

New `Linux.EffectiveSpec(hostname)` helper resolves the spec for a given
`--host` flag value: when the hostname matches a `HostsByName` key, the
host's spec is layered over top-level Linux fields (host wins); otherwise
top-level Linux is returned as-is. Slices and pointers replace rather than
merge — explicit operator override semantics.

Operators authoring stacks in the proxctl env.yaml convention (
`hosts: <hostname>: {...}` mirroring `spec.hypervisor.nodes`) can now
share that file shape between proxctl + linuxctl with no rewrite.

8 new unit tests; full test suite green (no regression).

Closes #48. Capstone follow-up #1 from infrastructure ADR-0109. Note: the
proxctl manifest schema also includes fields not yet on linuxctl Spec
(timezone, keymap, hostname, packages.present alias, services.enabled
list shape) — full schema reconciliation tracked separately.

## v2026.04.30.11 — 2026-04-30

### feat: dbx-host bundle preset (#44)

New conventions-library preset for dbx-control hosts (per ADR-0109 of itunified-io/infrastructure — agentic-AI hardening master plan Item 11 capstone).

- `pkg/presets/data/packages/dbx-host.yaml` — Docker + docker-compose-plugin, Vault agent, chrony, firewalld, cifs-utils, rsync, litestream, jq/curl/tar/xfsprogs, container-selinux
- `pkg/presets/data/users_groups/dbx-host.yaml` — `dbx` system user (UID/GID 60001) owning /var/lib/dbx
- `pkg/presets/data/sysctl/dbx-host.yaml` — vm.max_map_count=262144 (Tempo), net.core.somaxconn=4096, fs.inotify.max_user_watches=524288
- `pkg/presets/data/limits/dbx-host.yaml` — dbx ulimits (nofile 65536/131072, nproc 16384/32768, memlock unlimited)
- `pkg/presets/data/directories/dbx-host.yaml` — /var/lib/dbx hierarchy with container-matched UIDs (dbx 60001, tempo 10001, grafana 472)
- `pkg/presets/data/bundles/dbx-host.yaml` — composed bundle

Used by `infrastructure/stacks/dbx-control/linux.yaml` (`bundle_preset: dbx-host`). Discoverable via `linuxctl preset list` (Cluster A item 1 — unblocks live deployment of dbx01.itunified.io).

## v2026.04.30.9 — 2026-04-30

### fix: mkfs force flag per filesystem (#36)

DiskManager always emitted `mkfs.<fstype> -F <device>`. The `-F`
(uppercase) flag is correct for ext2/3/4 but invalid for xfs/btrfs,
which use `-f` (lowercase). Manifests with `fs: xfs` failed at the
mkfs step with `mkfs.xfs` printing usage help and exiting 1.

Fix: switch the flag based on fstype:
- ext2/3/4 → `-F` (force overwrite of existing fs)
- xfs/btrfs → `-f` (force)

Live-caught running /lab-up Phase C against ext3 — u01 LV created,
mkfs.xfs failed.


All notable changes to `linuxctl` are documented in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
CalVer (`vYYYY.MM.DD.TS`).

## v2026.04.30.8 — 2026-04-30

### feat: tag → device resolution for additional disks (#34)

`config.AdditionalDisk` declares either `device:` (explicit path) or
`tag:` (logical name set in the hypervisor manifest). The doc comment
on the struct already promised tag-based resolution:

> "non-root disk resolved either by device path or by role tag (which
> linuxctl looks up in hypervisor.disks)"

…but the disk manager never implemented it: empty `Device` was passed
to `planDisk` which produced "device not present — hypervisor must
provision first".

Fix: discovery now enumerates raw disks via `lsblk -bJ -d -o NAME,TYPE,
FSTYPE,PARTTYPE`, filters TYPE=disk + no FSTYPE + no PARTTYPE + not in
PVs + no `/sys/class/block/<n>/<n>1` partition node, and sorts the
result alphabetically (sda, sdb, sdc, …). `PlanLayout` claims raw disks
from this pool in manifest declaration order whenever an
`AdditionalDisk` entry has `tag:` set without `device:`. Each claim is
deterministic so re-runs converge.

A new validation error fires if more `additional` entries are declared
than there are raw disks available, so the operator can't silently end
up with unprovisioned LVs.

Live-caught running /lab-up Phase C against ext3 — manifest using
`tag: "u01"` now resolves to `/dev/sdb` automatically.

## v2026.04.30.7 — 2026-04-30

### fix: lvcreate switches to -l for percentage sizes (#32)

DiskManager always emitted `lvcreate -y -n <lv> -L <size> <vg>` regardless
of whether `size` had a `%` suffix. lvcreate's `-L` flag wants an absolute
size (`200G`, `4G`, `1T`); percentages like `100%FREE` / `50%VG` / `100%PVS`
need `-l` instead.

Manifests using `size: "100%"` (the operator-friendly idiom for "fill the
remaining VG space") errored out with `lvcreate ... -L 100%` exit 3.

Fix: detect `%` in size, switch flag to `-l`, and normalize bare `"N%"`
to `"N%FREE"` (most useful default — consume unallocated VG extents).

Live-caught running /lab-up Phase C against ext3 — u01 LV creation
errored at LVM step.

## v2026.04.30.6 — 2026-04-30

### feat: oracle-presets aligned with Oracle docs 19c/21c/23ai/26ai (#30)

Audited every value in `presets/data/{sysctl,limits,packages,bundles}/oracle-*`
against Oracle's official Linux Database Installation Guides (19c GUID-…,
21c, 23ai, and 26ai per https://docs.oracle.com/en/database/oracle/oracle-database/26/ladbi/).

**sysctl/oracle-19c.yaml — added 4 missing keys** that
`oracle-database-preinstall-19c` writes but our preset wasn't enforcing
(would let CIS scans / kernel upgrades silently drift values):

  - `net.core.rmem_default = 262144`
  - `net.core.wmem_default = 262144`
  - `net.ipv4.conf.all.rp_filter = 2` (loose; required for RAC interconnect)
  - `net.ipv4.conf.default.rp_filter = 2`

**limits/oracle-19c.yaml — added `data unlimited`** for both `oracle` and
`grid` users (MoS Doc ID 1264284.1 — RHEL/OL data-segment workaround;
without it ORA-27300/27301/27302/04030 surface under sustained load).

**New presets:**
- `packages/oracle-21c.yaml` — preinstall RPM `oracle-database-preinstall-21c`
- `packages/oracle-26ai.yaml` — preinstall RPM `oracle-database-preinstall-26ai`
  (PROVISIONAL — name predicted from Oracle's 19c→21c→23ai naming pattern;
  verify with `dnf list available 'oracle-database-preinstall-*'` once the
  RPM is published in `ol9_oracle_software`)
- `bundles/oracle-single-21c.yaml`, `oracle-single-23ai.yaml`,
  `oracle-single-26ai.yaml` — composing the corresponding package preset
  with the (unchanged) 19c sysctl + limits + users + OFA dirs.

**Cross-version alignment per Oracle docs**: sysctl and limits values are
identical across 19c → 21c → 23ai → 26ai, so all bundles reuse
`oracle-19c` for those layers. The 26ai-specific delta (transparent
hugepage = `madvise` for UEK7+, replacing the older `never`) is at the
kernel cmdline layer, out of scope for sysctl/limits presets — handled
separately by the preinstall RPM's grub config.

Tests in `pkg/presets/presets_test.go` + `pkg/managers/limits_test.go`
updated for the new value counts.

## v2026.04.30.5 — 2026-04-30

### feat: User.Sudo field — drop /etc/sudoers.d/<user> per user (#29)

`config.User` and `managers.UserSpec` gain a `sudo` field with values
`""` (no rule), `"NOPASSWD"` (passwordless), `"PASSWD"` (require
password). UserManager Apply path now calls `applySudo` for every
created/updated user, writing `/etc/sudoers.d/<name>` with the
appropriate line. File is mode 0440 and validated via `visudo -cf`
before placement so a malformed write can't break sudo cluster-wide.

Empty string is a no-op (existing files untouched). YAML schema:

    users_groups:
      users:
        - name: "buecheleb"
          groups: ["wheel"]
          sudo: "NOPASSWD"          # new
          ssh_keys:
            - "${vault:secret/data/stacks/ext3/ssh-keys#buecheleb}"

Validation: `validate:"omitempty,oneof=NOPASSWD PASSWD"` rejects unknown
strings at config load.

## v2026.04.30.4 — 2026-04-30

### fix: orchestrator desiredFor dispatches per-manager (#27)

Default branch in `pkg/apply.Orchestrator.desiredFor` passed
`*config.Linux` to every unknown manager. NetworkManager (and any
manager whose cast helper rejects `*config.Linux`) errored. Fix:
explicit per-manager dispatch for limits/sysctl/selinux/service/
firewall/hosts/ssh; unknown returns nil; user/package/dir keep full
`*config.Linux` for preset expansion.

## v2026.04.30.3 — 2026-04-30

### fix: openSession dials SSH + global --ssh-key / --ssh-user / --ssh-port flags (#25)

`internal/root/runtime.go:openSession` returned a non-dialed
`session.NewSSH(host, user)` descriptor, so the first manager call
errored with `ssh: not connected (call Dial first)`. Apply path was
unusable for any non-local target.

Also: no way to pass an alternate SSH key/user from the CLI. Default
`~/.ssh/id_ed25519` was hardcoded.

Fix:
- Three new persistent global flags: `--ssh-key`, `--ssh-user`,
  `--ssh-port` (defaults: `~/.ssh/id_ed25519`, `$USER` or `root`, 22).
- `openSessionReal` now calls `session.NewSSHDial(Opts{…})` to establish
  the transport before returning. Dial failures fall through to a
  non-dialed descriptor with a `warn:` line so dry-run paths still work.

### fix: orchestrator desiredFor pass *config.Linux for user + dir managers (#21 follow-up)

Same class of bug as #21: `pkg/apply.Orchestrator.desiredFor` returned
raw `*config.UsersGroups` and `[]config.Directory` for the `user` /
`dir` managers, which their cast helpers (`castUsersGroups`,
`castDirectories`) reject *and* which bypass bundle_preset expansion.

Fix: orchestrator passes the full `*config.Linux` for `user`, `dir`,
`package` (one consolidated case), letting each manager's cast
helper expand the preset and merge with explicit lists.

## v2026.04.30.2 — 2026-04-30

### fix: orchestrator binds Session to every manager (#23)

`pkg/apply.Orchestrator` pulled managers from the global registry but
never attached its `o.Session` to them. Plan/Apply/Verify/Rollback all
errored on first session-aware manager: `package.Plan: no session
attached`, `selinux.Apply: no session attached`, etc. The orchestrator
path was effectively unusable for real remote runs.

Fix: new `(*Orchestrator).bindSession(m Manager) Manager` type-switches
on all 13 concrete manager types and calls each one's `WithSession`
(signatures vary — `session.Session` / `SessionRunner` / `sudoRunner` —
but `session.Session` satisfies all of them). Plan/Apply/Verify/Rollback
all wrap each manager through `bindSession` before use.

Live-caught running `/lab-up` Phase C against ext3+ext4 — VMs were ready
on static IPs, package manager errored at session-attach.

## v2026.04.30.1 — 2026-04-30

### fix: orchestrator passes *config.Linux to PackageManager (#21)

`pkg/apply/orchestrator.go:desiredFor` returned `o.Linux.Packages` (raw
`*config.Packages`) for the `package` manager. `PackageManager.castPackages`
only handled `*config.Linux` / `PackagesSpec`; passing `*config.Packages`
was rejected with `package: unsupported desired-state type *config.Packages`,
AND skipped `bundle_preset` expansion.

Fix:
- Orchestrator now passes the full `*config.Linux` for the `package` case,
  matching `internal/root/runtime.go`'s already-correct path. Bundle presets
  (`oracle-single-19c`, etc.) now expand into the install/remove lists.
- `castPackages` defensively accepts `*config.Packages` / `config.Packages`
  too, returning `PackagesSpec{Install, Remove}` directly. Bundle presets
  cannot be expanded on this fallback path (no enclosing `*config.Linux`).
- Updated `pkg/apply/coverage_test.go` to assert the new behavior.

Live-caught running `/lab-up` Phase C against ext3+ext4 (infrastructure
plan 034) — VMs install + boot fine on static IPs, blocked at OS-config
layer until this fix.

## v2026.04.11.9 — 2026-04-22

### Added — Conventions library (plan 033, #19)

- `pkg/presets/` — embedded preset engine with 19 shipped presets (`go:embed`
  YAML under `pkg/presets/data/`). Categories: directories (3),
  users_groups (3), packages (4), sysctl (2), limits (2), bundles (5).
- 4 new Linux config fields: `directories_preset`, `users_groups_preset`,
  `packages_preset`, `bundle_preset`. All optional and additive — existing
  manifests continue to work unchanged.
- `linuxctl preset list [--category] [--tier]` — tabular discovery
- `linuxctl preset show <name> [--expand]` — inspect a preset or bundle
- `linuxctl config render` now implemented (was stub). Expands
  `bundle_preset` + per-category `*_preset` fields into the final desired
  state, redacts `${vault:...}` / `${gen:...}` placeholders.
- Tier gating: all Phase-1 presets are community tier.
  `hardened-cis-l1` / `hardened-cis-l2` reserved for business tier.

### Changed

- `pkg/managers/sysctl.go` + `pkg/managers/limits.go`: hardcoded
  `oracle-19c` preset data migrated to YAML under
  `pkg/presets/data/{sysctl,limits}/oracle-19c.yaml`. Behaviour preserved
  byte-for-byte; existing tests (`TestSysctl_PresetExpansion`,
  `TestSysctl_MergePresetExplicit`, `TestLimits_PresetExpansion`,
  `TestSysctl_Plan_PresetMerge`) remain green as regression gate.
- `pkg/managers/dir.go`, `user.go`, `package.go` accept `*config.Linux`
  and transparently merge the referenced preset (explicit entries win).
- `pkg/config.Linux` loader calls `expandBundleOnLinux` after YAML decode
  so `bundle_preset` populates per-category `*_preset` fields (only when
  the user has not set them explicitly).

### Migration

- Stacks using `bundle_preset: oracle-rac-19c` drop ~60 lines of
  boilerplate (see `infrastructure/stacks/clext5/os/linux.yaml` —
  migration lands in the infra repo alongside the linuxctl release).

## v2026.04.11.8 — 2026-04-22

### BREAKING — CLI verb rename `env` → `stack` (#17)

Mirrors the proxctl `env` → `stack` rename. Deprecated aliases are retained
for one release and emit warnings; they will be removed in the next release.

- **CLI subcommand:** `linuxctl env …` → `linuxctl stack …`. `env` remains
  as a hidden deprecated alias with a PersistentPreRun stderr warning.
- **Global flag:** `--env <name>` → `--stack <name>`. `--env` is a hidden
  deprecated alias (pflag `Deprecated` + `Hidden`).
- **Registry file:** `~/.linuxctl/envs.yaml` → `~/.linuxctl/stacks.yaml`.
  Auto-migration on startup: if only `envs.yaml` exists, it is renamed in
  place; if both exist, `stacks.yaml` wins and a warning is emitted on
  every run until the user deletes `envs.yaml`.
- **Environment variable:** `LINUXCTL_ENV` → `LINUXCTL_STACK`. Both
  accepted during the deprecation window; `LINUXCTL_STACK` wins on
  conflict. Flags still win over env vars.
- **Unchanged:** `pkg/config.Env` Go type, `kind: Env` YAML tag, and the
  manifest filename `env.yaml`. Only the CLI verb, flag, env var, and
  registry filename moved.

### Added

- `internal/root/stack.go` — `newStackCmd()` + `newEnvAliasCmd()` hidden
  deprecated alias
- `internal/root/stacks_registry.go` — `MigrateRegistry()`,
  `RegistryPathForRead()`, `registryPath()`, `legacyRegistryPath()`
- `applyEnvVarDefaults()` — honors `LINUXCTL_STACK`/`LINUXCTL_ENV`
- `stackPathFromArgs()` — canonical flag/env resolver (prefers `--stack`)

### Tests

- `TestStack_AllSubcommandsNotImplemented` + `TestEnv_DeprecatedAlias_StillWorks`
  + `TestEnv_DeprecatedAlias_EmitsWarning`
- `TestStackPathFromArgs` — --stack / --env / both / positional / default
- `TestMigrateRegistry_*` — no-files / legacy-only (renames) / both-exist
  (keeps both) / legacy-is-dir (no-op) / rename-fails (non-fatal)
- `TestApplyEnvVarDefaults` — LINUXCTL_STACK / LINUXCTL_ENV / both / flag-wins
- `TestRegistryHome_UserHomeDirFails` — UserHomeDir injection covers the
  error-path propagation through all registry helpers
- Coverage held ≥95% on pkg/config (97.7%), pkg/managers (95.4%),
  pkg/apply (97.7%), internal/root (95.1%)

### Docs

- `docs/user-guide.md` — "Envs" section → "Stacks"; registry + CLI
  examples; deprecation callout for `env` / `--env` / `envs.yaml`
- `docs/config-reference.md` — §6 renamed to `~/.linuxctl/stacks.yaml`;
  deprecation note covering CLI flag, env var, filename; `env.yaml` +
  `kind: Env` explicitly retained
- `docs/quick-start.md` + `docs/installation.md` — "stack registry"
  wording; `stacks.yaml` volume path note
- `cmd/docgen/main.go` — conventions footer references `--stack` with
  `--env` as deprecated alias note
- `docs/cli/*` — regenerated via `make docs-cli`, 78 pages reflecting
  `linuxctl stack` tree + `--stack` persistent flag
- `docs/cli-reference.md` — regenerated index

## v2026.04.11.7 — 2026-04-22

### Added — comprehensive documentation (#13)

- `docs/installation.md` — Homebrew, direct binary, Docker, build-from-source, air-gap, shell completion, license, SSH setup, first-run verification
- `docs/quick-start.md` — 5-minute walkthrough (validate, plan, apply, verify, orchestrator, diff) against `localhost`
- `docs/user-guide.md` — concepts, session model (local/SSH), env registry, 13 managers, orchestrator, fleet ops, rollback, presets, recommended operator workflow
- `docs/manager-reference.md` — per-manager deep dive: owns, YAML fields, idempotency, hazards, rollback, examples for all 13 managers, plus protocol invariants
- `docs/config-reference.md` — full YAML schema (env.yaml, linux.yaml, context, envs.yaml), secret resolvers, `$ref` composition, validation
- `docs/preset-guide.md` — preset model, shipped (`oracle-19c`), Business stubs (`pg-16`, `hardened-cis`), tier gating, custom preset roadmap
- `docs/distro-guide.md` — supported distros (Tier 1 / 2), detection, per-manager distro dispatch, distro-specific gotchas
- `docs/integration-guide.md` — composition with proxctl, mcp-host, dbx, Ansible, Terraform, GitOps / CI pattern, out-of-scope declarations
- `docs/licensing.md` — Community/Business/Enterprise tier matrix, JWT format, feature gating, grace / expiry, air-gap activation
- `docs/troubleshooting.md` — top 20 real-world issues with root cause + fix (SSH auth, drift, LVM partials, package locks, firewall reload, SELinux reboot, etc.)
- `docs/architecture.md` — component diagram, manager protocol invariants, orchestrator DAG, SQLite state schema, session abstraction, error classification
- `docs/contributing.md` — dev setup, test approach (unit + integration + race), coverage targets, branch/PR flow, adding a manager/distro, release process, code style
- `docs/examples/host-only`, `docs/examples/pg-single`, `docs/examples/oracle-rac-2node` — validated `linux.yaml` samples (pass `linuxctl config validate`)
- `cmd/docgen/main.go` + `make docs-cli` — cobra/doc-based CLI reference generator producing 78 per-command Markdown pages plus an aggregated `docs/cli-reference.md` index
- `README.md` — elevator pitch, badges, 30-second demo, doc index, tier brief

### Dependencies

- Added `github.com/spf13/cobra/doc`, upgraded `cobra` to v1.10.2, `pflag` to v1.0.9, `go-md2man` to v2.0.6

## v2026.04.11.6 — 2026-04-19

### Added — Phase 5: cluster SSH wiring (#11)

- `pkg/managers/ssh_auth.SetupClusterSSH` — complete implementation: concurrent per-node Ed25519 keypair gen (idempotent), serialized cross-authorization phase, `ssh-keyscan` seeds known_hosts; returns per-node result + accumulated errors
- `pkg/config.Env.NodeHostnames()` — accessor extracting hostnames from opaque Hypervisor spec (handles both inline and $ref-resolved forms)
- `linuxctl ssh setup-cluster <env.yaml>` — reads cluster nodes from env manifest (previously required repeated --host flags); `--user` repeatable (default `[grid, oracle]`); `--parallel` toggle

### Fixed — (#8, now closed)

- `runManager` config → managers type bridge via `usersGroupsSpec` / `packagesSpec` helpers in internal/root/runtime.go
- `apply.go:75` nil-guard on `*ApplyResult` error return

### Concurrency verified

- `TestSetupClusterSSH_Concurrent`: barrier session asserts ≥2 inflight per-node operations
- `go test -race ./...` clean (pre-existing race in `TestSSH_Run_ContextCancelled` unchanged, unrelated)

### Coverage (all ≥95%)

- pkg/managers: 95.0% → 95.4%
- pkg/config: 97.4% → 97.7%
- internal/root: 96.4% → 95.1% (held)

## v2026.04.11.5 — 2026-04-19

### Tests — final coverage push (#7)

| Package | Before | After |
|---------|--------|-------|
| pkg/license | 80.0% | **100.0%** |
| internal/root | 38.0% | **98.1%** |
| **Total** | 87.5% | **95.1%** |

- 81 new tests (11 license + 70 CLI handler)
- Minor refactor: `openSession` → package-level var for SSH DI (test fakes swap in)
- Cobra test harness with fresh `NewRootCmd` per test; fake manager registered for orchestrator tests; full apply DAG traversal without real infrastructure
- All 7 env subcommands + 7×3 subsystem stubs + config/license/ssh stub branches covered

### Bugs flagged (follow-up #8)

- `runManager` config.UsersGroups/Packages type mismatch with managers.*Spec — runtime "unsupported desired-state type"
- `apply.go:75` nil-deref on `r.Applied` when orchestrator returns nil on error path

Both documented with repro tests; fix deferred to #8.

## v2026.04.11.4 — 2026-04-19

### Tests — coverage hardening to ≥95% (#5)

| Package | Before | After | Delta |
|---------|--------|-------|-------|
| pkg/config | 78.3% | **97.4%** | +19.1pp |
| pkg/session | 20.2% | **91.8%** | +71.6pp |
| pkg/apply | 78.4% | **97.7%** | +19.3pp |
| pkg/managers (13 managers) | 77.1% | **95.0%** | +17.9pp |

- ~135 new tests across 4 packages; no public API changes
- **pkg/session: in-process SSH test server** using `gliderlabs/ssh` (ed25519 keys + rejects counter for retry tests); covers Run/RunSudo/WriteFile/ReadFile/FileExists/host-key/timeout/retry
- **pkg/managers**: DependsOn table-driven test for all 13 managers; ufw firewall backend now exercised; mount.applyOne refactored — pure `buildMountCmd` extracted for testability; every manager's Rollback error paths tested; distro detection fallback chain (RHEL7/SLES/Rocky/Alma/Mint via ID_LIKE)
- **pkg/apply**: orchestrator dependency-order invocation, continue-on-error, Rollback reverse order, unregistered-manager error, dry-run propagation
- Minor bug fix in `pkg/session/ssh.go`: renamed shadowing helper `net()` → `joinHostPort()`
- `pkg/license` (80.0%) and `internal/root` (38.0%) remain below 95% — CLI wiring tests are Phase 5 scope (integration tests)

## v2026.04.11.3 — 2026-04-19

### Added — Phase 4: 8 remaining managers + full 13-manager orchestrator (#3)

- **service**: systemd enable/disable/start/stop/restart with Before-snapshot rollback (81.2% coverage)
- **sysctl**: kernel params via `/etc/sysctl.d/99-linuxctl.conf` drop-in + live `sysctl -n` drift check; `oracle-19c` preset (10 params) (84.0%)
- **limits**: `/etc/security/limits.d/99-linuxctl.conf` drop-in; `oracle-19c` preset (16 entries for grid+oracle) (85.5%)
- **firewall**: firewalld / ufw distro dispatch; ports + sources add/remove; enable/disable (~65%)
- **hosts**: `/etc/hosts` managed block (`# BEGIN linuxctl` / `# END linuxctl`) (~85%)
- **network**: hostname + /etc/resolv.conf (NIC management deferred to Phase 4b) (~78%)
- **ssh**: authorized_keys + `/etc/ssh/sshd_config.d/99-linuxctl.conf` drop-in with `sshd -t` validate; `SetupClusterSSH` for RAC cluster trust
- **selinux**: mode (enforcing/permissive/disabled) + booleans; HazardDestructive flag on `disabled` (reboot required)
- `pkg/apply` orchestrator: full 13-manager dependency order (disk → package → user → dir → mount → sysctl → limits → hosts → ssh → selinux → firewall → network → service) (78.4%)
- `linuxctl ssh setup-cluster <env.yaml>` CLI for per-user Ed25519 keypair gen + cross-authorization across nodes

### Pending (Phase 4b)

- Full NIC management (nmcli/networkd connection add/modify)
- Additional presets: `pg-16`, `hardened-cis` (stubs exist, content TBD)

## v2026.04.11.2 — 2026-04-19

### Added — Phase 3 core implementation (#1)

- `pkg/config` (78.3% coverage): typed Linux layer with full struct schemas (DiskLayout, UsersGroups, Directories, Mounts, Packages, Sysctl, Limits, Firewall, HostsEntries, Services, SSHConfig, SELinuxConfig), $ref loader + profile extends, secret resolver (env/file/vault/gen/ref + pipes), cross-field validators
- `pkg/session` (~85% on exercised paths): SSH + localhost session abstraction with retry, key auth, sudo, WriteFile/ReadFile/FileExists
- `pkg/managers` (69.5% coverage): Manager interface + registry, 5 core managers implemented with idempotent Plan → Apply → Verify → Rollback:
  - **dir**: directory tree with owner/group/mode (recursive option)
  - **user**: user + group CRUD, SSH keys, sudo-enabled chpasswd
  - **package**: distro-aware (dnf/yum/apt/zypper) install/remove
  - **disk**: LVM (PV/VG/LV) + mkfs + fstab + mount with safety
  - **mount**: CIFS + NFS + bind + tmpfs with credentials file management
- `pkg/apply` (78.4% coverage): cross-manager orchestrator with dependency order (disk → user → dir → package → mount), Plan/Apply/Verify/Rollback/Diff
- CLI handlers wired: `config validate`, `dir/disk/mount/user/package {plan|apply|verify}`, `apply {plan|apply|verify|rollback}`, `diff`

### Verified

- `go build ./...`, `go vet ./...`, `staticcheck ./...`, `go test ./...` all clean

### Known limitations (Phase 3b follow-ups)

- `config render` returns "not implemented"
- Sample testdata $ref resolution has an edge case (manager unit tests pass on their own fixtures)
- SSH path tested via mock only; real-host integration deferred to Phase 5
- Managers 6-13 (service, sysctl, limits, firewall, hosts, network, ssh, selinux) scaffolded but not implemented — Phase 4 scope

## v2026.04.11.1 — 2026-04-22

Initial scaffold.

### Added
- Go/Cobra CLI skeleton (`cmd/linuxctl`) with global flags: `--context`, `--env`,
  `--host`, `--format`, `--yes`, `--dry-run`, `--license`, `--verbose`.
- 13 subsystem commands each with `plan`, `apply`, `verify` subcommands:
  `disk`, `user`, `package`, `service`, `mount`, `sysctl`, `limits`,
  `firewall`, `hosts`, `network`, `ssh`, `selinux`, `dir`.
- Orchestrator command group `apply` (plan / apply / verify / rollback) plus
  `diff`, `license`, `version`.
- `pkg/managers` — Manager interface + shared `Change`, `ChangeSet`,
  `ApplyResult`, `VerifyResult`, `HazardLevel` types. All 13 managers
  implemented as stubs returning `not implemented`.
- `pkg/license` — tier constants (Community / Business / Enterprise),
  35-tool catalog, no-op `Check()` that grants Community and refuses higher
  tiers in scaffold builds.
- `pkg/config` — `Env` / `Linux` / `Spec` YAML structs, loader, validator,
  resolver stub.
- `pkg/state` — SQLite `Open`/`Close` skeleton (`modernc.org/sqlite`).
- `pkg/session` — `SSHSession` and `LocalSession` stubs.
- `pkg/version` — build-info helper.
- Scaffold test suite (`go test ./...`): root `--help` smoke, version, manager
  interface compliance, license tier + catalog, empty config validation.
- Docs skeleton (`docs/`): installation, quick-start, user-guide,
  config-reference, cli-reference, manager-reference, preset-guide,
  distro-guide, integration-guide, licensing, troubleshooting, architecture,
  contributing.
- Build & release plumbing: `Dockerfile`, `Makefile`, `.goreleaser.yaml`,
  GitHub Actions CI (`.github/workflows/ci.yml`), `CODEOWNERS`, `.gitignore`.

### Notes
- Real manager logic, SSH execution, and license verification are **not**
  implemented in this scaffold; they ship in Phase 3 and Phase 4 per
  [Design 025](https://github.com/itunified-io/infrastructure/blob/main/docs/plans/025-linuxctl-design.md).
