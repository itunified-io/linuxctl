## linuxctl network plan

Preview network changes against the desired state

```
linuxctl network plan [flags]
```

### Options

```
  -h, --help   help for plan
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

* [linuxctl network](linuxctl_network.md)	 - Manage network state (plan / apply / verify)

