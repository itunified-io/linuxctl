package managers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/itunified-io/linuxctl/pkg/session"
)

// ErrNotImplemented is returned by scaffold manager methods.
var ErrNotImplemented = errors.New("not implemented")

// ErrSessionRequired is returned when an Apply/Verify path is invoked on a
// manager that was not bound to a session.
var ErrSessionRequired = errors.New("session is required")

// Registry holds every Manager registered via init(). Lookups are by Manager.Name().
var (
	regMu    sync.RWMutex
	registry = map[string]Manager{}
)

// Register adds m to the global registry. Safe for init-time use.
func Register(m Manager) {
	regMu.Lock()
	defer regMu.Unlock()
	if m == nil {
		return
	}
	registry[m.Name()] = m
}

// Lookup returns the registered manager for name, or nil.
func Lookup(name string) Manager {
	regMu.RLock()
	defer regMu.RUnlock()
	return registry[name]
}

// All returns a snapshot of every registered manager keyed by name.
func All() map[string]Manager {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make(map[string]Manager, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}

// RunAndCheck runs cmd through sess and returns stdout. Non-zero exits are
// wrapped with the captured stderr preview for context.
func RunAndCheck(ctx context.Context, sess session.Session, cmd string) (string, error) {
	if sess == nil {
		return "", ErrSessionRequired
	}
	out, stderr, err := sess.Run(ctx, cmd)
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, trimStderr(stderr))
	}
	return out, nil
}

// RunSudoAndCheck is RunAndCheck under sudo.
func RunSudoAndCheck(ctx context.Context, sess session.Session, cmd string) (string, error) {
	if sess == nil {
		return "", ErrSessionRequired
	}
	out, stderr, err := sess.RunSudo(ctx, cmd)
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, trimStderr(stderr))
	}
	return out, nil
}

// trimStderr returns a single-line preview of stderr for error wrapping.
func trimStderr(stderr string) string {
	s := strings.TrimSpace(stderr)
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// shellQuoteOne wraps s in single quotes for safe interpolation into sh -c.
func shellQuoteOne(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
