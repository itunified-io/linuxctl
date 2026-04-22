# User Guide

This guide walks through every concept an operator needs to run `linuxctl`
productively: contexts, stacks, sessions, the plan/apply/verify/rollback cycle,
the 13 subsystem managers, orchestration, fleet operations, drift detection,
and built-in presets.

For a hands-on five-minute tour, start with [`quick-start.md`](quick-start.md).
For the per-field YAML schema, see [`config-reference.md`](config-reference.md).

---

## 1. Concepts

### 1.1 Contexts

A **context** is a named bundle of operator-scoped defaults stored in
`~/.linuxctl/config.yaml`. A context pins a default stack, a default output
format, a license path, and connection defaults. Switch contexts with:

```bash
linuxctl config use-context lab
```

Contexts are analogous to `kubectl` contexts: they do not carry desired
state, only operator ergonomics.

### 1.2 Stacks

A **stack** maps a logical environment name (`lab`, `uat`, `prod`) to a
directory that contains at least a `linux.yaml` and optionally an
`env.yaml` with cluster metadata (roles, domains, tags, per-node
overrides). Stacks are registered in `~/.linuxctl/stacks.yaml`:

```bash
linuxctl stack add lab  --path ~/repos/infrastructure/envs/lab
linuxctl stack add prod --path ~/repos/infrastructure/envs/prod
linuxctl stack list
```

> **Deprecation (#17):** `linuxctl env …` and `--env` remain available for
> one release as hidden aliases, emitting a deprecation warning. A legacy
> `~/.linuxctl/envs.yaml` is auto-migrated to `stacks.yaml` on first run.
> The manifest filename `env.yaml` and the `kind: Env` YAML tag are
> **unchanged**; only the CLI verb, flag, and registry filename moved.

Resolved stack paths support relative-path imports via `$ref` composition
(see [`config-reference.md`](config-reference.md)).

### 1.3 Managers

A **manager** owns exactly one subsystem on a target host. `linuxctl` ships
with 13 managers:

| # | Name       | Owns                                            |
|---|------------|-------------------------------------------------|
| 1 | `disk`     | LVM PV/VG/LV, `mkfs`, `/etc/fstab`, mount       |
| 2 | `user`     | POSIX users, groups, `~/.ssh/authorized_keys`, sudoers |
| 3 | `package`  | `dnf` / `yum` / `apt` / `zypper` install + remove |
| 4 | `service`  | `systemctl enable`, `start`, `stop`             |
| 5 | `mount`    | CIFS, NFS, bind, tmpfs mounts                   |
| 6 | `sysctl`   | `/etc/sysctl.d/99-linuxctl.conf` keys           |
| 7 | `limits`   | `/etc/security/limits.d/99-linuxctl.conf`       |
| 8 | `firewall` | `firewalld` / `ufw` zones, ports, services      |
| 9 | `hosts`    | Managed block in `/etc/hosts`                   |
|10 | `network`  | Hostname, `resolv.conf` (NIC mgmt in 4b)        |
|11 | `ssh`      | `authorized_keys`, `sshd_config` drop-in, cluster SSH |
|12 | `selinux`  | Mode (enforcing/permissive/disabled), booleans  |
|13 | `dir`      | Directory tree + owner/group/mode/recursive     |

Each manager implements the same four-verb protocol.

### 1.4 Plan / Apply / Verify / Rollback

```
Plan()     -> []Change   (diff only, no mutation, no lock)
Apply(c)   -> ApplyResult (mutate, state persisted, one audit record)
Verify()   -> ok|drift   (Plan() must be empty)
Rollback() -> reverts in-session applied changes (in-memory only on OSS)
```

Every CLI verb maps 1:1 to one of these calls. The orchestrator (`linuxctl
apply`) runs the verbs across all managers in dependency order.

Rollback semantics:

- **Community tier:** in-session rollback only. If the process exits, the
  rollback record is gone.
- **Business tier:** persistent rollback via `--run-id` (records to SQLite
  in `~/.linuxctl/state.db`; recoverable across sessions).
- **Enterprise tier:** state sync + policy-lifecycle approved rollback.

### 1.5 Idempotency

Every manager is idempotent: running the same plan twice against the same
host yields zero new changes on the second run. `Verify()` is the contract:
after `Apply()`, `Verify()` MUST return zero changes. If it doesn't, the
manager has an idempotency bug — open an issue with `type:fix`.

---

## 2. Session model

`linuxctl` runs every manager over a `session.Session` abstraction in
`pkg/session`. Two implementations ship:

### 2.1 Local session

Used when `--host localhost` (or `--host` omitted and the stack declares a
single-node `localhost` target). The local session executes commands via
`exec.Command` directly; no SSH is involved. Ideal for:

- Unit / integration tests
- First-run verification
- Self-configuration of the operator workstation

### 2.2 SSH session

Used for every non-local host. Backed by `golang.org/x/crypto/ssh` with
these features:

- Ed25519 / RSA / ECDSA key support (no passwords).
- Agent forwarding disabled by default.
- Host-key verification via the operator's `~/.ssh/known_hosts`.
- Retry with exponential backoff on transient I/O errors.
- Per-session command audit trail (commands + exit codes written to
  SQLite via `pkg/state`).

Session errors are classified as transient (retry) or fatal (abort). The
classifier is tunable via `context` options and honors per-manager timeout
overrides.

---

## 3. Stack registry

`~/.linuxctl/stacks.yaml`:

```yaml
stacks:
  lab:
    path: /home/op/repos/infrastructure/envs/lab
    default: true
  prod:
    path: /home/op/repos/infrastructure/envs/prod
```

Commands:

```bash
linuxctl stack add    <name> --path <dir>
linuxctl stack list
linuxctl stack remove <name>
```

Every stack MUST contain a `linux.yaml`. Optional files:

- `env.yaml` — cluster metadata: name, domain, tags, per-role selectors,
  inventory source (e.g. Ansible, Terraform output, inline).
- `overrides/<host>.yaml` — per-host override layered on top of `linux.yaml`.
- `secrets/` — sealed secrets (pointers, never raw values).

---

## 4. Subsystem managers (overview)

Each manager is documented in depth in
[`manager-reference.md`](manager-reference.md). Brief summary:

- **disk** — Creates PV/VG/LV and filesystems. Never destroys existing
  storage; only additive operations.
- **user** — Declarative user/group management; SSH keys + sudoers entries.
- **package** — Distro-aware dispatch, picks the correct package manager.
- **service** — systemd only; `enabled` and `state` (`running|stopped`).
- **mount** — CIFS / NFS / bind / tmpfs with optional credentials from Vault.
- **sysctl** — Managed block in `/etc/sysctl.d/99-linuxctl.conf`, reload via
  `sysctl --system`. Supports named presets (e.g. `oracle-19c`).
- **limits** — Managed file in `/etc/security/limits.d/99-linuxctl.conf`.
- **firewall** — firewalld (preferred) or ufw; zone-level port and service
  rules.
- **hosts** — `# BEGIN linuxctl / # END linuxctl` block in `/etc/hosts`.
- **network** — Hostname via `hostnamectl`, DNS via NetworkManager or
  `/etc/resolv.conf` (depending on distro).
- **ssh** — `authorized_keys` per user; drop-in `sshd_config` under
  `/etc/ssh/sshd_config.d/99-linuxctl.conf`; Business-tier cluster-wide
  trust bootstrap (`SetupClusterSSH`).
- **selinux** — Mode + booleans only; policy modules out of scope.
- **dir** — Directory tree with `owner/group/mode/recursive`.

---

## 5. Orchestrator workflow

```bash
linuxctl apply plan <env.yaml>     # dry-run the full DAG
linuxctl apply apply <env.yaml>    # execute in dependency order
linuxctl apply verify <env.yaml>   # re-Plan, expect empty diff
linuxctl apply rollback <env.yaml> # reverse in-session Applies
```

The DAG is fixed: it mirrors the manager ordering in Section 1.3. It is
topologically sorted once at startup; circular dependencies are
impossible because managers declare no dynamic edges.

A typical production flow:

```bash
# 1. Review
linuxctl apply plan envs/prod/linux.yaml --host db01

# 2. Apply with interactive confirm
linuxctl apply apply envs/prod/linux.yaml --host db01

# 3. Verify
linuxctl apply verify envs/prod/linux.yaml --host db01

# 4. If anything is wrong, rollback THIS SESSION only
linuxctl apply rollback envs/prod/linux.yaml --host db01
```

Hazard gating: any change classified as `destructive` requires an explicit
confirmation prompt, even with `--yes`. Override with `--accept-destructive`
(Community tier: per-run; Business tier: policy-lifecycle approved).

---

## 6. Fleet operations (Business tier)

Community tier supports one host per invocation. Business tier adds fleet
mode:

```bash
linuxctl apply plan   envs/prod/linux.yaml --all
linuxctl apply apply  envs/prod/linux.yaml --all --parallel 4
linuxctl apply verify envs/prod/linux.yaml --all
```

The `--all` flag expands the host list from the stack manifest. `--parallel`
caps concurrent SSH sessions. Output aggregates per-host results; the exit
code is the max over all hosts. See [`licensing.md`](licensing.md) for
tier gating.

---

## 7. Rollback and drift detection

### 7.1 Rollback

- **In-session:** `linuxctl apply rollback <env.yaml>` during an active
  invocation reverts any Applies that completed in that process.
- **Persistent (Business):** `linuxctl apply rollback --run-id <uuid>`
  looks up the audit record in `pkg/state` (SQLite) and runs the
  per-manager `Rollback()` against the host in reverse dependency order.

### 7.2 Drift detection

Run `linuxctl diff` on a schedule (cron, systemd timer, or CI):

```bash
linuxctl diff envs/prod/linux.yaml --all --format json > drift.json
```

`linuxctl diff` never mutates. Exit code 0 = no drift, non-zero = drift
present. The JSON output is schema-stable and designed for ingestion by
mcp-host for Slack alerting or by any time-series backend.

---

## 8. Presets

Presets are **named bundles** of settings for sysctl + limits. They
reduce boilerplate for well-known workloads and guarantee consistent
tuning across a fleet.

Shipped presets:

| Name              | Tier       | Applies to                    |
|-------------------|------------|-------------------------------|
| `oracle-19c`      | Community  | `sysctl_preset`, `limits_preset` |
| `pg-16`           | Business   | `sysctl_preset`, `limits_preset` (stub) |
| `hardened-cis`    | Business   | `sysctl_preset`, `limits_preset` (stub) |

Activate in `linux.yaml`:

```yaml
sysctl_preset: oracle-19c
limits_preset: oracle-19c
```

Explicit `sysctl:` or `limits:` entries are **merged on top of** the
preset, with per-key last-write-wins. This lets you start with a shipped
baseline and override selectively.

See [`preset-guide.md`](preset-guide.md) for the full preset reference and
roadmap for custom presets.

---

## 9. Recommended operator workflow

1. **Design.** Commit `linux.yaml` to a git repository (`infrastructure`
   repo or the product repo).
2. **Validate.** `linuxctl config validate` runs in CI on every PR.
3. **Plan on PR.** Use `linuxctl apply plan --format json` from CI to
   post the plan as a PR comment.
4. **Apply on merge.** Gate behind manual approval; drive from a
   privileged runner with SSH keys loaded from Vault.
5. **Verify + drift.** Hourly `linuxctl apply verify` on prod; nightly
   `linuxctl diff` piped to Slack via mcp-host.
6. **Rollback.** If verify fails, page the on-call and run
   `linuxctl apply rollback --run-id <uuid>` (Business).

This is the same pattern used by the itunified.io fleet — `linuxctl`
was designed around this loop.

---

## Cross-references

- [`manager-reference.md`](manager-reference.md) — per-manager deep dive.
- [`config-reference.md`](config-reference.md) — YAML schema.
- [`cli-reference.md`](cli-reference.md) — auto-generated CLI pages.
- [`architecture.md`](architecture.md) — manager protocol + state model.
- [`licensing.md`](licensing.md) — tier gating.
- [`integration-guide.md`](integration-guide.md) — composition with
  proxctl, mcp-host, dbx, Ansible, Terraform.
