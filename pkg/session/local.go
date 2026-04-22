package session

import (
	"context"
	"os"
	"os/exec"
)

// LocalSession executes commands against the local host via /bin/sh.
type LocalSession struct {
	host         string
	sudoPassword string
}

// NewLocal returns a local session bound to the current host.
func NewLocal() *LocalSession { return &LocalSession{host: "localhost"} }

// NewLocalWithOpts honours Opts.Host + Opts.SudoPassword.
func NewLocalWithOpts(o Opts) *LocalSession {
	h := o.Host
	if h == "" {
		h = "localhost"
	}
	return &LocalSession{host: h, sudoPassword: o.SudoPassword}
}

// Host reports the session target.
func (l *LocalSession) Host() string {
	if l.host == "" {
		return "localhost"
	}
	return l.host
}

// Run executes cmd in /bin/sh -c. Captures stdout + stderr separately.
func (l *LocalSession) Run(ctx context.Context, cmd string) (string, string, error) {
	c := exec.CommandContext(ctx, "/bin/sh", "-c", cmd)
	var outBuf, errBuf []byte
	outBuf, err := c.Output()
	if ee, ok := err.(*exec.ExitError); ok {
		errBuf = ee.Stderr
	}
	return string(outBuf), string(errBuf), err
}

// RunSudo wraps cmd with `sudo -n` for non-interactive root.
func (l *LocalSession) RunSudo(ctx context.Context, cmd string) (string, string, error) {
	return l.Run(ctx, "sudo -n sh -c "+shellQuote(cmd))
}

// WriteFile writes content to path with the given mode. Uses sudo via tee if
// the path is not writable by the current user.
func (l *LocalSession) WriteFile(ctx context.Context, path string, content []byte, mode uint32) error {
	// Try direct write first.
	if err := os.WriteFile(path, content, os.FileMode(mode)); err == nil {
		return nil
	}
	// Fall back to sudo tee.
	tmp, err := os.CreateTemp("", "linuxctl-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	_, _, err = l.RunSudo(ctx, "install -m "+modeString(mode)+" "+shellQuote(tmp.Name())+" "+shellQuote(path))
	return err
}

// ReadFile reads path, falling back to sudo cat if permission denied.
func (l *LocalSession) ReadFile(ctx context.Context, path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		return b, nil
	}
	out, _, serr := l.RunSudo(ctx, "cat "+shellQuote(path))
	if serr != nil {
		return nil, serr
	}
	return []byte(out), nil
}

// FileExists reports whether path exists.
func (l *LocalSession) FileExists(ctx context.Context, path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		// Permission denied or similar — fall back to sudo test.
		_, _, serr := l.RunSudo(ctx, "test -e "+shellQuote(path))
		return serr == nil, nil
	}
	return false, nil
}

// Close is a no-op for local sessions.
func (l *LocalSession) Close() error { return nil }

func shellQuote(s string) string {
	// Simple single-quote escape.
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'', '\\', '\'', '\'')
			continue
		}
		out = append(out, s[i])
	}
	out = append(out, '\'')
	return string(out)
}

func modeString(mode uint32) string {
	const hex = "0123456789"
	b := []byte{'0', 0, 0, 0}
	b[1] = hex[(mode>>6)&7]
	b[2] = hex[(mode>>3)&7]
	b[3] = hex[mode&7]
	return string(b)
}
