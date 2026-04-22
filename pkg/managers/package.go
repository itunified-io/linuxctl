package managers

import (
	"context"
	"fmt"
)

// PackageManager handles the "package" subsystem. All methods are scaffold stubs.
type PackageManager struct{}

// NewPackageManager returns a scaffold package manager.
func NewPackageManager() *PackageManager { return &PackageManager{} }

// Name implements Manager.
func (*PackageManager) Name() string { return "package" }

// DependsOn implements Manager.
func (*PackageManager) DependsOn() []string { return nil }

// Plan implements Manager.
func (*PackageManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("package.Plan: not implemented")
}

// Apply implements Manager.
func (*PackageManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("package.Apply: not implemented")
}

// Verify implements Manager.
func (*PackageManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("package.Verify: not implemented")
}

// Rollback implements Manager.
func (*PackageManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("package.Rollback: not implemented")
}
