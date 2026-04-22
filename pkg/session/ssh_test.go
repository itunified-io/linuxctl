package session

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// Catch-all fallback: echoes the command back as stdout, exits 0.
func echoFallback(h *handlerCtx) int {
	_, _ = io.WriteString(h.out, h.cmd)
	return 0
}

func newSSHClient(t *testing.T, ts *testServer, extra ...func(*Opts)) *SSHSession {
	t.Helper()
	host, port := ts.hostPort()
	o := Opts{
		Host:    host,
		Port:    port,
		User:    "test",
		KeyFile: ts.keyPath,
		Timeout: 3 * time.Second,
	}
	for _, f := range extra {
		f(&o)
	}
	s, err := NewSSHDial(o)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// ---- Basic command execution ---------------------------------------------

func TestSSH_RunCommand_Echo(t *testing.T) {
	ts := startTestSSHServer(t, map[string]cmdHandler{
		"echo": func(h *handlerCtx) int {
			_, _ = io.WriteString(h.out, strings.TrimPrefix(h.cmd, "echo ")+"\n")
			return 0
		},
	}, echoFallback)
	s := newSSHClient(t, ts)

	out, _, err := s.Run(context.Background(), "echo hello")
	require.NoError(t, err)
	require.Equal(t, "hello\n", out)
}

func TestSSH_RunCommand_NonZeroExit(t *testing.T) {
	ts := startTestSSHServer(t, map[string]cmdHandler{
		"false": func(h *handlerCtx) int { return 1 },
	}, echoFallback)
	s := newSSHClient(t, ts)

	_, _, err := s.Run(context.Background(), "false")
	require.Error(t, err)
}

func TestSSH_Run_StderrCaptured(t *testing.T) {
	ts := startTestSSHServer(t, map[string]cmdHandler{
		"stderr": func(h *handlerCtx) int {
			_, _ = io.WriteString(h.err, "boom")
			return 2
		},
	}, echoFallback)
	s := newSSHClient(t, ts)

	_, stderr, err := s.Run(context.Background(), "stderr")
	require.Error(t, err)
	require.Contains(t, stderr, "boom")
}

func TestSSH_Run_NotConnected(t *testing.T) {
	s := NewSSH("h", "u")
	_, _, err := s.Run(context.Background(), "echo")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not connected")
}

// ---- Sudo paths ----------------------------------------------------------

func TestSSH_RunSudo_NOPASSWD(t *testing.T) {
	ts := startTestSSHServer(t, nil, func(h *handlerCtx) int {
		// Expect cmd to start with "sudo -n sh -c '…'".
		if !strings.HasPrefix(h.cmd, "sudo -n sh -c ") {
			_, _ = io.WriteString(h.err, "missing sudo prefix")
			return 1
		}
		_, _ = io.WriteString(h.out, "ok")
		return 0
	})
	s := newSSHClient(t, ts)
	out, _, err := s.RunSudo(context.Background(), "whoami")
	require.NoError(t, err)
	require.Equal(t, "ok", out)
}

func TestSSH_RunSudo_WithPassword(t *testing.T) {
	ts := startTestSSHServer(t, nil, func(h *handlerCtx) int {
		// Expect: printf '%s\n' 'PASSWORD' | sudo -S -p '' sh -c 'CMD'
		if !strings.Contains(h.cmd, "printf") || !strings.Contains(h.cmd, "sudo -S -p") {
			return 1
		}
		_, _ = io.WriteString(h.out, "pw-ok")
		return 0
	})
	s := newSSHClient(t, ts, func(o *Opts) { o.SudoPassword = "s3cret" })
	out, _, err := s.RunSudo(context.Background(), "whoami")
	require.NoError(t, err)
	require.Equal(t, "pw-ok", out)
}

// ---- WriteFile / ReadFile / FileExists -----------------------------------

func TestSSH_WriteFile_SendsContentViaStdin(t *testing.T) {
	var captured []byte
	ts := startTestSSHServer(t, nil, func(h *handlerCtx) int {
		b, _ := io.ReadAll(h.in)
		captured = b
		return 0
	})
	s := newSSHClient(t, ts)
	err := s.WriteFile(context.Background(), "/etc/foo.conf", []byte("payload"), 0o644)
	require.NoError(t, err)
	require.Equal(t, "payload", string(captured))
}

func TestSSH_WriteFile_NotConnected(t *testing.T) {
	s := NewSSH("h", "u")
	err := s.WriteFile(context.Background(), "/x", []byte("a"), 0o600)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not connected")
}

func TestSSH_WriteFile_RemoteFailure(t *testing.T) {
	ts := startTestSSHServer(t, nil, func(h *handlerCtx) int {
		_, _ = io.ReadAll(h.in)
		_, _ = io.WriteString(h.err, "disk full")
		return 1
	})
	s := newSSHClient(t, ts)
	err := s.WriteFile(context.Background(), "/x", []byte("a"), 0o600)
	require.Error(t, err)
	require.Contains(t, err.Error(), "disk full")
}

func TestSSH_WriteFile_PasswordedSudo(t *testing.T) {
	ts := startTestSSHServer(t, nil, func(h *handlerCtx) int {
		if !strings.Contains(h.cmd, "sudo -S -p") {
			return 1
		}
		_, _ = io.ReadAll(h.in)
		return 0
	})
	s := newSSHClient(t, ts, func(o *Opts) { o.SudoPassword = "pw" })
	require.NoError(t, s.WriteFile(context.Background(), "/x", []byte("a"), 0o600))
}

func TestSSH_ReadFile(t *testing.T) {
	ts := startTestSSHServer(t, nil, func(h *handlerCtx) int {
		_, _ = io.WriteString(h.out, "file contents")
		return 0
	})
	s := newSSHClient(t, ts)
	b, err := s.ReadFile(context.Background(), "/etc/hostname")
	require.NoError(t, err)
	require.Equal(t, "file contents", string(b))
}

func TestSSH_ReadFile_Error(t *testing.T) {
	ts := startTestSSHServer(t, nil, func(h *handlerCtx) int {
		_, _ = io.WriteString(h.err, "no such file")
		return 1
	})
	s := newSSHClient(t, ts)
	_, err := s.ReadFile(context.Background(), "/nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such file")
}

func TestSSH_FileExists_Yes(t *testing.T) {
	ts := startTestSSHServer(t, nil, func(h *handlerCtx) int { return 0 })
	s := newSSHClient(t, ts)
	ok, err := s.FileExists(context.Background(), "/etc/hostname")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestSSH_FileExists_No(t *testing.T) {
	ts := startTestSSHServer(t, nil, func(h *handlerCtx) int { return 1 })
	s := newSSHClient(t, ts)
	ok, err := s.FileExists(context.Background(), "/nope")
	require.NoError(t, err)
	require.False(t, ok)
}

// ---- Host / Close --------------------------------------------------------

func TestSSH_Host(t *testing.T) {
	s := NewSSH("10.0.0.1", "root")
	require.Equal(t, "root@10.0.0.1", s.Host())

	s2 := NewSSH("10.0.0.2", "")
	require.Equal(t, "10.0.0.2", s2.Host())
}

func TestSSH_CloseNilClient(t *testing.T) {
	require.NoError(t, NewSSH("h", "u").Close())
	require.NoError(t, (*SSHSession)(nil).Close())
}

// ---- Dial error paths ----------------------------------------------------

func TestSSH_Dial_EmptyHost(t *testing.T) {
	s := NewSSH("", "u")
	err := s.Dial(Opts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "host")
}

func TestSSH_Dial_NoAuthMethods(t *testing.T) {
	// Point keyfile at a path that doesn't exist → no keys loaded; no
	// password set → no auth methods.
	err := NewSSH("127.0.0.1", "u").Dial(Opts{
		Host:    "127.0.0.1",
		User:    "u",
		KeyFile: "/nonexistent/key",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no auth")
}

func TestSSH_Dial_DefaultsUserFromEnv(t *testing.T) {
	// Exercises user-defaulting branches without a real server.
	t.Setenv("USER", "someuser")
	err := NewSSH("127.0.0.1", "").Dial(Opts{
		Host:    "127.0.0.1",
		KeyFile: "/nonexistent",
	})
	require.Error(t, err) // still no auth, but user-branch ran
}

func TestSSH_Dial_DefaultsUserToRoot(t *testing.T) {
	t.Setenv("USER", "")
	err := NewSSH("127.0.0.1", "").Dial(Opts{
		Host:    "127.0.0.1",
		KeyFile: "/nonexistent",
	})
	require.Error(t, err)
}

func TestSSH_Dial_StrictHostKeyCheck_MissingKnownHostsFile(t *testing.T) {
	// Give a valid key but a bad known_hosts path.
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id")
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	privPEM, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(privPEM), 0o600))

	err = NewSSH("127.0.0.1", "u").Dial(Opts{
		Host:               "127.0.0.1",
		Port:               1,
		User:               "u",
		KeyFile:            keyPath,
		StrictHostKeyCheck: true,
		KnownHostsFile:     "/nonexistent/known_hosts",
		Timeout:            500 * time.Millisecond,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "known_hosts")
}

func TestSSH_Dial_NoServer_Retries(t *testing.T) {
	// Port 1 on localhost will reject — force retry loop to run all 3 attempts.
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id")
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	privPEM, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(privPEM), 0o600))

	start := time.Now()
	err = NewSSH("127.0.0.1", "u").Dial(Opts{
		Host:    "127.0.0.1",
		Port:    1,
		User:    "u",
		KeyFile: keyPath,
		Timeout: 300 * time.Millisecond,
	})
	require.Error(t, err)
	// 500 + 1000 ms of backoff minimum = ~1.5s.
	require.Greater(t, time.Since(start), 1*time.Second)
}

func TestSSH_Dial_DefaultKeyFile(t *testing.T) {
	// When KeyFile is empty, the code tries ~/.ssh/id_ed25519. We exercise
	// that branch by unsetting HOME; ReadFile will fail and we fall through
	// to "no auth methods".
	t.Setenv("HOME", "/nonexistent-home")
	err := NewSSH("127.0.0.1", "u").Dial(Opts{
		Host: "127.0.0.1",
		User: "u",
	})
	require.Error(t, err)
}

func TestSSH_Dial_PasswordOnly(t *testing.T) {
	// Password set, no key file → dial will still fail (no server), but the
	// password auth-method branch is covered.
	err := NewSSH("127.0.0.1", "u").Dial(Opts{
		Host:     "127.0.0.1",
		Port:     1,
		User:     "u",
		Password: "pw",
		KeyFile:  "/nonexistent",
		Timeout:  200 * time.Millisecond,
	})
	require.Error(t, err)
}

// ---- Retry-after-N-rejects ------------------------------------------------

func TestSSH_Dial_RetriesThenSucceeds(t *testing.T) {
	ts := startTestSSHServer(t, nil, echoFallback)
	ts.rejects.Store(1) // reject first connection, accept the rest

	host, port := ts.hostPort()
	s := NewSSH(host, "u")
	err := s.Dial(Opts{
		Host:    host,
		Port:    port,
		User:    "u",
		KeyFile: ts.keyPath,
		Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	_ = s.Close()
	require.GreaterOrEqual(t, ts.allowed.Load(), int32(1))
}

// ---- Context cancellation ------------------------------------------------

func TestSSH_Run_ContextCancelled(t *testing.T) {
	ts := startTestSSHServer(t, nil, func(h *handlerCtx) int {
		// Sleep "forever"; client-side ctx cancel kills the session.
		time.Sleep(5 * time.Second)
		return 0
	})
	s := newSSHClient(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, err := s.Run(ctx, "slow")
	require.Error(t, err)
}

// ---- sqq helper -----------------------------------------------------------

func TestSSH_SqqQuotesCorrectly(t *testing.T) {
	require.Equal(t, "'abc'", sqq("abc"))
	require.Equal(t, `'it'"'"'s'`, sqq("it's"))
}

// ---- joinHostPort ---------------------------------------------------------

func TestSSH_JoinHostPort(t *testing.T) {
	require.Equal(t, "1.2.3.4:22", joinHostPort("1.2.3.4", 22))
	// IPv6 (already contains colon) → returned as-is.
	require.Equal(t, "[::1]:22", joinHostPort("[::1]:22", 22))
}
