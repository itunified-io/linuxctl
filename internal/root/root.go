// Package root builds the linuxctl Cobra command tree.
package root

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// envVarNameStack is the canonical env var name for the default stack (#17).
const envVarNameStack = "LINUXCTL_STACK"

// envVarNameEnv is the deprecated alias for LINUXCTL_STACK. Kept for one
// release; remove in the next major release.
const envVarNameEnv = "LINUXCTL_ENV"

// applyEnvVarDefaults reads LINUXCTL_STACK / LINUXCTL_ENV and fills gf.stack
// / gf.env when the corresponding CLI flags were not explicitly set. Command-
// line flags still win over env vars. If both env vars are set, LINUXCTL_STACK
// wins and a warning is emitted.
func applyEnvVarDefaults() {
	stackEnv := os.Getenv(envVarNameStack)
	envEnv := os.Getenv(envVarNameEnv)
	if gf.stack == "" && stackEnv != "" {
		gf.stack = stackEnv
	}
	if gf.env == "" && envEnv != "" {
		if gf.stack != "" && stackEnv == "" {
			// --stack already set from flags; ignore legacy env var silently.
		}
		gf.env = envEnv
		if stackEnv == "" {
			fmt.Fprintln(os.Stderr, "warning: LINUXCTL_ENV is deprecated; use LINUXCTL_STACK instead")
		}
	}
	if stackEnv != "" && envEnv != "" && stackEnv != envEnv {
		fmt.Fprintln(os.Stderr, "warning: both LINUXCTL_STACK and LINUXCTL_ENV set; LINUXCTL_STACK wins (LINUXCTL_ENV is deprecated)")
	}
}

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
	sshUser string // SSH login user when --host is non-local. Default: $USER or root.
	sshKey  string // SSH private key path. Default: ~/.ssh/id_ed25519.
	sshPort int    // SSH port. Default 22.
	format  string
	yes     bool
	dryRun  bool
	license string
	verbose bool
	// reformatFilesystems opts in to destructive mkfs on LVs that already
	// hold a filesystem of a different type than the manifest requests
	// (linuxctl#52). Default false: disk.Plan returns an explicit error so
	// `linuxctl stack apply` can no longer silently destroy data.
	reformatFilesystems bool
}

var gf globalFlags

// NewRootCmd builds the root Cobra command with all subtrees registered.
func NewRootCmd(info BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "linuxctl",
		Short:        "Declarative, idempotent, auditable Linux host configuration",
		Long:         "linuxctl converges a Linux host to the desired state defined in linux.yaml. Plan / Apply / Verify / Rollback across 13 subsystems over SSH.",
		SilenceUsage: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			// Env var defaults (LINUXCTL_STACK; LINUXCTL_ENV deprecated).
			applyEnvVarDefaults()
			// Auto-migrate ~/.linuxctl/envs.yaml → ~/.linuxctl/stacks.yaml (#17).
			// Failures are non-fatal; MigrateRegistry prints its own warnings.
			_ = MigrateRegistry()
			return nil
		},
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
	pf.StringVar(&gf.sshUser, "ssh-user", "", "SSH login user (default: $USER or root)")
	pf.StringVar(&gf.sshKey, "ssh-key", "", "SSH private key path (default: ~/.ssh/id_ed25519)")
	pf.IntVar(&gf.sshPort, "ssh-port", 22, "SSH port")
	pf.StringVar(&gf.format, "format", "table", "table|json|yaml|plain")
	pf.BoolVar(&gf.yes, "yes", false, "Non-interactive; skip confirm prompts")
	pf.BoolVar(&gf.dryRun, "dry-run", false, "Alias for plan; never mutate")
	pf.StringVar(&gf.license, "license", "", "Override ~/.linuxctl/license.jwt")
	pf.BoolVarP(&gf.verbose, "verbose", "v", false, "Verbose logging")
	pf.BoolVar(&gf.reformatFilesystems, "reformat-filesystems", false,
		"Allow disk apply to mkfs over an existing, mismatched filesystem (DESTRUCTIVE)")

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

	// Conventions library (plan 033).
	cmd.AddCommand(newPresetCmd())

	// Meta.
	cmd.AddCommand(newLicenseCmd())
	cmd.AddCommand(newVersionCmd(info))

	return cmd
}

// Execute runs the linuxctl CLI.
func Execute(info BuildInfo) error {
	return NewRootCmd(info).Execute()
}
