## linuxctl network

Manage network state (plan / apply / verify)

### Options

```
  -h, --help   help for network
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
* [linuxctl network apply](linuxctl_network_apply.md)	 - Apply network changes to reach the desired state
* [linuxctl network plan](linuxctl_network_plan.md)	 - Preview network changes against the desired state
* [linuxctl network verify](linuxctl_network_verify.md)	 - Verify observed network state matches desired state

