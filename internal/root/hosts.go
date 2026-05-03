package root

import (
	"github.com/spf13/cobra"
)

// newHostsCmd wires the hosts subsystem CLI to the registered HostsManager via
// the shared runManager helper. Reads `hosts_entries:` from linux.yaml; edits
// /etc/hosts between `# BEGIN linuxctl` / `# END linuxctl` markers idempotently
// (linuxctl#51 — Phase C blocker for /lab-up).
func newHostsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hosts [env.yaml|linux.yaml]",
		Short: "Manage /etc/hosts state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan [env.yaml|linux.yaml]",
		Short: "Preview /etc/hosts changes against the desired state",
		RunE:  runManager("hosts", actionPlan),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply [env.yaml|linux.yaml]",
		Short: "Apply /etc/hosts changes (rewrites managed block only)",
		RunE:  runManager("hosts", actionApply),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify [env.yaml|linux.yaml]",
		Short: "Verify observed /etc/hosts managed block matches desired state",
		RunE:  runManager("hosts", actionVerify),
	})
	return cmd
}
