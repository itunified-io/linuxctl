package root

import "github.com/spf13/cobra"

func newPackageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package [env.yaml|linux.yaml]",
		Short: "Manage package state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan [env.yaml|linux.yaml]",
		Short: "Preview package changes",
		RunE:  runManager("package", actionPlan),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply [env.yaml|linux.yaml]",
		Short: "Apply package changes",
		RunE:  runManager("package", actionApply),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify [env.yaml|linux.yaml]",
		Short: "Verify package state",
		RunE:  runManager("package", actionVerify),
	})
	return cmd
}
