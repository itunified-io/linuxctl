# Config Reference

The full schema for every YAML file `linuxctl` consumes. Every field is
backed by a Go struct in `pkg/config` with `go-playground/validator` tags
enforced by `linuxctl config validate`.

- `env.yaml` — environment metadata and host inventory.
- `linux.yaml` — the desired-state manifest; the primary input.
- `~/.linuxctl/config.yaml` — operator-scoped context settings.
- `~/.linuxctl/stacks.yaml` — registered stacks (legacy `envs.yaml` auto-migrated, see #17).

---

## 1. `env.yaml`

Optional top-level metadata for a named environment. Referenced via
`linuxctl --stack <name>` and resolved from `~/.linuxctl/stacks.yaml`. The
file name `env.yaml` and the `kind: Env` YAML tag are retained for
backward compatibility.

```yaml
kind: Env
apiVersion: linuxctl.itunified.io/v1alpha1
metadata:
  name: prod-lab
  domain: lab.internal
  tags:
    location: munich
    tier: production
inventory:
  hosts:
    - name: rac01
      address: 10.0.10.11
      roles: [oracle, rac]
    - name: rac02
      address: 10.0.10.12
      roles: [oracle, rac]
```

### Fields

| Field                    | Type           | Description |
|--------------------------|----------------|-------------|
| `kind`                   | string         | MUST be `Env`. |
| `apiVersion`             | string         | `linuxctl.itunified.io/v1alpha1`. |
| `metadata.name`          | string         | Unique env name. |
| `metadata.domain`        | string         | DNS domain of the env; used for search-domain defaults. |
| `metadata.tags`          | `map[str]str`  | Arbitrary k/v; surfaced in audit records. |
| `inventory.hosts[]`      | `[]HostEntry`  | List of hosts. |
| `inventory.hosts[].name` | string         | Short hostname. |
| `inventory.hosts[].address` | string      | Reachable IP or FQDN. |
| `inventory.hosts[].roles[]` | `[]string` | Free-form role tags (`oracle`, `rac`, `grid`, `app`). |

---

## 2. `linux.yaml`

The primary manifest. All fields are optional except `kind`. The structure
mirrors `pkg/config/linux.go`.

```yaml
kind: Linux
apiVersion: linuxctl.itunified.io/v1alpha1
disk_layout:     ...
users_groups:    ...
directories:     ...
mounts:          ...
packages:        ...
sysctl:          ...
sysctl_preset:   oracle-19c
limits:          ...
limits_preset:   oracle-19c
firewall:        ...
hosts_entries:   ...
services:        ...
ssh:             ...
selinux:         ...
```

### 2.1 `disk_layout`

```yaml
disk_layout:
  root:
    device: /dev/sda            # required
    vg_name: vg_root            # optional
    logical_volumes:            # optional
      - name: root
        mount_point: /          # MUST begin with /
        size: 50G               # required
        fs: xfs                 # xfs | ext4 | btrfs
  additional:
    - device: /dev/sdb          # one of device, role, or tag
      role: oracle-data
      tag: primary
      vg_name: vg_data          # required
      logical_volumes:          # min=1
        - name: u01
          mount_point: /u01
          size: 100G
          fs: xfs
```

### 2.2 `users_groups`

```yaml
users_groups:
  groups:
    - name: dba                 # required
      gid: 5001                 # >= 1000 if set
  users:
    - name: oracle              # required
      uid: 54321                # >= 1000 if set
      gid: oinstall             # may reference a group by name
      groups: [dba, asmadmin]
      home: /home/oracle
      shell: /bin/bash
      ssh_keys:
        - "ssh-ed25519 AAAA..."
      password: "{{ vault: secret/data/users/oracle#password }}"
```

### 2.3 `directories`

```yaml
directories:
  - path: /u01/app              # required, must start with /
    owner: oracle
    group: oinstall
    mode: "0775"                # 4 digits exact
    recursive: true             # default false
```

### 2.4 `mounts`

```yaml
mounts:
  - type: cifs                  # cifs | nfs | bind | tmpfs
    source: //nas/backups       # type=bind
    server: nas01.lab.internal  # type=cifs or nfs
    share: backups
    mount_point: /mnt/backups   # required
    options: [vers=3.1.1]
    credentials_vault: secret/data/cifs/backups
    persistent: true            # add to /etc/fstab
```

### 2.5 `packages`

```yaml
packages:
  install:           [postgresql16, chrony]
  remove:            [firewalld]
  enabled_services:  [chronyd, postgresql-16]
  disabled_services: [kdump]
```

### 2.6 `sysctl` and `sysctl_preset`

```yaml
sysctl_preset: oracle-19c
sysctl:
  - key: vm.swappiness         # required
    value: "10"                # required, string form
```

See [`preset-guide.md`](preset-guide.md) for shipped presets.

### 2.7 `limits` and `limits_preset`

```yaml
limits_preset: oracle-19c
limits:
  - user: oracle               # required
    type: soft                 # soft | hard
    item: nofile               # required
    value: "65536"             # required
```

### 2.8 `firewall`

```yaml
firewall:
  enabled: true
  zones:
    public:
      ports:
        - name: postgres
          port: 5432            # OR port_range
          port_range: "6000-6010"
          proto: tcp            # tcp | udp
      sources: [10.0.0.0/8]
      sources_from_network: "lab.internal"
```

### 2.9 `hosts_entries`

```yaml
hosts_entries:
  - ip: 10.0.10.11              # validated as IP
    names: [rac01, rac01.lab.internal]
```

### 2.10 `services`

```yaml
services:
  - name: postgresql-16         # required
    enabled: true
    state: running              # running | stopped
```

### 2.11 `ssh`

```yaml
ssh:
  authorized_keys:
    root:
      - "ssh-ed25519 AAAA..."
    oracle:
      - "ssh-ed25519 AAAA..."
  sshd_config:
    PermitRootLogin: prohibit-password
    PasswordAuthentication: "no"
    X11Forwarding: "no"
```

### 2.12 `selinux`

```yaml
selinux:
  mode: enforcing               # enforcing | permissive | disabled
  booleans:
    httpd_can_network_connect: true
    samba_export_all_rw: false
```

---

## 3. Secret resolvers

Any string value may be replaced by a secret resolver. The resolver
syntax is recognized during render (not during parse), so the YAML is
valid as-is.

Supported resolver forms:

| Form                                                   | Example |
|--------------------------------------------------------|---------|
| `{{ vault: <path>#<key> }}`                            | `{{ vault: secret/data/cifs/backups#password }}` |
| `{{ env: <VAR> }}`                                     | `{{ env: POSTGRES_PASSWORD }}` |
| `{{ file: <path> }}`                                   | `{{ file: /etc/linuxctl/secrets/backup.key }}` |
| `{{ sops: <path> }}`                                   | `{{ sops: secrets/prod.yaml#mount.cifs.pwd }}` |

Resolvers are evaluated at apply time; `config render` prints the
resolved manifest with every secret value REDACTED.

---

## 4. `$ref` composition

Any sub-tree may be externalized and pulled in via `$ref`. Paths are
resolved relative to the manifest file.

```yaml
users_groups:
  $ref: ./shared/oracle-users.yaml

directories:
  - $ref: ./shared/oracle-dirs.yaml#/u01
  - $ref: ./shared/oracle-dirs.yaml#/u02
```

Fragment paths use JSON Pointer (`#/path/to/node`). Circular refs are
detected at load time.

---

## 5. `~/.linuxctl/config.yaml`

```yaml
contexts:
  - name: lab
    env: lab
    format: table
    license: ~/.linuxctl/license.jwt
  - name: prod
    env: prod
    format: json
    license: ~/.linuxctl/prod-license.jwt
current-context: lab
```

---

## 6. `~/.linuxctl/stacks.yaml`

```yaml
stacks:
  lab:
    path: /home/op/repos/infrastructure/envs/lab
    default: true
  prod:
    path: /home/op/repos/infrastructure/envs/prod
```

> **Deprecation note (#17):** `~/.linuxctl/envs.yaml` is the legacy name
> and is auto-migrated to `stacks.yaml` on first run. The CLI flag
> `--env` and env var `LINUXCTL_ENV` remain available as deprecated
> aliases for `--stack` / `LINUXCTL_STACK`; both emit a deprecation
> warning and will be removed in the next release. The manifest filename
> `env.yaml` and the `kind: Env` YAML tag are unchanged.

---

## 7. Validation

Run `linuxctl config validate <path>` in CI. It executes:

1. YAML parse.
2. `validator.Struct()` on every decoded type.
3. Cross-reference checks: user -> group, service -> package, mount
   credentials -> vault path shape.
4. Preset existence: any `sysctl_preset` / `limits_preset` name resolved
   against the shipped preset registry.

Validation is the only step `linuxctl config validate` performs; it does
not touch the filesystem and does not contact any host. Use it freely in
pre-commit hooks and CI.
