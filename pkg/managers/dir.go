package managers

import (
	"context"
	"fmt"
)

// DirManager handles the "dir" subsystem. All methods are scaffold stubs.
type DirManager struct{}

// NewDirManager returns a scaffold dir manager.
func NewDirManager() *DirManager { return &DirManager{} }

// Name implements Manager.
func (*DirManager) Name() string { return "dir" }

// DependsOn implements Manager.
func (*DirManager) DependsOn() []string { return []string{"mount"} }

// Plan implements Manager.
func (*DirManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("dir.Plan: not implemented")
}

// Apply implements Manager.
func (*DirManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("dir.Apply: not implemented")
}

// Verify implements Manager.
func (*DirManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("dir.Verify: not implemented")
}

// Rollback implements Manager.
func (*DirManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("dir.Rollback: not implemented")
}
