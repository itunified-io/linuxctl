package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newStackCmd builds the `linuxctl stack` subcommand tree that manages the
// local stack registry (~/.linuxctl/stacks.yaml). A deprecated `env` alias is
// registered separately in root.go for one-release backward compatibility.
func newStackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stack",
		Short: "Manage the local stack registry (~/.linuxctl/stacks.yaml)",
	}
	stub := func(use, short string) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("stack %s: not implemented", use) },
		}
	}
	cmd.AddCommand(stub("new <name>", "Create a new local stack pointer"))
	cmd.AddCommand(stub("list", "List registered stacks"))
	cmd.AddCommand(stub("use <name>", "Set the default stack"))
	cmd.AddCommand(stub("current", "Print the default stack"))
	cmd.AddCommand(stub("add <name>", "Register an existing stack directory"))
	cmd.AddCommand(stub("remove <name>", "Remove a registered stack"))
	cmd.AddCommand(stub("show <name>", "Dump the resolved env.yaml tree"))
	return cmd
}

// envDeprecationMsg is emitted whenever the `env` alias is invoked.
const envDeprecationMsg = "warning: `linuxctl env` is deprecated; use `linuxctl stack` instead (will be removed in the next release)"

// newEnvAliasCmd returns a hidden deprecated alias for `stack`. Kept for one
// release (#17); remove in the next major release. A PersistentPreRun on the
// alias tree prints a deprecation warning to stderr on every invocation.
func newEnvAliasCmd() *cobra.Command {
	cmd := newStackCmd()
	cmd.Use = "env"
	cmd.Short = "Deprecated: use `linuxctl stack` (kept for one release)"
	cmd.Hidden = true
	cmd.Deprecated = "use `linuxctl stack` instead; `env` will be removed in the next release"
	cmd.PersistentPreRun = func(c *cobra.Command, _ []string) {
		fmt.Fprintln(c.ErrOrStderr(), envDeprecationMsg)
	}
	return cmd
}
