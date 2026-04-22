# Quick Start

A five-minute walkthrough of `linuxctl` against `localhost`. You will:

1. Install the binary.
2. Validate a simple `linux.yaml`.
3. Plan a single-manager change.
4. Apply it.
5. Verify that no drift remains.
6. Run the full orchestrator (all 13 managers) in plan mode.
7. Run a read-only drift report with `linuxctl diff`.

No remote hosts are required. This uses only the `local` session driver
shipped in `pkg/session`.

---

## 0. Prerequisites

- `linuxctl` in `$PATH` (`brew install itunified-io/tap/linuxctl` or see
  [`installation.md`](installation.md)).
- A writable `/tmp` (the demo `linux.yaml` creates one harmless directory).
- `sudo` privileges if you want to exercise any mutating manager that
  touches `/etc/*` (optional).

---

## 1. Install

```bash
brew install itunified-io/tap/linuxctl
linuxctl version
```

Expected:

```
linuxctl v2026.04.11.7 (commit <sha>, built <date>)
```

---

## 2. Validate a simple linux.yaml

Use the example shipped with this repository:

```bash
cat docs/examples/host-only/linux.yaml
linuxctl config validate docs/examples/host-only/linux.yaml
```

Expected output:

```
OK
```

`config validate` parses the YAML, validates every field against the schema
in `pkg/config`, and checks cross-references (e.g. a user referencing a
group must see that group declared earlier). No network or SSH activity.

---

## 3. Plan a single-manager change

Plan directory-manager changes only, against `localhost`:

```bash
linuxctl dir plan docs/examples/host-only/linux.yaml --host localhost
```

Expected output: a list of changes, one per directory, with the action
(`create` or `noop`), the target path, and the hazard classification
(`none`, `warn`, `destructive`). No filesystem mutation yet.

Example output:

```
dir  create       /tmp/linuxctl-first-run   (hazard: none)
plan: 1 change, 0 destructive
```

Plan is always safe: it runs `Plan()` on the manager against the current
state of the target and prints the diff. It never applies, never mutates,
never opens a write transaction in the SQLite state store.

---

## 4. Apply the change

```bash
linuxctl dir apply docs/examples/host-only/linux.yaml --host localhost --yes
```

Expected:

```
dir  apply  /tmp/linuxctl-first-run   (ok)
apply: 1 ok, 0 failed
```

The `--yes` flag skips the confirmation prompt. Omit it in production to
review each destructive change interactively.

Confirm the change took:

```bash
ls -ld /tmp/linuxctl-first-run
```

---

## 5. Verify zero drift

```bash
linuxctl dir verify docs/examples/host-only/linux.yaml --host localhost
```

Expected:

```
dir  verify  /tmp/linuxctl-first-run   (ok)
verify: 1 ok, 0 drift
```

`verify` runs `Plan()` again and asserts the plan is empty — any diff
relative to the declared `linux.yaml` is drift. A non-zero exit code
signals drift and is safe to use in CI gates.

---

## 6. Full orchestrator plan (all 13 managers)

```bash
linuxctl apply plan docs/examples/host-only/linux.yaml --host localhost
```

This walks the full 13-manager dependency graph:

```
disk -> user -> package -> service -> mount -> sysctl -> limits ->
firewall -> hosts -> network -> ssh -> selinux -> dir
```

Each manager runs its `Plan()` in dependency order. Output groups changes
by manager and summarizes hazards:

```
hosts    create   127.0.1.1 -> [linuxctl.local]
sysctl   update   kernel.hostname=linuxctl.local
service  update   cron (enabled=true, state=running)
dir      noop     /tmp/linuxctl-first-run
plan: 3 changes across 3 managers, 0 destructive
```

Use `linuxctl apply apply ... --yes` to execute the plan. Use
`linuxctl apply rollback ...` during the same run to roll back
in-memory-applied changes (persistent rollback is Business tier — see
[`licensing.md`](licensing.md)).

---

## 7. Read-only drift report

For periodic drift checks (e.g. nightly cron or in CI), use the dedicated
`diff` command:

```bash
linuxctl diff docs/examples/host-only/linux.yaml --host localhost
```

If the host matches the manifest exactly:

```
diff: no drift
```

Otherwise, `linuxctl diff` prints the per-manager change set and exits
non-zero. Pipe to `jq` by passing `--format json` for machine-readable
output suitable for ingestion by mcp-host or any monitoring pipeline.

---

## What next?

- [`user-guide.md`](user-guide.md) — concepts, sessions, env registry,
  orchestrator deep dive, fleet operations.
- [`manager-reference.md`](manager-reference.md) — every manager, every
  YAML field.
- [`config-reference.md`](config-reference.md) — the full `linux.yaml`
  schema.
- [`troubleshooting.md`](troubleshooting.md) — top 20 issues and how to
  fix them.
