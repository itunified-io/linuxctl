package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDirCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dir",
		Short: "Manage dir state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview dir changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("dir plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply dir changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("dir apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed dir state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("dir verify: not implemented") },
	})
	return cmd
}
