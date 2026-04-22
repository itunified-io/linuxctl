# linuxctl

**Declarative, idempotent, auditable Linux host configuration — as a single
static Go binary.**

[![Go Report Card](https://goreportcard.com/badge/github.com/itunified-io/linuxctl)](https://goreportcard.com/report/github.com/itunified-io/linuxctl)
[![License](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![CalVer](https://img.shields.io/badge/calver-YYYY.MM.DD.TS-orange.svg)](CHANGELOG.md)
[![Coverage](https://img.shields.io/badge/coverage-%3E%3D95%25-brightgreen.svg)](docs/contributing.md#2-test-approach)

`linuxctl` converges a Linux host to the desired state described in a
`linux.yaml`. It is the mutation counterpart to the read-only
[`mcp-host`](https://github.com/itunified-io/mcp-host) monitoring server,
and is designed to compose with
[`proxctl`](https://github.com/itunified-io/proxctl),
[`dbx`](https://github.com/itunified-io/dbx), Ansible, and Terraform.

## 30-second demo

```bash
brew install itunified-io/tap/linuxctl

cat <<EOF > linux.yaml
kind: Linux
directories:
  - path: /tmp/linuxctl-demo
    owner: root
    group: root
    mode: "0755"
services:
  - name: cron
    enabled: true
    state: running
EOF

linuxctl config validate linux.yaml
linuxctl apply plan  linux.yaml --host localhost
linuxctl apply apply linux.yaml --host localhost --yes
linuxctl apply verify linux.yaml --host localhost   # zero drift
```

## Features

- **13 subsystem managers** — `disk`, `user`, `package`, `service`, `mount`,
  `sysctl`, `limits`, `firewall`, `hosts`, `network`, `ssh`, `selinux`, `dir`.
- **Plan / Apply / Verify / Rollback** on every manager and on the full DAG.
- **Distro-aware** — RHEL 8/9, Oracle Linux, Rocky, Alma, Ubuntu 22.04/24.04,
  Debian 12, SLES 15 SP5+.
- **Agentless** — only needs `sshd` on the target. No daemon, no runtime.
- **Auditable** — every change persists to a local SQLite state DB.
- **License-gated tiers** — Community is free forever; Business adds fleet
  ops, advanced presets, persistent rollback; Enterprise adds RBAC, audit
  export, SSO, policy lifecycle.

## Status

Phase 6 stable — all 13 managers pass live idempotency tests on Tier-1
distros. Coverage >= 95% across every package. See
[CHANGELOG.md](CHANGELOG.md) for the release timeline.

## Documentation

| Doc                                              | What |
|--------------------------------------------------|------|
| [installation.md](docs/installation.md)          | Install on every platform, license setup, shell completion |
| [quick-start.md](docs/quick-start.md)            | 5-minute walkthrough |
| [user-guide.md](docs/user-guide.md)              | Concepts, sessions, env registry, orchestrator, fleet ops |
| [manager-reference.md](docs/manager-reference.md)| Per-manager deep dive |
| [config-reference.md](docs/config-reference.md)  | Full YAML schema |
| [cli-reference.md](docs/cli-reference.md)        | Auto-generated CLI pages |
| [preset-guide.md](docs/preset-guide.md)          | Sysctl + limits presets |
| [distro-guide.md](docs/distro-guide.md)          | Supported distros + per-distro behavior |
| [integration-guide.md](docs/integration-guide.md)| proxctl / mcp-host / dbx / Ansible / Terraform composition |
| [licensing.md](docs/licensing.md)                | Tier matrix, JWT format, air-gap activation |
| [troubleshooting.md](docs/troubleshooting.md)    | Top 20 real-world issues |
| [architecture.md](docs/architecture.md)          | Components, protocol, DAG, state, session |
| [contributing.md](docs/contributing.md)          | Dev setup, tests, release process |

## Tier brief

| Tier       | Price             | What you get                                     |
|------------|-------------------|--------------------------------------------------|
| Community  | Free forever      | Single-host apply/verify/rollback, `oracle-19c` preset |
| Business   | Per seat          | Fleet ops, advanced presets, persistent rollback, cluster-SSH bootstrap |
| Enterprise | Annual contract   | RBAC, audit export, SSO, policy lifecycle, air-gap activation |

See [licensing.md](docs/licensing.md) for the full matrix.

## License

AGPL-3.0 — see [LICENSE](LICENSE). Commercial / Enterprise licenses are
available from [itunified.io](https://itunified.io/linuxctl).
