# Installation

`linuxctl` ships as a single static Go binary. No runtime, no daemon, no agent
on the managed hosts — only an `sshd` on the target and a TCP path from the
operator workstation to the target. This page covers every supported install
method, how to seed a license, and how to verify the first run.

Supported operator platforms: Linux amd64/arm64, macOS amd64/arm64, Windows
amd64. Supported target hosts: see [`distro-guide.md`](distro-guide.md).

---

## 1. Homebrew (recommended for macOS and Linux)

A public Homebrew tap will be published once the v1.0.0 CalVer release lands.
Until then, install the binary with `go install` or from a GitHub release
asset.

```bash
brew tap itunified-io/tap
brew install linuxctl
linuxctl version
```

Upgrading:

```bash
brew upgrade linuxctl
```

The formula is published as part of the `goreleaser` pipeline, so every CalVer
tag in `itunified-io/linuxctl` produces a matching Homebrew bottle.

---

## 2. Direct binary download

Every release publishes signed binaries on the GitHub release page:

```
https://github.com/itunified-io/linuxctl/releases/latest
```

Available archives:

| Platform       | Archive                                    |
| -------------- | ------------------------------------------ |
| Linux amd64    | `linuxctl_<VERSION>_linux_amd64.tar.gz`    |
| Linux arm64    | `linuxctl_<VERSION>_linux_arm64.tar.gz`    |
| macOS amd64    | `linuxctl_<VERSION>_darwin_amd64.tar.gz`   |
| macOS arm64    | `linuxctl_<VERSION>_darwin_arm64.tar.gz`   |
| Windows amd64  | `linuxctl_<VERSION>_windows_amd64.zip`     |

Install on Linux:

```bash
curl -L -o linuxctl.tgz \
  https://github.com/itunified-io/linuxctl/releases/latest/download/linuxctl_linux_amd64.tar.gz
tar -xzf linuxctl.tgz
sudo install -m 0755 linuxctl /usr/local/bin/linuxctl
linuxctl version
```

Every archive ships with a SHA-256 checksum and a cosign signature. Always
verify before installing in production:

```bash
cosign verify-blob \
  --certificate-identity-regexp 'https://github.com/itunified-io/linuxctl' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --signature linuxctl.tgz.sig \
  linuxctl.tgz
```

---

## 3. Docker image

Useful for CI systems and for operators who do not want to install a Go
toolchain on their workstation.

```bash
docker run --rm -it \
  -v $HOME/.linuxctl:/root/.linuxctl \
  -v $HOME/.ssh:/root/.ssh:ro \
  -v $PWD:/workspace -w /workspace \
  ghcr.io/itunified-io/linuxctl:latest \
  config validate envs/lab/linux.yaml
```

Volume conventions:

- `/root/.linuxctl` — license JWT, context, stack registry (`stacks.yaml`), SQLite state.
- `/root/.ssh` — operator private keys (read-only mount).
- `/workspace` — the directory containing your `linux.yaml` / `env.yaml`.

Tags follow CalVer: `ghcr.io/itunified-io/linuxctl:v2026.04.11.7`.
`latest` always tracks the most recent release; pin a tag in CI.

---

## 4. Build from source

Requires Go 1.22+.

```bash
git clone https://github.com/itunified-io/linuxctl
cd linuxctl
make build
./bin/linuxctl version
```

Or with `go install`:

```bash
go install github.com/itunified-io/linuxctl/cmd/linuxctl@latest
```

The binary is built with `CGO_ENABLED=0`, `-trimpath`, and version metadata
injected via `-ldflags`. See [`Makefile`](../Makefile).

Regenerate the CLI reference after adding new commands:

```bash
make docs-cli
```

---

## 5. Air-gapped environments

For environments without outbound internet access:

1. On a workstation with internet, download the release tarball and
   checksum file for every platform you need.
2. Mirror the tarballs to your internal artifact store (Artifactory, S3,
   internal Nexus, etc.).
3. Copy the binary into `/usr/local/bin/linuxctl` on the operator bastion.
4. Seed the license from your license vault (see
   [`licensing.md`](licensing.md)).

`linuxctl` itself is fully offline-capable. It only contacts your license
vault (or a local file) for license validation and the target hosts over
SSH. There is no telemetry and no phone-home.

---

## 6. Shell completion

Completion scripts are bundled. Install them once and they self-update as
the command tree evolves.

Bash:

```bash
linuxctl completion bash | sudo tee /etc/bash_completion.d/linuxctl
```

Zsh (oh-my-zsh):

```bash
linuxctl completion zsh > "${fpath[1]}/_linuxctl"
```

Fish:

```bash
linuxctl completion fish > ~/.config/fish/completions/linuxctl.fish
```

PowerShell:

```powershell
linuxctl completion powershell | Out-String | Invoke-Expression
```

---

## 7. License setup

Every Business / Enterprise feature is gated by a signed Ed25519 JWT. The
default path is `~/.linuxctl/license.jwt`; override with the `--license`
flag or the `LINUXCTL_LICENSE_FILE` environment variable.

```bash
mkdir -p ~/.linuxctl
chmod 700 ~/.linuxctl
# Write the JWT provided by itunified.io (contents are a single line starting eyJ…):
vim ~/.linuxctl/license.jwt
chmod 600 ~/.linuxctl/license.jwt

linuxctl license show
linuxctl license verify
```

Community tier works without a license file: core single-host workflows are
free forever. See [`licensing.md`](licensing.md) for tier details and
feature gating.

---

## 8. SSH client setup (target hosts)

`linuxctl` is agentless. It connects over `ssh` using the operator's normal
SSH config. Requirements on every target host:

- `sshd` on TCP/22 (or a reachable port defined in the env manifest).
- The operator's SSH public key in the target user's `~/.ssh/authorized_keys`.
- The target user has `sudo` with `NOPASSWD` for the commands linuxctl
  invokes (or `linuxctl` runs as `root` directly).

Recommended bootstrap user on a fresh host:

```bash
# On the target, once:
adduser --disabled-password linuxctl
usermod -aG wheel linuxctl      # or 'sudo' on Debian/Ubuntu
echo 'linuxctl ALL=(ALL) NOPASSWD:ALL' | sudo tee /etc/sudoers.d/linuxctl
sudo install -d -o linuxctl -g linuxctl -m 0700 /home/linuxctl/.ssh
sudo tee -a /home/linuxctl/.ssh/authorized_keys <<< "ssh-ed25519 AAAA... operator@ws"
```

After the first run, `linuxctl` can manage its own access via the `ssh`
manager (see [`manager-reference.md`](manager-reference.md)).

Key authentication only. Password authentication is disabled by convention
and never prompted. Host-key verification uses the operator's
`~/.ssh/known_hosts`; populate it out-of-band or accept host keys manually
before the first `linuxctl apply`.

---

## 9. First-run verification

After installing the binary:

```bash
linuxctl version
linuxctl --help
```

You should see a version matching the release you installed, the build
commit, and the build date. The top-level help lists the 13 manager
commands plus `apply`, `diff`, `config`, `env`, `license`, and `version`.

Run a trivial validation against the example `linux.yaml` shipped in this
repository:

```bash
linuxctl config validate docs/examples/host-only/linux.yaml
```

Expected output: `OK`.

Run a plan against `localhost` — this exercises the local session
implementation without touching any remote host:

```bash
linuxctl dir plan docs/examples/host-only/linux.yaml --host localhost
```

Expected output: a plan summary with zero hazards (the example only
declares a `/tmp/linuxctl-first-run` directory). No mutations are made by
`plan`.

Next: [`quick-start.md`](quick-start.md).
