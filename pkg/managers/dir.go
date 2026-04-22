package managers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/session"
)

// DirManager reconciles a list of config.Directory entries. It is the simplest
// manager and proves the Manager contract end-to-end.
//
// Safety: Plan/Apply only create or adjust ownership/mode. Delete is never
// planned — directories must be removed by a human. Rollback reverses only
// in-session creations.
type DirManager struct {
	sess session.Session
}

// NewDirManager returns a directory manager without a session. Use
// WithSession to bind one before Plan/Apply/Verify/Rollback.
func NewDirManager() *DirManager { return &DirManager{} }

// WithSession returns a copy bound to sess.
func (m *DirManager) WithSession(sess session.Session) *DirManager {
	cp := *m
	cp.sess = sess
	return &cp
}

// Name implements Manager.
func (*DirManager) Name() string { return "dir" }

// DependsOn implements Manager — directories are created after their mount
// parents exist.
func (*DirManager) DependsOn() []string { return []string{"mount"} }

func init() { Register(NewDirManager()) }

// currentDir captures an observed directory's ownership + mode.
type currentDir struct {
	Exists bool
	Owner  string
	Group  string
	Mode   string // 4-char octal, e.g. "0755"
}

// ---- Plan -----------------------------------------------------------------

// Plan implements Manager.
func (m *DirManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	dirs, err := castDirectories(desired)
	if err != nil {
		return nil, err
	}
	if m.sess == nil {
		return nil, ErrSessionRequired
	}
	var changes []Change
	for _, d := range dirs {
		cur, err := m.stat(ctx, d.Path)
		if err != nil {
			return nil, fmt.Errorf("dir.Plan: stat %s: %w", d.Path, err)
		}
		if !cur.Exists {
			changes = append(changes, Change{
				ID:      "dir:" + d.Path,
				Manager: "dir",
				Target:  d.Path,
				Action:  "create",
				After:   d,
				Hazard:  HazardNone,
			})
			continue
		}
		if dirDrift(d, cur) {
			changes = append(changes, Change{
				ID:      "dir:" + d.Path,
				Manager: "dir",
				Target:  d.Path,
				Action:  "update",
				Before:  cur,
				After:   d,
				Hazard:  HazardNone,
			})
		}
	}
	return changes, nil
}

func dirDrift(d config.Directory, cur currentDir) bool {
	if d.Owner != "" && d.Owner != cur.Owner {
		return true
	}
	if d.Group != "" && d.Group != cur.Group {
		return true
	}
	if d.Mode != "" && d.Mode != cur.Mode {
		return true
	}
	return false
}

// stat returns observed ownership + mode, normalising mode to 4 digits.
func (m *DirManager) stat(ctx context.Context, path string) (currentDir, error) {
	exists, err := m.sess.FileExists(ctx, path)
	if err != nil {
		return currentDir{}, err
	}
	if !exists {
		return currentDir{Exists: false}, nil
	}
	out, _, err := m.sess.Run(ctx, "stat -c '%U %G %a' "+shellQuoteOne(path))
	if err != nil {
		out2, stderr, err2 := m.sess.RunSudo(ctx, "stat -c '%U %G %a' "+shellQuoteOne(path))
		if err2 != nil {
			return currentDir{}, fmt.Errorf("stat %s: %w (%s)", path, err2, strings.TrimSpace(stderr))
		}
		out = out2
	}
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) != 3 {
		return currentDir{}, fmt.Errorf("stat %s: unexpected output %q", path, out)
	}
	mode := parts[2]
	if len(mode) == 3 {
		mode = "0" + mode
	}
	return currentDir{Exists: true, Owner: parts[0], Group: parts[1], Mode: mode}, nil
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *DirManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, ErrSessionRequired
	}
	for _, ch := range changes {
		d, ok := ch.After.(config.Directory)
		if !ok {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("dir.Apply: After is not config.Directory (%T)", ch.After)})
			continue
		}
		if err := m.applyOne(ctx, ch.Action, d); err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: err})
			continue
		}
		res.Applied = append(res.Applied, ch)
	}
	return res, nil
}

func (m *DirManager) applyOne(ctx context.Context, action string, d config.Directory) error {
	switch action {
	case "create":
		mkdir := "mkdir -p " + shellQuoteOne(d.Path)
		if _, err := RunSudoAndCheck(ctx, m.sess, mkdir); err != nil {
			return err
		}
		return m.applyAttrs(ctx, d)
	case "update":
		return m.applyAttrs(ctx, d)
	default:
		return fmt.Errorf("dir.Apply: unknown action %q", action)
	}
}

func (m *DirManager) applyAttrs(ctx context.Context, d config.Directory) error {
	recurse := ""
	if d.Recursive {
		recurse = "-R "
	}
	if d.Owner != "" || d.Group != "" {
		spec := d.Owner
		if d.Group != "" {
			spec += ":" + d.Group
		}
		cmd := "chown " + recurse + shellQuoteOne(spec) + " " + shellQuoteOne(d.Path)
		if _, err := RunSudoAndCheck(ctx, m.sess, cmd); err != nil {
			return err
		}
	}
	if d.Mode != "" {
		mode := strings.TrimPrefix(d.Mode, "0")
		if _, err := strconv.ParseUint(mode, 8, 32); err != nil {
			return fmt.Errorf("dir.Apply: invalid mode %q: %w", d.Mode, err)
		}
		cmd := "chmod " + recurse + d.Mode + " " + shellQuoteOne(d.Path)
		if _, err := RunSudoAndCheck(ctx, m.sess, cmd); err != nil {
			return err
		}
	}
	return nil
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *DirManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback implements Manager. Only reverses creations (`rmdir` if empty);
// ownership / mode updates are not reversed because the manager cannot know
// the pre-apply values without a persisted run record.
func (m *DirManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return ErrSessionRequired
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		if ch.Action != "create" {
			continue
		}
		d, ok := ch.After.(config.Directory)
		if !ok {
			continue
		}
		// Swallow failure: rmdir errors on non-empty dirs — safe semantics.
		_, _ = RunSudoAndCheck(ctx, m.sess, "rmdir "+shellQuoteOne(d.Path))
	}
	return nil
}

// castDirectories accepts either []config.Directory, *config.Linux, or a
// generic Spec carrying Directories.
func castDirectories(desired Spec) ([]config.Directory, error) {
	switch v := desired.(type) {
	case []config.Directory:
		return v, nil
	case *config.Linux:
		if v == nil {
			return nil, nil
		}
		return v.Directories, nil
	case config.Linux:
		return v.Directories, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("dir: unsupported desired-state type %T", desired)
	}
}
