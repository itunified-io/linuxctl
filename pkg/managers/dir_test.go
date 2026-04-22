package managers

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// dirMockSession is a scriptable session.Session implementation for dir tests.
// Commands are matched by substring; the first matching handler wins.
type dirMockSession struct {
	runs     []string
	sudoRuns []string
	handler  func(cmd string, sudo bool) (string, string, error)
	exists   map[string]bool
}

func newDirMockSession() *dirMockSession {
	return &dirMockSession{exists: map[string]bool{}}
}

func (m *dirMockSession) Host() string { return "mock" }
func (m *dirMockSession) Close() error { return nil }
func (m *dirMockSession) recordRun(cmd string, sudo bool) {
	if sudo {
		m.sudoRuns = append(m.sudoRuns, cmd)
	} else {
		m.runs = append(m.runs, cmd)
	}
}

func (m *dirMockSession) Run(_ context.Context, cmd string) (string, string, error) {
	m.recordRun(cmd, false)
	if m.handler != nil {
		return m.handler(cmd, false)
	}
	return "", "", nil
}

func (m *dirMockSession) RunSudo(_ context.Context, cmd string) (string, string, error) {
	m.recordRun(cmd, true)
	if m.handler != nil {
		return m.handler(cmd, true)
	}
	return "", "", nil
}

func (m *dirMockSession) WriteFile(_ context.Context, path string, _ []byte, _ uint32) error {
	m.exists[path] = true
	return nil
}

func (m *dirMockSession) ReadFile(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

func (m *dirMockSession) FileExists(_ context.Context, path string) (bool, error) {
	return m.exists[path], nil
}

// ---- Tests ----------------------------------------------------------------

func TestDir_PlanCreatesMissing(t *testing.T) {
	mock := newDirMockSession()
	dm := NewDirManager().WithSession(mock)
	desired := []config.Directory{
		{Path: "/opt/oracle", Owner: "oracle", Group: "oinstall", Mode: "0755"},
	}
	changes, err := dm.Plan(context.Background(), desired, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "create", changes[0].Action)
	require.Equal(t, "/opt/oracle", changes[0].Target)
}

func TestDir_PlanDetectsModeDrift(t *testing.T) {
	mock := newDirMockSession()
	mock.exists["/opt/oracle"] = true
	mock.handler = func(cmd string, _ bool) (string, string, error) {
		if strings.HasPrefix(cmd, "stat -c") {
			return "oracle oinstall 750\n", "", nil
		}
		return "", "", nil
	}
	dm := NewDirManager().WithSession(mock)
	desired := []config.Directory{
		{Path: "/opt/oracle", Owner: "oracle", Group: "oinstall", Mode: "0755"},
	}
	changes, err := dm.Plan(context.Background(), desired, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "update", changes[0].Action)
}

func TestDir_PlanNoDriftEmpty(t *testing.T) {
	mock := newDirMockSession()
	mock.exists["/opt/oracle"] = true
	mock.handler = func(cmd string, _ bool) (string, string, error) {
		if strings.HasPrefix(cmd, "stat -c") {
			return "oracle oinstall 755\n", "", nil
		}
		return "", "", nil
	}
	dm := NewDirManager().WithSession(mock)
	desired := []config.Directory{
		{Path: "/opt/oracle", Owner: "oracle", Group: "oinstall", Mode: "0755"},
	}
	changes, err := dm.Plan(context.Background(), desired, nil)
	require.NoError(t, err)
	require.Empty(t, changes)
}

func TestDir_ApplyCreate(t *testing.T) {
	mock := newDirMockSession()
	dm := NewDirManager().WithSession(mock)
	d := config.Directory{Path: "/opt/oracle", Owner: "oracle", Group: "oinstall", Mode: "0755"}
	changes := []Change{{ID: "dir:/opt/oracle", Target: "/opt/oracle", Action: "create", After: d}}
	res, err := dm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	require.Len(t, res.Applied, 1)
	require.Empty(t, res.Failed)
	joined := strings.Join(mock.sudoRuns, " | ")
	require.Contains(t, joined, "mkdir -p")
	require.Contains(t, joined, "chown")
	require.Contains(t, joined, "chmod 0755")
}

func TestDir_ApplyDryRun(t *testing.T) {
	mock := newDirMockSession()
	dm := NewDirManager().WithSession(mock)
	changes := []Change{{Action: "create", After: config.Directory{Path: "/x"}}}
	res, err := dm.Apply(context.Background(), changes, true)
	require.NoError(t, err)
	require.Len(t, res.Skipped, 1)
	require.Empty(t, mock.runs)
	require.Empty(t, mock.sudoRuns)
}

func TestDir_ApplyRecursive(t *testing.T) {
	mock := newDirMockSession()
	dm := NewDirManager().WithSession(mock)
	d := config.Directory{Path: "/srv/x", Owner: "app", Group: "app", Mode: "0775", Recursive: true}
	changes := []Change{{Action: "create", After: d}}
	_, err := dm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	joined := strings.Join(mock.sudoRuns, " | ")
	require.Contains(t, joined, "chown -R")
	require.Contains(t, joined, "chmod -R")
}

func TestDir_ApplyBadMode(t *testing.T) {
	mock := newDirMockSession()
	dm := NewDirManager().WithSession(mock)
	d := config.Directory{Path: "/x", Mode: "09ZZ"}
	changes := []Change{{Action: "create", After: d}}
	res, err := dm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	require.Len(t, res.Failed, 1)
	require.Contains(t, res.Failed[0].Err.Error(), "invalid mode")
}

func TestDir_VerifyOK(t *testing.T) {
	mock := newDirMockSession()
	mock.exists["/opt/oracle"] = true
	mock.handler = func(cmd string, _ bool) (string, string, error) {
		return "oracle oinstall 755\n", "", nil
	}
	dm := NewDirManager().WithSession(mock)
	desired := []config.Directory{
		{Path: "/opt/oracle", Owner: "oracle", Group: "oinstall", Mode: "0755"},
	}
	res, err := dm.Verify(context.Background(), desired)
	require.NoError(t, err)
	require.True(t, res.OK)
	require.Empty(t, res.Drift)
}

func TestDir_Rollback(t *testing.T) {
	mock := newDirMockSession()
	dm := NewDirManager().WithSession(mock)
	changes := []Change{
		{Action: "create", After: config.Directory{Path: "/a"}},
		{Action: "update", After: config.Directory{Path: "/b"}},
	}
	require.NoError(t, dm.Rollback(context.Background(), changes))
	joined := strings.Join(mock.sudoRuns, " | ")
	require.Contains(t, joined, "rmdir")
	require.Contains(t, joined, "/a")
	require.NotContains(t, joined, "rmdir '/b'")
}

func TestDir_NoSessionReturnsError(t *testing.T) {
	dm := NewDirManager()
	_, err := dm.Plan(context.Background(), []config.Directory{{Path: "/x"}}, nil)
	require.ErrorIs(t, err, ErrSessionRequired)
	_, err = dm.Apply(context.Background(), []Change{{Action: "create", After: config.Directory{Path: "/x"}}}, false)
	require.ErrorIs(t, err, ErrSessionRequired)
	require.ErrorIs(t, dm.Rollback(context.Background(), nil), ErrSessionRequired)
}

func TestDir_CastUnsupported(t *testing.T) {
	mock := newDirMockSession()
	dm := NewDirManager().WithSession(mock)
	_, err := dm.Plan(context.Background(), 42, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported")
}

func TestDir_CastFromLinux(t *testing.T) {
	mock := newDirMockSession()
	dm := NewDirManager().WithSession(mock)
	l := &config.Linux{Directories: []config.Directory{{Path: "/opt/x"}}}
	changes, err := dm.Plan(context.Background(), l, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
}

func TestDir_Registered(t *testing.T) {
	require.NotNil(t, Lookup("dir"))
	require.Equal(t, "dir", Lookup("dir").Name())
}

func TestDir_InterfaceCompliance(t *testing.T) {
	var _ Manager = NewDirManager()
}
