package managers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// sshdDropInPath is the sshd_config.d drop-in file linuxctl manages exclusively.
const sshdDropInPath = "/etc/ssh/sshd_config.d/99-linuxctl.conf"

func init() { Register(NewSSHAuthManager()) }

// SSHAuthManager handles the "ssh" subsystem:
//   - authorized_keys for users NOT owned by the user manager (e.g. pre-existing
//     system accounts that still need key drift control)
//   - sshd_config directives via a managed drop-in file
type SSHAuthManager struct {
	sess SessionRunner
}

// NewSSHAuthManager returns an ssh manager. Passing no session is valid for
// tests that only exercise Plan against a pre-populated mock.
func NewSSHAuthManager() *SSHAuthManager { return &SSHAuthManager{} }

// WithSession attaches a SessionRunner for Apply / Verify.
func (m *SSHAuthManager) WithSession(s SessionRunner) *SSHAuthManager {
	m.sess = s
	return m
}

// Name implements Manager.
func (*SSHAuthManager) Name() string { return "ssh" }

// DependsOn implements Manager. Keys are written into home dirs which are
// owned by the user manager; sshd_config is inert until the service manager
// reloads sshd.
func (*SSHAuthManager) DependsOn() []string { return []string{"user"} }

// ---- Plan -----------------------------------------------------------------

// Plan implements Manager.
func (m *SSHAuthManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	cfg, err := castSSHConfig(desired)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	var changes []Change

	users := make([]string, 0, len(cfg.AuthorizedKeys))
	for u := range cfg.AuthorizedKeys {
		users = append(users, u)
	}
	sort.Strings(users)
	for _, u := range users {
		desiredKeys := cfg.AuthorizedKeys[u]
		cur, err := m.readAuthorizedKeys(ctx, u)
		if err != nil {
			return nil, fmt.Errorf("ssh.Plan: read keys for %s: %w", u, err)
		}
		if !sameStringSet(desiredKeys, cur) {
			action := "update"
			if len(cur) == 0 {
				action = "create"
			}
			changes = append(changes, Change{
				ID:      "ssh:keys:" + u,
				Manager: "ssh",
				Target:  "authorized_keys/" + u,
				Action:  action,
				Before:  cur,
				After:   desiredKeys,
				Hazard:  HazardWarn,
			})
		}
	}

	if len(cfg.SSHDConfig) > 0 {
		cur, err := m.readSSHDDropIn(ctx)
		if err != nil {
			return nil, fmt.Errorf("ssh.Plan: read sshd drop-in: %w", err)
		}
		keys := make([]string, 0, len(cfg.SSHDConfig))
		for k := range cfg.SSHDConfig {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var dirty bool
		for _, k := range keys {
			if cur[k] != cfg.SSHDConfig[k] {
				dirty = true
				break
			}
		}
		if dirty {
			action := "update"
			if len(cur) == 0 {
				action = "create"
			}
			changes = append(changes, Change{
				ID:      "ssh:sshd_config",
				Manager: "ssh",
				Target:  "sshd_config/drop-in",
				Action:  action,
				Before:  cur,
				After:   cfg.SSHDConfig,
				Hazard:  HazardDestructive,
			})
		}
	}

	return changes, nil
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *SSHAuthManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, fmt.Errorf("ssh.Apply: no session attached")
	}
	for _, ch := range changes {
		var err error
		switch {
		case strings.HasPrefix(ch.Target, "authorized_keys/"):
			err = m.applyAuthorizedKeys(ctx, ch)
		case ch.Target == "sshd_config/drop-in":
			err = m.applySSHDConfig(ctx, ch)
		default:
			err = fmt.Errorf("ssh.Apply: unknown target %q", ch.Target)
		}
		if err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: err})
			continue
		}
		res.Applied = append(res.Applied, ch)
	}
	return res, nil
}

func (m *SSHAuthManager) applyAuthorizedKeys(ctx context.Context, ch Change) error {
	user := strings.TrimPrefix(ch.Target, "authorized_keys/")
	keys, ok := ch.After.([]string)
	if !ok {
		return fmt.Errorf("ssh.applyAuthorizedKeys: After is not []string (%T)", ch.After)
	}
	content := strings.Join(keys, "\n") + "\n"
	home := "/home/" + user
	if user == "root" {
		home = "/root"
	}
	script := fmt.Sprintf(
		"install -d -m 0700 -o %[1]s -g %[1]s %[2]s/.ssh && "+
			"umask 077 && printf %%s %[3]s | tee %[2]s/.ssh/authorized_keys >/dev/null && "+
			"chown %[1]s:%[1]s %[2]s/.ssh/authorized_keys && chmod 0600 %[2]s/.ssh/authorized_keys",
		shellQuote(user), shellQuote(home), shellQuote(content),
	)
	return m.run(ctx, script)
}

func (m *SSHAuthManager) applySSHDConfig(ctx context.Context, ch Change) error {
	desired, ok := ch.After.(map[string]string)
	if !ok {
		return fmt.Errorf("ssh.applySSHDConfig: After is not map[string]string (%T)", ch.After)
	}
	content := renderSSHDDropIn(desired)
	write := fmt.Sprintf(
		"install -d -m 0755 /etc/ssh/sshd_config.d && "+
			"printf %%s %s | tee %s >/dev/null && chmod 0644 %s",
		shellQuote(content), shellQuote(sshdDropInPath), shellQuote(sshdDropInPath),
	)
	if err := m.run(ctx, write); err != nil {
		return err
	}
	if err := m.run(ctx, "sshd -t"); err != nil {
		_ = m.run(ctx, "rm -f "+shellQuote(sshdDropInPath))
		return fmt.Errorf("ssh.applySSHDConfig: sshd -t failed: %w", err)
	}
	return m.run(ctx, "systemctl reload sshd 2>/dev/null || systemctl reload ssh 2>/dev/null || true")
}

// renderSSHDDropIn emits deterministic `Key Value` lines inside a managed
// marker comment. Sorted keys make re-runs byte-identical.
func renderSSHDDropIn(kv map[string]string) string {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# BEGIN linuxctl\n")
	b.WriteString("# Managed by linuxctl - do not edit by hand\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %s\n", k, kv[k])
	}
	b.WriteString("# END linuxctl\n")
	return b.String()
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *SSHAuthManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback implements Manager.
func (m *SSHAuthManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return fmt.Errorf("ssh.Rollback: no session attached")
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		switch {
		case strings.HasPrefix(ch.Target, "authorized_keys/"):
			user := strings.TrimPrefix(ch.Target, "authorized_keys/")
			before, _ := ch.Before.([]string)
			if before == nil {
				continue
			}
			if err := m.applyAuthorizedKeysRaw(ctx, user, before); err != nil {
				return err
			}
		case ch.Target == "sshd_config/drop-in":
			if err := m.run(ctx, "rm -f "+shellQuote(sshdDropInPath)); err != nil {
				return err
			}
			_ = m.run(ctx, "sshd -t && (systemctl reload sshd 2>/dev/null || systemctl reload ssh 2>/dev/null || true)")
		}
	}
	return nil
}

func (m *SSHAuthManager) applyAuthorizedKeysRaw(ctx context.Context, user string, keys []string) error {
	ch := Change{Target: "authorized_keys/" + user, After: keys}
	return m.applyAuthorizedKeys(ctx, ch)
}

// ---- Observation helpers -------------------------------------------------

func (m *SSHAuthManager) readAuthorizedKeys(ctx context.Context, user string) ([]string, error) {
	if m.sess == nil {
		return nil, nil
	}
	home := "/home/" + user
	if user == "root" {
		home = "/root"
	}
	cmd := "cat " + shellQuote(home+"/.ssh/authorized_keys") + " 2>/dev/null || true"
	out, _, err := m.sess.Run(ctx, cmd)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" && !strings.HasPrefix(ln, "#") {
			keys = append(keys, ln)
		}
	}
	return keys, nil
}

func (m *SSHAuthManager) readSSHDDropIn(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	if m.sess == nil {
		return out, nil
	}
	cmd := "cat " + shellQuote(sshdDropInPath) + " 2>/dev/null || true"
	stdout, _, err := m.sess.Run(ctx, cmd)
	if err != nil {
		return out, err
	}
	for _, ln := range strings.Split(stdout, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		parts := strings.SplitN(ln, " ", 2)
		if len(parts) != 2 {
			continue
		}
		out[parts[0]] = strings.TrimSpace(parts[1])
	}
	return out, nil
}

// ---- Cluster SSH setup (Phase 4 scaffold) --------------------------------

// SetupClusterSSH generates per-user SSH keypairs on each node, collects the
// public keys, and cross-authorizes them so the given users have passwordless
// ssh trust between all nodes. It also seeds known_hosts via ssh-keyscan.
//
// sessions is keyed by a stable node identifier (e.g. FQDN). users is the list
// of service accounts needing cluster trust (e.g. "grid", "oracle").
func SetupClusterSSH(ctx context.Context, sessions map[string]SessionRunner, users []string) error {
	if len(sessions) == 0 {
		return fmt.Errorf("SetupClusterSSH: no sessions provided")
	}
	if len(users) == 0 {
		return fmt.Errorf("SetupClusterSSH: no users provided")
	}

	nodes := make([]string, 0, len(sessions))
	for n := range sessions {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	pubs := map[string][]string{}
	for _, user := range users {
		for _, node := range nodes {
			sess := sessions[node]
			home := "/home/" + user
			if user == "root" {
				home = "/root"
			}
			keygen := fmt.Sprintf(
				"test -f %[1]s/.ssh/id_ed25519 || "+
					"(install -d -m 0700 -o %[2]s -g %[2]s %[1]s/.ssh && "+
					"su - %[2]s -c \"ssh-keygen -t ed25519 -N '' -f %[1]s/.ssh/id_ed25519 -q\")",
				shellQuote(home), shellQuote(user),
			)
			if _, _, err := sess.Run(ctx, keygen); err != nil {
				return fmt.Errorf("SetupClusterSSH: keygen %s@%s: %w", user, node, err)
			}
			pub, _, err := sess.Run(ctx, "cat "+shellQuote(home+"/.ssh/id_ed25519.pub"))
			if err != nil {
				return fmt.Errorf("SetupClusterSSH: read pub %s@%s: %w", user, node, err)
			}
			pub = strings.TrimSpace(pub)
			if pub != "" {
				pubs[user] = append(pubs[user], pub)
			}
		}
	}

	for _, user := range users {
		bundle := strings.Join(pubs[user], "\n") + "\n"
		for _, node := range nodes {
			sess := sessions[node]
			home := "/home/" + user
			if user == "root" {
				home = "/root"
			}
			auth := fmt.Sprintf(
				"install -d -m 0700 -o %[1]s -g %[1]s %[2]s/.ssh && "+
					"printf %%s %[3]s >> %[2]s/.ssh/authorized_keys && "+
					"chown %[1]s:%[1]s %[2]s/.ssh/authorized_keys && "+
					"chmod 0600 %[2]s/.ssh/authorized_keys",
				shellQuote(user), shellQuote(home), shellQuote(bundle),
			)
			if _, _, err := sess.Run(ctx, auth); err != nil {
				return fmt.Errorf("SetupClusterSSH: auth %s@%s: %w", user, node, err)
			}
			for _, other := range nodes {
				if other == node {
					continue
				}
				scan := fmt.Sprintf(
					"su - %[1]s -c \"ssh-keyscan -t ed25519 %[2]s >> %[3]s/.ssh/known_hosts 2>/dev/null\" && "+
						"chmod 0644 %[3]s/.ssh/known_hosts",
					shellQuote(user), shellQuote(other), shellQuote(home),
				)
				if _, _, err := sess.Run(ctx, scan); err != nil {
					return fmt.Errorf("SetupClusterSSH: keyscan %s->%s as %s: %w", node, other, user, err)
				}
			}
		}
	}
	return nil
}

// ---- helpers -------------------------------------------------------------

func (m *SSHAuthManager) run(ctx context.Context, cmd string) error {
	if m.sess == nil {
		return fmt.Errorf("no session attached")
	}
	_, stderr, err := m.sess.Run(ctx, cmd)
	if err != nil {
		if stderr != "" {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr))
		}
		return err
	}
	return nil
}

// castSSHConfig accepts either *config.SSHConfig, a *config.Linux (extracts
// .SSHConfig), or nil. Other types return an error.
func castSSHConfig(desired Spec) (*config.SSHConfig, error) {
	switch v := desired.(type) {
	case *config.SSHConfig:
		return v, nil
	case config.SSHConfig:
		return &v, nil
	case *config.Linux:
		if v == nil {
			return nil, nil
		}
		return v.SSHConfig, nil
	case config.Linux:
		return v.SSHConfig, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("ssh: unsupported desired-state type %T", desired)
	}
}
