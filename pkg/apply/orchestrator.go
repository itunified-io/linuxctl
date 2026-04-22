// Package apply implements the cross-manager orchestrator that aggregates
// Plan / Apply / Verify / Rollback across all 13 managers in dependency
// order.
package apply

import (
	"context"
	"fmt"
	"strings"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/managers"
	"github.com/itunified-io/linuxctl/pkg/session"
)

// defaultOrder is the hardcoded Phase-3 dependency order. Within the DAG this
// ordering satisfies: disk → user → dir → package → mount → remaining.
// Additional managers run last in stable name order.
var defaultOrder = []string{
	"disk",
	"user",
	"dir",
	"package",
	"mount",
	"service",
	"sysctl",
	"limits",
	"firewall",
	"hosts",
	"network",
	"ssh_auth",
	"selinux",
}

// Orchestrator runs the full plan/apply/verify/rollback pipeline across
// managers.
type Orchestrator struct {
	Env             *config.Env
	Linux           *config.Linux
	Session         session.Session
	Managers        []managers.Manager
	DryRun          bool
	ContinueOnError bool

	// applied records, per manager name, the changes we applied in the last
	// successful Apply. Used by Rollback.
	applied map[string][]managers.Change
}

// New constructs an orchestrator. mgrs is optional — when nil the global
// managers.Registry() snapshot is used and ordered by defaultOrder.
func New(env *config.Env, sess session.Session, dryRun bool) *Orchestrator {
	return &Orchestrator{
		Env:     env,
		Session: sess,
		DryRun:  dryRun,
		applied: map[string][]managers.Change{},
	}
}

// WithLinux sets the decoded linux.yaml used for per-manager desired-state
// extraction.
func (o *Orchestrator) WithLinux(l *config.Linux) *Orchestrator {
	o.Linux = l
	return o
}

// WithManagers overrides the default registry lookup. Order matters.
func (o *Orchestrator) WithManagers(mgrs []managers.Manager) *Orchestrator {
	o.Managers = mgrs
	return o
}

// PlanResult aggregates per-manager changes.
type PlanResult struct {
	Changes     []managers.Change
	ByManager   map[string][]managers.Change
	TotalCreate int
	TotalUpdate int
	TotalDelete int
}

// VerifyAggregate reports drift across managers.
type VerifyAggregate struct {
	InDrift []string
	Matches bool
	Detail  map[string]managers.VerifyResult
}

// DiffReport is a human-renderable drift diff.
type DiffReport struct {
	ByManager map[string][]managers.Change
	Empty     bool
}

// resolveManagers returns the ordered list of managers to run.
func (o *Orchestrator) resolveManagers() []managers.Manager {
	if len(o.Managers) > 0 {
		return o.Managers
	}
	reg := managers.All()
	var out []managers.Manager
	seen := map[string]bool{}
	for _, name := range defaultOrder {
		if m, ok := reg[name]; ok {
			out = append(out, m)
			seen[name] = true
		}
	}
	// Append any registered manager not in the hardcoded order (stable).
	for name, m := range reg {
		if !seen[name] {
			out = append(out, m)
		}
	}
	return out
}

// desiredFor returns the Spec for a given manager name based on o.Linux.
// Unknown managers receive nil.
func (o *Orchestrator) desiredFor(name string) managers.Spec {
	if o.Linux == nil {
		return nil
	}
	switch name {
	case "disk":
		return o.Linux.DiskLayout
	case "mount":
		return o.Linux.Mounts
	case "user":
		return o.Linux.UsersGroups
	case "dir":
		return o.Linux.Directories
	case "package":
		return o.Linux.Packages
	}
	// For other managers, pass the full Linux spec — each may ignore.
	return o.Linux
}

// Plan runs Plan on every manager and aggregates the resulting changes.
func (o *Orchestrator) Plan(ctx context.Context) (PlanResult, error) {
	res := PlanResult{ByManager: map[string][]managers.Change{}}
	for _, m := range o.resolveManagers() {
		cs, err := m.Plan(ctx, o.desiredFor(m.Name()), nil)
		if err != nil {
			return res, fmt.Errorf("%s plan: %w", m.Name(), err)
		}
		if len(cs) == 0 {
			continue
		}
		res.ByManager[m.Name()] = cs
		res.Changes = append(res.Changes, cs...)
		for _, c := range cs {
			switch c.Action {
			case "create":
				res.TotalCreate++
			case "update":
				res.TotalUpdate++
			case "delete":
				res.TotalDelete++
			}
		}
	}
	return res, nil
}

// Apply runs Plan then executes changes in dependency order. On error, it
// stops unless ContinueOnError is set, and records applied changes for
// subsequent Rollback.
func (o *Orchestrator) Apply(ctx context.Context) (*managers.ApplyResult, error) {
	plan, err := o.Plan(ctx)
	if err != nil {
		return nil, err
	}
	agg := &managers.ApplyResult{RunID: "apply-all"}
	for _, m := range o.resolveManagers() {
		cs, ok := plan.ByManager[m.Name()]
		if !ok {
			continue
		}
		r, err := m.Apply(ctx, cs, o.DryRun)
		agg.Applied = append(agg.Applied, r.Applied...)
		agg.Skipped = append(agg.Skipped, r.Skipped...)
		agg.Failed = append(agg.Failed, r.Failed...)
		agg.Duration += r.Duration
		if len(r.Applied) > 0 {
			o.applied[m.Name()] = append(o.applied[m.Name()], r.Applied...)
		}
		if err != nil {
			if o.ContinueOnError {
				continue
			}
			return agg, fmt.Errorf("%s apply: %w", m.Name(), err)
		}
	}
	return agg, nil
}

// Verify runs Verify on every manager and reports which have drift.
func (o *Orchestrator) Verify(ctx context.Context) (*VerifyAggregate, error) {
	agg := &VerifyAggregate{Matches: true, Detail: map[string]managers.VerifyResult{}}
	for _, m := range o.resolveManagers() {
		r, err := m.Verify(ctx, o.desiredFor(m.Name()))
		if err != nil {
			return agg, fmt.Errorf("%s verify: %w", m.Name(), err)
		}
		agg.Detail[m.Name()] = r
		if !r.OK {
			agg.Matches = false
			agg.InDrift = append(agg.InDrift, m.Name())
		}
	}
	return agg, nil
}

// Rollback reverses previously applied changes in reverse manager order.
func (o *Orchestrator) Rollback(ctx context.Context) error {
	order := o.resolveManagers()
	var errs []string
	for i := len(order) - 1; i >= 0; i-- {
		m := order[i]
		cs, ok := o.applied[m.Name()]
		if !ok || len(cs) == 0 {
			continue
		}
		if err := m.Rollback(ctx, cs); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.Name(), err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("rollback errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Diff is a read-only drift report.
func (o *Orchestrator) Diff(ctx context.Context) (*DiffReport, error) {
	plan, err := o.Plan(ctx)
	if err != nil {
		return nil, err
	}
	return &DiffReport{ByManager: plan.ByManager, Empty: len(plan.Changes) == 0}, nil
}
