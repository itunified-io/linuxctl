# Changelog

All notable changes to `linuxctl` are documented in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
CalVer (`vYYYY.MM.DD.TS`).

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
