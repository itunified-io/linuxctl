## linuxctl dir

Manage dir state (plan / apply / verify)

### Options

```
  -f, --file string   Path to linux.yaml or env.yaml manifest (default "linux.yaml")
  -h, --help          help for dir
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
* [linuxctl dir apply](linuxctl_dir_apply.md)	 - Apply dir changes to reach the desired state
* [linuxctl dir plan](linuxctl_dir_plan.md)	 - Preview dir changes against the desired state
* [linuxctl dir verify](linuxctl_dir_verify.md)	 - Verify observed dir state matches desired state

