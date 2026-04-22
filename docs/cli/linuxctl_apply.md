## linuxctl apply

Orchestrate plan / apply / verify / rollback across all managers

### Options

```
  -h, --help   help for apply
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
* [linuxctl apply apply](linuxctl_apply_apply.md)	 - Apply the full DAG
* [linuxctl apply plan](linuxctl_apply_plan.md)	 - Full DAG plan across all managers
* [linuxctl apply rollback](linuxctl_apply_rollback.md)	 - Rollback the in-memory applied set (non-persistent Phase-3 rollback)
* [linuxctl apply verify](linuxctl_apply_verify.md)	 - Verify the full DAG

