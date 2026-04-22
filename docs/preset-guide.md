# Preset Guide

Presets are named bundles of kernel-level (`sysctl`) and session-level
(`limits`) settings shipped with `linuxctl`. They give you a known-good
baseline for common workloads without copy-pasting dozens of keys into
every `linux.yaml`.

---

## 1. What a preset is

A preset consists of two parallel lookups:

- **sysctl preset:** maps a named bundle to a list of `SysctlEntry`
  records, written to `/etc/sysctl.d/99-linuxctl.conf`.
- **limits preset:** maps a named bundle to a list of `LimitEntry`
  records, written to `/etc/security/limits.d/99-linuxctl.conf`.

A preset is activated via top-level manifest keys:

```yaml
sysctl_preset: oracle-19c
limits_preset: oracle-19c
```

Explicit `sysctl:` and `limits:` entries in the same manifest are
**merged on top of** the preset, with per-key last-write-wins. This lets
you start with a shipped baseline and tweak selectively:

```yaml
sysctl_preset: oracle-19c
sysctl:
  - key: vm.swappiness
    value: "5"            # override the preset's default (10)
```

---

## 2. Shipped presets

| Preset name     | sysctl | limits | Tier       | Status  |
|-----------------|--------|--------|------------|---------|
| `oracle-19c`    | yes    | yes    | Community  | Stable  |
| `pg-16`         | yes    | yes    | Business   | Stub    |
| `hardened-cis`  | yes    | yes    | Business   | Stub    |

### 2.1 `oracle-19c`

The Oracle 19c RDBMS preinstall package `oracle-database-preinstall-19c`
(available on Oracle Linux 8/9 and Rocky/Alma 8/9) writes a specific
set of kernel parameters and ulimits. The `oracle-19c` linuxctl preset
mirrors that set exactly so it works on any RHEL-family distro without
the preinstall RPM.

sysctl keys included (abridged):

```
kernel.shmmax = 4398046511104
kernel.shmall = 1073741824
kernel.shmmni = 4096
kernel.sem    = 250 32000 100 128
fs.aio-max-nr = 1048576
fs.file-max   = 6815744
net.ipv4.ip_local_port_range = 9000 65500
net.core.rmem_default = 262144
net.core.rmem_max     = 4194304
net.core.wmem_default = 262144
net.core.wmem_max     = 1048576
vm.swappiness = 10
```

limits entries included:

```
oracle  soft nofile  1024
oracle  hard nofile  65536
oracle  soft nproc   16384
oracle  hard nproc   16384
oracle  soft stack   10240
oracle  hard stack   32768
grid    soft nofile  1024
grid    hard nofile  65536
grid    soft nproc   16384
grid    hard nproc   16384
```

Use this preset for any Oracle 19c (single-instance or RAC) host. Pair
with the [`oracle-rac-2node` example](examples/oracle-rac-2node/linux.yaml)
for a full RAC host baseline.

### 2.2 `pg-16` (Business tier — stub)

Planned coverage:

- `kernel.shmmax`, `kernel.shmall` sized for the declared RAM budget.
- `vm.overcommit_memory = 2`, `vm.overcommit_ratio = 80`.
- `vm.swappiness = 10`.
- `nofile`, `nproc` limits for the `postgres` user.

Current status: the name is reserved and `linuxctl config validate`
accepts it, but the registry returns a `TierRequiredError` on apply
until the Business license is loaded.

### 2.3 `hardened-cis` (Business tier — stub)

Planned coverage: the CIS Benchmark Level 1 kernel baseline:

- IPv4 + IPv6 forwarding + redirect settings.
- `kernel.randomize_va_space = 2`.
- `fs.suid_dumpable = 0`.
- Tightened `limits` for `root`.

Current status: stub (same tier gating as `pg-16`).

---

## 3. Tier gating

- **Community tier:** `oracle-19c` is available. Any other preset name
  returns a `TierRequiredError` at apply time — validation still passes
  so your CI pipeline can describe the full manifest, but actual mutation
  is gated.
- **Business tier:** unlocks `pg-16`, `hardened-cis`, and future shipped
  presets plus custom preset paths.
- **Enterprise tier:** additionally unlocks policy-controlled preset
  inheritance and fleet-wide preset rollout metrics.

See [`licensing.md`](licensing.md) for a full tier matrix.

---

## 4. Custom presets (roadmap)

Custom user-defined presets are planned for the Business tier. The design:

```yaml
# ~/.linuxctl/presets/acme-oracle.yaml
kind: SysctlPreset
name: acme-oracle
entries:
  - key: kernel.shmmax
    value: "8796093022208"
  - key: vm.swappiness
    value: "5"
```

```yaml
# linux.yaml
sysctl_preset: acme-oracle
```

Resolution order:

1. Built-in presets (shipped with the binary).
2. `~/.linuxctl/presets/*.yaml` (user).
3. `$ENV_DIR/presets/*.yaml` (repo-scoped).

Last match wins. A shipped preset can be shadowed by a same-named file
in the env directory, enabling org-wide baselines committed alongside
the manifest.

This feature is not yet implemented. Track progress in linuxctl#14.

---

## 5. Writing a preset well

When contributing or extending presets:

- **One workload per preset.** `oracle-19c` is for Oracle 19c only;
  `pg-16` is for PostgreSQL 16 only. Do not combine workloads.
- **Pin by major version.** `oracle-19c`, `pg-16` — not `oracle` or `pg`.
- **Match the upstream vendor values exactly** when a vendor preset exists
  (Oracle preinstall RPM, PostgreSQL `postgresql-tuned`, CIS Benchmarks).
- **Document the source.** The preset's doc comment MUST cite the source
  spec (vendor PDF, RPM version, benchmark issue number).
- **Test on every supported distro.** See
  [`distro-guide.md`](distro-guide.md) for the test matrix.

See `pkg/config/preset_*.go` (roadmap) for the registry API.
