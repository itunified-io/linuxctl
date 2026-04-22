package managers

import (
	"context"
	"fmt"
)

// ServiceManager handles the "service" subsystem. All methods are scaffold stubs.
type ServiceManager struct{}

// NewServiceManager returns a scaffold service manager.
func NewServiceManager() *ServiceManager { return &ServiceManager{} }

// Name implements Manager.
func (*ServiceManager) Name() string { return "service" }

// DependsOn implements Manager.
func (*ServiceManager) DependsOn() []string { return []string{"package","ssh"} }

// Plan implements Manager.
func (*ServiceManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("service.Plan: not implemented")
}

// Apply implements Manager.
func (*ServiceManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("service.Apply: not implemented")
}

// Verify implements Manager.
func (*ServiceManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("service.Verify: not implemented")
}

// Rollback implements Manager.
func (*ServiceManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("service.Rollback: not implemented")
}
