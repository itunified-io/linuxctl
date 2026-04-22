package managers

import (
	"context"
	"fmt"
)

// UserManager handles the "user" subsystem. All methods are scaffold stubs.
type UserManager struct{}

// NewUserManager returns a scaffold user manager.
func NewUserManager() *UserManager { return &UserManager{} }

// Name implements Manager.
func (*UserManager) Name() string { return "user" }

// DependsOn implements Manager.
func (*UserManager) DependsOn() []string { return []string{"package"} }

// Plan implements Manager.
func (*UserManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("user.Plan: not implemented")
}

// Apply implements Manager.
func (*UserManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("user.Apply: not implemented")
}

// Verify implements Manager.
func (*UserManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("user.Verify: not implemented")
}

// Rollback implements Manager.
func (*UserManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("user.Rollback: not implemented")
}
