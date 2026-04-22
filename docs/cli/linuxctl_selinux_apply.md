## linuxctl selinux apply

Apply selinux changes to reach the desired state

```
linuxctl selinux apply [flags]
```

### Options

```
  -h, --help   help for apply
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

* [linuxctl selinux](linuxctl_selinux.md)	 - Manage selinux state (plan / apply / verify)

