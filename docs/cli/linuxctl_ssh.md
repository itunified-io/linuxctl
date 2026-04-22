## linuxctl ssh

Manage ssh state (authorized_keys, sshd_config, cluster trust)

### Options

```
  -h, --help   help for ssh
```

### Options inherited from parent commands

```
      --context string   Named context from ~/.linuxctl/config.yaml
      --dry-run          Alias for plan; never mutate
      --format string    table|json|yaml|plain (default "table")
      --host string      Restrict to a single host from the stack
      --license string   Override ~/.linuxctl/license.jwt
      --stack string     Named stack from ~/.linuxctl/stacks.yaml
  -v, --verbose          Verbose logging
      --yes              Non-interactive; skip confirm prompts
```

### SEE ALSO

* [linuxctl](linuxctl.md)	 - Declarative, idempotent, auditable Linux host configuration
* [linuxctl ssh apply](linuxctl_ssh_apply.md)	 - Apply ssh changes
* [linuxctl ssh plan](linuxctl_ssh_plan.md)	 - Preview ssh changes
* [linuxctl ssh setup-cluster](linuxctl_ssh_setup-cluster.md)	 - Generate per-user SSH keypairs and cross-authorize across cluster nodes
* [linuxctl ssh verify](linuxctl_ssh_verify.md)	 - Verify ssh state

