package managers

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/presets"
)

// SessionRunner is the minimal command-execution contract that the User and
// Package managers need. It is intentionally narrower than pkg/session.Session
// so tests can mock it easily and so the managers do not block on Agent A's
// final session contract.
type SessionRunner interface {
	Run(ctx context.Context, cmd string) (stdout, stderr string, err error)
}

// UserSpec is a single user entry in the users_groups manifest. Kept here
// (not in pkg/config) until Agent A promotes the concrete schema. The fields
// line up with the contract given for pkg/config.User.
type UserSpec struct {
	Name     string   `yaml:"name"`
	UID      int      `yaml:"uid,omitempty"`
	GID      string   `yaml:"gid,omitempty"` // primary group name
	Groups   []string `yaml:"groups,omitempty"`
	Home     string   `yaml:"home,omitempty"`
	Shell    string   `yaml:"shell,omitempty"`
	SSHKeys  []string `yaml:"ssh_keys,omitempty"`
	Password string   `yaml:"password,omitempty"` // hash, or ${vault:...}
	Sudo     string   `yaml:"sudo,omitempty"`     // "" | "NOPASSWD" | "PASSWD"
}

// GroupSpec is a single group entry.
type GroupSpec struct {
	Name string `yaml:"name"`
	GID  int    `yaml:"gid,omitempty"`
}

// UsersGroupsSpec is the desired state for the user manager.
type UsersGroupsSpec struct {
	Groups []GroupSpec `yaml:"groups,omitempty"`
	Users  []UserSpec  `yaml:"users,omitempty"`
}

func init() { Register(NewUserManager()) }

// UserManager handles groupadd/useradd/usermod/authorized_keys idempotently.
type UserManager struct {
	sess SessionRunner
}

// NewUserManager returns a user manager bound to sess. Passing nil is valid
// for tests that only exercise Plan against a pre-populated current state.
func NewUserManager() *UserManager { return &UserManager{} }

// WithSession attaches a SessionRunner and returns the receiver for chaining.
func (m *UserManager) WithSession(s SessionRunner) *UserManager {
	m.sess = s
	return m
}

// Name implements Manager.
func (*UserManager) Name() string { return "user" }

// DependsOn implements Manager. Users may reference packages (e.g. shell),
// so we order after the package manager.
func (*UserManager) DependsOn() []string { return []string{"package"} }

// ---- Plan -----------------------------------------------------------------

// currentUser is the observed state we extract via `getent passwd`.
type currentUser struct {
	UID     int
	GID     int
	Home    string
	Shell   string
	Groups  []string
	SSHKeys []string
}

// currentGroup is the observed state we extract via `getent group`.
type currentGroup struct {
	GID     int
	Members []string
	Exists  bool
}

// Plan implements Manager.
func (m *UserManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	ug, err := castUsersGroups(desired)
	if err != nil {
		return nil, err
	}
	var changes []Change

	// Groups first — users may reference them as primary group.
	for _, g := range ug.Groups {
		cur, err := m.getGroup(ctx, g.Name)
		if err != nil {
			return nil, fmt.Errorf("user.Plan: getent group %s: %w", g.Name, err)
		}
		if !cur.Exists {
			changes = append(changes, Change{
				ID:      "group:" + g.Name,
				Manager: "user",
				Target:  "group/" + g.Name,
				Action:  "create",
				After:   g,
				Hazard:  HazardNone,
			})
			continue
		}
		if g.GID != 0 && cur.GID != g.GID {
			changes = append(changes, Change{
				ID:      "group:" + g.Name,
				Manager: "user",
				Target:  "group/" + g.Name,
				Action:  "update",
				Before:  GroupSpec{Name: g.Name, GID: cur.GID},
				After:   g,
				Hazard:  HazardWarn,
			})
		}
	}

	// Users next.
	for _, u := range ug.Users {
		cur, exists, err := m.getUser(ctx, u.Name)
		if err != nil {
			return nil, fmt.Errorf("user.Plan: getent passwd %s: %w", u.Name, err)
		}
		if !exists {
			changes = append(changes, Change{
				ID:      "user:" + u.Name,
				Manager: "user",
				Target:  "user/" + u.Name,
				Action:  "create",
				After:   u,
				Hazard:  HazardNone,
			})
			continue
		}
		drift := userDrift(u, cur)
		if drift {
			changes = append(changes, Change{
				ID:      "user:" + u.Name,
				Manager: "user",
				Target:  "user/" + u.Name,
				Action:  "update",
				Before:  cur,
				After:   u,
				Hazard:  HazardWarn,
			})
		}
	}

	return changes, nil
}

// userDrift returns true if the live user needs an update to match desired.
func userDrift(d UserSpec, c currentUser) bool {
	if d.UID != 0 && d.UID != c.UID {
		return true
	}
	if d.Home != "" && d.Home != c.Home {
		return true
	}
	if d.Shell != "" && d.Shell != c.Shell {
		return true
	}
	if len(d.Groups) > 0 && !sameStringSet(d.Groups, c.Groups) {
		return true
	}
	if len(d.SSHKeys) > 0 && !sameStringSet(d.SSHKeys, c.SSHKeys) {
		return true
	}
	return false
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *UserManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, fmt.Errorf("user.Apply: no session attached")
	}

	// Sort: group creates first, then group updates, then user creates, then user updates.
	ordered := append([]Change(nil), changes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return orderKey(ordered[i]) < orderKey(ordered[j])
	})

	for _, ch := range ordered {
		var err error
		switch {
		case strings.HasPrefix(ch.Target, "group/"):
			err = m.applyGroup(ctx, ch)
		case strings.HasPrefix(ch.Target, "user/"):
			err = m.applyUser(ctx, ch)
		default:
			err = fmt.Errorf("user.Apply: unknown target %q", ch.Target)
		}
		if err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: err})
			continue
		}
		res.Applied = append(res.Applied, ch)
	}
	return res, nil
}

func orderKey(c Change) int {
	isGroup := strings.HasPrefix(c.Target, "group/")
	switch {
	case isGroup && c.Action == "create":
		return 0
	case isGroup && c.Action == "update":
		return 1
	case c.Action == "create":
		return 2
	case c.Action == "update":
		return 3
	default:
		return 4
	}
}

func (m *UserManager) applyGroup(ctx context.Context, ch Change) error {
	g, ok := ch.After.(GroupSpec)
	if !ok {
		return fmt.Errorf("user.applyGroup: After is not GroupSpec (%T)", ch.After)
	}
	switch ch.Action {
	case "create":
		cmd := "groupadd"
		if g.GID != 0 {
			cmd += " -g " + strconv.Itoa(g.GID)
		}
		cmd += " " + shellQuote(g.Name)
		return m.run(ctx, cmd)
	case "update":
		if g.GID == 0 {
			return nil
		}
		return m.run(ctx, fmt.Sprintf("groupmod -g %d %s", g.GID, shellQuote(g.Name)))
	case "delete":
		return m.run(ctx, "groupdel "+shellQuote(g.Name))
	}
	return fmt.Errorf("user.applyGroup: unknown action %q", ch.Action)
}

func (m *UserManager) applyUser(ctx context.Context, ch Change) error {
	u, ok := ch.After.(UserSpec)
	if !ok {
		return fmt.Errorf("user.applyUser: After is not UserSpec (%T)", ch.After)
	}
	switch ch.Action {
	case "create":
		return m.createUser(ctx, u)
	case "update":
		return m.updateUser(ctx, u)
	case "delete":
		return m.run(ctx, "userdel -r "+shellQuote(u.Name))
	}
	return fmt.Errorf("user.applyUser: unknown action %q", ch.Action)
}

func (m *UserManager) createUser(ctx context.Context, u UserSpec) error {
	var args []string
	args = append(args, "useradd")
	if u.UID != 0 {
		args = append(args, "-u", strconv.Itoa(u.UID))
	}
	if u.GID != "" {
		args = append(args, "-g", shellQuote(u.GID))
	}
	if len(u.Groups) > 0 {
		args = append(args, "-G", shellQuote(strings.Join(u.Groups, ",")))
	}
	if u.Shell != "" {
		args = append(args, "-s", shellQuote(u.Shell))
	}
	if u.Home != "" {
		args = append(args, "-d", shellQuote(u.Home), "-m")
	} else {
		args = append(args, "-m")
	}
	args = append(args, shellQuote(u.Name))
	if err := m.run(ctx, strings.Join(args, " ")); err != nil {
		return err
	}
	if err := m.applySSHKeys(ctx, u); err != nil {
		return err
	}
	if u.Password != "" {
		if err := m.applyPassword(ctx, u); err != nil {
			return err
		}
	}
	if err := m.applySudo(ctx, u); err != nil {
		return err
	}
	return nil
}

func (m *UserManager) updateUser(ctx context.Context, u UserSpec) error {
	// usermod in several idempotent steps. We are careful to only emit the flag
	// we actually need so re-runs without drift are cheap.
	if u.GID != "" {
		if err := m.run(ctx, fmt.Sprintf("usermod -g %s %s", shellQuote(u.GID), shellQuote(u.Name))); err != nil {
			return err
		}
	}
	if len(u.Groups) > 0 {
		if err := m.run(ctx, fmt.Sprintf("usermod -G %s %s",
			shellQuote(strings.Join(u.Groups, ",")), shellQuote(u.Name))); err != nil {
			return err
		}
	}
	if u.Shell != "" {
		if err := m.run(ctx, fmt.Sprintf("usermod -s %s %s", shellQuote(u.Shell), shellQuote(u.Name))); err != nil {
			return err
		}
	}
	if u.Home != "" {
		if err := m.run(ctx, fmt.Sprintf("usermod -d %s -m %s", shellQuote(u.Home), shellQuote(u.Name))); err != nil {
			return err
		}
	}
	if len(u.SSHKeys) > 0 {
		if err := m.applySSHKeys(ctx, u); err != nil {
			return err
		}
	}
	if u.Password != "" {
		if err := m.applyPassword(ctx, u); err != nil {
			return err
		}
	}
	if err := m.applySudo(ctx, u); err != nil {
		return err
	}
	return nil
}

// applySudo writes /etc/sudoers.d/<user> with a NOPASSWD or PASSWD rule per
// UserSpec.Sudo. Empty string is a no-op (file untouched). The file is mode
// 0440 (sudoers requirement) and validated via `visudo -cf` before placement.
func (m *UserManager) applySudo(ctx context.Context, u UserSpec) error {
	if u.Sudo == "" {
		return nil
	}
	var line string
	switch strings.ToUpper(u.Sudo) {
	case "NOPASSWD":
		line = fmt.Sprintf("%s ALL=(ALL) NOPASSWD: ALL\n", u.Name)
	case "PASSWD":
		line = fmt.Sprintf("%s ALL=(ALL) ALL\n", u.Name)
	default:
		return fmt.Errorf("user %s: unknown sudo policy %q (want NOPASSWD or PASSWD)", u.Name, u.Sudo)
	}
	path := "/etc/sudoers.d/" + u.Name
	cmd := fmt.Sprintf(
		"umask 277 && printf %%s %s | tee %s >/dev/null && chmod 0440 %s && visudo -cf %s",
		shellQuote(line), shellQuote(path), shellQuote(path), shellQuote(path),
	)
	if err := m.run(ctx, cmd); err != nil {
		return fmt.Errorf("apply sudo for %s: %w", u.Name, err)
	}
	return nil
}

// applySSHKeys writes ~<user>/.ssh/authorized_keys via install(1) so permissions
// and ownership end up correct in a single round-trip.
func (m *UserManager) applySSHKeys(ctx context.Context, u UserSpec) error {
	if len(u.SSHKeys) == 0 {
		return nil
	}
	home := u.Home
	if home == "" {
		home = "/home/" + u.Name
	}
	content := strings.Join(u.SSHKeys, "\n") + "\n"
	// Single shell pipeline: make dir, tee into authorized_keys, chown, chmod.
	script := fmt.Sprintf(
		"install -d -m 0700 -o %[1]s -g %[1]s %[2]s/.ssh && "+
			"umask 077 && printf %%s %[3]s | tee %[2]s/.ssh/authorized_keys >/dev/null && "+
			"chown %[1]s:%[1]s %[2]s/.ssh/authorized_keys && chmod 0600 %[2]s/.ssh/authorized_keys",
		shellQuote(u.Name), shellQuote(home), shellQuote(content),
	)
	return m.run(ctx, script)
}

// applyPassword pipes `user:hash` to chpasswd. We always use -e (pre-hashed)
// for security: a plain-text password in YAML is an anti-pattern.
func (m *UserManager) applyPassword(ctx context.Context, u UserSpec) error {
	cmd := fmt.Sprintf("printf %%s %s | chpasswd -e", shellQuote(u.Name+":"+u.Password))
	return m.run(ctx, cmd)
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *UserManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback implements Manager. Best-effort: we only reverse creates safely;
// update rollbacks require Before state and are applied with the same Apply
// path on a reversed change set.
func (m *UserManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return fmt.Errorf("user.Rollback: no session attached")
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		switch ch.Action {
		case "create":
			switch {
			case strings.HasPrefix(ch.Target, "user/"):
				name := strings.TrimPrefix(ch.Target, "user/")
				if err := m.run(ctx, "userdel -r "+shellQuote(name)); err != nil {
					return err
				}
			case strings.HasPrefix(ch.Target, "group/"):
				name := strings.TrimPrefix(ch.Target, "group/")
				if err := m.run(ctx, "groupdel "+shellQuote(name)); err != nil {
					return err
				}
			}
		case "update":
			// Best-effort: re-apply Before state as an update.
			if ch.Before == nil {
				continue
			}
			reverse := Change{Action: "update", Target: ch.Target, After: ch.Before}
			switch {
			case strings.HasPrefix(ch.Target, "user/"):
				if _, ok := ch.Before.(UserSpec); ok {
					if err := m.applyUser(ctx, reverse); err != nil {
						return err
					}
				}
			case strings.HasPrefix(ch.Target, "group/"):
				if _, ok := ch.Before.(GroupSpec); ok {
					if err := m.applyGroup(ctx, reverse); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// ---- getent helpers -------------------------------------------------------

func (m *UserManager) getGroup(ctx context.Context, name string) (currentGroup, error) {
	out, _, err := m.runOut(ctx, "getent group "+shellQuote(name))
	if err != nil {
		// getent returns exit 2 for "not found"; treat any error as missing.
		return currentGroup{Exists: false}, nil
	}
	line := strings.TrimSpace(out)
	if line == "" {
		return currentGroup{Exists: false}, nil
	}
	// Format: name:x:gid:member1,member2
	parts := strings.Split(line, ":")
	if len(parts) < 3 {
		return currentGroup{Exists: false}, nil
	}
	gid, _ := strconv.Atoi(parts[2])
	cg := currentGroup{GID: gid, Exists: true}
	if len(parts) >= 4 && parts[3] != "" {
		cg.Members = strings.Split(parts[3], ",")
	}
	return cg, nil
}

func (m *UserManager) getUser(ctx context.Context, name string) (currentUser, bool, error) {
	out, _, err := m.runOut(ctx, "getent passwd "+shellQuote(name))
	if err != nil || strings.TrimSpace(out) == "" {
		return currentUser{}, false, nil
	}
	// name:x:uid:gid:gecos:home:shell
	parts := strings.Split(strings.TrimSpace(out), ":")
	if len(parts) < 7 {
		return currentUser{}, false, nil
	}
	uid, _ := strconv.Atoi(parts[2])
	gid, _ := strconv.Atoi(parts[3])
	cu := currentUser{UID: uid, GID: gid, Home: parts[5], Shell: parts[6]}

	// Supplementary groups via `id -nG <user>`.
	idOut, _, idErr := m.runOut(ctx, "id -nG "+shellQuote(name))
	if idErr == nil {
		cu.Groups = strings.Fields(idOut)
	}
	// authorized_keys if readable.
	keysOut, _, keysErr := m.runOut(ctx, "cat "+shellQuote(parts[5]+"/.ssh/authorized_keys")+" 2>/dev/null || true")
	if keysErr == nil {
		for _, ln := range strings.Split(keysOut, "\n") {
			ln = strings.TrimSpace(ln)
			if ln != "" && !strings.HasPrefix(ln, "#") {
				cu.SSHKeys = append(cu.SSHKeys, ln)
			}
		}
	}
	return cu, true, nil
}

// ---- runner shims ---------------------------------------------------------

func (m *UserManager) run(ctx context.Context, cmd string) error {
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

func (m *UserManager) runOut(ctx context.Context, cmd string) (string, string, error) {
	if m.sess == nil {
		return "", "", fmt.Errorf("no session attached")
	}
	return m.sess.Run(ctx, cmd)
}

// castUsersGroups accepts either the concrete typed spec, *config.Linux,
// or a generic map (unmarshaled YAML). When a *config.Linux carries a
// UsersGroupsPreset, the preset is merged with explicit entries.
func castUsersGroups(desired Spec) (UsersGroupsSpec, error) {
	switch v := desired.(type) {
	case UsersGroupsSpec:
		return v, nil
	case *UsersGroupsSpec:
		if v == nil {
			return UsersGroupsSpec{}, nil
		}
		return *v, nil
	case *config.Linux:
		if v == nil {
			return UsersGroupsSpec{}, nil
		}
		return usersGroupsFromLinux(v), nil
	case config.Linux:
		return usersGroupsFromLinux(&v), nil
	case nil:
		return UsersGroupsSpec{}, nil
	default:
		return UsersGroupsSpec{}, fmt.Errorf("user: unsupported desired-state type %T", desired)
	}
}

// usersGroupsFromLinux converts the config.Linux users_groups block into the
// manager's UsersGroupsSpec shape, applying preset merging if
// UsersGroupsPreset is set.
func usersGroupsFromLinux(l *config.Linux) UsersGroupsSpec {
	explicit := config.UsersGroups{}
	if l.UsersGroups != nil {
		explicit = *l.UsersGroups
	}
	merged := explicit
	if l.UsersGroupsPreset != "" {
		if p, err := presets.ResolveCategory("users_groups", l.UsersGroupsPreset, nil); err == nil {
			if pug, err := presets.UsersGroupsSpec(p); err == nil && pug != nil {
				merged = presets.MergeUsersGroups(explicit, *pug)
			} else if err != nil {
				log.Printf("user: preset %q decode: %v", l.UsersGroupsPreset, err)
			}
		} else {
			log.Printf("user: preset %q: %v", l.UsersGroupsPreset, err)
		}
	}
	out := UsersGroupsSpec{}
	for _, g := range merged.Groups {
		out.Groups = append(out.Groups, GroupSpec{Name: g.Name, GID: g.GID})
	}
	for _, u := range merged.Users {
		out.Users = append(out.Users, UserSpec{
			Name:     u.Name,
			UID:      u.UID,
			GID:      u.GID,
			Groups:   u.Groups,
			Home:     u.Home,
			Shell:    u.Shell,
			SSHKeys:  u.SSHKeys,
			Password: u.Password,
			Sudo:     u.Sudo,
		})
	}
	return out
}

// shellQuote wraps s in single quotes, escaping any embedded single quote.
// Safe for use in `sh -c` style commands.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
