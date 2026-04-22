package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSysctlCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sysctl",
		Short: "Manage sysctl state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview sysctl changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("sysctl plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply sysctl changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("sysctl apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed sysctl state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("sysctl verify: not implemented") },
	})
	return cmd
}
