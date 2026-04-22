## linuxctl service

Manage service state (plan / apply / verify)

### Options

```
  -h, --help   help for service
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
* [linuxctl service apply](linuxctl_service_apply.md)	 - Apply service changes to reach the desired state
* [linuxctl service plan](linuxctl_service_plan.md)	 - Preview service changes against the desired state
* [linuxctl service verify](linuxctl_service_verify.md)	 - Verify observed service state matches desired state

