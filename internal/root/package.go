package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPackageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package",
		Short: "Manage package state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview package changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("package plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply package changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("package apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed package state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("package verify: not implemented") },
	})
	return cmd
}
