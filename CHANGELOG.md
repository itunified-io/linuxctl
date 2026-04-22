# Changelog

All notable changes to `linuxctl` are documented in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
CalVer (`vYYYY.MM.DD.TS`).

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
