# Architecture

This document describes the internal architecture of `linuxctl`:
components, the manager protocol, the orchestrator DAG, the SQLite state
model, and the session abstraction.

---

## 1. Component overview

```
   +--------------------------+
   |        cmd/linuxctl      |   single-binary entrypoint
   +-----------+--------------+
               |
   +-----------v--------------+
   |     internal/root        |   Cobra tree, flag wiring, I/O formatting
   +-----------+--------------+
               |
   +-----------v--------------+   +----------------------+
   |        pkg/apply         |-->|   pkg/managers       |
   | (orchestrator + DAG)     |   | (13 subsystem mgrs)  |
   +-----+--------+-----------+   +-----------+----------+
         |        |                           |
         |        |                           |
+--------v----+  +v------------+  +-----------v----------+
| pkg/config  |  | pkg/state   |  |    pkg/session       |
| (schema +   |  | (SQLite +   |  | (local + ssh + retry)|
|  validate)  |  |  audit)     |  +----------------------+
+-------------+  +-------------+

   +--------------------------+
   |       pkg/license        |   Ed25519 JWT verification + gating
   +--------------------------+
```

- **cmd/linuxctl** — 20-line `main.go`; only wires build metadata into
  `internal/root.Execute`.
- **internal/root** — every Cobra command and its flag bindings. One
  file per subcommand group (`apply.go`, `disk.go`, `env.go`, ...).
- **pkg/apply** — the orchestrator. Walks the manager DAG, enforces
  hazard gates, aggregates results. Used by `linuxctl apply *` and
  `linuxctl diff`.
- **pkg/managers** — the 13 Manager implementations. One `.go` file per
  manager plus a `base.go` with shared helpers.
- **pkg/config** — YAML schema, `go-playground/validator` tags, the
  loader (`$ref` resolution), secret resolver, preset registry.
- **pkg/state** — SQLite persistence for run records, per-change audit
  entries, and Business-tier persistent rollback.
- **pkg/session** — command execution abstraction. `local` uses
  `exec.Command`; `ssh` uses `golang.org/x/crypto/ssh` with
  retry/backoff classification.
- **pkg/license** — Ed25519 JWT parse, verify, and feature gate.

---

## 2. Manager protocol

Every manager implements four verbs:

```go
type Manager interface {
    Name() string                                                  // "disk"
    DependsOn() []string                                           // [] or ["user", ...]
    Plan(ctx, spec, state) ([]Change, error)                       // pure, no mutation
    Apply(ctx, change) (*ApplyResult, error)                       // writer, one record
    Verify(ctx, spec) (bool, []Change, error)                      // Plan() == empty?
    Rollback(ctx, change) error                                    // best-effort reverse
}
```

Invariants:

1. **Plan() is pure.** No filesystem, no network (except read-only
   probes). No state-db write. Safe to call in parallel.
2. **Apply() is the only mutator.** Writes the `ApplyResult` to
   `pkg/state` before returning. If the writer fails, the mutation is
   undone best-effort.
3. **Verify()** runs `Plan()` again and returns `(len(changes) == 0,
   changes, err)`. Equivalent to `linuxctl <manager> verify`.
4. **Rollback()** reverses a specific `Change`. The orchestrator calls
   `Rollback` in reverse DAG order when a run fails partway through.

### 2.1 Change

```go
type Change struct {
    ID          string       // stable hash over (Manager, Target, Action, Spec)
    Manager     string
    Target      string       // e.g. "/etc/sysctl.d/99-linuxctl.conf" or "user:oracle"
    Action      string       // create | update | delete | noop
    Description string
    Hazard      HazardLevel  // none | warn | destructive
    Before      any          // observed state (for diff rendering)
    After       any          // declared state
}
```

### 2.2 ApplyResult

```go
type ApplyResult struct {
    Change   Change
    OK       bool
    Error    error
    Duration time.Duration
    Commands []CommandTrace  // for audit
    RunID    string          // UUID from the enclosing run
}
```

---

## 3. Orchestrator DAG

The 13 managers form a fixed topological order:

```
disk
 -> user
    -> package
       -> service
          -> mount
             -> sysctl
                -> limits
                   -> firewall
                      -> hosts
                         -> network
                            -> ssh
                               -> selinux
                                  -> dir
```

Rationale:

- **disk** must exist before anything can reference the mount points.
- **user** + **package** must exist before **service** can enable things.
- **mount** after services so systemd mount units are already known.
- **sysctl / limits / firewall / hosts / network** are system-level
  configs; order among them is irrelevant but pinned for reproducibility.
- **ssh** after firewall (firewall must permit SSH before we tighten
  sshd_config).
- **selinux** late — a mode change may require reboot, so we prefer to
  have every other subsystem in place first.
- **dir** is last because it may `chown -R` onto paths created by
  preceding managers.

The DAG is defined in `pkg/apply/dag.go` and verified by
`pkg/apply/dag_test.go` (no cycles, every manager present, stable
ordering).

The orchestrator walks the DAG sequentially per host. Across hosts (fleet
mode), it runs in parallel up to `--parallel N`. Each host has an
independent DAG execution.

---

## 4. State model (SQLite)

Persistent state lives in `~/.linuxctl/state.db` (override with
`LINUXCTL_STATE_DB`). Schema (abbreviated):

```sql
CREATE TABLE runs (
    id          TEXT PRIMARY KEY,       -- UUID
    started_at  TIMESTAMP NOT NULL,
    ended_at    TIMESTAMP,
    mode        TEXT NOT NULL,          -- plan | apply | verify | rollback
    env         TEXT,
    host        TEXT,
    operator    TEXT,
    success     BOOLEAN
);

CREATE TABLE changes (
    id          TEXT PRIMARY KEY,       -- change ID
    run_id      TEXT NOT NULL REFERENCES runs(id),
    manager     TEXT NOT NULL,
    target      TEXT NOT NULL,
    action      TEXT NOT NULL,
    hazard      TEXT NOT NULL,
    applied     BOOLEAN NOT NULL,
    error       TEXT,
    before_json TEXT,                   -- serialized observed
    after_json  TEXT,                   -- serialized declared
    at          TIMESTAMP NOT NULL
);

CREATE TABLE commands (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    change_id   TEXT NOT NULL REFERENCES changes(id),
    cmd         TEXT NOT NULL,
    exit_code   INT  NOT NULL,
    stderr_snip TEXT,
    duration_ms INT  NOT NULL
);
```

Write pattern: `Apply()` opens a write transaction, inserts the `runs`
row on first Apply of the run, inserts the `changes` row, then inserts
each `commands` row, then commits. Read pattern (used by rollback):
select from `changes` by `run_id`, walk in reverse order.

State is advisory. Deleting `state.db` does not break idempotency:
every manager's `Plan()` re-derives state from the host, not from the
DB. The DB exists for rollback and audit.

---

## 5. Session abstraction

```go
type Session interface {
    Run(ctx, cmd, args...) (stdout, stderr []byte, exit int, err error)
    Write(ctx, path string, content []byte, mode os.FileMode) error
    Read(ctx, path string) ([]byte, error)
    Stat(ctx, path string) (FileInfo, error)
    Distro(ctx) (Distro, error)
    Close() error
}
```

Two implementations:

### 5.1 Local

`pkg/session/local.go` — `exec.Command` with a workdir override. Used
when `--host localhost` or when running tests. Writes go via `os.WriteFile`;
reads via `os.ReadFile`.

### 5.2 SSH

`pkg/session/ssh.go` — backed by `golang.org/x/crypto/ssh`. Features:

- Connection pool of 1 per host (SSH multiplex).
- Retry policy with exponential backoff on transient errors
  (`ConnectionReset`, `i/o timeout`, `temporary failure in name resolution`).
- Fatal errors (auth failure, host key mismatch) are not retried.
- Distro cached per session; `/etc/os-release` read once.

### 5.3 Error classification

```go
type Classifier interface {
    Classify(err error) ErrorClass  // Transient | Fatal | Permission
}
```

The orchestrator retries `Transient` errors up to `--retry N` times. A
`Permission` error aborts the run and prints a remediation hint.

---

## 6. Tests and coverage

- `pkg/managers` — 95%+ per manager, enforced by CI.
- `pkg/config` — 97%+, validates every YAML corner case from testdata.
- `pkg/apply` — 95%+, including a barrier-based concurrency test
  (`TestSetupClusterSSH_Concurrent`).
- `internal/root` — 95%+, uses a fake session and captures stdout for
  golden-file assertions.
- `go test -race ./...` is clean.

Total repo coverage is tracked in the CHANGELOG per release.

---

## 7. Release engineering

- Tags follow CalVer: `vYYYY.MM.DD.TS`.
- `goreleaser` builds for 5 platforms, signs with cosign, uploads to the
  GitHub release.
- Homebrew tap formula auto-updated on release.
- Docker image pushed to `ghcr.io/itunified-io/linuxctl` with matching
  CalVer tag + `latest`.

See `.goreleaser.yaml` for the exact pipeline.
