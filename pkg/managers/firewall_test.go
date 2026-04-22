package managers

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// fwMockSession is a lightweight scriptable Session for firewall tests.
type fwMockSession struct {
	runs     []string
	sudoRuns []string
	// responses matches commands by prefix; first match wins.
	responses []fwResp
}

type fwResp struct {
	match  string
	stdout string
	stderr string
	err    error
}

func newFWMock() *fwMockSession { return &fwMockSession{} }

func (m *fwMockSession) on(match, stdout string) *fwMockSession {
	m.responses = append(m.responses, fwResp{match: match, stdout: stdout})
	return m
}

func (m *fwMockSession) Host() string { return "mock" }
func (m *fwMockSession) Close() error { return nil }
func (m *fwMockSession) respond(cmd string) (string, string, error) {
	for _, r := range m.responses {
		if strings.Contains(cmd, r.match) {
			return r.stdout, r.stderr, r.err
		}
	}
	return "", "", nil
}
func (m *fwMockSession) Run(_ context.Context, cmd string) (string, string, error) {
	m.runs = append(m.runs, cmd)
	return m.respond(cmd)
}
func (m *fwMockSession) RunSudo(_ context.Context, cmd string) (string, string, error) {
	m.sudoRuns = append(m.sudoRuns, cmd)
	return m.respond(cmd)
}
func (m *fwMockSession) WriteFile(_ context.Context, _ string, _ []byte, _ uint32) error { return nil }
func (m *fwMockSession) ReadFile(_ context.Context, _ string) ([]byte, error)            { return nil, nil }
func (m *fwMockSession) FileExists(_ context.Context, _ string) (bool, error)            { return true, nil }

// ---- Tests ----------------------------------------------------------------

func TestFirewall_Registered(t *testing.T) {
	require.NotNil(t, Lookup("firewall"))
}

func TestFirewall_InterfaceCompliance(t *testing.T) {
	var _ Manager = NewFirewallManager()
}

func TestFirewall_DetectBackendFirewalld(t *testing.T) {
	mock := newFWMock().on("/etc/os-release", `ID="ol"
ID_LIKE="rhel fedora"
`)
	fm := NewFirewallManager().WithSession(mock)
	b, err := fm.detectBackend(context.Background())
	require.NoError(t, err)
	require.Equal(t, FirewallBackendFirewalld, b)
}

func TestFirewall_DetectBackendUFW(t *testing.T) {
	mock := newFWMock().on("/etc/os-release", `ID=ubuntu
`)
	fm := NewFirewallManager().WithSession(mock)
	b, err := fm.detectBackend(context.Background())
	require.NoError(t, err)
	require.Equal(t, FirewallBackendUFW, b)
}

func TestFirewall_PlanDisableWhenActive(t *testing.T) {
	mock := newFWMock().
		on("firewall-cmd --state", "running\n").
		on("firewall-cmd --list-all-zones", "public (active)\n  ports: \n  sources: \n")
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	fw := &config.Firewall{Enabled: false}
	changes, err := fm.Plan(context.Background(), fw, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "update", changes[0].Action)
	after := changes[0].After.(map[string]any)
	require.Equal(t, "disable_firewall", after["op"])
}

func TestFirewall_PlanFirewalldAddsPorts(t *testing.T) {
	mock := newFWMock().
		on("firewall-cmd --state", "running\n").
		on("firewall-cmd --list-all-zones", `public (active)
  ports: 22/tcp
  sources:
`)
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	fw := &config.Firewall{
		Enabled: true,
		Zones: map[string]config.FirewallZone{
			"public": {
				Ports: []config.PortRule{
					{Port: 22, Proto: "tcp"},
					{Port: 443, Proto: "tcp"},
				},
				Sources: []string{"10.0.0.0/24"},
			},
		},
	}
	changes, err := fm.Plan(context.Background(), fw, nil)
	require.NoError(t, err)
	// Expect: add 443/tcp + add source 10.0.0.0/24.
	kinds := map[string]int{}
	for _, c := range changes {
		kinds[c.Action]++
	}
	require.Equal(t, 1, kinds["add_port"])
	require.Equal(t, 1, kinds["add_source"])
}

func TestFirewall_PlanFirewalldRemovesExtras(t *testing.T) {
	mock := newFWMock().
		on("firewall-cmd --state", "running\n").
		on("firewall-cmd --list-all-zones", `public (active)
  ports: 22/tcp 8080/tcp
  sources:
`)
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	fw := &config.Firewall{
		Enabled: true,
		Zones: map[string]config.FirewallZone{
			"public": {Ports: []config.PortRule{{Port: 22, Proto: "tcp"}}},
		},
	}
	changes, err := fm.Plan(context.Background(), fw, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "remove_port", changes[0].Action)
}

func TestFirewall_PlanUFWEnablesWhenInactive(t *testing.T) {
	mock := newFWMock().on("ufw status verbose", "Status: inactive\n")
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendUFW)
	fw := &config.Firewall{
		Enabled: true,
		Zones: map[string]config.FirewallZone{
			"default": {Ports: []config.PortRule{{Port: 22, Proto: "tcp"}}},
		},
	}
	changes, err := fm.Plan(context.Background(), fw, nil)
	require.NoError(t, err)
	// Expect: enable service + add port.
	found := map[string]bool{}
	for _, c := range changes {
		found[c.Action] = true
	}
	require.True(t, found["update"])
	require.True(t, found["add_port"])
}

func TestFirewall_ApplyAddPortFirewalld(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	changes := []Change{
		{Action: "add_port", After: map[string]string{"zone": "public", "port": "443/tcp"}},
	}
	res, err := fm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	require.Len(t, res.Applied, 1)
	joined := strings.Join(mock.sudoRuns, " | ")
	require.Contains(t, joined, "firewall-cmd --permanent --zone='public' --add-port='443'/'tcp'")
	require.Contains(t, joined, "firewall-cmd --reload")
}

func TestFirewall_ApplyAddPortUFW(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendUFW)
	changes := []Change{
		{Action: "add_port", After: map[string]string{"zone": "default", "port": "22/tcp"}},
	}
	res, err := fm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	require.Len(t, res.Applied, 1)
	joined := strings.Join(mock.sudoRuns, " | ")
	require.Contains(t, joined, "ufw allow '22'/'tcp'")
	// No reload on ufw.
	require.NotContains(t, joined, "firewall-cmd --reload")
}

func TestFirewall_ApplyEnable(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	changes := []Change{
		{Action: "update", After: map[string]any{"op": "enable_firewall"}},
	}
	res, err := fm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	require.Len(t, res.Applied, 1)
	require.Contains(t, strings.Join(mock.sudoRuns, " | "), "systemctl enable --now firewalld")
}

func TestFirewall_ApplyDryRun(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	changes := []Change{{Action: "add_port", After: map[string]string{"zone": "public", "port": "80/tcp"}}}
	res, err := fm.Apply(context.Background(), changes, true)
	require.NoError(t, err)
	require.Len(t, res.Skipped, 1)
	require.Empty(t, mock.sudoRuns)
}

func TestFirewall_VerifyOK(t *testing.T) {
	mock := newFWMock().
		on("firewall-cmd --state", "running\n").
		on("firewall-cmd --list-all-zones", `public (active)
  ports: 22/tcp
  sources:
`)
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	fw := &config.Firewall{
		Enabled: true,
		Zones: map[string]config.FirewallZone{
			"public": {Ports: []config.PortRule{{Port: 22, Proto: "tcp"}}},
		},
	}
	res, err := fm.Verify(context.Background(), fw)
	require.NoError(t, err)
	require.True(t, res.OK)
}

func TestFirewall_Rollback(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	changes := []Change{
		{Action: "add_port", After: map[string]string{"zone": "public", "port": "8080/tcp"}},
	}
	require.NoError(t, fm.Rollback(context.Background(), changes))
	require.Contains(t, strings.Join(mock.sudoRuns, " | "), "--remove-port=")
}

func TestFirewall_NoSessionReturnsError(t *testing.T) {
	fm := NewFirewallManager()
	_, err := fm.Plan(context.Background(), &config.Firewall{Enabled: true}, nil)
	require.ErrorIs(t, err, ErrSessionRequired)
}

func TestFirewall_CastNilLinux(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	changes, err := fm.Plan(context.Background(), &config.Linux{}, nil)
	require.NoError(t, err)
	require.Empty(t, changes)
}

func TestFirewall_CastUnsupported(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock)
	_, err := fm.Plan(context.Background(), 42, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported")
}

func TestFirewall_ParseOSReleaseDebianViaIDLike(t *testing.T) {
	id, idLike := parseFirewallOSRelease(`ID=linuxmint
ID_LIKE="ubuntu debian"
`)
	require.Equal(t, "linuxmint", id)
	require.Contains(t, idLike, "ubuntu")
}
