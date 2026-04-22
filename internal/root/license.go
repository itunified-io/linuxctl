package root

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/itunified-io/linuxctl/pkg/license"
)

func newLicenseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "license",
		Short: "Manage the linuxctl license",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show active tier, expiry, and seat usage",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Printf("tier: %s\n", license.TierCommunity)
			fmt.Printf("tools: %d in catalog\n", len(license.ToolCatalog))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "activate <path>",
		Short: "Install a new JWT",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("license activate: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print claims (redacted signature)",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("license show: not implemented") },
	})
	return cmd
}
