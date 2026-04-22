package managers

import (
	"context"
	"fmt"
)

// NetworkManager handles the "network" subsystem. All methods are scaffold stubs.
type NetworkManager struct{}

// NewNetworkManager returns a scaffold network manager.
func NewNetworkManager() *NetworkManager { return &NetworkManager{} }

// Name implements Manager.
func (*NetworkManager) Name() string { return "network" }

// DependsOn implements Manager.
func (*NetworkManager) DependsOn() []string { return []string{"hosts"} }

// Plan implements Manager.
func (*NetworkManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("network.Plan: not implemented")
}

// Apply implements Manager.
func (*NetworkManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("network.Apply: not implemented")
}

// Verify implements Manager.
func (*NetworkManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("network.Verify: not implemented")
}

// Rollback implements Manager.
func (*NetworkManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("network.Rollback: not implemented")
}
