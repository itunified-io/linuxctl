package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSSHCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh",
		Short: "Manage ssh state (authorized_keys, sshd_config, cluster trust)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview ssh changes",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("ssh plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply ssh changes",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("ssh apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify ssh state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("ssh verify: not implemented") },
	})
	return cmd
}
