## linuxctl package

Manage package state (plan / apply / verify)

### Options

```
  -h, --help   help for package
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
* [linuxctl package apply](linuxctl_package_apply.md)	 - Apply package changes
* [linuxctl package plan](linuxctl_package_plan.md)	 - Preview package changes
* [linuxctl package verify](linuxctl_package_verify.md)	 - Verify package state

