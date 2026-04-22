// Package session abstracts command execution against local or remote hosts.
package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHSession is a remote command pipeline backed by golang.org/x/crypto/ssh.
// A single Session multiplexes commands over one TCP/TLS connection; callers
// should Close() when done.
type SSHSession struct {
	HostAddr     string
	User         string
	client       *ssh.Client
	sudoPassword string
}

// NewSSH returns a lightweight SSH descriptor. It does NOT dial — call Dial()
// (or NewSSHDial) to establish the transport. This two-phase API keeps the
// zero-value useful for tests that never touch the network.
func NewSSH(host, user string) *SSHSession {
	return &SSHSession{HostAddr: host, User: user}
}

// NewSSHDial constructs a session from Opts and dials immediately.
func NewSSHDial(o Opts) (*SSHSession, error) {
	s := &SSHSession{HostAddr: o.Host, User: o.User, sudoPassword: o.SudoPassword}
	if err := s.Dial(o); err != nil {
		return nil, err
	}
	return s, nil
}

// Dial opens the SSH connection with up to 3 attempts (exponential backoff).
func (s *SSHSession) Dial(o Opts) error {
	if o.Host == "" {
		return errors.New("ssh: host is required")
	}
	if o.Port == 0 {
		o.Port = 22
	}
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	if o.User == "" {
		o.User = s.User
	}
	if o.User == "" {
		u := os.Getenv("USER")
		if u == "" {
			u = "root"
		}
		o.User = u
	}
	s.User = o.User
	s.HostAddr = o.Host

	var auths []ssh.AuthMethod
	keyFile := o.KeyFile
	if keyFile == "" {
		home, _ := os.UserHomeDir()
		keyFile = filepath.Join(home, ".ssh", "id_ed25519")
	}
	if b, err := os.ReadFile(keyFile); err == nil {
		if signer, err := ssh.ParsePrivateKey(b); err == nil {
			auths = append(auths, ssh.PublicKeys(signer))
		}
	}
	if o.Password != "" {
		auths = append(auths, ssh.Password(o.Password))
	}
	if len(auths) == 0 {
		return fmt.Errorf("ssh: no auth methods available (tried %s)", keyFile)
	}

	hostKeyCb := ssh.InsecureIgnoreHostKey() // #nosec G106 — opt-in StrictHostKeyCheck.
	if o.StrictHostKeyCheck {
		khFile := o.KnownHostsFile
		if khFile == "" {
			home, _ := os.UserHomeDir()
			khFile = filepath.Join(home, ".ssh", "known_hosts")
		}
		cb, err := knownhosts.New(khFile)
		if err != nil {
			return fmt.Errorf("ssh: known_hosts %s: %w", khFile, err)
		}
		hostKeyCb = cb
	}

	cfg := &ssh.ClientConfig{
		User:            o.User,
		Auth:            auths,
		HostKeyCallback: hostKeyCb,
		Timeout:         o.Timeout,
	}
	addr := net(o.Host, o.Port)

	// Up to 3 attempts with exponential backoff.
	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= 3; attempt++ {
		c, err := ssh.Dial("tcp", addr, cfg)
		if err == nil {
			s.client = c
			return nil
		}
		lastErr = err
		if attempt < 3 {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return fmt.Errorf("ssh dial %s: %w", addr, lastErr)
}

func net(host string, port int) string {
	if strings.Contains(host, ":") {
		return host
	}
	return host + ":" + strconv.Itoa(port)
}

// Host reports user@host.
func (s *SSHSession) Host() string {
	if s.User == "" {
		return s.HostAddr
	}
	return s.User + "@" + s.HostAddr
}

// Close tears down the SSH connection.
func (s *SSHSession) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	err := s.client.Close()
	s.client = nil
	return err
}

// Run executes cmd and returns stdout / stderr.
func (s *SSHSession) Run(ctx context.Context, cmd string) (string, string, error) {
	if s.client == nil {
		return "", "", errors.New("ssh: not connected (call Dial first)")
	}
	sess, err := s.client.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("ssh: new session: %w", err)
	}
	defer sess.Close()
	var so, se bytes.Buffer
	sess.Stdout = &so
	sess.Stderr = &se

	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()
	select {
	case <-ctx.Done():
		_ = sess.Signal(ssh.SIGKILL)
		return so.String(), se.String(), ctx.Err()
	case err := <-done:
		if err != nil {
			return so.String(), se.String(), fmt.Errorf("ssh run %q: %w", cmd, err)
		}
		return so.String(), se.String(), nil
	}
}

// RunSudo wraps cmd under sudo. Uses -n (requires NOPASSWD) unless
// SudoPassword is set.
func (s *SSHSession) RunSudo(ctx context.Context, cmd string) (string, string, error) {
	var full string
	if s.sudoPassword != "" {
		full = fmt.Sprintf("printf '%%s\\n' %s | sudo -S -p '' sh -c %s",
			sqq(s.sudoPassword), sqq(cmd))
	} else {
		full = "sudo -n sh -c " + sqq(cmd)
	}
	return s.Run(ctx, full)
}

// WriteFile writes content to path with mode via sudo tee.
func (s *SSHSession) WriteFile(ctx context.Context, path string, content []byte, mode uint32) error {
	cmd := fmt.Sprintf(
		"mkdir -p %s && cat > %s && chmod %o %s",
		sqq(filepath.Dir(path)), sqq(path), mode, sqq(path),
	)
	if s.client == nil {
		return errors.New("ssh: not connected")
	}
	sess, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh: new session: %w", err)
	}
	defer sess.Close()

	// Prefix with sudo if password supplied; otherwise rely on -n.
	var shell string
	if s.sudoPassword != "" {
		shell = fmt.Sprintf("printf '%%s\\n' %s | sudo -S -p '' sh -c %s",
			sqq(s.sudoPassword), sqq(cmd))
	} else {
		shell = "sudo -n sh -c " + sqq(cmd)
	}

	sess.Stdin = bytes.NewReader(content)
	var se bytes.Buffer
	sess.Stderr = &se

	done := make(chan error, 1)
	go func() { done <- sess.Run(shell) }()
	select {
	case <-ctx.Done():
		_ = sess.Signal(ssh.SIGKILL)
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("ssh write %s: %w (%s)", path, err, strings.TrimSpace(se.String()))
		}
		return nil
	}
}

// ReadFile returns remote contents via sudo cat.
func (s *SSHSession) ReadFile(ctx context.Context, path string) ([]byte, error) {
	out, stderr, err := s.RunSudo(ctx, "cat "+sqq(path))
	if err != nil {
		return nil, fmt.Errorf("ssh read %s: %w (%s)", path, err, strings.TrimSpace(stderr))
	}
	return []byte(out), nil
}

// FileExists tests via `test -e`.
func (s *SSHSession) FileExists(ctx context.Context, path string) (bool, error) {
	_, _, err := s.RunSudo(ctx, "test -e "+sqq(path))
	if err == nil {
		return true, nil
	}
	// Exit code 1 on test -e means "not found" — that's not a transport error.
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	// Our wrapper wraps the error; detect non-zero exit by substring as fallback.
	if strings.Contains(err.Error(), "Process exited") || strings.Contains(err.Error(), "exit status") {
		return false, nil
	}
	return false, nil
}

// sqq single-quotes s for the remote shell.
func sqq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
