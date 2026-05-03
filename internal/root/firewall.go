package root

import (
	"github.com/spf13/cobra"
)

// newFirewallCmd wires the firewall subsystem CLI to the registered
// FirewallManager via the shared runManager helper. Reads `firewall:` from
// linux.yaml; reconciles ports + sources per zone (firewalld / ufw / nftables).
// linuxctl#51 — Phase C blocker for /lab-up.
func newFirewallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "firewall [env.yaml|linux.yaml]",
		Short: "Manage host firewall state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan [env.yaml|linux.yaml]",
		Short: "Preview firewall changes against the desired state",
		RunE:  runManager("firewall", actionPlan),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply [env.yaml|linux.yaml]",
		Short: "Apply firewall changes (idempotent: only adds missing ports/sources)",
		RunE:  runManager("firewall", actionApply),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify [env.yaml|linux.yaml]",
		Short: "Verify observed firewall ports/sources match desired state",
		RunE:  runManager("firewall", actionVerify),
	})
	return cmd
}
