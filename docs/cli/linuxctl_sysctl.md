## linuxctl sysctl

Manage sysctl state (plan / apply / verify)

### Options

```
  -h, --help   help for sysctl
```

### Options inherited from parent commands

```
      --context string   Named context from ~/.linuxctl/config.yaml
      --dry-run          Alias for plan; never mutate
      --env string       Named env from ~/.linuxctl/envs.yaml
      --format string    table|json|yaml|plain (default "table")
      --host string      Restrict to a single host from the env
      --license string   Override ~/.linuxctl/license.jwt
  -v, --verbose          Verbose logging
      --yes              Non-interactive; skip confirm prompts
```

### SEE ALSO

* [linuxctl](linuxctl.md)	 - Declarative, idempotent, auditable Linux host configuration
* [linuxctl sysctl apply](linuxctl_sysctl_apply.md)	 - Apply sysctl changes to reach the desired state
* [linuxctl sysctl plan](linuxctl_sysctl_plan.md)	 - Preview sysctl changes against the desired state
* [linuxctl sysctl verify](linuxctl_sysctl_verify.md)	 - Verify observed sysctl state matches desired state

