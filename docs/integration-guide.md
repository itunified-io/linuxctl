# Integration Guide

`linuxctl` is one piece of a layered toolchain. It does not replace Ansible or
Terraform; it does not duplicate `mcp-host` monitoring; it does not provision
VMs. It converges a running Linux host to a declared state and does so well.

This guide shows how `linuxctl` composes with its siblings in the itunified.io
toolchain and with the wider IaC ecosystem.

---

## 1. `proxctl` — Proxmox VM + container provisioning

`proxctl` is the mutation counterpart to `mcp-proxmox`. It creates VMs and
LXC containers, attaches disks, and boots them. Its output is a running
Linux host reachable over SSH.

`linuxctl` picks up from there. The recommended flow:

```
env.yaml ---> proxctl apply apply env.yaml    (VM exists, boots, sshd up)
env.yaml ---> linuxctl apply apply env.yaml   (host layout matches linux.yaml)
```

Shared `env.yaml` convention:

```yaml
kind: Env
apiVersion: itunified.io/v1alpha1
metadata:
  name: lab
  domain: lab.internal
inventory:
  hosts:
    - name: db01
      address: 10.0.10.21
      proxmox:            # consumed by proxctl
        node: proximo01
        vmid: 1021
        template: rocky-9-cloudinit
        cpu: 4
        memory: 16384
        disks:
          - size: 100G
            role: oracle-data
      linux:              # consumed by linuxctl
        role: oracle
```

Both tools read the same file. `proxctl` only looks at `proxmox:` blocks;
`linuxctl` only looks at the `linux:` block and its own top-level
manifest keys.

After `proxctl` creates the VM, it writes the VM's `10.x.y.z` address
back into the env file (or publishes it via an inventory helper); the
subsequent `linuxctl apply` consumes the updated address.

---

## 2. `mcp-host` — read-only monitoring

`mcp-host` is the read-only monitoring companion. It exposes 20 MCP tools
for CPU, memory, disk, network, processes, services, kernel, and package
observability. It never mutates state.

Composition pattern:

```
linuxctl apply apply env.yaml    (apply desired state)
mcp-host --host db01 tools       (read live state, feed to LLM / Slack)
```

Typical uses:

- **Post-apply sanity check.** After `linuxctl apply`, an agent calls
  `host_service_status` via mcp-host to confirm the declared services
  are actually `active`.
- **Drift alerting.** Schedule `linuxctl diff --format json` nightly; pipe
  the output to an agent that correlates with mcp-host metrics and posts
  a Slack summary.
- **Incident response.** An operator asks Claude "what's wrong with
  db01?" — the agent walks through mcp-host tools, then (if allowed)
  calls `linuxctl apply plan` to propose a remediation.

`linuxctl` and `mcp-host` are deliberately partitioned: one writes, one
reads. They never share state files.

---

## 3. `dbx` — database-aware host config

`dbx` is the cross-engine database platform (PostgreSQL, Oracle, MSSQL,
MySQL, MongoDB, Redis — 29 repos, 955 tools). Its `dbxcli provision`
command wraps common DB setup and will delegate host-level prep to
`linuxctl`:

```
dbxcli provision postgres-16 --env lab --host pg01
  -> reads envs/lab/dbx/pg01.yaml (DB-level: data dir, tablespaces, users)
  -> calls linuxctl apply apply envs/lab/linux.yaml --host pg01
     (Linux-level: postgres user, /var/lib/postgresql, sysctl, limits, firewall)
  -> runs initdb and configures HA / backup / TLS
```

The `dbx` user supplies the high-level intent ("deploy PG 16 on pg01");
`dbx` renders the matching `linux.yaml` fragment and hands it to
`linuxctl` for the host layer.

---

## 4. Ansible

Ansible and `linuxctl` solve overlapping problems with different
trade-offs. Use Ansible when you already have a battle-tested playbook,
when you need ad-hoc orchestration across heterogeneous tooling, or when
your team is Ansible-native.

Composition patterns:

- **linuxctl INSIDE Ansible.** Call `linuxctl apply apply` from a role:

  ```yaml
  - name: Converge Linux layer
    command: linuxctl apply apply /etc/linuxctl/linux.yaml --yes
    register: linuxctl_out
    changed_when: "'apply: 0 failed' not in linuxctl_out.stdout"
  ```

- **Ansible INSIDE linuxctl (not recommended).** `linuxctl` deliberately
  does not shell out to Ansible. If you need playbook execution, run
  Ansible separately.

When to prefer `linuxctl` over a pure-Ansible equivalent:

- You want a machine-verifiable `Verify()` contract.
- You want signed audit trails of every change (SQLite + JWT-signed
  license).
- You want single-binary install (no Python runtime, no virtualenv).
- You manage Oracle RAC / dbx workloads where the 13-manager taxonomy
  maps cleanly.

---

## 5. Terraform

Terraform provisions infrastructure resources (VMs, DNS, load balancers,
cloud storage). It does not manage OS-level configuration well. The
proven pattern:

```
Terraform (infra)  ->  linuxctl (OS)  ->  dbx / app deployers (workload)
```

Terraform outputs feed the `env.yaml` inventory:

```hcl
resource "proxmox_vm_qemu" "pg01" {
  name = "pg01"
  ...
}

output "env_yaml" {
  value = templatefile("env.yaml.tpl", {
    hosts = [{ name = proxmox_vm_qemu.pg01.name, address = proxmox_vm_qemu.pg01.default_ipv4_address }]
  })
}
```

`terraform apply` -> writes `env.yaml` -> `linuxctl apply apply env.yaml`.

---

## 6. CI / GitOps pattern

A typical merge-to-main flow:

```
1. PR opened
   - CI runs `linuxctl config validate` on every linux.yaml.
   - CI runs `linuxctl apply plan --format json` and posts the plan as a
     PR comment (dry-run against a read-only SSH key).

2. PR merged
   - Protected runner pulls latest main.
   - `terraform apply -auto-approve` (optional).
   - `linuxctl apply apply --yes` (with destructive gate).
   - Slack notification on success / failure.

3. Hourly
   - `linuxctl apply verify --all` reports drift per host.
   - `linuxctl diff --format json --all > drift.json` piped to mcp-host
     for trend analysis.
```

This is the pattern used in the itunified.io infrastructure repo. See
the `linuxctl-deploy` skill (roadmap) for a ready-made wrapper.

---

## 7. What `linuxctl` will never do

To keep the tool focused, the following are deliberately out of scope:

- VM / container provisioning -> use `proxctl`, Terraform, or a cloud CLI.
- Database setup (initdb, clusters, users) -> use `dbx`.
- Live monitoring, metrics, alerting -> use `mcp-host`.
- Ad-hoc command execution -> use `ssh` or Ansible `ad-hoc`.
- Package authoring (building RPMs / debs) -> out of scope.
- Config file templating for arbitrary apps -> use Ansible `template` or
  a Helm-style tool.

If you find yourself reaching for `linuxctl` to do one of these, the
answer is to pick the right tool in the stack, not to grow `linuxctl`.
