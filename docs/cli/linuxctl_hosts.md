## linuxctl hosts

Manage hosts state (plan / apply / verify)

### Options

```
  -h, --help   help for hosts
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
* [linuxctl hosts apply](linuxctl_hosts_apply.md)	 - Apply hosts changes to reach the desired state
* [linuxctl hosts plan](linuxctl_hosts_plan.md)	 - Preview hosts changes against the desired state
* [linuxctl hosts verify](linuxctl_hosts_verify.md)	 - Verify observed hosts state matches desired state

