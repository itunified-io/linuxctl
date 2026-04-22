package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newNetworkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network",
		Short: "Manage network state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview network changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("network plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply network changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("network apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed network state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("network verify: not implemented") },
	})
	return cmd
}
