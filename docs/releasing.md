# Releasing linuxctl

This runbook documents the multi-arch release pipeline for `linuxctl`: binaries on GitHub Releases, Docker images on GHCR, and a Homebrew tap formula.

## Channels

1. **GitHub Releases** — multi-arch tarballs (linux/darwin × amd64/arm64) + `checksums.txt`
2. **GHCR** — `ghcr.io/itunified-io/linuxctl:<version>` and `:latest` (linux/amd64 + linux/arm64 manifest)
3. **Homebrew tap** — formula pushed to `itunified-io/homebrew-tap`

## Prerequisites

- `goreleaser` ≥ 2.15 (`brew install goreleaser/tap/goreleaser`)
- Docker Desktop running with buildx (`docker buildx inspect --bootstrap`)
- `gh auth login` with `write:packages` scope
- `GITHUB_TOKEN` exported:
  ```bash
  export GITHUB_TOKEN=$(gh auth token)
  ```
- Docker logged into GHCR:
  ```bash
  echo "$GITHUB_TOKEN" | docker login ghcr.io -u itunified-buecheleb --password-stdin
  ```
- `itunified-io/homebrew-tap` repo must exist. If missing:
  ```bash
  gh repo create itunified-io/homebrew-tap --public \
      --description "Homebrew tap for itunified-io tools"
  ```

## Known config issues to fix before first real release

`.goreleaser.yaml` currently has a `snapshot.name_template` that references `{{ incpatch .Version }}`, which requires SemVer. With CalVer this breaks even `--snapshot`. Fix:

```yaml
snapshot:
  version_template: "{{ .Version }}-SNAPSHOT-{{ .ShortCommit }}"
```

Also remove `archives[0].format: tar.gz` (deprecated) → `formats: [tar.gz]`.

Track these under a GH issue with label `type:maintenance` before the next release.

## Important: CalVer tags need `--skip=validate`

Tags use **CalVer** (`v2026.04.11.7`), not SemVer. Always pass `--skip=validate`.

## Dry-run (snapshot)

```bash
cd /path/to/linuxctl
goreleaser release --clean --snapshot --skip=publish
ls dist/
```

## Real release

```bash
cd /path/to/linuxctl
git checkout v2026.04.11.7        # HEAD must be at the tag
export GITHUB_TOKEN=$(gh auth token)
echo "$GITHUB_TOKEN" | docker login ghcr.io -u itunified-buecheleb --password-stdin
goreleaser release --clean --skip=validate
```

## Mandatory post-publish cleanup

```bash
docker image prune -f
docker buildx prune --keep-storage=2GB -f
```

## Verification

```bash
gh release view v2026.04.11.7 --repo itunified-io/linuxctl
docker pull ghcr.io/itunified-io/linuxctl:v2026.04.11.7
docker run --rm ghcr.io/itunified-io/linuxctl:v2026.04.11.7 version
brew tap itunified-io/tap
brew install linuxctl
linuxctl version
```

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `invalid semantic version` | Use `--skip=validate` |
| `failed to parse snapshot name` (on `--snapshot`) | Apply the `version_template` fix above |
| `unauthorized: authentication required` | Re-run `docker login ghcr.io` |
| `403` pushing to homebrew-tap | Check tap repo exists + `GITHUB_TOKEN` has `repo` scope |
| Dirty tree | Commit or stash before running |
