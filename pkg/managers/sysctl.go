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

// SysctlManagedPath is the single file sysctl manager owns. Every Apply
// rewrites it atomically; the rest of /etc/sysctl.d/ is left alone.
const SysctlManagedPath = "/etc/sysctl.d/99-linuxctl.conf"

// SysctlManager reconciles kernel parameters using a single managed drop-in.
type SysctlManager struct {
	sess session.Session
}

// NewSysctlManager returns a sysctl manager without a session.
func NewSysctlManager() *SysctlManager { return &SysctlManager{} }

// WithSession returns a copy bound to sess.
func (m *SysctlManager) WithSession(sess session.Session) *SysctlManager {
	cp := *m
	cp.sess = sess
	return &cp
}

// Name implements Manager.
func (*SysctlManager) Name() string { return "sysctl" }

// DependsOn implements Manager.
func (*SysctlManager) DependsOn() []string { return nil }

func init() { Register(NewSysctlManager()) }

// ---- Preset expansion ------------------------------------------------------

// presetSysctl returns the hardcoded sysctl entries for a named preset. Returns
// nil + false when the preset is unknown. The Oracle 19c preset ships with
// published Oracle RDBMS pre-install kernel params.
func presetSysctl(name string) []config.SysctlEntry {
	switch name {
	case "":
		return nil
	case "oracle-19c":
		return []config.SysctlEntry{
			{Key: "fs.aio-max-nr", Value: "1048576"},
			{Key: "fs.file-max", Value: "6815744"},
			{Key: "kernel.panic_on_oops", Value: "1"},
			{Key: "kernel.sem", Value: "250 32000 100 128"},
			{Key: "kernel.shmall", Value: "1073741824"},
			{Key: "kernel.shmmax", Value: "4398046511104"},
			{Key: "kernel.shmmni", Value: "4096"},
			{Key: "net.core.rmem_max", Value: "4194304"},
			{Key: "net.core.wmem_max", Value: "1048576"},
			{Key: "vm.swappiness", Value: "10"},
		}
	case "pg-16", "hardened-cis":
		log.Printf("sysctl: preset %q not yet populated; Phase 4b", name)
		return nil
	default:
		log.Printf("sysctl: unknown preset %q", name)
		return nil
	}
}

// mergeSysctl merges explicit entries with a preset's entries. Explicit wins
// on key conflicts. Output is sorted by key for deterministic file content.
func mergeSysctl(explicit []config.SysctlEntry, preset []config.SysctlEntry) []config.SysctlEntry {
	byKey := map[string]string{}
	for _, p := range preset {
		byKey[p.Key] = p.Value
	}
	for _, e := range explicit {
		byKey[e.Key] = e.Value
	}
	out := make([]config.SysctlEntry, 0, len(byKey))
	for k, v := range byKey {
		out = append(out, config.SysctlEntry{Key: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// renderSysctl produces the file body for SysctlManagedPath.
func renderSysctl(entries []config.SysctlEntry) string {
	var b strings.Builder
	b.WriteString("# Managed by linuxctl. Do not edit by hand.\n")
	for _, e := range entries {
		b.WriteString(e.Key)
		b.WriteString(" = ")
		b.WriteString(e.Value)
		b.WriteString("\n")
	}
	return b.String()
}

// parseSysctlFile parses the managed file body into a map[key]=value. Blank
// lines and comments are ignored.
func parseSysctlFile(body string) map[string]string {
	out := map[string]string{}
	for _, ln := range strings.Split(body, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") || strings.HasPrefix(ln, ";") {
			continue
		}
		eq := strings.IndexByte(ln, '=')
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(ln[:eq])
		v := strings.TrimSpace(ln[eq+1:])
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// ---- Plan -----------------------------------------------------------------

// Plan implements Manager.
func (m *SysctlManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	lin, err := castLinuxForSysctl(desired)
	if err != nil {
		return nil, err
	}
	if m.sess == nil {
		return nil, ErrSessionRequired
	}
	desiredEntries := mergeSysctl(lin.Sysctl, presetSysctl(lin.SysctlPreset))
	if len(desiredEntries) == 0 {
		return nil, nil
	}

	// file drift
	var before string
	exists, err := m.sess.FileExists(ctx, SysctlManagedPath)
	if err != nil {
		return nil, fmt.Errorf("sysctl.Plan: stat %s: %w", SysctlManagedPath, err)
	}
	if exists {
		raw, err := m.sess.ReadFile(ctx, SysctlManagedPath)
		if err != nil {
			return nil, fmt.Errorf("sysctl.Plan: read %s: %w", SysctlManagedPath, err)
		}
		before = string(raw)
	}
	want := renderSysctl(desiredEntries)

	fileDrift := before != want

	// live drift: query kernel for each desired key
	var liveDrift []config.SysctlEntry
	for _, e := range desiredEntries {
		out, _, err := m.sess.Run(ctx, "sysctl -n "+shellQuoteOne(e.Key))
		if err != nil {
			// key unknown on this kernel — flag via liveDrift so Apply will try
			liveDrift = append(liveDrift, e)
			continue
		}
		live := strings.TrimSpace(strings.ReplaceAll(out, "\t", " "))
		want := strings.TrimSpace(strings.ReplaceAll(e.Value, "\t", " "))
		// normalise multi-space for tuples like kernel.sem
		live = strings.Join(strings.Fields(live), " ")
		want = strings.Join(strings.Fields(want), " ")
		if live != want {
			liveDrift = append(liveDrift, e)
		}
	}

	if !fileDrift && len(liveDrift) == 0 {
		return nil, nil
	}
	return []Change{{
		ID:      "sysctl:" + SysctlManagedPath,
		Manager: "sysctl",
		Target:  SysctlManagedPath,
		Action:  "update",
		Before:  sysctlSnap{Body: before},
		After:   sysctlApply{Body: want, Entries: desiredEntries, Live: liveDrift},
		Hazard:  HazardWarn,
	}}, nil
}

// sysctlSnap / sysctlApply are Before/After payloads for sysctl Changes.
type sysctlSnap struct {
	Body string
}
type sysctlApply struct {
	Body    string
	Entries []config.SysctlEntry
	Live    []config.SysctlEntry // keys out of sync with live kernel
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *SysctlManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, ErrSessionRequired
	}
	for _, ch := range changes {
		a, ok := ch.After.(sysctlApply)
		if !ok {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("sysctl.Apply: After is not sysctlApply (%T)", ch.After)})
			continue
		}
		if err := m.writeAndReload(ctx, a); err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: err})
			continue
		}
		res.Applied = append(res.Applied, ch)
	}
	return res, nil
}

func (m *SysctlManager) writeAndReload(ctx context.Context, a sysctlApply) error {
	if err := m.sess.WriteFile(ctx, SysctlManagedPath, []byte(a.Body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", SysctlManagedPath, err)
	}
	if _, err := RunSudoAndCheck(ctx, m.sess, "sysctl -p "+shellQuoteOne(SysctlManagedPath)); err != nil {
		return fmt.Errorf("sysctl -p: %w", err)
	}
	return nil
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *SysctlManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback implements Manager. Restores the previous file body captured in
// Before and re-runs `sysctl -p`.
func (m *SysctlManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return ErrSessionRequired
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		b, ok := ch.Before.(sysctlSnap)
		if !ok {
			continue
		}
		if b.Body == "" {
			// file did not exist before — remove our drop-in.
			_, _ = RunSudoAndCheck(ctx, m.sess, "rm -f "+shellQuoteOne(SysctlManagedPath))
			continue
		}
		if err := m.sess.WriteFile(ctx, SysctlManagedPath, []byte(b.Body), 0o644); err != nil {
			return err
		}
		_, _ = RunSudoAndCheck(ctx, m.sess, "sysctl -p "+shellQuoteOne(SysctlManagedPath))
	}
	return nil
}

// castLinuxForSysctl accepts *config.Linux, config.Linux, or a bare slice of
// entries. A bare slice skips preset expansion.
func castLinuxForSysctl(desired Spec) (*config.Linux, error) {
	switch v := desired.(type) {
	case *config.Linux:
		if v == nil {
			return &config.Linux{}, nil
		}
		return v, nil
	case config.Linux:
		lin := v
		return &lin, nil
	case []config.SysctlEntry:
		return &config.Linux{Sysctl: v}, nil
	case nil:
		return &config.Linux{}, nil
	default:
		return nil, fmt.Errorf("sysctl: unsupported desired-state type %T", desired)
	}
}
