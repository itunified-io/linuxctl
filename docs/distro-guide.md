# Distro Guide

`linuxctl` is distro-aware. Each manager detects the target's distro family
and dispatches to the correct backend (package manager, firewall, init
system, etc.). This page documents the supported distros, detection
details, and per-manager behavior differences.

---

## 1. Supported distros

| Distro                    | Version        | Family  | Status   |
|---------------------------|----------------|---------|----------|
| Red Hat Enterprise Linux  | 8, 9           | RHEL    | Tier 1   |
| Oracle Linux              | 8, 9           | RHEL    | Tier 1   |
| Rocky Linux               | 8, 9           | RHEL    | Tier 1   |
| AlmaLinux                 | 8, 9           | RHEL    | Tier 1   |
| Ubuntu                    | 22.04, 24.04   | Debian  | Tier 1   |
| Debian                    | 12             | Debian  | Tier 2   |
| SLES                      | 15 SP5+        | SUSE    | Tier 2   |
| openSUSE Leap             | 15.5+          | SUSE    | Tier 2   |

Tiers:

- **Tier 1** — CI runs the full integration suite on every PR.
- **Tier 2** — CI runs smoke tests on every PR; full suite nightly.

Unsupported (by design): Alpine (no systemd), Gentoo, Arch, FreeBSD,
NixOS (out-of-scope — NixOS manages its own state).

---

## 2. Distro detection

`linuxctl` reads `/etc/os-release` from the target on session open and
caches the result for the lifetime of the session. The fields consulted:

- `ID` — primary identifier (`rhel`, `ol`, `rocky`, `almalinux`, `ubuntu`,
  `debian`, `sles`, `opensuse-leap`).
- `ID_LIKE` — fallback family (`rhel fedora`, `debian`).
- `VERSION_ID` — major version for feature flags (`8`, `9`, `22.04`).

The internal `session.Distro` struct carries `Family`, `ID`, `VersionID`,
and a predicate helper `IsRHEL() / IsDebian() / IsSUSE()`.

---

## 3. Per-manager distro behavior

### 3.1 package

| Family | Backend         | Install            | Remove             | Query |
|--------|-----------------|--------------------|--------------------|-------|
| RHEL   | `dnf` (`yum` on EL7) | `dnf install -y`   | `dnf remove -y`    | `rpm -q` |
| Debian | `apt`           | `apt-get install -y` | `apt-get remove -y` | `dpkg -l` |
| SUSE   | `zypper`        | `zypper -n install` | `zypper -n remove` | `rpm -q` |

Package name mapping is **not** automatic. `postgresql16-server` is the
RHEL name; on Ubuntu the same package is `postgresql-16`. Declare the
distro-correct name, or use a `$ref` overlay per distro family.

### 3.2 firewall

| Family | Default backend | Also supported |
|--------|-----------------|----------------|
| RHEL   | `firewalld`     | `ufw` (override) |
| Debian | `ufw`           | `firewalld` (override) |
| SUSE   | `firewalld`     | `ufw` (override) |

Override via `firewall.backend: firewalld|ufw` (roadmap). Auto-detection
looks for the active service; if both are installed, firewalld wins.

### 3.3 service

All supported distros ship systemd. The `service` manager uses `systemctl`
exclusively. Hosts without systemd (Alpine, busybox-init) are rejected at
session open.

### 3.4 network

| Family | Hostname        | DNS config                              |
|--------|-----------------|-----------------------------------------|
| RHEL 8+  | `hostnamectl` | `/etc/resolv.conf` OR NetworkManager connection |
| Ubuntu 22+ | `hostnamectl` | `systemd-resolved` via `/etc/systemd/resolved.conf` |
| Debian 12  | `hostnamectl` | `/etc/resolv.conf` |
| SLES 15    | `hostnamectl` | `/etc/sysconfig/network/config` (netconfig) |

On Ubuntu with `systemd-resolved`, `/etc/resolv.conf` is a stub; the
network manager writes to `resolved.conf` and triggers `systemctl restart
systemd-resolved`.

### 3.5 selinux

| Family | Status                               |
|--------|--------------------------------------|
| RHEL   | Full support (enforcing/permissive/disabled + booleans) |
| Debian | SELinux package `selinux-basics` required; booleans supported |
| SUSE   | AppArmor preferred by distro — `selinux` manager **skipped** unless SELinux is explicitly installed |

When selinux is skipped, the manager emits an informational log line and
produces zero changes — this is not a drift.

### 3.6 disk / mount

Distro-agnostic. All supported distros ship LVM2, `xfsprogs`,
`e2fsprogs`, and `btrfs-progs` out of the box or via the `install` list.

### 3.7 user

Distro-agnostic. `useradd`, `usermod`, `groupadd`, and PAM behavior are
POSIX-standardized. Exception: the `wheel` vs `sudo` group. The `user`
manager maps `groups: [sudo]` on RHEL to the `wheel` group if `sudo`
doesn't exist.

### 3.8 sysctl / limits / hosts / dir / ssh

All distro-agnostic. These managers write to standardized paths
(`/etc/sysctl.d/`, `/etc/security/limits.d/`, `/etc/hosts`, the FS,
`/etc/ssh/sshd_config.d/`) that every supported distro honors.

---

## 4. Distro-specific gotchas

### RHEL 8 / Oracle Linux 8

- `dnf` is default; `yum` is a compatibility alias. `linuxctl` uses
  `dnf` unconditionally.
- `firewalld` + `nftables` backend: `firewall-cmd --reload` is required
  after rule changes (the `firewall` manager defers this to end-of-apply).
- SELinux is `enforcing` by default.

### Ubuntu 22.04 / 24.04

- `apt` wrapper: `linuxctl` uses `apt-get` (stable for scripting) rather
  than `apt` (interactive).
- `ufw` default on desktop spins; server images often ship without a
  firewall enabled.
- `systemd-resolved` active; see network table above.

### SLES 15 SP5+

- `zypper` returns exit code 106 when a repository refresh is pending;
  the `package` manager treats this as transient and retries after
  `zypper ref`.
- `netconfig` wraps `/etc/resolv.conf` — writing to it directly is
  overwritten on the next DHCP event. The `network` manager writes to
  `/etc/sysconfig/network/config` and runs `netconfig update -f`.

### Oracle Linux UEK vs RHCK

Both kernels behave identically for every linuxctl manager. `sysctl`
keys vary only for kernel-module-specific tunables (not set by any
shipped preset).

---

## 5. Roadmap

- **Tier 1 for Debian 12** — currently Tier 2; upgrade target is the
  v2026.06.x release train.
- **Alpine support** — blocked on systemd dependency. May ship as a
  subset that skips `service`, `network`, `selinux`.
- **FreeBSD** — out of scope.

Track on [GitHub Milestones](https://github.com/itunified-io/linuxctl/milestones).
