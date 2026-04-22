package managers

import (
	"context"
	"fmt"
)

// LimitsManager handles the "limits" subsystem. All methods are scaffold stubs.
type LimitsManager struct{}

// NewLimitsManager returns a scaffold limits manager.
func NewLimitsManager() *LimitsManager { return &LimitsManager{} }

// Name implements Manager.
func (*LimitsManager) Name() string { return "limits" }

// DependsOn implements Manager.
func (*LimitsManager) DependsOn() []string { return []string{"sysctl"} }

// Plan implements Manager.
func (*LimitsManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("limits.Plan: not implemented")
}

// Apply implements Manager.
func (*LimitsManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("limits.Apply: not implemented")
}

// Verify implements Manager.
func (*LimitsManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("limits.Verify: not implemented")
}

// Rollback implements Manager.
func (*LimitsManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("limits.Rollback: not implemented")
}
