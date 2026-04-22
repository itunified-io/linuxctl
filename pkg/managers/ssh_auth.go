package managers

import (
	"context"
	"fmt"
)

// SSHAuthManager handles the "ssh" subsystem. All methods are scaffold stubs.
type SSHAuthManager struct{}

// NewSSHAuthManager returns a scaffold ssh manager.
func NewSSHAuthManager() *SSHAuthManager { return &SSHAuthManager{} }

// Name implements Manager.
func (*SSHAuthManager) Name() string { return "ssh" }

// DependsOn implements Manager.
func (*SSHAuthManager) DependsOn() []string { return []string{"user"} }

// Plan implements Manager.
func (*SSHAuthManager) Plan(ctx context.Context, desired Spec, current State) ([]Change, error) {
	_, _, _ = ctx, desired, current
	return nil, fmt.Errorf("ssh.Plan: not implemented")
}

// Apply implements Manager.
func (*SSHAuthManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	_, _, _ = ctx, changes, dryRun
	return ApplyResult{}, fmt.Errorf("ssh.Apply: not implemented")
}

// Verify implements Manager.
func (*SSHAuthManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	_, _ = ctx, desired
	return VerifyResult{}, fmt.Errorf("ssh.Verify: not implemented")
}

// Rollback implements Manager.
func (*SSHAuthManager) Rollback(ctx context.Context, changes []Change) error {
	_, _ = ctx, changes
	return fmt.Errorf("ssh.Rollback: not implemented")
}
