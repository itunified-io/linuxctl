## linuxctl license

Manage the linuxctl license

### Options

```
  -h, --help   help for license
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

* [linuxctl](linuxctl.md)	 - Declarative, idempotent, auditable Linux host configuration
* [linuxctl license activate](linuxctl_license_activate.md)	 - Install a new JWT
* [linuxctl license show](linuxctl_license_show.md)	 - Print claims (redacted signature)
* [linuxctl license status](linuxctl_license_status.md)	 - Show active tier, expiry, and seat usage

