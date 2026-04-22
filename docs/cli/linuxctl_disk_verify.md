## linuxctl disk verify

Verify disk state

```
linuxctl disk verify [env.yaml|linux.yaml] [flags]
```

### Options

```
  -h, --help   help for verify
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

* [linuxctl disk](linuxctl_disk.md)	 - Manage disk state (plan / apply / verify)

