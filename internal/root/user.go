package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage user state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview user changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("user plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply user changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("user apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed user state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("user verify: not implemented") },
	})
	return cmd
}
