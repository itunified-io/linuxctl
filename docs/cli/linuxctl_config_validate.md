## linuxctl config validate

Validate linux.yaml + cross-references

```
linuxctl config validate <path> [flags]
```

### Options

```
  -h, --help   help for validate
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

* [linuxctl config](linuxctl_config.md)	 - Validate and render linux.yaml configuration

