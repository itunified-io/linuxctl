package root

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/itunified-io/linuxctl/pkg/config"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Validate and render linux.yaml configuration",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "validate <path>",
		Short: "Validate linux.yaml + cross-references",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			l, err := config.LoadLinux(args[0])
			if err != nil {
				return err
			}
			if err := config.Validate(l); err != nil {
				return err
			}
			fmt.Println("OK")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "render <path>",
		Short: "Render with resolved secrets (redacted)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("config render: not implemented")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "use-context <name>",
		Short: "Set default context",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("config use-context: not implemented")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print active config.yaml (redacted)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("config show: not implemented")
		},
	})
	return cmd
}
