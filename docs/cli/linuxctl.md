## linuxctl

Declarative, idempotent, auditable Linux host configuration

### Synopsis

linuxctl converges a Linux host to the desired state defined in linux.yaml. Plan / Apply / Verify / Rollback across 13 subsystems over SSH.

### Options

```
      --context string   Named context from ~/.linuxctl/config.yaml
      --dry-run          Alias for plan; never mutate
      --format string    table|json|yaml|plain (default "table")
  -h, --help             help for linuxctl
      --host string      Restrict to a single host from the stack
      --license string   Override ~/.linuxctl/license.jwt
      --stack string     Named stack from ~/.linuxctl/stacks.yaml
  -v, --verbose          Verbose logging
      --yes              Non-interactive; skip confirm prompts
```

### SEE ALSO

* [linuxctl apply](linuxctl_apply.md)	 - Orchestrate plan / apply / verify / rollback across all managers
* [linuxctl config](linuxctl_config.md)	 - Validate and render linux.yaml configuration
* [linuxctl diff](linuxctl_diff.md)	 - Read-only drift report across all managers
* [linuxctl dir](linuxctl_dir.md)	 - Manage dir state (plan / apply / verify)
* [linuxctl disk](linuxctl_disk.md)	 - Manage disk state (plan / apply / verify)
* [linuxctl firewall](linuxctl_firewall.md)	 - Manage firewall state (plan / apply / verify)
* [linuxctl hosts](linuxctl_hosts.md)	 - Manage hosts state (plan / apply / verify)
* [linuxctl license](linuxctl_license.md)	 - Manage the linuxctl license
* [linuxctl limits](linuxctl_limits.md)	 - Manage limits state (plan / apply / verify)
* [linuxctl mount](linuxctl_mount.md)	 - Manage mount state (plan / apply / verify)
* [linuxctl network](linuxctl_network.md)	 - Manage network state (plan / apply / verify)
* [linuxctl package](linuxctl_package.md)	 - Manage package state (plan / apply / verify)
* [linuxctl selinux](linuxctl_selinux.md)	 - Manage selinux state (plan / apply / verify)
* [linuxctl service](linuxctl_service.md)	 - Manage service state (plan / apply / verify)
* [linuxctl ssh](linuxctl_ssh.md)	 - Manage ssh state (authorized_keys, sshd_config, cluster trust)
* [linuxctl stack](linuxctl_stack.md)	 - Manage the local stack registry (~/.linuxctl/stacks.yaml)
* [linuxctl sysctl](linuxctl_sysctl.md)	 - Manage sysctl state (plan / apply / verify)
* [linuxctl user](linuxctl_user.md)	 - Manage user state (plan / apply / verify)
* [linuxctl version](linuxctl_version.md)	 - Print linuxctl version information

