## linuxctl config

Validate and render linux.yaml configuration

### Options

```
  -h, --help   help for config
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
* [linuxctl config render](linuxctl_config_render.md)	 - Render with resolved secrets (redacted)
* [linuxctl config show](linuxctl_config_show.md)	 - Print active config.yaml (redacted)
* [linuxctl config use-context](linuxctl_config_use-context.md)	 - Set default context
* [linuxctl config validate](linuxctl_config_validate.md)	 - Validate linux.yaml + cross-references

