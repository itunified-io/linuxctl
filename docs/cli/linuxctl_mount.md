## linuxctl mount

Manage mount state (plan / apply / verify)

### Options

```
  -h, --help   help for mount
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
* [linuxctl mount apply](linuxctl_mount_apply.md)	 - Apply mount changes
* [linuxctl mount plan](linuxctl_mount_plan.md)	 - Preview mount changes
* [linuxctl mount verify](linuxctl_mount_verify.md)	 - Verify mount state

