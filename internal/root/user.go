package root

import "github.com/spf13/cobra"

func newUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user [env.yaml|linux.yaml]",
		Short: "Manage user state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan [env.yaml|linux.yaml]",
		Short: "Preview user changes",
		RunE:  runManager("user", actionPlan),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply [env.yaml|linux.yaml]",
		Short: "Apply user changes",
		RunE:  runManager("user", actionApply),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify [env.yaml|linux.yaml]",
		Short: "Verify user state",
		RunE:  runManager("user", actionVerify),
	})
	return cmd
}
