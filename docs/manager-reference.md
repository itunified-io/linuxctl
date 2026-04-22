# Manager Reference

Every `linuxctl` manager owns exactly one Linux subsystem and implements the
`Manager` interface in `pkg/managers/manager.go`:

```go
type Manager interface {
    Name() string
    DependsOn() []string
    Plan(ctx, spec, state) ([]Change, error)
    Apply(ctx, change) (*ApplyResult, error)
    Verify(ctx, spec) (bool, []Change, error)
    Rollback(ctx, change) error
}
```

Each manager section below documents:

- **Owns** — what subsystem state the manager owns.
- **YAML fields** — the `linux.yaml` keys consumed.
- **Idempotency** — what `Verify()` guarantees.
- **Hazards** — destructive operations and guardrails.
- **Example** — a minimal snippet.

---

## 1. disk

**Owns:** LVM physical volumes, volume groups, logical volumes, filesystems
(`xfs` / `ext4` / `btrfs`), and persistent mount entries in `/etc/fstab`.

**YAML fields:** `disk_layout.root`, `disk_layout.additional[]`.

```yaml
disk_layout:
  root:
    device: /dev/sda
    vg_name: vg_root
    logical_volumes:
      - name: root
        mount_point: /
        size: 50G
        fs: xfs
  additional:
    - role: oracle-data
      vg_name: vg_data
      logical_volumes:
        - name: u01
          mount_point: /u01
          size: 100G
          fs: xfs
```

**Idempotency:** Discovery uses `pvs/vgs/lvs --reportformat=json` and
`blkid`. Existing PVs, VGs, LVs with matching names and sizes are a no-op.

**Hazards:** `disk` is **additive only**. It never `pvremove`, `vgremove`,
`lvremove`, or reshapes an existing filesystem. A size mismatch on an
existing LV raises a `destructive` hazard and refuses to apply without
`--accept-destructive`.

**Rollback:** Reverses in-session creations (`lvremove`, `vgremove`,
`pvremove`). LVs that existed before the run are never touched.

---

## 2. user

**Owns:** POSIX users, groups, user home directories, `~/.ssh/authorized_keys`
per user, and per-user sudoers drop-ins under `/etc/sudoers.d/`.

**YAML fields:** `users_groups.groups[]`, `users_groups.users[]`.

```yaml
users_groups:
  groups:
    - name: dba
      gid: 5001
  users:
    - name: oracle
      uid: 54321
      gid: oinstall
      groups: [dba, asmadmin]
      home: /home/oracle
      shell: /bin/bash
      ssh_keys:
        - "ssh-ed25519 AAAA... oracle@ws"
```

**Idempotency:** `getent passwd/group` is the source of truth for state.
UID/GID mismatches on existing accounts are flagged as drift but never
rewritten (`usermod -u` is destructive — out of scope).

**Hazards:** User deletion is out of scope. Removing a user from the
manifest leaves the account in place (use a dedicated cleanup workflow).

**Rollback:** Reverses `useradd` / `groupadd` performed in-session.

---

## 3. package

**Owns:** Package install and removal across RHEL-family (dnf/yum),
Debian-family (apt), and SUSE-family (zypper).

**YAML fields:** `packages.install[]`, `packages.remove[]`,
`packages.enabled_services[]`, `packages.disabled_services[]`.

```yaml
packages:
  install:
    - postgresql16-server
    - chrony
  remove:
    - firewalld     # if you prefer ufw
  enabled_services:
    - chronyd
```

**Idempotency:** Query uses `rpm -q` / `dpkg -l` / `rpm --whatprovides`
dispatch. Installed packages are a no-op. Version pinning (`pkg-X.Y`) is
respected only if the distro's package manager supports exact-version
match semantics.

**Hazards:** `remove` is always a `warn` hazard; removing a package
cascades to services that depend on it.

**Rollback:** Best-effort reverse — `dnf remove` the packages installed
in-session, `dnf install` the packages removed in-session. Rollback may
fail if dependencies were recomputed.

---

## 4. service

**Owns:** systemd unit state — `enabled` (on boot) and `state`
(`running` / `stopped`). Only systemd is supported; sysvinit is out of
scope.

**YAML fields:** `services[]`.

```yaml
services:
  - name: postgresql-16
    enabled: true
    state: running
  - name: telemetry-agent
    enabled: false
    state: stopped
```

**Idempotency:** `systemctl is-enabled` + `systemctl is-active` are the
state probes. Matching manifest values are a no-op.

**Hazards:** Stopping a service that depends on others emits a `warn`
hazard but is not blocked.

**Rollback:** Restore prior enabled + active states.

---

## 5. mount

**Owns:** CIFS / NFS / bind / tmpfs mount points, persistent via
`/etc/fstab` when `persistent: true`.

**YAML fields:** `mounts[]`.

```yaml
mounts:
  - type: cifs
    server: nas01.lab.internal
    share: backups
    mount_point: /mnt/backups
    options: [credentials=/etc/cifs/backups.cred, vers=3.1.1]
    credentials_vault: secret/data/cifs/backups
    persistent: true
  - type: tmpfs
    mount_point: /run/oracle
    options: [size=8G,mode=0755]
    persistent: false
```

**Idempotency:** Existing fstab entries with matching `source` +
`mount_point` + `type` + sorted options are a no-op. Credentials written
to a root-only file path and chmod'd `0600` automatically.

**Hazards:** Changing mount options on an active mount requires remount;
this is classified as `warn`.

**Rollback:** Unmount and remove fstab entries added in-session.

---

## 6. sysctl

**Owns:** the managed file `/etc/sysctl.d/99-linuxctl.conf`. Never writes
to `/etc/sysctl.conf`.

**YAML fields:** `sysctl[]`, `sysctl_preset`.

```yaml
sysctl_preset: oracle-19c
sysctl:
  - key: vm.swappiness
    value: "10"
```

**Idempotency:** The file is re-rendered from the declared list on every
apply; re-reading the live kernel value (`sysctl -n <key>`) is the
verification probe.

**Hazards:** None. Kernel parameters are reversible.

**Rollback:** Previous file contents restored from in-memory snapshot.

---

## 7. limits

**Owns:** the managed file `/etc/security/limits.d/99-linuxctl.conf`.

**YAML fields:** `limits[]`, `limits_preset`.

```yaml
limits_preset: oracle-19c
limits:
  - user: oracle
    type: soft
    item: nofile
    value: "65536"
  - user: oracle
    type: hard
    item: nofile
    value: "65536"
```

**Idempotency:** File is rendered deterministically from the declared
list; no merging with other `limits.d` files.

**Hazards:** None. Changes apply to new PAM sessions.

**Rollback:** Restore previous file contents.

---

## 8. firewall

**Owns:** firewalld (preferred on RHEL family) or ufw (Debian family).
Manages zones, permanent port rules, and source-address rules. The
backend is auto-detected; override with `firewall.backend` (roadmap).

**YAML fields:** `firewall.enabled`, `firewall.zones{}`.

```yaml
firewall:
  enabled: true
  zones:
    public:
      ports:
        - port: 5432
          proto: tcp
      sources:
        - 10.0.0.0/8
```

**Idempotency:** `firewall-cmd --list-all --permanent` per zone is the
probe. Reloads are deferred to the end of the apply phase.

**Hazards:** Closing port 22 is flagged `destructive`.

**Rollback:** Reverse each port/source operation.

---

## 9. hosts

**Owns:** a managed block in `/etc/hosts`:

```
# BEGIN linuxctl
10.0.10.11  rac01 rac01.lab.internal
# END linuxctl
```

**YAML fields:** `hosts_entries[]`.

```yaml
hosts_entries:
  - ip: 10.0.10.11
    names: [rac01, rac01.lab.internal]
```

**Idempotency:** The managed block is rewritten atomically. Content
outside the markers is preserved.

**Hazards:** None. Writes to `/etc/hosts` are a file replace with backup.

**Rollback:** Restore the previous block from the `.linuxctl.bak` file.

---

## 10. network

**Owns:** Hostname (`/etc/hostname`, `hostnamectl`) and DNS resolver
configuration (`/etc/resolv.conf` on non-NetworkManager hosts;
NetworkManager connection settings otherwise). NIC-level management
(bonding, VLAN, static IPs) is **Phase 4b** and ships in `mcp-host-enterprise`.

**YAML fields:** `hostname`, `resolv_conf.nameservers[]`, `resolv_conf.search[]`.

```yaml
hostname: db01.lab.internal
resolv_conf:
  nameservers: [10.0.0.10, 10.0.0.11]
  search: [lab.internal]
```

**Idempotency:** `hostnamectl status` and `cat /etc/resolv.conf` are the
probes.

**Hazards:** Changing hostname is `warn` — some services (rsyslog,
postgres) pick up the new value only on restart.

**Rollback:** Restore previous hostname + resolv.conf snapshot.

---

## 11. ssh

**Owns:** `~/.ssh/authorized_keys` for any declared user, a drop-in
`sshd_config` at `/etc/ssh/sshd_config.d/99-linuxctl.conf`, and — with
`linuxctl ssh setup-cluster` — cross-node Ed25519 trust for the `grid`
/ `oracle` cluster users.

**YAML fields:** `ssh.authorized_keys{user:[keys]}`, `ssh.sshd_config{key:value}`.

```yaml
ssh:
  authorized_keys:
    root:
      - "ssh-ed25519 AAAA... operator-ws"
  sshd_config:
    PermitRootLogin: prohibit-password
    PasswordAuthentication: "no"
```

**Idempotency:** `authorized_keys` is rewritten deterministically from
the declared list; duplicate keys are collapsed. The `sshd_config`
drop-in is the only file touched — the main config is never modified.

**Hazards:** Removing the operator's own key is classified `destructive`.
Locking out over SSH requires `--accept-destructive`.

**Rollback:** Restore previous authorized_keys + drop-in contents.

**Cluster SSH:** `linuxctl ssh setup-cluster <env.yaml>` reads the
cluster hostnames from the env, generates per-node Ed25519 keypairs
(idempotent), cross-authorizes them in a serialized phase, and seeds
`known_hosts` via `ssh-keyscan`. See
`pkg/managers/ssh_auth.SetupClusterSSH`.

---

## 12. selinux

**Owns:** SELinux mode (`enforcing` / `permissive` / `disabled`) and
boolean overrides. Does NOT manage policy modules, file contexts, or
ports — those are workload concerns handled outside linuxctl.

**YAML fields:** `selinux.mode`, `selinux.booleans{}`.

```yaml
selinux:
  mode: enforcing
  booleans:
    httpd_can_network_connect: true
```

**Idempotency:** `getenforce` and `getsebool -a` are the probes.

**Hazards:** `enforcing` -> `disabled` requires a reboot and is
classified `destructive`. The manager refuses without
`--accept-destructive` and writes a reboot-required marker to
`/etc/linuxctl/reboot-required`.

**Rollback:** Restore previous mode + boolean values (no reboot; the
`disabled` -> `enforcing` transition also requires a reboot).

---

## 13. dir

**Owns:** filesystem directories with declared owner, group, mode, and
optional recursive application. Does not manage files — only directories.

**YAML fields:** `directories[]`.

```yaml
directories:
  - path: /u01/app/oracle
    owner: oracle
    group: oinstall
    mode: "0775"
    recursive: true
  - path: /var/log/linuxctl
    owner: root
    group: root
    mode: "0755"
```

**Idempotency:** `stat` on each path compares owner/group/mode. Missing
directories are created with `mkdir -p`. If `recursive: true`, the
owner/group/mode is applied recursively with `chown -R` / `chmod -R`.

**Hazards:** `recursive: true` on a large tree is `warn`; on a system
directory (`/`, `/etc`, `/var`) it is `destructive`.

**Rollback:** Remove directories created in-session. Pre-existing
directories are never touched.

---

## Manager protocol invariants

1. **Plan() is pure.** No mutation, no lock, no state-db write. Runs
   repeatedly are idempotent and parallel-safe.
2. **Apply() is the only writer.** One `ApplyResult` record per change,
   flushed to SQLite before the function returns.
3. **Verify() MUST equal Plan() == empty.** Any manager that does not
   satisfy this has a bug.
4. **Rollback(c) reverses Apply(c).** The manager may not always have
   enough information to fully reverse (e.g. package dependencies); in
   that case it returns a `PartialRollbackError`.
5. **No manager writes outside its declared footprint.** Footprints are
   enforced in code and audited in tests (`pkg/managers/*_test.go`).

For the generated per-command CLI help, see
[`cli-reference.md`](cli-reference.md) and `docs/cli/linuxctl_<manager>.md`.
