package managers

import (
	"context"
	"fmt"
	"strings"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/session"
)

// Managed-block markers in /etc/hosts. Lines outside the sandwich are never
// touched.
const (
	hostsBeginMarker = "# BEGIN linuxctl"
	hostsEndMarker   = "# END linuxctl"
	hostsPath        = "/etc/hosts"
)

// HostsManager reconciles a config.HostEntry list with the managed block in
// /etc/hosts.
//
// Safety: the manager only rewrites content between the BEGIN/END markers.
// Operator-managed entries outside the sandwich are preserved verbatim. A
// warning is surfaced (via Change.Hazard=warn) when a desired hostname would
// collide with a non-managed entry, but the apply still proceeds — the managed
// block is authoritative because the resolver reads /etc/hosts top-to-bottom.
type HostsManager struct {
	sess session.Session
}

// NewHostsManager returns a hosts manager without a session.
func NewHostsManager() *HostsManager { return &HostsManager{} }

// WithSession returns a copy bound to sess.
func (m *HostsManager) WithSession(sess session.Session) *HostsManager {
	cp := *m
	cp.sess = sess
	return &cp
}

// Name implements Manager.
func (*HostsManager) Name() string { return "hosts" }

// DependsOn implements Manager.
func (*HostsManager) DependsOn() []string { return nil }

func init() { Register(NewHostsManager()) }

// ---- Plan -----------------------------------------------------------------

// Plan implements Manager.
func (m *HostsManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	entries, err := castHostEntries(desired)
	if err != nil {
		return nil, err
	}
	if m.sess == nil {
		return nil, ErrSessionRequired
	}

	body, err := readHostsFile(ctx, m.sess)
	if err != nil {
		return nil, fmt.Errorf("hosts.Plan: read %s: %w", hostsPath, err)
	}
	currentBlock, _, _ := extractBlock(body)
	outsideLines := outsideBlock(body)

	desiredBlock := renderBlock(entries)

	var changes []Change
	if currentBlock == desiredBlock {
		return nil, nil
	}

	// Warn if any desired name already resolves outside the managed block.
	hazard := HazardNone
	for _, e := range entries {
		for _, name := range e.Names {
			if conflict := findConflict(outsideLines, name); conflict != "" && conflict != e.IP {
				hazard = HazardWarn
				break
			}
		}
		if hazard == HazardWarn {
			break
		}
	}

	changes = append(changes, Change{
		ID:      "hosts:" + hostsPath,
		Manager: "hosts",
		Target:  hostsPath,
		Action:  "update",
		Before:  currentBlock,
		After:   desiredBlock,
		Hazard:  hazard,
	})
	return changes, nil
}

// findConflict returns the IP of any non-managed line whose host list contains
// name, or empty string if none.
func findConflict(outsideLines []string, name string) string {
	for _, ln := range outsideLines {
		trim := strings.TrimSpace(ln)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		fields := strings.Fields(trim)
		if len(fields) < 2 {
			continue
		}
		for _, n := range fields[1:] {
			if n == name {
				return fields[0]
			}
		}
	}
	return ""
}

// renderBlock produces the multi-line managed block body (without markers).
// Each entry is rendered `<ip>  <name1> <name2> ...`.
func renderBlock(entries []config.HostEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	for _, e := range entries {
		if e.IP == "" || len(e.Names) == 0 {
			continue
		}
		b.WriteString(e.IP)
		b.WriteString("  ")
		b.WriteString(strings.Join(e.Names, " "))
		b.WriteString("\n")
	}
	return b.String()
}

// extractBlock finds the managed block and returns its body (lines between
// markers, without the markers), the begin index, and the end index into the
// lines slice. If no block is found, body is "" and indices are -1.
func extractBlock(body string) (string, int, int) {
	lines := strings.Split(body, "\n")
	begin, end := -1, -1
	for i, ln := range lines {
		trim := strings.TrimSpace(ln)
		switch trim {
		case hostsBeginMarker:
			begin = i
		case hostsEndMarker:
			if begin >= 0 && end == -1 {
				end = i
			}
		}
	}
	if begin == -1 || end == -1 || end <= begin {
		return "", -1, -1
	}
	inner := lines[begin+1 : end]
	return strings.Join(inner, "\n") + func() string {
		// Append final newline only if inner non-empty, to match renderBlock.
		if len(inner) == 0 {
			return ""
		}
		return "\n"
	}(), begin, end
}

// outsideBlock returns the lines of body that are NOT inside the managed block.
func outsideBlock(body string) []string {
	lines := strings.Split(body, "\n")
	_, begin, end := extractBlock(body)
	if begin == -1 {
		return lines
	}
	var out []string
	out = append(out, lines[:begin]...)
	if end+1 < len(lines) {
		out = append(out, lines[end+1:]...)
	}
	return out
}

// mergeHosts composes the full /etc/hosts content given outside lines and a
// desired managed block body.
func mergeHosts(outsideLines []string, block string) string {
	// Trim trailing empty lines from the outside prefix/suffix to avoid
	// accidental drift from excess blank lines.
	var b strings.Builder
	for _, ln := range outsideLines {
		b.WriteString(ln)
		b.WriteString("\n")
	}
	// If outside content didn't end with newline, we've added one; strip one if
	// the original was empty-terminated.
	s := b.String()
	// Ensure exactly one newline separator before our block when outside is non-empty.
	s = strings.TrimRight(s, "\n")
	if s != "" {
		s += "\n"
	}
	s += hostsBeginMarker + "\n"
	if block != "" {
		s += block
	}
	s += hostsEndMarker + "\n"
	return s
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *HostsManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, ErrSessionRequired
	}
	for _, ch := range changes {
		if ch.Action != "update" {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("hosts.Apply: unknown action %q", ch.Action)})
			continue
		}
		desiredBlock, ok := ch.After.(string)
		if !ok {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("hosts.Apply: After is not string (%T)", ch.After)})
			continue
		}
		body, err := readHostsFile(ctx, m.sess)
		if err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: err})
			continue
		}
		outside := outsideBlock(body)
		newContent := mergeHosts(outside, desiredBlock)
		if err := m.sess.WriteFile(ctx, hostsPath, []byte(newContent), 0o644); err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: err})
			continue
		}
		res.Applied = append(res.Applied, ch)
	}
	return res, nil
}

// readHostsFile reads /etc/hosts via session.ReadFile.
func readHostsFile(ctx context.Context, sess session.Session) (string, error) {
	b, err := sess.ReadFile(ctx, hostsPath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *HostsManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback implements Manager. Restores the Before block content for each
// recorded update.
func (m *HostsManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return ErrSessionRequired
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		if ch.Action != "update" {
			continue
		}
		before, ok := ch.Before.(string)
		if !ok {
			continue
		}
		body, err := readHostsFile(ctx, m.sess)
		if err != nil {
			return err
		}
		outside := outsideBlock(body)
		if err := m.sess.WriteFile(ctx, hostsPath, []byte(mergeHosts(outside, before)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ---- Spec casting ---------------------------------------------------------

func castHostEntries(desired Spec) ([]config.HostEntry, error) {
	switch v := desired.(type) {
	case []config.HostEntry:
		return v, nil
	case *config.Linux:
		if v == nil {
			return nil, nil
		}
		return v.HostsEntries, nil
	case config.Linux:
		return v.HostsEntries, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("hosts: unsupported desired-state type %T", desired)
	}
}
