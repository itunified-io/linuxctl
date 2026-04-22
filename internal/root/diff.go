package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "Read-only drift report across all managers",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("diff: not implemented") },
	}
}
