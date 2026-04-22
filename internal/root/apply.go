package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Orchestrate plan / apply / verify / rollback across all managers",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Full DAG plan across all managers",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("apply plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply the full DAG",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("apply apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify the full DAG",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("apply verify: not implemented") },
	})
	rb := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback a prior apply by run-id",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("apply rollback: not implemented") },
	}
	rb.Flags().String("run-id", "", "UUID of the apply run to rollback")
	cmd.AddCommand(rb)
	return cmd
}
