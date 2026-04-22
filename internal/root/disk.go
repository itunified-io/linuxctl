package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDiskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disk",
		Short: "Manage disk state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview disk changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("disk plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply disk changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("disk apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed disk state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("disk verify: not implemented") },
	})
	return cmd
}
