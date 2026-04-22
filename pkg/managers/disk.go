package managers

import (
	"context"
	"fmt"
)

// DiskManager handles the "disk" subsystem. All methods are scaffold stubs.
type DiskManager struct{}

// NewDiskManager returns a scaffold disk manager.
func NewDiskManager() *DiskManager { return &DiskManager{} }

// Name implements Manager.
func (*DiskManager) Name() string { return "disk" }

// DependsOn implements Manager.
func (*DiskManager) DependsOn() []string { return nil }

// Plan implements Manager.
func (*DiskManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("disk.Plan: not implemented")
}

// Apply implements Manager.
func (*DiskManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("disk.Apply: not implemented")
}

// Verify implements Manager.
func (*DiskManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("disk.Verify: not implemented")
}

// Rollback implements Manager.
func (*DiskManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("disk.Rollback: not implemented")
}
