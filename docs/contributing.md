# Contributing

Thanks for helping make `linuxctl` better. This page covers the
development environment, the test approach, the branch / PR flow, and
the release process.

---

## 1. Dev setup

Requirements:

- Go 1.22+
- `make`
- `git` (2.30+)
- A throwaway VM or container for integration tests (Rocky 9 or
  Ubuntu 24.04 are the Tier-1 targets).

```bash
git clone https://github.com/itunified-io/linuxctl
cd linuxctl
make build
./bin/linuxctl version
make test
```

Install the pre-commit hook (formatting + vet):

```bash
ln -s ../../scripts/pre-commit .git/hooks/pre-commit   # when scripts/ lands
```

Every command that touches a host must be covered by a test against a
fake session (`pkg/session/fake.go`) so unit tests run with no network
dependency.

---

## 2. Test approach

- **Unit tests** — every manager has a `_test.go` that covers `Plan()`,
  `Apply()`, `Verify()`, `Rollback()` against a fake session.
- **Integration tests** — live runs against a throwaway host, gated
  behind `-tags integration`. Run with `make test-integration HOST=rocky9`.
- **Race tests** — `go test -race ./...` MUST be clean for every PR.
- **Golden-file tests** — `internal/root` compares CLI stdout against
  `testdata/*.golden`. Regenerate with `go test ./internal/root -update`.

Coverage targets (CI enforces these thresholds):

| Package          | Target   |
|------------------|----------|
| `pkg/managers`   | >= 95%   |
| `pkg/config`     | >= 95%   |
| `pkg/apply`      | >= 95%   |
| `internal/root`  | >= 95%   |
| `pkg/license`    | 100%     |
| `pkg/session`    | >= 90%   |

Run coverage locally:

```bash
go test -cover ./...
go test -coverprofile=cover.out ./... && go tool cover -func=cover.out
```

---

## 3. Branch and PR flow

- **Every change has an issue** (`type:feature`, `type:fix`, `type:docs`,
  `type:maintenance`, `type:security`).
- **Branch name:** `feature/<issue>-<slug>`, `fix/<issue>-<slug>`,
  `chore/<slug>`, `docs/<issue>-<slug>`.
- **Commit message:** imperative, references the issue: `feat: add
  resolved.conf support (#42)`.
- **PR title:** short, under 70 characters.
- **PR body:** must include `Closes #<nr>` and a test plan.

Acceptance-criteria gate (same as the infrastructure repo):

- All issue acceptance criteria MUST be checked and verified before
  merge.
- `go test -race ./...` clean.
- `go vet ./...` clean.
- Coverage thresholds maintained or improved.
- CHANGELOG.md updated.
- Public docs updated if behavior changed.

---

## 4. Adding a new manager

Rare event — the 13 managers are the committed taxonomy. If you have a
compelling reason:

1. Open a design issue (`type:feature`, label `scope:manager`).
2. Write an ADR in `docs/adr/` summarizing the proposal.
3. Implement the manager in `pkg/managers/<name>.go` with a matching
   `<name>_test.go` that hits >= 95% coverage.
4. Wire a new Cobra subcommand in `internal/root/<name>.go`.
5. Add the manager to the DAG in `pkg/apply/dag.go`.
6. Regenerate CLI docs: `make docs-cli`.
7. Document in `docs/manager-reference.md` + `docs/config-reference.md`.
8. Add an integration test against a Tier-1 distro.

---

## 5. Adding a new distro

1. Extend `session.Distro` detection in `pkg/session/distro.go`.
2. Add the distro to the per-manager dispatch tables in `pkg/managers/*.go`.
3. Add an integration job in `.github/workflows/integration.yml`.
4. Document in `docs/distro-guide.md`.

---

## 6. Release process

Releases are cut against `main` using CalVer tags:

```bash
# 1. Ensure CHANGELOG.md has a new dated section.
# 2. Create tag (TS is the sequence number for same-day releases):
git tag -a v2026.04.11.7 -m "v2026.04.11.7: comprehensive docs"
git push origin --tags

# 3. goreleaser picks up the tag via GitHub Actions:
#    - builds 5 platforms
#    - signs with cosign
#    - uploads assets to the GH release
#    - publishes the Docker image to ghcr.io
#    - opens a Homebrew formula PR
```

Release notes are auto-generated from the CHANGELOG section for that tag.

Before tagging, check for CalVer collisions:

```bash
git tag -l 'v2026.04.11*'
```

---

## 7. Code style

- `gofmt` / `goimports` — enforced by CI (`make vet`).
- Package comment on every package (one line summary).
- Exported types / funcs have a doc comment starting with the identifier.
- No panics outside `main()` except when deserialization is fundamentally
  broken (e.g. decoding `internal/testdata`).
- Errors wrapped with `%w` — never `fmt.Errorf(\"%v\")`.
- No global mutable state outside `internal/root/globalFlags` (Cobra
  binding).

---

## 8. Getting help

- Open a draft PR early — we prefer small, incremental reviews.
- Ping `@itunified-io/maintainers` on any PR waiting > 48 h.
- For design-level questions, open a `type:docs` issue with an ADR
  draft attached.
