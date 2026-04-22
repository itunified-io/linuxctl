// Package root builds the linuxctl Cobra command tree.
package root

import (
	"github.com/spf13/cobra"
)

// BuildInfo is injected from main.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// globalFlags holds the values bound to persistent root flags. `stack` is the
// canonical field; `env` is a deprecated alias retained for one release (#17).
// If both are set, `stack` wins and a warning is emitted in stackFromFlags().
type globalFlags struct {
	context string
	stack   string
	env     string // deprecated alias for --stack; remove next release
	host    string
	format  string
	yes     bool
	dryRun  bool
	license string
	verbose bool
}

var gf globalFlags

// NewRootCmd builds the root Cobra command with all subtrees registered.
func NewRootCmd(info BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "linuxctl",
		Short:        "Declarative, idempotent, auditable Linux host configuration",
		Long:         "linuxctl converges a Linux host to the desired state defined in linux.yaml. Plan / Apply / Verify / Rollback across 13 subsystems over SSH.",
		SilenceUsage: true,
	}

	pf := cmd.PersistentFlags()
	pf.StringVar(&gf.context, "context", "", "Named context from ~/.linuxctl/config.yaml")
	pf.StringVar(&gf.stack, "stack", "", "Named stack from ~/.linuxctl/stacks.yaml")
	// Deprecated: `--env` is an alias for `--stack` (#17). Kept for one release.
	pf.StringVar(&gf.env, "env", "", "Deprecated alias for --stack (will be removed in the next release)")
	if f := pf.Lookup("env"); f != nil {
		f.Deprecated = "use --stack instead"
		f.Hidden = true
	}
	pf.StringVar(&gf.host, "host", "", "Restrict to a single host from the stack")
	pf.StringVar(&gf.format, "format", "table", "table|json|yaml|plain")
	pf.BoolVar(&gf.yes, "yes", false, "Non-interactive; skip confirm prompts")
	pf.BoolVar(&gf.dryRun, "dry-run", false, "Alias for plan; never mutate")
	pf.StringVar(&gf.license, "license", "", "Override ~/.linuxctl/license.jwt")
	pf.BoolVarP(&gf.verbose, "verbose", "v", false, "Verbose logging")

	// Groups.
	cmd.AddCommand(newConfigCmd())
	cmd.AddCommand(newStackCmd())
	cmd.AddCommand(newEnvAliasCmd())

	// 13 subsystem manager commands, each with plan/apply/verify.
	cmd.AddCommand(newDiskCmd())
	cmd.AddCommand(newUserCmd())
	cmd.AddCommand(newPackageCmd())
	cmd.AddCommand(newServiceCmd())
	cmd.AddCommand(newMountCmd())
	cmd.AddCommand(newSysctlCmd())
	cmd.AddCommand(newLimitsCmd())
	cmd.AddCommand(newFirewallCmd())
	cmd.AddCommand(newHostsCmd())
	cmd.AddCommand(newNetworkCmd())
	cmd.AddCommand(newSSHCmd())
	cmd.AddCommand(newSELinuxCmd())
	cmd.AddCommand(newDirCmd())

	// Orchestrator + observation.
	cmd.AddCommand(newApplyCmd())
	cmd.AddCommand(newDiffCmd())

	// Meta.
	cmd.AddCommand(newLicenseCmd())
	cmd.AddCommand(newVersionCmd(info))

	return cmd
}

// Execute runs the linuxctl CLI.
func Execute(info BuildInfo) error {
	return NewRootCmd(info).Execute()
}
