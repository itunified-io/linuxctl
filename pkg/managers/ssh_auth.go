package managers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

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

// ---- Cluster SSH setup ---------------------------------------------------

// ClusterSSHResult summarises what SetupClusterSSH did per node, plus any
// errors that were captured without aborting the run.
type ClusterSSHResult struct {
	PerNode map[string]*NodeSSHResult
	Errors  []error
}

// NodeSSHResult is a per-host summary. All maps are keyed by service-account
// username (e.g. "grid", "oracle").
type NodeSSHResult struct {
	Hostname       string
	GeneratedKeys  map[string]string // user → key fingerprint (may be empty on existing keys)
	AuthorizedKeys map[string]int    // user → count of keys installed
	KnownHosts     int               // count of scanned known_hosts entries
}

// SetupClusterSSH generates Ed25519 keypairs for each (node, user) pair,
// cross-authorizes the collected public keys, and seeds per-user known_hosts
// via ssh-keyscan. Idempotent: re-running yields no drift.
//
// Phase 1 (parallel per node): ensure ~user/.ssh/id_ed25519 exists, read
// pubkey. Phase 2 (serialised): merge collected pubkeys into each node's
// authorized_keys + seed known_hosts for every peer.
//
// Per-node failures are accumulated in Result.Errors rather than aborting the
// whole run — the rest of the cluster may still get partial setup, which is
// more useful than a half-broken RAC bootstrap with no visibility.
func SetupClusterSSH(ctx context.Context, sessions map[string]SessionRunner, users []string) (*ClusterSSHResult, error) {
	if len(sessions) == 0 {
		return nil, fmt.Errorf("SetupClusterSSH: no sessions provided")
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("SetupClusterSSH: no users provided")
	}

	nodes := make([]string, 0, len(sessions))
	for n := range sessions {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	res := &ClusterSSHResult{PerNode: make(map[string]*NodeSSHResult, len(nodes))}
	for _, n := range nodes {
		res.PerNode[n] = &NodeSSHResult{
			Hostname:       n,
			GeneratedKeys:  map[string]string{},
			AuthorizedKeys: map[string]int{},
		}
	}

	// ---- Phase 1: parallel key generation + pubkey read ------------------
	type phase1Out struct {
		node    string
		pubs    map[string]string // user → pubkey line
		errs    []error
	}
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out = make([]phase1Out, 0, len(nodes))
	)
	for _, node := range nodes {
		wg.Add(1)
		go func(node string, sess SessionRunner) {
			defer wg.Done()
			p := phase1Out{node: node, pubs: map[string]string{}}
			for _, user := range users {
				pub, fp, err := ensureKeypair(ctx, sess, user)
				if err != nil {
					p.errs = append(p.errs, fmt.Errorf("%s@%s: %w", user, node, err))
					continue
				}
				if pub != "" {
					p.pubs[user] = pub
				}
				// The fingerprint is blank when the key already existed and we
				// don't re-read it; that's fine — GeneratedKeys only records
				// freshly-minted keys.
				if fp != "" {
					mu.Lock()
					res.PerNode[node].GeneratedKeys[user] = fp
					mu.Unlock()
				}
			}
			mu.Lock()
			out = append(out, p)
			mu.Unlock()
		}(node, sessions[node])
	}
	wg.Wait()

	// Sort phase1 output deterministically so downstream merge order is stable.
	sort.Slice(out, func(i, j int) bool { return out[i].node < out[j].node })

	// Aggregate pubs per user across the cluster.
	pubsByUser := map[string][]string{}
	for _, p := range out {
		if len(p.errs) > 0 {
			res.Errors = append(res.Errors, p.errs...)
		}
		for u, k := range p.pubs {
			pubsByUser[u] = append(pubsByUser[u], k)
		}
	}

	// ---- Phase 2: serialised authorized_keys + known_hosts merge ---------
	for _, node := range nodes {
		sess := sessions[node]
		nr := res.PerNode[node]
		for _, user := range users {
			count, err := mergeAuthorizedKeys(ctx, sess, user, pubsByUser[user])
			if err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("auth %s@%s: %w", user, node, err))
				continue
			}
			nr.AuthorizedKeys[user] = count
		}
		peers := make([]string, 0, len(nodes)-1)
		for _, other := range nodes {
			if other != node {
				peers = append(peers, other)
			}
		}
		n, err := seedKnownHosts(ctx, sess, users, peers)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("keyscan on %s: %w", node, err))
		}
		nr.KnownHosts = n
	}

	return res, nil
}

// ensureKeypair creates ~user/.ssh/id_ed25519 if missing and returns the
// public key string. The second return is the key fingerprint if the key was
// freshly generated, empty if it already existed (idempotency signal).
func ensureKeypair(ctx context.Context, sess SessionRunner, user string) (pubkey, fingerprint string, err error) {
	home := userHome(user)
	keyPath := home + "/.ssh/id_ed25519"
	pubPath := keyPath + ".pub"

	// Idempotency probe. We use a deterministic string the script echoes to
	// tell us whether the key already existed.
	probe := fmt.Sprintf("test -f %s && echo __EXISTS__ || echo __MISSING__",
		shellQuote(keyPath))
	stdout, _, err := sess.Run(ctx, probe)
	if err != nil {
		return "", "", fmt.Errorf("probe: %w", err)
	}
	exists := strings.Contains(stdout, "__EXISTS__")
	if !exists {
		gen := fmt.Sprintf(
			"install -d -m 0700 -o %[1]s -g %[1]s %[2]s/.ssh && "+
				"ssh-keygen -t ed25519 -N '' -f %[3]s -q && "+
				"chown %[1]s:%[1]s %[3]s %[3]s.pub && "+
				"chmod 0600 %[3]s && chmod 0644 %[3]s.pub",
			shellQuote(user), shellQuote(home), shellQuote(keyPath),
		)
		if _, _, err := sess.Run(ctx, gen); err != nil {
			return "", "", fmt.Errorf("keygen: %w", err)
		}
		fpOut, _, err := sess.Run(ctx, "ssh-keygen -lf "+shellQuote(pubPath))
		if err == nil {
			fingerprint = strings.TrimSpace(fpOut)
		}
	}
	pub, _, err := sess.Run(ctx, "cat "+shellQuote(pubPath))
	if err != nil {
		return "", fingerprint, fmt.Errorf("read pub: %w", err)
	}
	return strings.TrimSpace(pub), fingerprint, nil
}

// mergeAuthorizedKeys reads ~user/.ssh/authorized_keys, de-dups against the
// provided bundle (by full line match), writes back with 0600 / user:user.
// Returns the total count of keys present after the merge.
func mergeAuthorizedKeys(ctx context.Context, sess SessionRunner, user string, pubs []string) (int, error) {
	home := userHome(user)
	path := home + "/.ssh/authorized_keys"

	curOut, _, err := sess.Run(ctx, "cat "+shellQuote(path)+" 2>/dev/null || true")
	if err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	seen := map[string]struct{}{}
	var merged []string
	for _, ln := range strings.Split(curOut, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		if _, ok := seen[ln]; ok {
			continue
		}
		seen[ln] = struct{}{}
		merged = append(merged, ln)
	}
	for _, p := range pubs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		merged = append(merged, p)
	}
	content := strings.Join(merged, "\n") + "\n"
	write := fmt.Sprintf(
		"install -d -m 0700 -o %[1]s -g %[1]s %[2]s/.ssh && "+
			"umask 077 && printf %%s %[3]s | tee %[4]s >/dev/null && "+
			"chown %[1]s:%[1]s %[4]s && chmod 0600 %[4]s",
		shellQuote(user), shellQuote(home), shellQuote(content), shellQuote(path),
	)
	if _, _, err := sess.Run(ctx, write); err != nil {
		return 0, fmt.Errorf("write: %w", err)
	}
	return len(merged), nil
}

// seedKnownHosts scans each peer via ssh-keyscan and appends (de-duped) into
// every target user's ~/.ssh/known_hosts. Returns the number of unique peer
// entries installed.
func seedKnownHosts(ctx context.Context, sess SessionRunner, users, peers []string) (int, error) {
	if len(peers) == 0 {
		return 0, nil
	}
	// Do a single ssh-keyscan for all peers, then fan the output out per-user.
	scanCmd := "ssh-keyscan -t ed25519 " + strings.Join(shellQuoteEach(peers), " ") + " 2>/dev/null"
	scan, _, err := sess.Run(ctx, scanCmd)
	if err != nil {
		return 0, fmt.Errorf("ssh-keyscan: %w", err)
	}
	scan = strings.TrimSpace(scan)
	if scan == "" {
		return 0, nil
	}
	lines := strings.Split(scan, "\n")
	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if _, ok := seen[ln]; ok {
			continue
		}
		seen[ln] = struct{}{}
		uniq = append(uniq, ln)
	}
	for _, user := range users {
		home := userHome(user)
		path := home + "/.ssh/known_hosts"
		cur, _, _ := sess.Run(ctx, "cat "+shellQuote(path)+" 2>/dev/null || true")
		have := map[string]struct{}{}
		for _, ln := range strings.Split(cur, "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			have[ln] = struct{}{}
		}
		merged := make([]string, 0, len(have)+len(uniq))
		for ln := range have {
			merged = append(merged, ln)
		}
		for _, ln := range uniq {
			if _, ok := have[ln]; ok {
				continue
			}
			merged = append(merged, ln)
		}
		sort.Strings(merged)
		content := strings.Join(merged, "\n") + "\n"
		write := fmt.Sprintf(
			"install -d -m 0700 -o %[1]s -g %[1]s %[2]s/.ssh && "+
				"printf %%s %[3]s | tee %[4]s >/dev/null && "+
				"chown %[1]s:%[1]s %[4]s && chmod 0644 %[4]s",
			shellQuote(user), shellQuote(home), shellQuote(content), shellQuote(path),
		)
		if _, _, err := sess.Run(ctx, write); err != nil {
			return 0, fmt.Errorf("write known_hosts for %s: %w", user, err)
		}
	}
	return len(uniq), nil
}

func userHome(user string) string {
	if user == "root" {
		return "/root"
	}
	return "/home/" + user
}

func shellQuoteEach(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = shellQuote(x)
	}
	return out
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
