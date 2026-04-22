package managers

import (
	"context"
	"fmt"
	"strings"

	"github.com/itunified-io/linuxctl/pkg/session"
)

// NetworkBackend identifies the live network stack.
type NetworkBackend string

const (
	NetworkBackendNetworkManager NetworkBackend = "NetworkManager"
	NetworkBackendSystemd        NetworkBackend = "systemd-networkd"
	NetworkBackendUnknown        NetworkBackend = "unknown"
)

// NetworkSpec is the Phase 4 minimal network desired state.
//
// For Phase 4 we only reconcile hostname + /etc/resolv.conf. Full NIC management
// (nmcli connection add/modify, VIP, SCAN interfaces for RAC) will land in
// Phase 4b / Phase 5.
//
// TODO(Phase 4b): full nmcli/networkd NIC management — bond, vlan, team,
// static IPv4/IPv6, routes, SCAN VIPs for Oracle RAC.
type NetworkSpec struct {
	Hostname   string   // desired hostname (ex. metadata.name)
	DNSServers []string // /etc/resolv.conf nameserver entries
	Search     []string // /etc/resolv.conf search domains
}

// NetworkManager reconciles basic network settings (hostname + DNS). It is
// deliberately narrow for Phase 4 — see TODO above.
type NetworkManager struct {
	sess    session.Session
	backend NetworkBackend // override for tests
}

// NewNetworkManager returns a network manager without a session.
func NewNetworkManager() *NetworkManager { return &NetworkManager{} }

// WithSession returns a copy bound to sess.
func (m *NetworkManager) WithSession(sess session.Session) *NetworkManager {
	cp := *m
	cp.sess = sess
	return &cp
}

// WithBackend forces a specific backend (test-only).
func (m *NetworkManager) WithBackend(b NetworkBackend) *NetworkManager {
	cp := *m
	cp.backend = b
	return &cp
}

// Name implements Manager.
func (*NetworkManager) Name() string { return "network" }

// DependsOn implements Manager.
func (*NetworkManager) DependsOn() []string { return []string{"hosts"} }

func init() { Register(NewNetworkManager()) }

// resolvConfPath is the canonical resolver file. Kept as a var so tests can
// override if they ever run against a chroot, but unused today.
const resolvConfPath = "/etc/resolv.conf"

// ---- Plan -----------------------------------------------------------------

// Plan implements Manager. Emits an "update" change for hostname drift and a
// second for /etc/resolv.conf drift.
func (m *NetworkManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	ns, err := castNetworkSpec(desired)
	if err != nil {
		return nil, err
	}
	if ns == nil {
		return nil, nil
	}
	if m.sess == nil {
		return nil, ErrSessionRequired
	}

	var changes []Change

	// Hostname drift.
	if ns.Hostname != "" {
		cur, err := m.currentHostname(ctx)
		if err != nil {
			return nil, fmt.Errorf("network.Plan: hostname: %w", err)
		}
		if cur != ns.Hostname {
			changes = append(changes, Change{
				ID:      "network:hostname",
				Manager: "network",
				Target:  "hostname",
				Action:  "update",
				Before:  cur,
				After:   ns.Hostname,
				Hazard:  HazardWarn,
			})
		}
	}

	// DNS / resolv.conf drift.
	if len(ns.DNSServers) > 0 || len(ns.Search) > 0 {
		curServers, curSearch, err := m.currentResolvConf(ctx)
		if err != nil {
			return nil, fmt.Errorf("network.Plan: resolv.conf: %w", err)
		}
		if !sameStringSet(curServers, ns.DNSServers) || !sameStringSet(curSearch, ns.Search) {
			changes = append(changes, Change{
				ID:      "network:resolv.conf",
				Manager: "network",
				Target:  resolvConfPath,
				Action:  "update",
				Before:  map[string]any{"servers": curServers, "search": curSearch},
				After:   map[string]any{"servers": ns.DNSServers, "search": ns.Search},
				Hazard:  HazardWarn,
			})
		}
	}

	return changes, nil
}

// currentHostname returns the active hostname via `hostname` (or `hostnamectl
// --static` as a fallback).
func (m *NetworkManager) currentHostname(ctx context.Context) (string, error) {
	out, _, err := m.sess.Run(ctx, "hostname")
	if err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out), nil
	}
	out, _, err = m.sess.Run(ctx, "hostnamectl --static")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// currentResolvConf parses /etc/resolv.conf and returns the nameserver +
// search lists. Unparseable files are treated as empty.
func (m *NetworkManager) currentResolvConf(ctx context.Context) ([]string, []string, error) {
	b, err := m.sess.ReadFile(ctx, resolvConfPath)
	if err != nil {
		// Missing file is not fatal — treat as empty.
		return nil, nil, nil
	}
	var servers, search []string
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "nameserver":
			servers = append(servers, fields[1])
		case "search":
			search = append(search, fields[1:]...)
		}
	}
	return servers, search, nil
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *NetworkManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, ErrSessionRequired
	}
	for _, ch := range changes {
		if err := m.applyOne(ctx, ch); err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: err})
			continue
		}
		res.Applied = append(res.Applied, ch)
	}
	return res, nil
}

func (m *NetworkManager) applyOne(ctx context.Context, ch Change) error {
	switch ch.Target {
	case "hostname":
		name, ok := ch.After.(string)
		if !ok {
			return fmt.Errorf("network.applyOne: hostname After is not string (%T)", ch.After)
		}
		_, err := RunSudoAndCheck(ctx, m.sess, "hostnamectl set-hostname "+shellQuoteOne(name))
		return err
	case resolvConfPath:
		after, ok := ch.After.(map[string]any)
		if !ok {
			return fmt.Errorf("network.applyOne: resolv.conf After has unexpected type %T", ch.After)
		}
		servers, _ := after["servers"].([]string)
		search, _ := after["search"].([]string)
		body := renderResolvConf(servers, search)
		return m.sess.WriteFile(ctx, resolvConfPath, []byte(body), 0o644)
	}
	return fmt.Errorf("network.applyOne: unknown target %q", ch.Target)
}

// renderResolvConf builds a resolv.conf body with a managed-file warning.
func renderResolvConf(servers, search []string) string {
	var b strings.Builder
	b.WriteString("# Managed by linuxctl — edits may be overwritten.\n")
	b.WriteString("# Note: if NetworkManager/systemd-resolved owns this file,\n")
	b.WriteString("# changes here may be reverted. Configure via nmcli / resolved instead.\n")
	if len(search) > 0 {
		b.WriteString("search ")
		b.WriteString(strings.Join(search, " "))
		b.WriteString("\n")
	}
	for _, s := range servers {
		if s == "" {
			continue
		}
		b.WriteString("nameserver ")
		b.WriteString(s)
		b.WriteString("\n")
	}
	return b.String()
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *NetworkManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback implements Manager. Restores the Before hostname / resolv.conf.
func (m *NetworkManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return ErrSessionRequired
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		reverse := Change{Action: ch.Action, Target: ch.Target, After: ch.Before}
		if reverse.After == nil {
			continue
		}
		_ = m.applyOne(ctx, reverse)
	}
	return nil
}

// ---- Spec casting ---------------------------------------------------------

func castNetworkSpec(desired Spec) (*NetworkSpec, error) {
	switch v := desired.(type) {
	case *NetworkSpec:
		return v, nil
	case NetworkSpec:
		return &v, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("network: unsupported desired-state type %T", desired)
	}
}
