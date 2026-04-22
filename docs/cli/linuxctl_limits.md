## linuxctl limits

Manage limits state (plan / apply / verify)

### Options

```
  -h, --help   help for limits
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
* [linuxctl limits apply](linuxctl_limits_apply.md)	 - Apply limits changes to reach the desired state
* [linuxctl limits plan](linuxctl_limits_plan.md)	 - Preview limits changes against the desired state
* [linuxctl limits verify](linuxctl_limits_verify.md)	 - Verify observed limits state matches desired state

