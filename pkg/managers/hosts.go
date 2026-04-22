package managers

import (
	"context"
	"fmt"
)

// HostsManager handles the "hosts" subsystem. All methods are scaffold stubs.
type HostsManager struct{}

// NewHostsManager returns a scaffold hosts manager.
func NewHostsManager() *HostsManager { return &HostsManager{} }

// Name implements Manager.
func (*HostsManager) Name() string { return "hosts" }

// DependsOn implements Manager.
func (*HostsManager) DependsOn() []string { return nil }

// Plan implements Manager.
func (*HostsManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("hosts.Plan: not implemented")
}

// Apply implements Manager.
func (*HostsManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("hosts.Apply: not implemented")
}

// Verify implements Manager.
func (*HostsManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("hosts.Verify: not implemented")
}

// Rollback implements Manager.
func (*HostsManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("hosts.Rollback: not implemented")
}
