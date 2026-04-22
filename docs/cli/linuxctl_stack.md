## linuxctl stack

Manage the local stack registry (~/.linuxctl/stacks.yaml)

### Options

```
  -h, --help   help for stack
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
* [linuxctl stack add](linuxctl_stack_add.md)	 - Register an existing stack directory
* [linuxctl stack current](linuxctl_stack_current.md)	 - Print the default stack
* [linuxctl stack list](linuxctl_stack_list.md)	 - List registered stacks
* [linuxctl stack new](linuxctl_stack_new.md)	 - Create a new local stack pointer
* [linuxctl stack remove](linuxctl_stack_remove.md)	 - Remove a registered stack
* [linuxctl stack show](linuxctl_stack_show.md)	 - Dump the resolved env.yaml tree
* [linuxctl stack use](linuxctl_stack_use.md)	 - Set the default stack

