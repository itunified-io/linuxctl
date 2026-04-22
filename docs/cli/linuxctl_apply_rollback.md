## linuxctl apply rollback

Rollback the in-memory applied set (non-persistent Phase-3 rollback)

```
linuxctl apply rollback [env.yaml] [flags]
```

### Options

```
  -h, --help            help for rollback
      --run-id string   UUID of the apply run to rollback (ignored in Phase 3)
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

* [linuxctl apply](linuxctl_apply.md)	 - Orchestrate plan / apply / verify / rollback across all managers

