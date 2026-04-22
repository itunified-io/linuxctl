package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newFirewallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "firewall",
		Short: "Manage firewall state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview firewall changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("firewall plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply firewall changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("firewall apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed firewall state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("firewall verify: not implemented") },
	})
	return cmd
}
