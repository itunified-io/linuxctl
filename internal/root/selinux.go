package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSELinuxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "selinux",
		Short: "Manage selinux state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview selinux changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("selinux plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply selinux changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("selinux apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed selinux state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("selinux verify: not implemented") },
	})
	return cmd
}
