package managers

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// netMockSession is a scriptable session for network tests.
type netMockSession struct {
	runs      []string
	sudoRuns  []string
	writes    map[string][]byte
	fileData  map[string][]byte
	responses []fwResp // reuse fwResp match/stdout struct
}

func newNetMock() *netMockSession {
	return &netMockSession{
		writes:   map[string][]byte{},
		fileData: map[string][]byte{},
	}
}

func (m *netMockSession) on(match, stdout string) *netMockSession {
	m.responses = append(m.responses, fwResp{match: match, stdout: stdout})
	return m
}

func (*netMockSession) Host() string { return "mock" }
func (*netMockSession) Close() error { return nil }
func (m *netMockSession) respond(cmd string) (string, string, error) {
	for _, r := range m.responses {
		if strings.Contains(cmd, r.match) {
			return r.stdout, r.stderr, r.err
		}
	}
	return "", "", nil
}
func (m *netMockSession) Run(_ context.Context, cmd string) (string, string, error) {
	m.runs = append(m.runs, cmd)
	return m.respond(cmd)
}
func (m *netMockSession) RunSudo(_ context.Context, cmd string) (string, string, error) {
	m.sudoRuns = append(m.sudoRuns, cmd)
	return m.respond(cmd)
}
func (m *netMockSession) WriteFile(_ context.Context, path string, content []byte, _ uint32) error {
	m.writes[path] = content
	m.fileData[path] = content
	return nil
}
func (m *netMockSession) ReadFile(_ context.Context, path string) ([]byte, error) {
	if b, ok := m.fileData[path]; ok {
		return b, nil
	}
	return nil, nil
}
func (m *netMockSession) FileExists(_ context.Context, path string) (bool, error) {
	_, ok := m.fileData[path]
	return ok, nil
}

// ---- Tests ----------------------------------------------------------------

func TestNetwork_Registered(t *testing.T) {
	require.NotNil(t, Lookup("network"))
}

func TestNetwork_InterfaceCompliance(t *testing.T) {
	var _ Manager = NewNetworkManager()
}

func TestNetwork_PlanHostnameChange(t *testing.T) {
	mock := newNetMock().on("hostname", "old-host\n")
	nm := NewNetworkManager().WithSession(mock)
	ns := &NetworkSpec{Hostname: "rac1.example.com"}
	changes, err := nm.Plan(context.Background(), ns, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "hostname", changes[0].Target)
	require.Equal(t, "rac1.example.com", changes[0].After)
}

func TestNetwork_PlanHostnameNoOp(t *testing.T) {
	mock := newNetMock().on("hostname", "rac1.example.com\n")
	nm := NewNetworkManager().WithSession(mock)
	ns := &NetworkSpec{Hostname: "rac1.example.com"}
	changes, err := nm.Plan(context.Background(), ns, nil)
	require.NoError(t, err)
	require.Empty(t, changes)
}

func TestNetwork_PlanDNSChange(t *testing.T) {
	mock := newNetMock().on("hostname", "rac1\n")
	mock.fileData[resolvConfPath] = []byte("nameserver 8.8.8.8\n")
	nm := NewNetworkManager().WithSession(mock)
	ns := &NetworkSpec{
		Hostname:   "rac1",
		DNSServers: []string{"10.0.0.1", "10.0.0.2"},
		Search:     []string{"example.com"},
	}
	changes, err := nm.Plan(context.Background(), ns, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, resolvConfPath, changes[0].Target)
}

func TestNetwork_PlanDNSNoOp(t *testing.T) {
	mock := newNetMock().on("hostname", "rac1\n")
	mock.fileData[resolvConfPath] = []byte("search example.com\nnameserver 10.0.0.1\nnameserver 10.0.0.2\n")
	nm := NewNetworkManager().WithSession(mock)
	ns := &NetworkSpec{
		Hostname:   "rac1",
		DNSServers: []string{"10.0.0.1", "10.0.0.2"},
		Search:     []string{"example.com"},
	}
	changes, err := nm.Plan(context.Background(), ns, nil)
	require.NoError(t, err)
	require.Empty(t, changes)
}

func TestNetwork_ApplyHostname(t *testing.T) {
	mock := newNetMock()
	nm := NewNetworkManager().WithSession(mock)
	changes := []Change{
		{Action: "update", Target: "hostname", Before: "old", After: "new-host"},
	}
	res, err := nm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	require.Len(t, res.Applied, 1)
	require.Contains(t, strings.Join(mock.sudoRuns, " | "), "hostnamectl set-hostname 'new-host'")
}

func TestNetwork_ApplyResolvConf(t *testing.T) {
	mock := newNetMock()
	nm := NewNetworkManager().WithSession(mock)
	changes := []Change{
		{Action: "update", Target: resolvConfPath, After: map[string]any{
			"servers": []string{"1.1.1.1"},
			"search":  []string{"example.com"},
		}},
	}
	res, err := nm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	require.Len(t, res.Applied, 1)
	written := string(mock.writes[resolvConfPath])
	require.Contains(t, written, "nameserver 1.1.1.1")
	require.Contains(t, written, "search example.com")
	require.Contains(t, written, "Managed by linuxctl")
}

func TestNetwork_ApplyDryRun(t *testing.T) {
	mock := newNetMock()
	nm := NewNetworkManager().WithSession(mock)
	changes := []Change{{Action: "update", Target: "hostname", After: "x"}}
	res, err := nm.Apply(context.Background(), changes, true)
	require.NoError(t, err)
	require.Len(t, res.Skipped, 1)
	require.Empty(t, mock.sudoRuns)
}

func TestNetwork_VerifyOK(t *testing.T) {
	mock := newNetMock().on("hostname", "rac1\n")
	nm := NewNetworkManager().WithSession(mock)
	res, err := nm.Verify(context.Background(), &NetworkSpec{Hostname: "rac1"})
	require.NoError(t, err)
	require.True(t, res.OK)
}

func TestNetwork_Rollback(t *testing.T) {
	mock := newNetMock()
	nm := NewNetworkManager().WithSession(mock)
	changes := []Change{
		{Action: "update", Target: "hostname", Before: "old", After: "new"},
	}
	require.NoError(t, nm.Rollback(context.Background(), changes))
	require.Contains(t, strings.Join(mock.sudoRuns, " | "), "'old'")
}

func TestNetwork_NoSessionReturnsError(t *testing.T) {
	nm := NewNetworkManager()
	_, err := nm.Plan(context.Background(), &NetworkSpec{Hostname: "x"}, nil)
	require.ErrorIs(t, err, ErrSessionRequired)
}

func TestNetwork_CastNil(t *testing.T) {
	mock := newNetMock()
	nm := NewNetworkManager().WithSession(mock)
	changes, err := nm.Plan(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Empty(t, changes)
}

func TestNetwork_CastUnsupported(t *testing.T) {
	mock := newNetMock()
	nm := NewNetworkManager().WithSession(mock)
	_, err := nm.Plan(context.Background(), 42, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported")
}

func TestNetwork_RenderResolvConfEmpty(t *testing.T) {
	body := renderResolvConf(nil, nil)
	require.Contains(t, body, "Managed by linuxctl")
	require.NotContains(t, body, "nameserver")
}
