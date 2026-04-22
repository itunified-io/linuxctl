package managers

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/session"
)

// LimitsManagedPath is the single drop-in file limits manager owns.
const LimitsManagedPath = "/etc/security/limits.d/99-linuxctl.conf"

// LimitsManager reconciles /etc/security/limits.d/99-linuxctl.conf.
type LimitsManager struct {
	sess session.Session
}

// NewLimitsManager returns a limits manager without a session.
func NewLimitsManager() *LimitsManager { return &LimitsManager{} }

// WithSession returns a copy bound to sess.
func (m *LimitsManager) WithSession(sess session.Session) *LimitsManager {
	cp := *m
	cp.sess = sess
	return &cp
}

// Name implements Manager.
func (*LimitsManager) Name() string { return "limits" }

// DependsOn implements Manager.
func (*LimitsManager) DependsOn() []string { return []string{"sysctl", "user"} }

func init() { Register(NewLimitsManager()) }

// ---- Preset ---------------------------------------------------------------

// presetLimits returns hardcoded limits entries for a named preset.
func presetLimits(name string) []config.LimitEntry {
	switch name {
	case "":
		return nil
	case "oracle-19c":
		var out []config.LimitEntry
		for _, user := range []string{"grid", "oracle"} {
			out = append(out,
				config.LimitEntry{User: user, Type: "soft", Item: "nofile", Value: "1024"},
				config.LimitEntry{User: user, Type: "hard", Item: "nofile", Value: "65536"},
				config.LimitEntry{User: user, Type: "soft", Item: "nproc", Value: "16384"},
				config.LimitEntry{User: user, Type: "hard", Item: "nproc", Value: "16384"},
				config.LimitEntry{User: user, Type: "soft", Item: "stack", Value: "10240"},
				config.LimitEntry{User: user, Type: "hard", Item: "stack", Value: "32768"},
				config.LimitEntry{User: user, Type: "soft", Item: "memlock", Value: "134217728"},
				config.LimitEntry{User: user, Type: "hard", Item: "memlock", Value: "134217728"},
			)
		}
		return out
	case "pg-16", "hardened-cis":
		log.Printf("limits: preset %q not yet populated; Phase 4b", name)
		return nil
	default:
		log.Printf("limits: unknown preset %q", name)
		return nil
	}
}

// limitKey uniquely identifies a limit row for dedup / merge.
func limitKey(l config.LimitEntry) string { return l.User + "|" + l.Type + "|" + l.Item }

// mergeLimits merges explicit entries with a preset's entries. Explicit wins
// on key conflicts. Output is sorted for deterministic file content.
func mergeLimits(explicit []config.LimitEntry, preset []config.LimitEntry) []config.LimitEntry {
	byKey := map[string]config.LimitEntry{}
	for _, p := range preset {
		byKey[limitKey(p)] = p
	}
	for _, e := range explicit {
		byKey[limitKey(e)] = e
	}
	out := make([]config.LimitEntry, 0, len(byKey))
	for _, v := range byKey {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].User != out[j].User {
			return out[i].User < out[j].User
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Item < out[j].Item
	})
	return out
}

// renderLimits produces the file body for LimitsManagedPath.
func renderLimits(entries []config.LimitEntry) string {
	var b strings.Builder
	b.WriteString("# Managed by linuxctl. Do not edit by hand.\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "%s %s %s %s\n", e.User, e.Type, e.Item, e.Value)
	}
	return b.String()
}

// ---- Plan -----------------------------------------------------------------

// Plan implements Manager.
func (m *LimitsManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	lin, err := castLinuxForLimits(desired)
	if err != nil {
		return nil, err
	}
	if m.sess == nil {
		return nil, ErrSessionRequired
	}
	desiredEntries := mergeLimits(lin.Limits, presetLimits(lin.LimitsPreset))
	if len(desiredEntries) == 0 {
		return nil, nil
	}

	var before string
	exists, err := m.sess.FileExists(ctx, LimitsManagedPath)
	if err != nil {
		return nil, fmt.Errorf("limits.Plan: stat %s: %w", LimitsManagedPath, err)
	}
	if exists {
		raw, err := m.sess.ReadFile(ctx, LimitsManagedPath)
		if err != nil {
			return nil, fmt.Errorf("limits.Plan: read %s: %w", LimitsManagedPath, err)
		}
		before = string(raw)
	}
	want := renderLimits(desiredEntries)
	if before == want {
		return nil, nil
	}
	return []Change{{
		ID:      "limits:" + LimitsManagedPath,
		Manager: "limits",
		Target:  LimitsManagedPath,
		Action:  "update",
		Before:  limitsSnap{Body: before},
		After:   limitsApply{Body: want, Entries: desiredEntries},
		Hazard:  HazardNone,
	}}, nil
}

type limitsSnap struct{ Body string }
type limitsApply struct {
	Body    string
	Entries []config.LimitEntry
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *LimitsManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, ErrSessionRequired
	}
	for _, ch := range changes {
		a, ok := ch.After.(limitsApply)
		if !ok {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("limits.Apply: After is not limitsApply (%T)", ch.After)})
			continue
		}
		if err := m.sess.WriteFile(ctx, LimitsManagedPath, []byte(a.Body), 0o644); err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("write %s: %w", LimitsManagedPath, err)})
			continue
		}
		res.Applied = append(res.Applied, ch)
	}
	return res, nil
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *LimitsManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback implements Manager. Restores the previous file body captured in
// Before, or removes the drop-in if no previous content existed.
func (m *LimitsManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return ErrSessionRequired
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		b, ok := ch.Before.(limitsSnap)
		if !ok {
			continue
		}
		if b.Body == "" {
			_, _ = RunSudoAndCheck(ctx, m.sess, "rm -f "+shellQuoteOne(LimitsManagedPath))
			continue
		}
		if err := m.sess.WriteFile(ctx, LimitsManagedPath, []byte(b.Body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// castLinuxForLimits accepts *config.Linux, config.Linux, or a bare slice.
func castLinuxForLimits(desired Spec) (*config.Linux, error) {
	switch v := desired.(type) {
	case *config.Linux:
		if v == nil {
			return &config.Linux{}, nil
		}
		return v, nil
	case config.Linux:
		lin := v
		return &lin, nil
	case []config.LimitEntry:
		return &config.Linux{Limits: v}, nil
	case nil:
		return &config.Linux{}, nil
	default:
		return nil, fmt.Errorf("limits: unsupported desired-state type %T", desired)
	}
}
