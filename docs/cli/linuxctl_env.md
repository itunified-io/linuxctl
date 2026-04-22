## linuxctl env

Manage the local env registry (~/.linuxctl/envs.yaml)

### Options

```
  -h, --help   help for env
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
* [linuxctl env add](linuxctl_env_add.md)	 - Register an existing env directory
* [linuxctl env current](linuxctl_env_current.md)	 - Print the default env
* [linuxctl env list](linuxctl_env_list.md)	 - List registered envs
* [linuxctl env new](linuxctl_env_new.md)	 - Create a new local env pointer
* [linuxctl env remove](linuxctl_env_remove.md)	 - Remove a registered env
* [linuxctl env show](linuxctl_env_show.md)	 - Dump the resolved env.yaml tree
* [linuxctl env use](linuxctl_env_use.md)	 - Set the default env

