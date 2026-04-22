package session

import (
	"context"
	"time"
)

// Session abstracts command execution + file I/O against local or remote hosts.
// Implementations: LocalSession (current host) and SSHSession (remote).
type Session interface {
	// Host returns a human-readable identifier ("localhost" or "user@host").
	Host() string

	// Run executes cmd in the session context. Returns stdout, stderr, error.
	Run(ctx context.Context, cmd string) (stdout, stderr string, err error)

	// RunSudo executes cmd with sudo/root privileges.
	RunSudo(ctx context.Context, cmd string) (stdout, stderr string, err error)

	// WriteFile writes content to path with mode. Uses sudo if needed.
	WriteFile(ctx context.Context, path string, content []byte, mode uint32) error

	// ReadFile reads the entire contents of path.
	ReadFile(ctx context.Context, path string) ([]byte, error)

	// FileExists returns true if path exists on the target host.
	FileExists(ctx context.Context, path string) (bool, error)

	// Close releases the transport (no-op for local sessions).
	Close() error
}

// Opts configures a session's transport + auth.
type Opts struct {
	Host               string        // hostname or "localhost"
	User               string        // SSH user
	Port               int           // default 22
	KeyFile            string        // default ~/.ssh/id_ed25519
	Password           string        // optional SSH password (rarely used)
	StrictHostKeyCheck bool          // if true, verify known_hosts
	KnownHostsFile     string        // default ~/.ssh/known_hosts
	Timeout            time.Duration // default 30s
	SudoPassword       string        // empty => `sudo -n` (NOPASSWD required)
}
