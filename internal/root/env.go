package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage the local env registry (~/.linuxctl/envs.yaml)",
	}
	stub := func(use, short string) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("env %s: not implemented", use) },
		}
	}
	cmd.AddCommand(stub("new <name>", "Create a new local env pointer"))
	cmd.AddCommand(stub("list", "List registered envs"))
	cmd.AddCommand(stub("use <name>", "Set the default env"))
	cmd.AddCommand(stub("current", "Print the default env"))
	cmd.AddCommand(stub("add <name>", "Register an existing env directory"))
	cmd.AddCommand(stub("remove <name>", "Remove a registered env"))
	cmd.AddCommand(stub("show <name>", "Dump the resolved env.yaml tree"))
	return cmd
}
