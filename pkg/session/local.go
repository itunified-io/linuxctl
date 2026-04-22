package session

import (
	"context"
	"fmt"
)

// LocalSession executes commands against the local host.
type LocalSession struct{}

// NewLocal returns a scaffold local session.
func NewLocal() *LocalSession { return &LocalSession{} }

// Run executes a local command. Not implemented.
func (l *LocalSession) Run(ctx context.Context, cmd string) (stdout, stderr string, err error) {
	_, _ = ctx, cmd
	return "", "", fmt.Errorf("local.Run: not implemented")
}
