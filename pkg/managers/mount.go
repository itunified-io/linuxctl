package managers

import (
	"context"
	"fmt"
)

// MountManager handles the "mount" subsystem. All methods are scaffold stubs.
type MountManager struct{}

// NewMountManager returns a scaffold mount manager.
func NewMountManager() *MountManager { return &MountManager{} }

// Name implements Manager.
func (*MountManager) Name() string { return "mount" }

// DependsOn implements Manager.
func (*MountManager) DependsOn() []string { return []string{"disk"} }

// Plan implements Manager.
func (*MountManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("mount.Plan: not implemented")
}

// Apply implements Manager.
func (*MountManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("mount.Apply: not implemented")
}

// Verify implements Manager.
func (*MountManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("mount.Verify: not implemented")
}

// Rollback implements Manager.
func (*MountManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("mount.Rollback: not implemented")
}
