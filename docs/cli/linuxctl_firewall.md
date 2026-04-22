## linuxctl firewall

Manage firewall state (plan / apply / verify)

### Options

```
  -h, --help   help for firewall
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
* [linuxctl firewall apply](linuxctl_firewall_apply.md)	 - Apply firewall changes to reach the desired state
* [linuxctl firewall plan](linuxctl_firewall_plan.md)	 - Preview firewall changes against the desired state
* [linuxctl firewall verify](linuxctl_firewall_verify.md)	 - Verify observed firewall state matches desired state

