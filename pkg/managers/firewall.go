package managers

import (
	"context"
	"fmt"
	"strings"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/session"
)

// FirewallBackend identifies which host firewall implementation is in use.
type FirewallBackend string

const (
	FirewallBackendFirewalld FirewallBackend = "firewalld"
	FirewallBackendUFW       FirewallBackend = "ufw"
	FirewallBackendNftables  FirewallBackend = "nftables"
	FirewallBackendUnknown   FirewallBackend = "unknown"
)

// FirewallManager reconciles a config.Firewall spec across firewalld (EL/Fedora),
// ufw (Debian/Ubuntu), and nftables (fallback). Distro detection is based on
// /etc/os-release. Operations are idempotent: Plan diffs desired vs live and
// emits minimal Changes; Apply applies them under sudo.
//
// Safety: FirewallManager never removes zones; it only adds/removes ports and
// sources within a zone, and toggles the master service. Rollback reverses
// port/source changes applied in-session.
type FirewallManager struct {
	sess    session.Session
	backend FirewallBackend // override for tests; zero value => detect
}

// NewFirewallManager returns a firewall manager without a session.
func NewFirewallManager() *FirewallManager { return &FirewallManager{} }

// WithSession returns a copy bound to sess.
func (m *FirewallManager) WithSession(sess session.Session) *FirewallManager {
	cp := *m
	cp.sess = sess
	return &cp
}

// WithBackend forces a specific backend (test-only).
func (m *FirewallManager) WithBackend(b FirewallBackend) *FirewallManager {
	cp := *m
	cp.backend = b
	return &cp
}

// Name implements Manager.
func (*FirewallManager) Name() string { return "firewall" }

// DependsOn implements Manager — firewall layers on top of networking + packages.
func (*FirewallManager) DependsOn() []string { return []string{"network", "package"} }

func init() { Register(NewFirewallManager()) }

// currentFirewall is the minimal snapshot we reconcile against.
type currentFirewall struct {
	Backend FirewallBackend
	Active  bool
	// zone -> observed ports like "22/tcp", "8080-8090/tcp"
	Ports map[string][]string
	// zone -> observed sources (CIDRs)
	Sources map[string][]string
}

// ---- Plan -----------------------------------------------------------------

// Plan implements Manager.
func (m *FirewallManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	fw, err := castFirewall(desired)
	if err != nil {
		return nil, err
	}
	if fw == nil {
		return nil, nil
	}
	if m.sess == nil {
		return nil, ErrSessionRequired
	}

	cur, err := m.snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("firewall.Plan: snapshot: %w", err)
	}

	var changes []Change

	if !fw.Enabled && cur.Active {
		changes = append(changes, Change{
			ID:      "firewall:service",
			Manager: "firewall",
			Target:  "service/" + string(cur.Backend),
			Action:  "update",
			Before:  map[string]any{"active": true},
			After:   map[string]any{"active": false, "op": "disable_firewall"},
			Hazard:  HazardWarn,
		})
		return changes, nil
	}
	if fw.Enabled && !cur.Active {
		changes = append(changes, Change{
			ID:      "firewall:service",
			Manager: "firewall",
			Target:  "service/" + string(cur.Backend),
			Action:  "update",
			Before:  map[string]any{"active": false},
			After:   map[string]any{"active": true, "op": "enable_firewall"},
			Hazard:  HazardWarn,
		})
	}

	if !fw.Enabled {
		return changes, nil
	}

	for zone, desiredZone := range fw.Zones {
		desiredPorts := portSet(desiredZone.Ports)
		curPorts := map[string]bool{}
		for _, p := range cur.Ports[zone] {
			curPorts[p] = true
		}
		for p := range desiredPorts {
			if !curPorts[p] {
				changes = append(changes, Change{
					ID:      "firewall:port:" + zone + ":" + p,
					Manager: "firewall",
					Target:  "zone/" + zone + "/port/" + p,
					Action:  "add_port",
					After:   map[string]string{"zone": zone, "port": p},
					Hazard:  HazardNone,
				})
			}
		}
		for p := range curPorts {
			if !desiredPorts[p] {
				changes = append(changes, Change{
					ID:      "firewall:port-del:" + zone + ":" + p,
					Manager: "firewall",
					Target:  "zone/" + zone + "/port/" + p,
					Action:  "remove_port",
					Before:  map[string]string{"zone": zone, "port": p},
					Hazard:  HazardWarn,
				})
			}
		}

		desiredSrc := map[string]bool{}
		for _, s := range desiredZone.Sources {
			desiredSrc[s] = true
		}
		curSrc := map[string]bool{}
		for _, s := range cur.Sources[zone] {
			curSrc[s] = true
		}
		for s := range desiredSrc {
			if !curSrc[s] {
				changes = append(changes, Change{
					ID:      "firewall:src:" + zone + ":" + s,
					Manager: "firewall",
					Target:  "zone/" + zone + "/source/" + s,
					Action:  "add_source",
					After:   map[string]string{"zone": zone, "source": s},
					Hazard:  HazardNone,
				})
			}
		}
		for s := range curSrc {
			if !desiredSrc[s] {
				changes = append(changes, Change{
					ID:      "firewall:src-del:" + zone + ":" + s,
					Manager: "firewall",
					Target:  "zone/" + zone + "/source/" + s,
					Action:  "remove_source",
					Before:  map[string]string{"zone": zone, "source": s},
					Hazard:  HazardWarn,
				})
			}
		}
	}

	return changes, nil
}

// portSet converts a []PortRule into a canonical "port/proto" set.
func portSet(rules []config.PortRule) map[string]bool {
	out := map[string]bool{}
	for _, r := range rules {
		proto := r.Proto
		if proto == "" {
			proto = "tcp"
		}
		switch {
		case r.Range != "":
			out[r.Range+"/"+proto] = true
		case r.Port != 0:
			out[fmt.Sprintf("%d/%s", r.Port, proto)] = true
		}
	}
	return out
}

// ---- snapshot -------------------------------------------------------------

func (m *FirewallManager) snapshot(ctx context.Context) (currentFirewall, error) {
	backend := m.backend
	if backend == "" {
		b, err := m.detectBackend(ctx)
		if err != nil {
			return currentFirewall{}, err
		}
		backend = b
	}
	cf := currentFirewall{
		Backend: backend,
		Ports:   map[string][]string{},
		Sources: map[string][]string{},
	}
	switch backend {
	case FirewallBackendFirewalld:
		stateOut, _, _ := m.sess.Run(ctx, "firewall-cmd --state")
		cf.Active = strings.TrimSpace(stateOut) == "running"
		if !cf.Active {
			return cf, nil
		}
		zonesOut, _, err := m.sess.Run(ctx, "firewall-cmd --list-all-zones")
		if err != nil {
			return cf, nil
		}
		parseFirewalldZones(zonesOut, &cf)
	case FirewallBackendUFW:
		out, _, _ := m.sess.RunSudo(ctx, "ufw status verbose")
		parseUFWStatus(out, &cf)
	case FirewallBackendNftables:
		out, _, _ := m.sess.RunSudo(ctx, "systemctl is-active nftables")
		cf.Active = strings.TrimSpace(out) == "active"
	}
	return cf, nil
}

// detectBackend reads /etc/os-release to pick a backend.
func (m *FirewallManager) detectBackend(ctx context.Context) (FirewallBackend, error) {
	out, _, err := m.sess.Run(ctx, "cat /etc/os-release 2>/dev/null || true")
	if err == nil {
		id, idLike := parseFirewallOSRelease(out)
		for _, s := range append([]string{id}, strings.Fields(idLike)...) {
			switch s {
			case "rhel", "ol", "rocky", "almalinux", "centos", "fedora":
				return FirewallBackendFirewalld, nil
			case "debian", "ubuntu":
				return FirewallBackendUFW, nil
			}
		}
	}
	if _, _, err := m.sess.Run(ctx, "command -v firewall-cmd >/dev/null 2>&1"); err == nil {
		return FirewallBackendFirewalld, nil
	}
	if _, _, err := m.sess.Run(ctx, "command -v ufw >/dev/null 2>&1"); err == nil {
		return FirewallBackendUFW, nil
	}
	return FirewallBackendNftables, nil
}

// parseFirewallOSRelease extracts ID= and ID_LIKE= values (lowercase, unquoted).
// Named to avoid collision with package.go's parseOSRelease.
func parseFirewallOSRelease(body string) (id, idLike string) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		v = strings.ToLower(v)
		switch k {
		case "ID":
			id = v
		case "ID_LIKE":
			idLike = v
		}
	}
	return id, idLike
}

// parseFirewalldZones parses `firewall-cmd --list-all-zones` output.
func parseFirewalldZones(body string, cf *currentFirewall) {
	var zone string
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			zone = strings.Fields(line)[0]
			continue
		}
		trim := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trim, "ports:"):
			val := strings.TrimSpace(strings.TrimPrefix(trim, "ports:"))
			if val == "" || zone == "" {
				continue
			}
			cf.Ports[zone] = append(cf.Ports[zone], strings.Fields(val)...)
		case strings.HasPrefix(trim, "sources:"):
			val := strings.TrimSpace(strings.TrimPrefix(trim, "sources:"))
			if val == "" || zone == "" {
				continue
			}
			cf.Sources[zone] = append(cf.Sources[zone], strings.Fields(val)...)
		}
	}
}

// parseUFWStatus parses `ufw status verbose`. ufw has no zone concept — we
// model it as a synthetic "default" zone.
func parseUFWStatus(body string, cf *currentFirewall) {
	lines := strings.Split(body, "\n")
	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, "Status:") {
			val := strings.TrimSpace(strings.TrimPrefix(trim, "Status:"))
			cf.Active = val == "active"
		}
	}
	if !cf.Active {
		return
	}
	for _, ln := range lines {
		fields := strings.Fields(ln)
		if len(fields) < 3 {
			continue
		}
		if !strings.EqualFold(fields[1], "ALLOW") {
			continue
		}
		target := fields[0]
		if strings.Contains(target, "/") && (strings.HasSuffix(target, "/tcp") || strings.HasSuffix(target, "/udp")) {
			cf.Ports["default"] = append(cf.Ports["default"], target)
		}
	}
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *FirewallManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, ErrSessionRequired
	}
	backend := m.backend
	if backend == "" {
		b, err := m.detectBackend(ctx)
		if err != nil {
			return res, fmt.Errorf("firewall.Apply: detect backend: %w", err)
		}
		backend = b
	}
	needsReload := false
	for _, ch := range changes {
		if err := m.applyOne(ctx, backend, ch); err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: err})
			continue
		}
		if backend == FirewallBackendFirewalld && ch.Action != "update" {
			needsReload = true
		}
		res.Applied = append(res.Applied, ch)
	}
	if needsReload {
		if _, err := RunSudoAndCheck(ctx, m.sess, "firewall-cmd --reload"); err != nil {
			return res, fmt.Errorf("firewall.Apply: reload: %w", err)
		}
	}
	return res, nil
}

func (m *FirewallManager) applyOne(ctx context.Context, backend FirewallBackend, ch Change) error {
	switch ch.Action {
	case "update":
		after, _ := ch.After.(map[string]any)
		op, _ := after["op"].(string)
		switch op {
		case "disable_firewall":
			return m.disable(ctx, backend)
		case "enable_firewall":
			return m.enable(ctx, backend)
		}
		return fmt.Errorf("firewall.Apply: unknown update op %q", op)
	case "add_port", "remove_port":
		m2, _ := chooseMap(ch)
		return m.portOp(ctx, backend, ch.Action, m2["zone"], m2["port"])
	case "add_source", "remove_source":
		m2, _ := chooseMap(ch)
		return m.sourceOp(ctx, backend, ch.Action, m2["zone"], m2["source"])
	}
	return fmt.Errorf("firewall.Apply: unknown action %q", ch.Action)
}

func chooseMap(ch Change) (map[string]string, bool) {
	if m, ok := ch.After.(map[string]string); ok {
		return m, true
	}
	if m, ok := ch.Before.(map[string]string); ok {
		return m, true
	}
	return map[string]string{}, false
}

func (m *FirewallManager) enable(ctx context.Context, b FirewallBackend) error {
	switch b {
	case FirewallBackendFirewalld:
		_, err := RunSudoAndCheck(ctx, m.sess, "systemctl enable --now firewalld")
		return err
	case FirewallBackendUFW:
		_, err := RunSudoAndCheck(ctx, m.sess, "ufw --force enable")
		return err
	case FirewallBackendNftables:
		_, err := RunSudoAndCheck(ctx, m.sess, "systemctl enable --now nftables")
		return err
	}
	return fmt.Errorf("firewall.enable: unsupported backend %q", b)
}

func (m *FirewallManager) disable(ctx context.Context, b FirewallBackend) error {
	switch b {
	case FirewallBackendFirewalld:
		_, err := RunSudoAndCheck(ctx, m.sess, "systemctl disable --now firewalld")
		return err
	case FirewallBackendUFW:
		_, err := RunSudoAndCheck(ctx, m.sess, "ufw --force disable")
		return err
	case FirewallBackendNftables:
		_, err := RunSudoAndCheck(ctx, m.sess, "systemctl disable --now nftables")
		return err
	}
	return fmt.Errorf("firewall.disable: unsupported backend %q", b)
}

func (m *FirewallManager) portOp(ctx context.Context, b FirewallBackend, action, zone, port string) error {
	spec, proto, ok := strings.Cut(port, "/")
	if !ok {
		return fmt.Errorf("firewall.portOp: malformed port %q", port)
	}
	switch b {
	case FirewallBackendFirewalld:
		flag := "--add-port"
		if action == "remove_port" {
			flag = "--remove-port"
		}
		cmd := fmt.Sprintf("firewall-cmd --permanent --zone=%s %s=%s/%s",
			shellQuoteOne(zone), flag, shellQuoteOne(spec), shellQuoteOne(proto))
		_, err := RunSudoAndCheck(ctx, m.sess, cmd)
		return err
	case FirewallBackendUFW:
		verb := "allow"
		if action == "remove_port" {
			verb = "delete allow"
		}
		cmd := fmt.Sprintf("ufw %s %s/%s", verb, shellQuoteOne(spec), shellQuoteOne(proto))
		_, err := RunSudoAndCheck(ctx, m.sess, cmd)
		return err
	}
	return fmt.Errorf("firewall.portOp: unsupported backend %q", b)
}

func (m *FirewallManager) sourceOp(ctx context.Context, b FirewallBackend, action, zone, src string) error {
	switch b {
	case FirewallBackendFirewalld:
		flag := "--add-source"
		if action == "remove_source" {
			flag = "--remove-source"
		}
		cmd := fmt.Sprintf("firewall-cmd --permanent --zone=%s %s=%s",
			shellQuoteOne(zone), flag, shellQuoteOne(src))
		_, err := RunSudoAndCheck(ctx, m.sess, cmd)
		return err
	case FirewallBackendUFW:
		verb := "allow"
		if action == "remove_source" {
			verb = "delete allow"
		}
		cmd := fmt.Sprintf("ufw %s from %s", verb, shellQuoteOne(src))
		_, err := RunSudoAndCheck(ctx, m.sess, cmd)
		return err
	}
	return fmt.Errorf("firewall.sourceOp: unsupported backend %q", b)
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *FirewallManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback implements Manager.
func (m *FirewallManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return ErrSessionRequired
	}
	backend := m.backend
	if backend == "" {
		b, err := m.detectBackend(ctx)
		if err != nil {
			return err
		}
		backend = b
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		var reverseAction string
		switch ch.Action {
		case "add_port":
			reverseAction = "remove_port"
		case "remove_port":
			reverseAction = "add_port"
		case "add_source":
			reverseAction = "remove_source"
		case "remove_source":
			reverseAction = "add_source"
		case "update":
			after, _ := ch.After.(map[string]any)
			op, _ := after["op"].(string)
			switch op {
			case "enable_firewall":
				_ = m.disable(ctx, backend)
			case "disable_firewall":
				_ = m.enable(ctx, backend)
			}
			continue
		default:
			continue
		}
		m2, _ := chooseMap(ch)
		if strings.HasSuffix(reverseAction, "_port") {
			_ = m.portOp(ctx, backend, reverseAction, m2["zone"], m2["port"])
		} else {
			_ = m.sourceOp(ctx, backend, reverseAction, m2["zone"], m2["source"])
		}
	}
	return nil
}

// ---- Spec casting ---------------------------------------------------------

func castFirewall(desired Spec) (*config.Firewall, error) {
	switch v := desired.(type) {
	case *config.Firewall:
		return v, nil
	case config.Firewall:
		return &v, nil
	case *config.Linux:
		if v == nil {
			return nil, nil
		}
		return v.Firewall, nil
	case config.Linux:
		return v.Firewall, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("firewall: unsupported desired-state type %T", desired)
	}
}
