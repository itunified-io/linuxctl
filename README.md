# linuxctl

`linuxctl` is a declarative, idempotent, auditable CLI that converges a Linux host
to the desired state described in a `linux.yaml` file. It is the mutation
counterpart to the read-only `mcp-host` monitoring server and ships as a single
static Go binary (AGPL-3.0, Ed25519-signed per-tool license).

> Status: **Phase 0 scaffold** — the Cobra tree, 13 manager stubs, and the
> SQLite / license / session packages compile and are under active implementation.
> Plan / Apply / Verify / Rollback logic lands in Phase 3.

## Features (target)

- 13 subsystem managers: `disk`, `user`, `package`, `service`, `mount`,
  `sysctl`, `limits`, `firewall`, `hosts`, `network`, `ssh`, `selinux`, `dir`.
- Orchestrated `apply plan / apply / verify / rollback` across the full DAG.
- Distro-aware dispatch (RHEL 8/9, Oracle Linux, Rocky, Alma, Ubuntu 22.04/24.04, SLES 15 SP5+).
- SSH-only, no agent. Works against Proxmox VMs, bare metal, Hostinger VPS,
  cloud instances — anything with `sshd`.
- License-gated: Community (single host), Business (fleet + presets),
  Enterprise (state sync, RBAC, policy lifecycle, air-gap).
- Audit trail powered by `github.com/itunified-io/dbx/pkg/core/audit`.

## Install

```bash
# Homebrew (once published)
brew install itunified-io/tap/linuxctl

# From source
go install github.com/itunified-io/linuxctl/cmd/linuxctl@latest
```

## Quickstart

```bash
# 1. Register an env pointing at a master config directory
linuxctl env add lab --path ~/repos/infrastructure/envs/lab

# 2. Validate the linux.yaml
linuxctl config validate envs/lab/linux.yaml

# 3. Preview changes
linuxctl apply plan --env lab --host db01

# 4. Apply
linuxctl apply apply --env lab --host db01 --yes

# 5. Verify
linuxctl apply verify --env lab --host db01
```

## Documentation

- [docs/installation.md](docs/installation.md)
- [docs/quick-start.md](docs/quick-start.md)
- [docs/user-guide.md](docs/user-guide.md)
- [docs/config-reference.md](docs/config-reference.md)
- [docs/cli-reference.md](docs/cli-reference.md)
- [docs/manager-reference.md](docs/manager-reference.md)
- [docs/preset-guide.md](docs/preset-guide.md)
- [docs/distro-guide.md](docs/distro-guide.md)
- [docs/integration-guide.md](docs/integration-guide.md)
- [docs/licensing.md](docs/licensing.md)
- [docs/troubleshooting.md](docs/troubleshooting.md)
- [docs/architecture.md](docs/architecture.md)
- [docs/contributing.md](docs/contributing.md)

## License

AGPL-3.0 — see [LICENSE](LICENSE). Commercial / Enterprise licenses available
from itunified.io.
