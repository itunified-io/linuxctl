package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newMountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mount",
		Short: "Manage mount state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview mount changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("mount plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply mount changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("mount apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed mount state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("mount verify: not implemented") },
	})
	return cmd
}
