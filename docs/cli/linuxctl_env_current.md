## linuxctl env current

Print the default env

```
linuxctl env current [flags]
```

### Options

```
  -h, --help   help for current
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

* [linuxctl env](linuxctl_env.md)	 - Manage the local env registry (~/.linuxctl/envs.yaml)

