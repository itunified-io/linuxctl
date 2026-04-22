package session

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	gssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"
)

// ---------------------------------------------------------------------------
// Test SSH server
// ---------------------------------------------------------------------------

// handlerCtx is passed to each handler so it can read stdin / write stdout / exit code.
type handlerCtx struct {
	cmd string
	in  io.Reader
	out io.Writer
	err io.Writer
}

// cmdHandler returns an exit code.
type cmdHandler func(*handlerCtx) int

// testServer is an in-process SSH server for session tests.
type testServer struct {
	srv      *gssh.Server
	listener net.Listener
	keyPath  string
	handlers map[string]cmdHandler
	fallback cmdHandler
	rejects  atomic.Int32 // #connections to reject before accepting (for retry test)
	allowed  atomic.Int32
}

// startTestSSHServer boots a server on a random loopback port and writes the
// client key file to disk. Pass handlers keyed by the first token of each
// command; if no match, fallback runs.
func startTestSSHServer(t *testing.T, handlers map[string]cmdHandler, fallback cmdHandler) *testServer {
	t.Helper()

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")

	// Generate a client ed25519 keypair and write the private key (PEM).
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen client key: %v", err)
	}
	privPEM, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal priv: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(privPEM), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	// Derive the corresponding ssh public key for authentication.
	clientSSHPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("pubkey: %v", err)
	}

	// Host key for the server (separate ed25519 pair).
	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}

	ts := &testServer{
		keyPath:  keyPath,
		handlers: handlers,
		fallback: fallback,
	}

	sess := gssh.Server{
		Handler: func(s gssh.Session) {
			hc := &handlerCtx{
				cmd: strings.Join(s.Command(), " "),
				in:  s,
				out: s,
				err: s.Stderr(),
			}
			// If no explicit command (Command() empty), pass whole raw cmd.
			rawCmd := s.RawCommand()
			if hc.cmd == "" {
				hc.cmd = rawCmd
			}
			// Dispatch: exact match on first token, then fallback.
			first := hc.cmd
			if i := strings.Index(first, " "); i > 0 {
				first = first[:i]
			}
			if h, ok := ts.handlers[first]; ok {
				_ = s.Exit(h(hc))
				return
			}
			// Also allow exact full-command match.
			if h, ok := ts.handlers[hc.cmd]; ok {
				_ = s.Exit(h(hc))
				return
			}
			if ts.fallback != nil {
				_ = s.Exit(ts.fallback(hc))
				return
			}
			_ = s.Exit(127)
		},
		PublicKeyHandler: func(_ gssh.Context, k gssh.PublicKey) bool {
			// Accept only our client key.
			return bytes.Equal(k.Marshal(), clientSSHPub.Marshal())
		},
		ChannelHandlers: map[string]gssh.ChannelHandler{
			"session": gssh.DefaultSessionHandler,
		},
	}
	sess.AddHostKey(hostSigner)
	ts.srv = &sess

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Custom accept loop so we can reject the first N connections (retry test).
	ts.listener = ln

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			if ts.rejects.Load() > 0 {
				ts.rejects.Add(-1)
				_ = c.Close()
				continue
			}
			ts.allowed.Add(1)
			go sess.HandleConn(c)
		}
	}()

	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	return ts
}

func (ts *testServer) addr() string { return ts.listener.Addr().String() }

// hostPort splits "host:port" → host, int.
func (ts *testServer) hostPort() (string, int) {
	host, portStr, _ := net.SplitHostPort(ts.addr())
	var p int
	_, _ = strings.NewReader(portStr).Read([]byte{}) // no-op, just silence linter
	// parse port via fmt
	if _, err := fmtSscanInt(portStr, &p); err != nil {
		return host, 0
	}
	return host, p
}

// fmtSscanInt is an inline atoi to avoid importing strconv here (already
// transitively imported elsewhere, but keeps the helper local).
func fmtSscanInt(s string, out *int) (int, error) {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, io.ErrUnexpectedEOF
		}
		n = n*10 + int(s[i]-'0')
	}
	*out = n
	return 1, nil
}
