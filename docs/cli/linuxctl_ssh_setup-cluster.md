## linuxctl ssh setup-cluster

Generate per-user SSH keypairs and cross-authorize across cluster nodes

### Synopsis

Reads the cluster node list from spec.hypervisor.nodes in env.yaml (or from repeated --host flags), creates ~<user>/.ssh/id_ed25519 on every host, collects all public keys, merges them into each host's authorized_keys, and seeds known_hosts via ssh-keyscan. Required for Oracle RAC grid/oracle user trust.

```
linuxctl ssh setup-cluster [env.yaml] [flags]
```

### Options

```
  -h, --help               help for setup-cluster
      --host stringArray   Cluster node hostname/IP (repeatable; overrides env.yaml)
      --parallel           Parallelise per-node key generation (default true)
      --ssh-key string     Path to SSH private key
      --ssh-port int       SSH port (default 22)
      --ssh-user string    SSH login user for cluster dial
      --user stringArray   Service account to cross-authorize (repeatable, default: grid + oracle)
```

### Options inherited from parent commands

```
      --context string   Named context from ~/.linuxctl/config.yaml
      --dry-run          Alias for plan; never mutate
      --format string    table|json|yaml|plain (default "table")
      --license string   Override ~/.linuxctl/license.jwt
      --stack string     Named stack from ~/.linuxctl/stacks.yaml
  -v, --verbose          Verbose logging
      --yes              Non-interactive; skip confirm prompts
```

### SEE ALSO

* [linuxctl ssh](linuxctl_ssh.md)	 - Manage ssh state (authorized_keys, sshd_config, cluster trust)

