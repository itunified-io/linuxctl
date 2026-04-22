## linuxctl disk

Manage disk state (plan / apply / verify)

### Options

```
  -h, --help   help for disk
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
* [linuxctl disk apply](linuxctl_disk_apply.md)	 - Apply disk changes (DESTRUCTIVE — confirms unless --yes)
* [linuxctl disk plan](linuxctl_disk_plan.md)	 - Preview disk changes
* [linuxctl disk verify](linuxctl_disk_verify.md)	 - Verify disk state

