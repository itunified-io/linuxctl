package root

import "github.com/spf13/cobra"

func newMountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mount [env.yaml|linux.yaml]",
		Short: "Manage mount state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan [env.yaml|linux.yaml]",
		Short: "Preview mount changes",
		RunE:  runManager("mount", actionPlan),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply [env.yaml|linux.yaml]",
		Short: "Apply mount changes",
		RunE:  runManager("mount", actionApply),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify [env.yaml|linux.yaml]",
		Short: "Verify mount state",
		RunE:  runManager("mount", actionVerify),
	})
	return cmd
}
