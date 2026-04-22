package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage service state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview service changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("service plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply service changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("service apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed service state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("service verify: not implemented") },
	})
	return cmd
}
