package managers

import (
	"context"
	"fmt"
	"strings"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// sudoRunner is the narrow session contract needed by ServiceManager.
// pkg/session.Session satisfies it.
type sudoRunner interface {
	Run(ctx context.Context, cmd string) (stdout, stderr string, err error)
	RunSudo(ctx context.Context, cmd string) (stdout, stderr string, err error)
}

// ServiceManager reconciles desired systemd unit state (enabled/active) against
// the live system via `systemctl`.
//
// Safety: Plan never proposes masking or removing units. Rollback best-effort
// reverses the enable/active flip captured in Change.Before.
type ServiceManager struct {
	sess sudoRunner
}

// NewServiceManager returns a service manager without a session.
func NewServiceManager() *ServiceManager { return &ServiceManager{} }

// WithSession returns a copy bound to sess.
func (m *ServiceManager) WithSession(sess sudoRunner) *ServiceManager {
	cp := *m
	cp.sess = sess
	return &cp
}

// Name implements Manager.
func (*ServiceManager) Name() string { return "service" }

// DependsOn implements Manager. Services depend on packages being installed
// and on ssh hardening (which may restart sshd).
func (*ServiceManager) DependsOn() []string { return []string{"package", "ssh"} }

func init() { Register(NewServiceManager()) }

// serviceObs captures observed systemd state for a unit.
type serviceObs struct {
	Exists  bool
	Masked  bool
	Enabled bool
	Active  bool
}

// ---- Plan -----------------------------------------------------------------

// Plan implements Manager.
func (m *ServiceManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	svcs, err := castServices(desired)
	if err != nil {
		return nil, err
	}
	if m.sess == nil {
		return nil, ErrSessionRequired
	}
	var changes []Change
	for _, s := range svcs {
		obs, err := m.observe(ctx, s.Name)
		if err != nil {
			return nil, fmt.Errorf("service.Plan: observe %s: %w", s.Name, err)
		}
		if !obs.Exists {
			changes = append(changes, Change{
				ID:      "service:" + s.Name,
				Manager: "service",
				Target:  "service/" + s.Name,
				Action:  "error",
				After:   s,
				Hazard:  HazardWarn,
			})
			continue
		}
		// enable/disable drift
		if s.Enabled != obs.Enabled {
			op := "disable"
			if s.Enabled {
				op = "enable"
			}
			changes = append(changes, Change{
				ID:      "service:" + s.Name + ":enable",
				Manager: "service",
				Target:  "service/" + s.Name,
				Action:  "update",
				Before:  serviceEnableSnap{Name: s.Name, Enabled: obs.Enabled},
				After:   serviceEnableOp{Name: s.Name, Op: op, Masked: obs.Masked},
				Hazard:  HazardNone,
			})
		}
		// state drift — only when a desired state is set.
		if s.State != "" {
			wantActive := s.State == "running"
			if wantActive != obs.Active {
				op := "stop"
				if wantActive {
					op = "start"
				}
				changes = append(changes, Change{
					ID:      "service:" + s.Name + ":state",
					Manager: "service",
					Target:  "service/" + s.Name,
					Action:  "update",
					Before:  serviceStateSnap{Name: s.Name, Active: obs.Active},
					After:   serviceStateOp{Name: s.Name, Op: op, Masked: obs.Masked},
					Hazard:  HazardWarn,
				})
			}
		}
	}
	return changes, nil
}

// serviceEnableOp / serviceStateOp are the After payloads for service Changes.
type serviceEnableOp struct {
	Name   string
	Op     string // enable | disable
	Masked bool
}
type serviceStateOp struct {
	Name   string
	Op     string // start | stop | restart
	Masked bool
}

// serviceEnableSnap / serviceStateSnap are the Before payloads (pre-apply).
type serviceEnableSnap struct {
	Name    string
	Enabled bool
}
type serviceStateSnap struct {
	Name   string
	Active bool
}

// observe queries `systemctl is-enabled` + `is-active` + `status` for a unit.
// Treats "not-found" / "unknown unit" as Exists=false.
func (m *ServiceManager) observe(ctx context.Context, name string) (serviceObs, error) {
	obs := serviceObs{}
	enOut, _, enErr := m.sess.Run(ctx, "systemctl is-enabled "+shellQuoteOne(name))
	enTrim := strings.TrimSpace(enOut)
	// is-enabled prints "enabled"/"disabled"/"masked"/"static"/"not-found" etc.
	// Non-zero exit for anything that isn't enabled — that's normal.
	switch enTrim {
	case "enabled", "alias", "enabled-runtime", "static", "indirect":
		obs.Exists = true
		obs.Enabled = enTrim == "enabled" || enTrim == "enabled-runtime" || enTrim == "alias"
	case "disabled":
		obs.Exists = true
		obs.Enabled = false
	case "masked", "masked-runtime":
		obs.Exists = true
		obs.Masked = true
	case "not-found", "":
		// Distinguish "unit doesn't exist" from "command failed entirely".
		if enErr != nil && !strings.Contains(strings.ToLower(enTrim), "not-found") {
			// fall through to active check — some distros print nothing
		}
		// Try `systemctl status` to confirm.
		stOut, _, _ := m.sess.Run(ctx, "systemctl status "+shellQuoteOne(name)+" --no-pager 2>&1 || true")
		if strings.Contains(stOut, "could not be found") || strings.Contains(stOut, "Loaded: not-found") {
			return serviceObs{Exists: false}, nil
		}
		obs.Exists = true
	default:
		obs.Exists = true
	}
	acOut, _, _ := m.sess.Run(ctx, "systemctl is-active "+shellQuoteOne(name))
	acTrim := strings.TrimSpace(acOut)
	obs.Active = acTrim == "active" || acTrim == "activating" || acTrim == "reloading"
	return obs, nil
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *ServiceManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, ErrSessionRequired
	}
	for _, ch := range changes {
		if ch.Action == "error" {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("unit %s not found", ch.Target)})
			continue
		}
		if err := m.applyOne(ctx, ch); err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: err})
			continue
		}
		res.Applied = append(res.Applied, ch)
	}
	return res, nil
}

func (m *ServiceManager) applyOne(ctx context.Context, ch Change) error {
	switch after := ch.After.(type) {
	case serviceEnableOp:
		if after.Masked {
			return fmt.Errorf("service %s is masked; refuse to %s", after.Name, after.Op)
		}
		return m.sudoRun(ctx, "systemctl "+after.Op+" "+shellQuoteOne(after.Name))
	case serviceStateOp:
		if after.Masked {
			return fmt.Errorf("service %s is masked; refuse to %s", after.Name, after.Op)
		}
		cmd := "systemctl " + after.Op + " " + shellQuoteOne(after.Name)
		if err := m.sudoRun(ctx, cmd); err != nil {
			// retry once for transient dependency ordering issues.
			if err2 := m.sudoRun(ctx, cmd); err2 != nil {
				return err2
			}
		}
		return nil
	default:
		return fmt.Errorf("service.Apply: unexpected After type %T", ch.After)
	}
}

// sudoRun runs cmd via RunSudo and wraps the stderr into the returned error.
func (m *ServiceManager) sudoRun(ctx context.Context, cmd string) error {
	_, stderr, err := m.sess.RunSudo(ctx, cmd)
	if err != nil {
		return fmt.Errorf("%w: %s", err, trimStderr(stderr))
	}
	return nil
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *ServiceManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback implements Manager. Reverses each enable/disable + start/stop by
// applying the inverse captured in Before.
func (m *ServiceManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return ErrSessionRequired
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		switch before := ch.Before.(type) {
		case serviceEnableSnap:
			op := "disable"
			if before.Enabled {
				op = "enable"
			}
			_ = m.sudoRun(ctx, "systemctl "+op+" "+shellQuoteOne(before.Name))
		case serviceStateSnap:
			op := "stop"
			if before.Active {
				op = "start"
			}
			_ = m.sudoRun(ctx, "systemctl "+op+" "+shellQuoteOne(before.Name))
		}
	}
	return nil
}

// castServices extracts a []config.ServiceState from supported Spec forms.
func castServices(desired Spec) ([]config.ServiceState, error) {
	switch v := desired.(type) {
	case []config.ServiceState:
		return v, nil
	case *config.Linux:
		if v == nil {
			return nil, nil
		}
		return v.Services, nil
	case config.Linux:
		return v.Services, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("service: unsupported desired-state type %T", desired)
	}
}
