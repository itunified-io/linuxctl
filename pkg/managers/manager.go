// Package managers defines the Manager interface and shared plan/apply/verify
// types used by each of the 13 linuxctl subsystem managers.
package managers

import (
	"context"
	"time"
)

// HazardLevel classifies the blast radius of a proposed change.
type HazardLevel string

const (
	HazardNone        HazardLevel = "none"
	HazardWarn        HazardLevel = "warn"
	HazardDestructive HazardLevel = "destructive"
)

// Spec is a manager-specific YAML-decoded desired-state struct.
type Spec any

// State is a manager-specific snapshot of observed runtime state.
type State any

// Change describes a single planned mutation.
type Change struct {
	ID          string
	Manager     string
	Target      string
	Action      string // create|update|delete|noop
	Before      any
	After       any
	RollbackCmd string
	Hazard      HazardLevel
}

// ChangeSet is a convenience alias for an ordered slice of changes.
type ChangeSet = []Change

// ChangeErr pairs a failed change with its error.
type ChangeErr struct {
	Change Change
	Err    error
}

// ApplyResult is returned from Manager.Apply.
type ApplyResult struct {
	RunID    string
	Applied  []Change
	Skipped  []Change
	Failed   []ChangeErr
	Duration time.Duration
}

// VerifyResult is returned from Manager.Verify.
type VerifyResult struct {
	OK       bool
	Drift    []Change
	Messages []string
}

// Manager is the contract every subsystem (disk, user, package, …) must satisfy.
type Manager interface {
	Name() string
	DependsOn() []string

	Plan(ctx context.Context, desired Spec, current State) ([]Change, error)
	Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error)
	Verify(ctx context.Context, desired Spec) (VerifyResult, error)
	Rollback(ctx context.Context, changes []Change) error
}
