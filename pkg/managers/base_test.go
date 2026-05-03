package managers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// baseMockSession is a minimal Session stub for base_test helpers.
type baseMockSession struct {
	runOut, runErrOut string
	runErr            error
	sudoOut, sudoErr  string
	sudoErrVal        error
}

func (m *baseMockSession) Host() string { return "mock" }
func (m *baseMockSession) Close() error { return nil }
func (m *baseMockSession) Run(_ context.Context, _ string) (string, string, error) {
	return m.runOut, m.runErrOut, m.runErr
}
func (m *baseMockSession) RunSudo(_ context.Context, _ string) (string, string, error) {
	return m.sudoOut, m.sudoErr, m.sudoErrVal
}
func (m *baseMockSession) WriteFile(context.Context, string, []byte, uint32) error { return nil }
func (m *baseMockSession) ReadFile(context.Context, string) ([]byte, error)        { return nil, nil }
func (m *baseMockSession) FileExists(context.Context, string) (bool, error)        { return true, nil }

// TestAllManagers_DependsOn asserts the dependency graph + Name for every
// registered manager. It keeps DependsOn accessors from silently changing
// and ensures the init() Register calls all run.
func TestAllManagers_DependsOn(t *testing.T) {
	cases := map[string]struct {
		deps []string
	}{
		"dir":      {deps: []string{"mount"}},
		"disk":     {deps: nil},
		"firewall": {deps: []string{"network", "package"}},
		"hosts":    {deps: nil},
		"limits":   {deps: []string{"sysctl", "user"}},
		"mount":    {deps: []string{"disk"}},
		"network":  {deps: []string{"hosts"}},
		"package":  {deps: nil},
		"selinux":  {deps: []string{"package"}},
		"service":  {deps: []string{"package", "ssh"}},
		"ssh":      {deps: []string{"user"}},
		"sysctl":   {deps: nil},
		"user":     {deps: []string{"package"}},
		"repo":     {deps: nil},
		"file":     {deps: nil},
	}

	all := All()
	require.Len(t, all, len(cases), "All() count mismatch")

	for name, want := range cases {
		m, ok := all[name]
		require.True(t, ok, "manager %q not registered", name)
		require.Equal(t, name, m.Name(), "Name() mismatch for %q", name)
		require.Equal(t, want.deps, m.DependsOn(), "DependsOn() mismatch for %q", name)
	}
}

func TestRegister_NilIsNoop(t *testing.T) {
	before := len(All())
	Register(nil)
	require.Equal(t, before, len(All()))
}

func TestAll_ReturnsSnapshot(t *testing.T) {
	a := All()
	delete(a, "disk")
	// mutating the returned map must not affect the registry
	require.NotNil(t, Lookup("disk"))
}

func TestRunAndCheck_NilSession(t *testing.T) {
	_, err := RunAndCheck(context.Background(), nil, "whoami")
	require.ErrorIs(t, err, ErrSessionRequired)
}

func TestRunAndCheck_Success(t *testing.T) {
	s := &baseMockSession{runOut: "hello\n"}
	out, err := RunAndCheck(context.Background(), s, "echo hello")
	require.NoError(t, err)
	require.Equal(t, "hello\n", out)
}

func TestRunAndCheck_ErrorWrapsStderr(t *testing.T) {
	s := &baseMockSession{runErr: errors.New("boom"), runErrOut: "something broke"}
	_, err := RunAndCheck(context.Background(), s, "false")
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
	require.Contains(t, err.Error(), "something broke")
}

func TestRunSudoAndCheck_NilSession(t *testing.T) {
	_, err := RunSudoAndCheck(context.Background(), nil, "whoami")
	require.ErrorIs(t, err, ErrSessionRequired)
}

func TestRunSudoAndCheck_ErrorWrapsStderr(t *testing.T) {
	s := &baseMockSession{sudoErrVal: errors.New("denied"), sudoErr: "Permission denied"}
	_, err := RunSudoAndCheck(context.Background(), s, "true")
	require.Error(t, err)
	require.Contains(t, err.Error(), "denied")
	require.Contains(t, err.Error(), "Permission denied")
}

func TestTrimStderr_Truncation(t *testing.T) {
	s := strings.Repeat("a", 250)
	got := trimStderr(s + "   ")
	require.True(t, strings.HasSuffix(got, "…"))
	require.LessOrEqual(t, len(got), 210)
}

func TestTrimStderr_Short(t *testing.T) {
	require.Equal(t, "ok", trimStderr("  ok  "))
}

func TestShellQuoteOne(t *testing.T) {
	require.Equal(t, "'foo'", shellQuoteOne("foo"))
	require.Equal(t, `'o'"'"'brien'`, shellQuoteOne("o'brien"))
}

// TestCastsAcceptValueLinux exercises cast* helpers against the value form of
// config.Linux for managers that support it (covers a branch each cast leaves
// when tests only exercise the pointer form).
func TestCastsAcceptValueLinux(t *testing.T) {
	// DirManager: value *config.Linux is tested elsewhere; here just the
	// nil-pointer fall-through.
	if _, err := castDirectories((*config.Linux)(nil)); err != nil {
		t.Error(err)
	}
	// Hosts
	if _, err := castHostEntries((*config.Linux)(nil)); err != nil {
		t.Error(err)
	}
}

// TestAllManagers_Verify_PropagatesPlanError ensures every manager's Verify
// surfaces the error produced by Plan (e.g. unsupported spec type), not just
// succeeds with empty drift.
func TestAllManagers_Verify_PropagatesPlanError(t *testing.T) {
	// Bogus spec values — most managers return "unsupported" from their cast.
	cases := []struct {
		name string
		mgr  Manager
		spec Spec
	}{
		{"firewall", NewFirewallManager().WithSession(newFWMock()), 42},
		{"hosts", NewHostsManager().WithSession(newHostsMock("")), 42},
		{"dir", NewDirManager().WithSession(newDirMockSession()), 42},
		{"network", NewNetworkManager().WithSession(newNetMock()), 42},
		{"service", NewServiceManager().WithSession(newSvcMock()), 42},
		{"ssh", NewSSHAuthManager().WithSession(newMockSession()), "bogus"},
		{"selinux", NewSELinuxManager().WithSession(newMockSession()), "bogus"},
		{"user", NewUserManager().WithSession(newMockSession()), "bogus"},
		{"sysctl", NewSysctlManager().WithSession(newFileMock()), "bogus"},
		{"limits", NewLimitsManager().WithSession(newFileMock()), "bogus"},
		{"package", NewPackageManager().WithSession(newMockSession()), "bogus"},
		{"disk", NewDiskManager().WithSession(newFullMock()), "bogus"},
		{"mount", NewMountManager().WithSession(newFullMock()), "bogus"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.mgr.Verify(context.Background(), tc.spec)
			require.Error(t, err, "%s.Verify should propagate Plan error", tc.name)
		})
	}
}
