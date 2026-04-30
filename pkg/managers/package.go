package managers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/presets"
)

// PackagesSpec is the desired state for the package manager. Install and
// Remove are opaque package names (distro-specific). Services are owned by
// ServiceManager and intentionally not represented here.
type PackagesSpec struct {
	Install []string `yaml:"install,omitempty"`
	Remove  []string `yaml:"remove,omitempty"`
}

// distroFamily groups distros by their native package manager.
type distroFamily string

const (
	familyRPM    distroFamily = "rpm"    // OL, RHEL, Rocky, CentOS, Fedora
	familyDEB    distroFamily = "deb"    // Ubuntu, Debian
	familyZYPPER distroFamily = "zypper" // SLES, openSUSE
	familyUnknown distroFamily = "unknown"
)

func init() { Register(NewPackageManager()) }

// PackageManager is distro-aware and batches installs/removes for speed.
type PackageManager struct {
	sess SessionRunner

	// cached distro detection (per-instance, one per host).
	family distroFamily
	tool   string // "dnf", "yum", "apt-get", "zypper"
}

// NewPackageManager returns a package manager with no session attached.
func NewPackageManager() *PackageManager { return &PackageManager{} }

// WithSession attaches a SessionRunner and returns the receiver for chaining.
func (m *PackageManager) WithSession(s SessionRunner) *PackageManager {
	m.sess = s
	return m
}

// Name implements Manager.
func (*PackageManager) Name() string { return "package" }

// DependsOn implements Manager.
func (*PackageManager) DependsOn() []string { return nil }

// ---- Plan -----------------------------------------------------------------

// Plan implements Manager.
func (m *PackageManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	ps, err := castPackages(desired)
	if err != nil {
		return nil, err
	}
	if len(ps.Install) == 0 && len(ps.Remove) == 0 {
		return nil, nil
	}
	if m.sess == nil {
		return nil, fmt.Errorf("package.Plan: no session attached")
	}
	if err := m.detectDistro(ctx); err != nil {
		return nil, err
	}
	var changes []Change
	for _, pkg := range ps.Install {
		installed, err := m.isInstalled(ctx, pkg)
		if err != nil {
			return nil, err
		}
		if installed {
			continue
		}
		changes = append(changes, Change{
			ID:      "pkg-install:" + pkg,
			Manager: "package",
			Target:  "pkg/" + pkg,
			Action:  "create",
			After:   pkg,
			Hazard:  HazardNone,
		})
	}
	for _, pkg := range ps.Remove {
		installed, err := m.isInstalled(ctx, pkg)
		if err != nil {
			return nil, err
		}
		if !installed {
			continue
		}
		changes = append(changes, Change{
			ID:      "pkg-remove:" + pkg,
			Manager: "package",
			Target:  "pkg/" + pkg,
			Action:  "delete",
			Before:  pkg,
			Hazard:  HazardDestructive,
		})
	}
	return changes, nil
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager. Batches all install/remove ops into one command
// per direction for efficiency; per-package Change entries are still kept in
// ApplyResult so callers get fine-grained visibility.
func (m *PackageManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if len(changes) == 0 {
		return res, nil
	}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, fmt.Errorf("package.Apply: no session attached")
	}
	if err := m.detectDistro(ctx); err != nil {
		return res, err
	}

	var installs, removes []Change
	for _, ch := range changes {
		switch ch.Action {
		case "create":
			installs = append(installs, ch)
		case "delete":
			removes = append(removes, ch)
		default:
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("unknown action %q", ch.Action)})
		}
	}

	if len(installs) > 0 {
		names := make([]string, 0, len(installs))
		for _, c := range installs {
			if n, ok := c.After.(string); ok {
				names = append(names, n)
			}
		}
		if err := m.runPackageOp(ctx, "install", names); err != nil {
			for _, c := range installs {
				res.Failed = append(res.Failed, ChangeErr{Change: c, Err: err})
			}
		} else {
			res.Applied = append(res.Applied, installs...)
		}
	}
	if len(removes) > 0 {
		names := make([]string, 0, len(removes))
		for _, c := range removes {
			if n, ok := c.Before.(string); ok {
				names = append(names, n)
			}
		}
		if err := m.runPackageOp(ctx, "remove", names); err != nil {
			for _, c := range removes {
				res.Failed = append(res.Failed, ChangeErr{Change: c, Err: err})
			}
		} else {
			res.Applied = append(res.Applied, removes...)
		}
	}
	return res, nil
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *PackageManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback implements Manager. Reverses create→remove and delete→install.
// Best-effort: a removed package may no longer exist in the repo.
func (m *PackageManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return fmt.Errorf("package.Rollback: no session attached")
	}
	if err := m.detectDistro(ctx); err != nil {
		return err
	}
	var toRemove, toInstall []string
	for _, ch := range changes {
		switch ch.Action {
		case "create":
			if n, ok := ch.After.(string); ok {
				toRemove = append(toRemove, n)
			}
		case "delete":
			if n, ok := ch.Before.(string); ok {
				toInstall = append(toInstall, n)
			}
		}
	}
	if len(toRemove) > 0 {
		if err := m.runPackageOp(ctx, "remove", toRemove); err != nil {
			return err
		}
	}
	if len(toInstall) > 0 {
		if err := m.runPackageOp(ctx, "install", toInstall); err != nil {
			return err
		}
	}
	return nil
}

// ---- Distro detection + ops ----------------------------------------------

func (m *PackageManager) detectDistro(ctx context.Context) error {
	if m.family != "" {
		return nil
	}
	out, _, err := m.sess.Run(ctx, "cat /etc/os-release 2>/dev/null || true")
	if err != nil {
		return fmt.Errorf("package.detectDistro: %w", err)
	}
	kv := parseOSRelease(out)
	id := strings.ToLower(kv["ID"])
	idLike := strings.ToLower(kv["ID_LIKE"])
	merged := id + " " + idLike

	switch {
	case containsAny(merged, "rhel", "fedora", "centos", "rocky", "ol", "oracle", "almalinux"):
		m.family = familyRPM
		// Prefer dnf if available, fall back to yum.
		if _, _, err := m.sess.Run(ctx, "command -v dnf >/dev/null 2>&1"); err == nil {
			m.tool = "dnf"
		} else {
			m.tool = "yum"
		}
	case containsAny(merged, "ubuntu", "debian"):
		m.family = familyDEB
		m.tool = "apt-get"
	case containsAny(merged, "sles", "suse", "opensuse"):
		m.family = familyZYPPER
		m.tool = "zypper"
	default:
		m.family = familyUnknown
		return fmt.Errorf("package: unsupported distro (ID=%q ID_LIKE=%q)", id, idLike)
	}
	return nil
}

func (m *PackageManager) isInstalled(ctx context.Context, pkg string) (bool, error) {
	var cmd string
	switch m.family {
	case familyRPM:
		cmd = "rpm -q " + shellQuote(pkg) + " >/dev/null 2>&1"
	case familyDEB:
		// dpkg-query is more reliable than `dpkg -s` for scripting.
		cmd = "dpkg-query -W -f='${Status}' " + shellQuote(pkg) + " 2>/dev/null | grep -q 'install ok installed'"
	case familyZYPPER:
		cmd = "rpm -q " + shellQuote(pkg) + " >/dev/null 2>&1"
	default:
		return false, fmt.Errorf("package.isInstalled: unknown family")
	}
	_, _, err := m.sess.Run(ctx, cmd)
	return err == nil, nil
}

// runPackageOp batches install or remove across all names. Retries once on
// transient lock contention (dnf/yum/apt all emit a "locked"-ish message).
func (m *PackageManager) runPackageOp(ctx context.Context, op string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = shellQuote(n)
	}
	joined := strings.Join(quoted, " ")

	var cmd string
	switch m.family {
	case familyRPM:
		cmd = fmt.Sprintf("%s %s -y %s", m.tool, op, joined)
	case familyDEB:
		prefix := "DEBIAN_FRONTEND=noninteractive apt-get"
		verb := op
		if op == "remove" {
			verb = "purge"
		}
		// apt-get update is a no-op if sources haven't changed; skip it here
		// to keep Apply idempotent. Caller can run an explicit refresh before
		// invoking us.
		cmd = fmt.Sprintf("%s -y %s %s", prefix, verb, joined)
	case familyZYPPER:
		verb := op
		if op == "install" {
			verb = "install --no-confirm"
		} else {
			verb = "remove --no-confirm"
		}
		cmd = fmt.Sprintf("zypper --non-interactive %s %s", verb, joined)
	default:
		return fmt.Errorf("package.runPackageOp: unknown family")
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, stderr, err := m.sess.Run(ctx, cmd)
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr))
		if !isLockError(stderr) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	return fmt.Errorf("package.runPackageOp: %d attempts: %w", maxAttempts, lastErr)
}

// isLockError detects common "package manager is locked" messages across
// dnf/yum/apt/zypper. Kept forgiving — false positives just trigger a retry.
func isLockError(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "locked") ||
		strings.Contains(s, "could not get lock") ||
		strings.Contains(s, "another app is currently holding")
}

// parseOSRelease turns /etc/os-release into a map, stripping surrounding quotes.
func parseOSRelease(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		k := line[:eq]
		v := strings.Trim(line[eq+1:], `"'`)
		out[k] = v
	}
	return out
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// castPackages normalises a desired Spec into a PackagesSpec. When a
// *config.Linux is supplied and PackagesPreset is set, the preset is merged
// with explicit package lists (union of Install + Remove, explicit wins on
// install/remove conflict).
func castPackages(desired Spec) (PackagesSpec, error) {
	switch v := desired.(type) {
	case PackagesSpec:
		return v, nil
	case *PackagesSpec:
		if v == nil {
			return PackagesSpec{}, nil
		}
		return *v, nil
	case *config.Linux:
		if v == nil {
			return PackagesSpec{}, nil
		}
		return packagesFromLinux(v), nil
	case config.Linux:
		return packagesFromLinux(&v), nil
	case *config.Packages:
		// Raw config.Packages (no bundle preset context). Convert
		// install/remove lists directly. This is the fallback path when
		// a caller passes the typed config struct without the enclosing
		// Linux — bundle presets cannot be expanded here.
		if v == nil {
			return PackagesSpec{}, nil
		}
		return PackagesSpec{Install: v.Install, Remove: v.Remove}, nil
	case config.Packages:
		return PackagesSpec{Install: v.Install, Remove: v.Remove}, nil
	case nil:
		return PackagesSpec{}, nil
	default:
		return PackagesSpec{}, fmt.Errorf("package: unsupported desired-state type %T", desired)
	}
}

func packagesFromLinux(l *config.Linux) PackagesSpec {
	explicit := config.Packages{}
	if l.Packages != nil {
		explicit = *l.Packages
	}
	merged := explicit
	if l.PackagesPreset != "" {
		if p, err := presets.ResolveCategory("packages", l.PackagesPreset, nil); err == nil {
			if pp, err := presets.PackagesSpec(p); err == nil && pp != nil {
				merged = presets.MergePackages(explicit, *pp)
			} else if err != nil {
				log.Printf("package: preset %q decode: %v", l.PackagesPreset, err)
			}
		} else {
			log.Printf("package: preset %q: %v", l.PackagesPreset, err)
		}
	}
	return PackagesSpec{Install: merged.Install, Remove: merged.Remove}
}
