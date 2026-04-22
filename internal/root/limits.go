package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLimitsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "limits",
		Short: "Manage limits state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview limits changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("limits plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply limits changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("limits apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed limits state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("limits verify: not implemented") },
	})
	return cmd
}
