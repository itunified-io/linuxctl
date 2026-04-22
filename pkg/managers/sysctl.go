package managers

import (
	"context"
	"fmt"
)

// SysctlManager handles the "sysctl" subsystem. All methods are scaffold stubs.
type SysctlManager struct{}

// NewSysctlManager returns a scaffold sysctl manager.
func NewSysctlManager() *SysctlManager { return &SysctlManager{} }

// Name implements Manager.
func (*SysctlManager) Name() string { return "sysctl" }

// DependsOn implements Manager.
func (*SysctlManager) DependsOn() []string { return nil }

// Plan implements Manager.
func (*SysctlManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("sysctl.Plan: not implemented")
}

// Apply implements Manager.
func (*SysctlManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("sysctl.Apply: not implemented")
}

// Verify implements Manager.
func (*SysctlManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("sysctl.Verify: not implemented")
}

// Rollback implements Manager.
func (*SysctlManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("sysctl.Rollback: not implemented")
}
