# Changelog

All notable changes to `linuxctl` are documented in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
CalVer (`vYYYY.MM.DD.TS`).

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
