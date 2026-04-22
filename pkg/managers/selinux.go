package managers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// selinuxConfigPath is the file managed for persistent SELinux mode.
const selinuxConfigPath = "/etc/selinux/config"

func init() { Register(NewSELinuxManager()) }

// SELinuxManager handles the "selinux" subsystem: enforcing mode + boolean
// overrides. Persistent mode is written to /etc/selinux/config; runtime mode
// is toggled via setenforce. Booleans are persisted with `setsebool -P`.
type SELinuxManager struct {
	sess SessionRunner
}

// NewSELinuxManager returns a selinux manager.
func NewSELinuxManager() *SELinuxManager { return &SELinuxManager{} }

// WithSession attaches a SessionRunner for Apply / Verify.
func (m *SELinuxManager) WithSession(s SessionRunner) *SELinuxManager {
	m.sess = s
	return m
}

// Name implements Manager.
func (*SELinuxManager) Name() string { return "selinux" }

// DependsOn implements Manager. selinux-policy-* is installed by the package
// manager; the service manager owns auditd which integrates with SELinux.
func (*SELinuxManager) DependsOn() []string { return []string{"package"} }

// ---- Plan -----------------------------------------------------------------

// Plan implements Manager.
func (m *SELinuxManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	cfg, err := castSELinuxConfig(desired)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	var changes []Change

	if cfg.Mode != "" {
		cur, err := m.readMode(ctx)
		if err != nil {
			return nil, fmt.Errorf("selinux.Plan: read mode: %w", err)
		}
		if cur != cfg.Mode {
			requiresReboot := cfg.Mode == "disabled" || cur == "disabled"
			changes = append(changes, Change{
				ID:      "selinux:mode",
				Manager: "selinux",
				Target:  "selinux/mode",
				Action:  "update",
				Before:  cur,
				After:   cfg.Mode,
				Hazard:  hazardForMode(cfg.Mode, cur),
				RollbackCmd: func() string {
					if requiresReboot {
						return "reboot required to restore mode"
					}
					return ""
				}(),
			})
		}
	}

	// Booleans.
	keys := make([]string, 0, len(cfg.Booleans))
	for k := range cfg.Booleans {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		want := cfg.Booleans[k]
		cur, err := m.readBoolean(ctx, k)
		if err != nil {
			return nil, fmt.Errorf("selinux.Plan: getsebool %s: %w", k, err)
		}
		if cur != want {
			changes = append(changes, Change{
				ID:      "selinux:bool:" + k,
				Manager: "selinux",
				Target:  "selinux/bool/" + k,
				Action:  "update",
				Before:  cur,
				After:   want,
				Hazard:  HazardWarn,
			})
		}
	}
	return changes, nil
}

func hazardForMode(desired, current string) HazardLevel {
	if desired == "disabled" || current == "disabled" {
		return HazardDestructive
	}
	return HazardWarn
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *SELinuxManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, fmt.Errorf("selinux.Apply: no session attached")
	}
	for _, ch := range changes {
		var err error
		switch {
		case ch.Target == "selinux/mode":
			err = m.applyMode(ctx, &ch)
		case strings.HasPrefix(ch.Target, "selinux/bool/"):
			err = m.applyBoolean(ctx, ch)
		default:
			err = fmt.Errorf("selinux.Apply: unknown target %q", ch.Target)
		}
		if err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: err})
			continue
		}
		res.Applied = append(res.Applied, ch)
	}
	return res, nil
}

// applyMode writes /etc/selinux/config and — for enforcing/permissive — calls
// setenforce for runtime effect. Transitioning to/from "disabled" requires a
// reboot; we only edit the config file and mark the change appropriately.
// Note: ch is a pointer so callers can observe the annotation on After.
func (m *SELinuxManager) applyMode(ctx context.Context, ch *Change) error {
	mode, ok := ch.After.(string)
	if !ok {
		return fmt.Errorf("selinux.applyMode: After is not string (%T)", ch.After)
	}
	before, _ := ch.Before.(string)
	// Runtime toggle — only valid between enforcing/permissive.
	if mode == "enforcing" && before != "disabled" {
		if err := m.run(ctx, "setenforce 1"); err != nil {
			return err
		}
	} else if mode == "permissive" && before != "disabled" {
		if err := m.run(ctx, "setenforce 0"); err != nil {
			return err
		}
	}
	// Persist config file. Uses sed to replace the SELINUX= line.
	cmd := fmt.Sprintf(
		"sed -i 's/^SELINUX=.*/SELINUX=%s/' %s",
		mode, selinuxConfigPath,
	)
	if err := m.run(ctx, cmd); err != nil {
		return err
	}
	return nil
}

func (m *SELinuxManager) applyBoolean(ctx context.Context, ch Change) error {
	key := strings.TrimPrefix(ch.Target, "selinux/bool/")
	want, ok := ch.After.(bool)
	if !ok {
		return fmt.Errorf("selinux.applyBoolean: After is not bool (%T)", ch.After)
	}
	state := "off"
	if want {
		state = "on"
	}
	return m.run(ctx, fmt.Sprintf("setsebool -P %s %s", shellQuote(key), state))
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *SELinuxManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback implements Manager. Restores Before state by re-issuing the same
// Apply path against a reversed change. Mode transitions involving "disabled"
// still require a reboot.
func (m *SELinuxManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return fmt.Errorf("selinux.Rollback: no session attached")
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		if ch.Before == nil {
			continue
		}
		reverse := Change{Target: ch.Target, Action: "update", Before: ch.After, After: ch.Before}
		var err error
		switch {
		case ch.Target == "selinux/mode":
			err = m.applyMode(ctx, &reverse)
		case strings.HasPrefix(ch.Target, "selinux/bool/"):
			err = m.applyBoolean(ctx, reverse)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ---- Observation helpers -------------------------------------------------

// readMode returns the effective SELinux mode. Prefers /etc/selinux/config so
// we compare on persistent state; falls back to getenforce output.
func (m *SELinuxManager) readMode(ctx context.Context) (string, error) {
	if m.sess == nil {
		return "", nil
	}
	out, _, err := m.sess.Run(ctx,
		"awk -F= '/^SELINUX=/{print tolower($2); exit}' "+selinuxConfigPath+" 2>/dev/null || true")
	if err != nil {
		return "", err
	}
	mode := strings.TrimSpace(out)
	if mode == "enforcing" || mode == "permissive" || mode == "disabled" {
		return mode, nil
	}
	// Fallback: getenforce.
	out2, _, err := m.sess.Run(ctx, "getenforce 2>/dev/null || true")
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(out2)) {
	case "enforcing":
		return "enforcing", nil
	case "permissive":
		return "permissive", nil
	case "disabled":
		return "disabled", nil
	}
	return "", nil
}

// readBoolean reports the current state of an SELinux boolean.
// `getsebool foo` outputs: `foo --> on` or `foo --> off`.
func (m *SELinuxManager) readBoolean(ctx context.Context, key string) (bool, error) {
	if m.sess == nil {
		return false, nil
	}
	out, _, err := m.sess.Run(ctx, "getsebool "+shellQuote(key)+" 2>/dev/null || true")
	if err != nil {
		return false, err
	}
	s := strings.TrimSpace(out)
	return strings.HasSuffix(s, " on"), nil
}

// ---- helpers -------------------------------------------------------------

func (m *SELinuxManager) run(ctx context.Context, cmd string) error {
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

// castSELinuxConfig accepts *config.SELinuxConfig, *config.Linux (extracts
// .SELinux), or nil.
func castSELinuxConfig(desired Spec) (*config.SELinuxConfig, error) {
	switch v := desired.(type) {
	case *config.SELinuxConfig:
		return v, nil
	case config.SELinuxConfig:
		return &v, nil
	case *config.Linux:
		if v == nil {
			return nil, nil
		}
		return v.SELinux, nil
	case config.Linux:
		return v.SELinux, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("selinux: unsupported desired-state type %T", desired)
	}
}
