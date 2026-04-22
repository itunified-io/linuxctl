package managers

import (
	"context"
	"fmt"
)

// SELinuxManager handles the "selinux" subsystem. All methods are scaffold stubs.
type SELinuxManager struct{}

// NewSELinuxManager returns a scaffold selinux manager.
func NewSELinuxManager() *SELinuxManager { return &SELinuxManager{} }

// Name implements Manager.
func (*SELinuxManager) Name() string { return "selinux" }

// DependsOn implements Manager.
func (*SELinuxManager) DependsOn() []string { return []string{"service"} }

// Plan implements Manager.
func (*SELinuxManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("selinux.Plan: not implemented")
}

// Apply implements Manager.
func (*SELinuxManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("selinux.Apply: not implemented")
}

// Verify implements Manager.
func (*SELinuxManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("selinux.Verify: not implemented")
}

// Rollback implements Manager.
func (*SELinuxManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("selinux.Rollback: not implemented")
}
