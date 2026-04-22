package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print linuxctl version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "linuxctl %s (commit %s, built %s)\n", info.Version, info.Commit, info.Date)
			return nil
		},
	}
}
