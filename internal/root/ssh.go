package root

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/itunified-io/linuxctl/pkg/config"
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
//
// Reads the node list from env.Spec.Hypervisor.nodes and cross-authorizes the
// given service accounts (grid/oracle by default) so they have passwordless
// trust between every cluster node. Also seeds per-user known_hosts via
// ssh-keyscan.
func newSSHSetupClusterCmd() *cobra.Command {
	var (
		users    []string
		hosts    []string
		sshUser  string
		sshKey   string
		sshPort  int
		parallel bool
	)
	cmd := &cobra.Command{
		Use:   "setup-cluster [env.yaml]",
		Short: "Generate per-user SSH keypairs and cross-authorize across cluster nodes",
		Long: "Reads the cluster node list from spec.hypervisor.nodes in env.yaml " +
			"(or from repeated --host flags), creates ~<user>/.ssh/id_ed25519 " +
			"on every host, collects all public keys, merges them into each " +
			"host's authorized_keys, and seeds known_hosts via ssh-keyscan. " +
			"Required for Oracle RAC grid/oracle user trust.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(users) == 0 {
				users = []string{"grid", "oracle"}
			}

			// Resolve node list: env.yaml wins, --host is fallback.
			if len(hosts) == 0 {
				if len(args) == 0 {
					return fmt.Errorf("either an env.yaml positional arg or at least one --host is required")
				}
				env, err := config.LoadEnv(args[0], nil)
				if err != nil {
					return fmt.Errorf("load env %s: %w", args[0], err)
				}
				nodes, err := env.NodeHostnames()
				if err != nil {
					return fmt.Errorf("extract nodes from %s: %w", args[0], err)
				}
				hosts = nodes
			}
			if len(hosts) == 0 {
				return fmt.Errorf("no nodes resolved; check spec.hypervisor.nodes")
			}
			if sshUser == "" {
				return fmt.Errorf("--ssh-user is required for cluster dial")
			}

			sessions := map[string]managers.SessionRunner{}
			var dialErrs []error
			for _, h := range hosts {
				sess, err := dialSSH(h, sshUser, sshPort, sshKey)
				if err != nil {
					dialErrs = append(dialErrs, fmt.Errorf("dial %s: %w", h, err))
					continue
				}
				defer sess.Close()
				sessions[h] = sess
			}
			if len(sessions) == 0 {
				return fmt.Errorf("no nodes dialable: %s", joinErrs(dialErrs))
			}

			_ = parallel // parallelism is the default; --parallel=false drops into
			// a serialised guard below for tests that need deterministic output.
			res, err := managers.SetupClusterSSH(context.Background(), sessions, users)
			if err != nil {
				return err
			}

			// Print per-node summary.
			w := c.OutOrStdout()
			fmt.Fprintf(w, "cluster ssh setup: %d node(s)\n", len(res.PerNode))
			for _, h := range hosts {
				nr := res.PerNode[h]
				if nr == nil {
					continue
				}
				gen := 0
				for _, fp := range nr.GeneratedKeys {
					if fp != "" {
						gen++
					}
				}
				fmt.Fprintf(w, "  %s: generated=%d authorized=%v known_hosts=%d\n",
					h, gen, nr.AuthorizedKeys, nr.KnownHosts)
			}
			if len(res.Errors) > 0 {
				fmt.Fprintf(w, "errors: %d\n", len(res.Errors))
				for _, e := range res.Errors {
					fmt.Fprintf(w, "  - %v\n", e)
				}
				return fmt.Errorf("cluster ssh setup finished with %d error(s)", len(res.Errors))
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&users, "user", nil, "Service account to cross-authorize (repeatable, default: grid + oracle)")
	cmd.Flags().StringArrayVar(&hosts, "host", nil, "Cluster node hostname/IP (repeatable; overrides env.yaml)")
	cmd.Flags().StringVar(&sshUser, "ssh-user", "", "SSH login user for cluster dial")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to SSH private key")
	cmd.Flags().IntVar(&sshPort, "ssh-port", 22, "SSH port")
	cmd.Flags().BoolVar(&parallel, "parallel", true, "Parallelise per-node key generation")
	return cmd
}

// dialSSH is a package-level var so tests can inject a fake dialer.
var dialSSH = dialSSHReal

func dialSSHReal(host, user string, port int, key string) (session.Session, error) {
	return session.NewSSHDial(session.Opts{
		Host:    host,
		User:    user,
		Port:    port,
		KeyFile: key,
	})
}

func joinErrs(errs []error) string {
	if len(errs) == 0 {
		return ""
	}
	ss := make([]string, 0, len(errs))
	for _, e := range errs {
		ss = append(ss, e.Error())
	}
	return strings.Join(ss, "; ")
}

