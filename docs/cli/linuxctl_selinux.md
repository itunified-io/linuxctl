## linuxctl selinux

Manage selinux state (plan / apply / verify)

### Options

```
  -h, --help   help for selinux
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
* [linuxctl selinux apply](linuxctl_selinux_apply.md)	 - Apply selinux changes to reach the desired state
* [linuxctl selinux plan](linuxctl_selinux_plan.md)	 - Preview selinux changes against the desired state
* [linuxctl selinux verify](linuxctl_selinux_verify.md)	 - Verify observed selinux state matches desired state

