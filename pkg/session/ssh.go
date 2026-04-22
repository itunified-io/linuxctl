// Package session abstracts command execution against local or remote hosts.
package session

import (
	"context"
	"fmt"
)

// SSHSession represents a remote command pipeline.
// Scaffold only — real implementation wraps golang.org/x/crypto/ssh + bastion.
type SSHSession struct {
	Host string
	User string
}

// NewSSH returns a scaffold SSH session descriptor.
func NewSSH(host, user string) *SSHSession { return &SSHSession{Host: host, User: user} }

// Run executes a command and returns stdout / stderr. Not implemented.
func (s *SSHSession) Run(ctx context.Context, cmd string) (stdout, stderr string, err error) {
	_, _ = ctx, cmd
	return "", "", fmt.Errorf("ssh.Run: not implemented")
}
