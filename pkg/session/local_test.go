package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocal_RunEcho(t *testing.T) {
	s := NewLocal()
	out, _, err := s.Run(context.Background(), "echo hello")
	require.NoError(t, err)
	require.Equal(t, "hello\n", out)
}

func TestLocal_RunNonZero(t *testing.T) {
	s := NewLocal()
	_, _, err := s.Run(context.Background(), "false")
	require.Error(t, err)
}

func TestLocal_Host(t *testing.T) {
	require.Equal(t, "localhost", NewLocal().Host())
	s := NewLocalWithOpts(Opts{Host: "h1"})
	require.Equal(t, "h1", s.Host())
}

func TestLocal_CloseNoop(t *testing.T) {
	require.NoError(t, NewLocal().Close())
}

func TestLocal_WriteAndReadFile(t *testing.T) {
	s := NewLocal()
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	require.NoError(t, s.WriteFile(context.Background(), p, []byte("hello"), 0o600))

	b, err := s.ReadFile(context.Background(), p)
	require.NoError(t, err)
	require.Equal(t, "hello", string(b))

	fi, err := os.Stat(p)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
}

func TestLocal_FileExists(t *testing.T) {
	s := NewLocal()
	dir := t.TempDir()
	p := filepath.Join(dir, "x")
	ok, err := s.FileExists(context.Background(), p)
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, os.WriteFile(p, []byte{}, 0o644))
	ok, err = s.FileExists(context.Background(), p)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestLocal_ReadMissing(t *testing.T) {
	s := NewLocal()
	_, err := s.ReadFile(context.Background(), "/definitely/does/not/exist/xyz")
	require.Error(t, err)
}

func TestLocal_InterfaceCompliance(t *testing.T) {
	var _ Session = NewLocal()
	var _ Session = NewLocalWithOpts(Opts{})
	var _ Session = NewSSH("h", "u")
}

func TestLocal_ShellInjectionSafe(t *testing.T) {
	s := NewLocal()
	dir := t.TempDir()
	weird := filepath.Join(dir, "a b c.txt")
	require.NoError(t, s.WriteFile(context.Background(), weird, []byte("ok"), 0o600))
	b, err := s.ReadFile(context.Background(), weird)
	require.NoError(t, err)
	require.Equal(t, "ok", string(b))
}

func TestLocal_RunStderrCaptured(t *testing.T) {
	s := NewLocal()
	_, stderr, err := s.Run(context.Background(), "sh -c 'echo oops 1>&2; exit 3'")
	require.Error(t, err)
	require.True(t, strings.Contains(stderr, "oops"))
}
