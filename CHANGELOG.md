# Changelog

All notable changes to `linuxctl` are documented in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
CalVer (`vYYYY.MM.DD.TS`).

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
