package managers

import (
	"context"
	"fmt"
)

// FirewallManager handles the "firewall" subsystem. All methods are scaffold stubs.
type FirewallManager struct{}

// NewFirewallManager returns a scaffold firewall manager.
func NewFirewallManager() *FirewallManager { return &FirewallManager{} }

// Name implements Manager.
func (*FirewallManager) Name() string { return "firewall" }

// DependsOn implements Manager.
func (*FirewallManager) DependsOn() []string { return []string{"network"} }

// Plan implements Manager.
func (*FirewallManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("firewall.Plan: not implemented")
}

// Apply implements Manager.
func (*FirewallManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("firewall.Apply: not implemented")
}

// Verify implements Manager.
func (*FirewallManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("firewall.Verify: not implemented")
}

// Rollback implements Manager.
func (*FirewallManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("firewall.Rollback: not implemented")
}
