package root

import (
	"github.com/spf13/cobra"
)

// newFileCmd wires the file subsystem CLI to FileManager. Reads `files:`
// from linux.yaml (typically populated by a bundle preset); writes literal
// file payloads with idempotent body comparison. linuxctl#57.
func newFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file [env.yaml|linux.yaml]",
		Short: "Manage literal file payloads (plan / apply / verify / rollback)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan [env.yaml|linux.yaml]",
		Short: "Preview which files would be written",
		RunE:  runManager("file", actionPlan),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply [env.yaml|linux.yaml]",
		Short: "Write any drifted files (idempotent: same body + mode → no-op)",
		RunE:  runManager("file", actionApply),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify [env.yaml|linux.yaml]",
		Short: "Verify on-disk files match desired body + mode",
		RunE:  runManager("file", actionVerify),
	})
	return cmd
}
