package root

import (
	"github.com/spf13/cobra"
)

// newRepoCmd wires the repo subsystem CLI to RepoManager. Reads
// `repos_enable:` from linux.yaml (typically populated by a bundle preset);
// idempotently enables the listed dnf repository IDs. linuxctl#57.
func newRepoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo [env.yaml|linux.yaml]",
		Short: "Manage dnf repository enablement (plan / apply / verify / rollback)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan [env.yaml|linux.yaml]",
		Short: "Preview which repos would be enabled",
		RunE:  runManager("repo", actionPlan),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply [env.yaml|linux.yaml]",
		Short: "Enable any repos in repos_enable: that are currently disabled",
		RunE:  runManager("repo", actionApply),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify [env.yaml|linux.yaml]",
		Short: "Verify all repos in repos_enable: are currently enabled",
		RunE:  runManager("repo", actionVerify),
	})
	return cmd
}
