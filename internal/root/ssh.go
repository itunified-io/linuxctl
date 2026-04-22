package root

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/itunified-io/linuxctl/pkg/managers"
	"github.com/itunified-io/linuxctl/pkg/session"
)

func newSSHCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh",
		Short: "Manage ssh state (authorized_keys, sshd_config, cluster trust)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview ssh changes",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("ssh plan: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply ssh changes",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("ssh apply: not implemented") },
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify ssh state",
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("ssh verify: not implemented") },
	})
	cmd.AddCommand(newSSHSetupClusterCmd())
	return cmd
}

// newSSHSetupClusterCmd wires `linuxctl ssh setup-cluster <env.yaml> --user <u>`.
// Generates per-user SSH keypairs on every host in the env and cross-
// authorizes them so service accounts (grid/oracle) have passwordless trust.
func newSSHSetupClusterCmd() *cobra.Command {
	var (
		users    []string
		hosts    []string
		sshUser  string
		sshKey   string
		sshPort  int
	)
	cmd := &cobra.Command{
		Use:   "setup-cluster [env.yaml]",
		Short: "Generate per-user SSH keypairs and cross-authorize across cluster nodes",
		Long: "Creates ~<user>/.ssh/id_ed25519 on every host (if missing), " +
			"collects all public keys, appends them to each host's " +
			"authorized_keys, and seeds known_hosts via ssh-keyscan. " +
			"Required for Oracle RAC grid/oracle user trust.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(users) == 0 {
				return fmt.Errorf("at least one --user is required (e.g. --user grid --user oracle)")
			}
			if len(hosts) == 0 {
				// env.yaml wiring is owned by pkg/config/resolver; until the loader
				// surfaces cluster node list, require --host explicitly.
				return fmt.Errorf("at least one --host is required until env-wiring lands; got args=%v", args)
			}
			if sshUser == "" {
				return fmt.Errorf("--ssh-user is required for cluster dial")
			}
			sessions := map[string]managers.SessionRunner{}
			for _, h := range hosts {
				sess, err := session.NewSSHDial(session.Opts{
					Host:    h,
					User:    sshUser,
					Port:    sshPort,
					KeyFile: sshKey,
				})
				if err != nil {
					return fmt.Errorf("dial %s: %w", h, err)
				}
				defer sess.Close()
				sessions[h] = sess
			}
			return managers.SetupClusterSSH(context.Background(), sessions, users)
		},
	}
	cmd.Flags().StringArrayVar(&users, "user", nil, "Service account to cross-authorize (repeatable)")
	cmd.Flags().StringArrayVar(&hosts, "host", nil, "Cluster node hostname/IP (repeatable)")
	cmd.Flags().StringVar(&sshUser, "ssh-user", "", "SSH login user for cluster dial")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to SSH private key")
	cmd.Flags().IntVar(&sshPort, "ssh-port", 22, "SSH port")
	return cmd
}
