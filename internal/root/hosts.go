package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newHostsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hosts",
		Short: "Manage hosts state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview hosts changes against the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("hosts plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply hosts changes to reach the desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("hosts apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed hosts state matches desired state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("hosts verify: not implemented") },
	})
	return cmd
}
