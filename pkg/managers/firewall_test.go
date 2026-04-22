package managers

import (
	"context"
	"errors"
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

// ---- UFW backend ----------------------------------------------------------

func TestFirewall_UFW_ParseStatusActive(t *testing.T) {
	body := `Status: active

To                         Action      From
--                         ------      ----
22/tcp                     ALLOW       Anywhere
443/tcp                    ALLOW       Anywhere
`
	cf := currentFirewall{Ports: map[string][]string{}, Sources: map[string][]string{}}
	parseUFWStatus(body, &cf)
	require.True(t, cf.Active)
	require.ElementsMatch(t, []string{"22/tcp", "443/tcp"}, cf.Ports["default"])
}

func TestFirewall_UFW_ParseStatusInactive(t *testing.T) {
	cf := currentFirewall{Ports: map[string][]string{}, Sources: map[string][]string{}}
	parseUFWStatus("Status: inactive\n", &cf)
	require.False(t, cf.Active)
	require.Empty(t, cf.Ports["default"])
}

func TestFirewall_UFW_Snapshot(t *testing.T) {
	mock := newFWMock().on("ufw status verbose", "Status: active\n22/tcp ALLOW Anywhere\n")
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendUFW)
	cf, err := fm.snapshot(context.Background())
	require.NoError(t, err)
	require.True(t, cf.Active)
	require.Contains(t, cf.Ports["default"], "22/tcp")
}

func TestFirewall_UFW_PlanFreshMachine(t *testing.T) {
	// Fresh Ubuntu: ufw inactive, desired enabled with ports. Expect enable + add_port.
	mock := newFWMock().on("ufw status verbose", "Status: inactive\n")
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendUFW)
	fw := &config.Firewall{
		Enabled: true,
		Zones: map[string]config.FirewallZone{
			"default": {Ports: []config.PortRule{{Port: 22, Proto: "tcp"}, {Port: 80, Proto: "tcp"}}},
		},
	}
	changes, err := fm.Plan(context.Background(), fw, nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(changes), 3)
}

func TestFirewall_UFW_PlanIdle(t *testing.T) {
	mock := newFWMock().on("ufw status verbose", `Status: active

22/tcp ALLOW Anywhere
`)
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendUFW)
	fw := &config.Firewall{
		Enabled: true,
		Zones: map[string]config.FirewallZone{
			"default": {Ports: []config.PortRule{{Port: 22, Proto: "tcp"}}},
		},
	}
	changes, err := fm.Plan(context.Background(), fw, nil)
	require.NoError(t, err)
	require.Empty(t, changes)
}

func TestFirewall_UFW_Enable(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendUFW)
	require.NoError(t, fm.enable(context.Background(), FirewallBackendUFW))
	require.Contains(t, strings.Join(mock.sudoRuns, " | "), "ufw --force enable")
}

func TestFirewall_UFW_Disable(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendUFW)
	require.NoError(t, fm.disable(context.Background(), FirewallBackendUFW))
	require.Contains(t, strings.Join(mock.sudoRuns, " | "), "ufw --force disable")
}

func TestFirewall_Firewalld_Disable(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	require.NoError(t, fm.disable(context.Background(), FirewallBackendFirewalld))
	require.Contains(t, strings.Join(mock.sudoRuns, " | "), "systemctl disable --now firewalld")
}

func TestFirewall_Nftables_EnableDisable(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendNftables)
	require.NoError(t, fm.enable(context.Background(), FirewallBackendNftables))
	require.NoError(t, fm.disable(context.Background(), FirewallBackendNftables))
	joined := strings.Join(mock.sudoRuns, " | ")
	require.Contains(t, joined, "enable --now nftables")
	require.Contains(t, joined, "disable --now nftables")
}

func TestFirewall_EnableDisable_UnsupportedBackend(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock)
	require.Error(t, fm.enable(context.Background(), FirewallBackend("bogus")))
	require.Error(t, fm.disable(context.Background(), FirewallBackend("bogus")))
}

func TestFirewall_UFW_AddRemovePort(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendUFW)
	require.NoError(t, fm.portOp(context.Background(), FirewallBackendUFW, "add_port", "default", "80/tcp"))
	require.NoError(t, fm.portOp(context.Background(), FirewallBackendUFW, "remove_port", "default", "80/tcp"))
	joined := strings.Join(mock.sudoRuns, " | ")
	require.Contains(t, joined, "ufw allow '80'/'tcp'")
	require.Contains(t, joined, "ufw delete allow '80'/'tcp'")
}

func TestFirewall_UFW_AddRemoveSource(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendUFW)
	require.NoError(t, fm.sourceOp(context.Background(), FirewallBackendUFW, "add_source", "default", "10.0.0.0/24"))
	require.NoError(t, fm.sourceOp(context.Background(), FirewallBackendUFW, "remove_source", "default", "10.0.0.0/24"))
	joined := strings.Join(mock.sudoRuns, " | ")
	require.Contains(t, joined, "ufw allow from '10.0.0.0/24'")
	require.Contains(t, joined, "ufw delete allow from '10.0.0.0/24'")
}

func TestFirewall_Firewalld_AddRemoveSource(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	require.NoError(t, fm.sourceOp(context.Background(), FirewallBackendFirewalld, "add_source", "public", "10.0.0.0/24"))
	require.NoError(t, fm.sourceOp(context.Background(), FirewallBackendFirewalld, "remove_source", "public", "10.0.0.0/24"))
	joined := strings.Join(mock.sudoRuns, " | ")
	require.Contains(t, joined, "--add-source='10.0.0.0/24'")
	require.Contains(t, joined, "--remove-source='10.0.0.0/24'")
}

func TestFirewall_PortOp_UnsupportedBackend(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock)
	err := fm.portOp(context.Background(), FirewallBackend("bogus"), "add_port", "public", "22/tcp")
	require.Error(t, err)
}

func TestFirewall_PortOp_MalformedPort(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	err := fm.portOp(context.Background(), FirewallBackendFirewalld, "add_port", "public", "noproto")
	require.Error(t, err)
	require.Contains(t, err.Error(), "malformed")
}

func TestFirewall_SourceOp_UnsupportedBackend(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock)
	err := fm.sourceOp(context.Background(), FirewallBackend("bogus"), "add_source", "public", "1.2.3.4/32")
	require.Error(t, err)
}

func TestFirewall_ApplyOne_UnknownAction(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	err := fm.applyOne(context.Background(), FirewallBackendFirewalld, Change{Action: "mystery"})
	require.Error(t, err)
}

func TestFirewall_ApplyOne_UnknownUpdateOp(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	err := fm.applyOne(context.Background(), FirewallBackendFirewalld,
		Change{Action: "update", After: map[string]any{"op": "bogus"}})
	require.Error(t, err)
}

func TestFirewall_ApplyDisable(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendUFW)
	changes := []Change{{Action: "update", After: map[string]any{"op": "disable_firewall"}}}
	res, err := fm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	require.Len(t, res.Applied, 1)
	require.Contains(t, strings.Join(mock.sudoRuns, " | "), "ufw --force disable")
}

func TestFirewall_ApplyNoSession(t *testing.T) {
	fm := NewFirewallManager()
	_, err := fm.Apply(context.Background(), []Change{{Action: "add_port"}}, false)
	require.ErrorIs(t, err, ErrSessionRequired)
}

func TestFirewall_RollbackNoSession(t *testing.T) {
	fm := NewFirewallManager()
	err := fm.Rollback(context.Background(), []Change{{Action: "add_port"}})
	require.ErrorIs(t, err, ErrSessionRequired)
}

func TestFirewall_RollbackReversesAll(t *testing.T) {
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendUFW)
	changes := []Change{
		{Action: "add_port", After: map[string]string{"zone": "default", "port": "80/tcp"}},
		{Action: "remove_port", Before: map[string]string{"zone": "default", "port": "22/tcp"}},
		{Action: "add_source", After: map[string]string{"zone": "default", "source": "1.2.3.4/32"}},
		{Action: "remove_source", Before: map[string]string{"zone": "default", "source": "5.6.7.8/32"}},
		{Action: "update", After: map[string]any{"op": "enable_firewall"}},
		{Action: "update", After: map[string]any{"op": "disable_firewall"}},
		{Action: "unknown"},
	}
	require.NoError(t, fm.Rollback(context.Background(), changes))
	joined := strings.Join(mock.sudoRuns, " | ")
	require.Contains(t, joined, "ufw delete allow '80'/'tcp'") // add_port reversed
	require.Contains(t, joined, "ufw allow '22'/'tcp'")        // remove_port reversed
	require.Contains(t, joined, "ufw delete allow from '1.2.3.4/32'")
	require.Contains(t, joined, "ufw allow from '5.6.7.8/32'")
	require.Contains(t, joined, "ufw --force disable") // rollback of enable
	require.Contains(t, joined, "ufw --force enable")  // rollback of disable
}

func TestFirewall_DetectBackend_FromCommand(t *testing.T) {
	// no /etc/os-release match, falls through to command -v checks
	mock := newFWMock().on("/etc/os-release", "ID=weird\n")
	fm := NewFirewallManager().WithSession(mock)
	b, err := fm.detectBackend(context.Background())
	require.NoError(t, err)
	// mock.Run returns nil error for any command-v, so firewall-cmd wins first
	require.Equal(t, FirewallBackendFirewalld, b)
}

func TestFirewall_DetectBackend_NftablesFallback(t *testing.T) {
	mock := newFWMock()
	// /etc/os-release errors → skip to command -v probes.
	mock.responses = append(mock.responses,
		fwResp{match: "/etc/os-release", err: errors.New("read err")},
		fwResp{match: "command -v firewall-cmd", err: errors.New("not found")},
		fwResp{match: "command -v ufw", err: errors.New("not found")})
	fm := NewFirewallManager().WithSession(mock)
	b, err := fm.detectBackend(context.Background())
	require.NoError(t, err)
	require.Equal(t, FirewallBackendNftables, b)
}

func TestFirewall_DetectBackend_UFWFromCommand(t *testing.T) {
	mock := newFWMock()
	mock.responses = append(mock.responses,
		fwResp{match: "/etc/os-release", err: errors.New("read err")},
		fwResp{match: "command -v firewall-cmd", err: errors.New("not found")})
	fm := NewFirewallManager().WithSession(mock)
	b, err := fm.detectBackend(context.Background())
	require.NoError(t, err)
	require.Equal(t, FirewallBackendUFW, b)
}

func TestFirewall_ChooseMap_BothNilReturnsEmpty(t *testing.T) {
	m, ok := chooseMap(Change{})
	require.False(t, ok)
	require.Empty(t, m)
}

func TestFirewall_Snapshot_FirewalldInactive(t *testing.T) {
	mock := newFWMock().on("firewall-cmd --state", "not running\n")
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	cf, err := fm.snapshot(context.Background())
	require.NoError(t, err)
	require.False(t, cf.Active)
}

func TestFirewall_Snapshot_Nftables(t *testing.T) {
	mock := newFWMock().on("is-active nftables", "active\n")
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendNftables)
	cf, err := fm.snapshot(context.Background())
	require.NoError(t, err)
	require.True(t, cf.Active)
}

func TestFirewall_Snapshot_FirewalldZonesErrReturnsEmpty(t *testing.T) {
	mock := newFWMock()
	mock.responses = append(mock.responses,
		fwResp{match: "firewall-cmd --state", stdout: "running\n"},
		fwResp{match: "firewall-cmd --list-all-zones", err: errors.New("boom")})
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	cf, err := fm.snapshot(context.Background())
	require.NoError(t, err)
	require.True(t, cf.Active)
	require.Empty(t, cf.Ports)
}

func TestFirewall_Snapshot_DetectFromScratch(t *testing.T) {
	// No backend set; forces detectBackend — mock returns ID=ubuntu → UFW.
	mock := newFWMock().on("/etc/os-release", "ID=ubuntu\n").
		on("ufw status verbose", "Status: inactive\n")
	fm := NewFirewallManager().WithSession(mock)
	cf, err := fm.snapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, FirewallBackendUFW, cf.Backend)
}

func TestFirewall_PortSet_Range(t *testing.T) {
	s := portSet([]config.PortRule{
		{Range: "8000-8100", Proto: "tcp"},
		{Port: 22},          // default proto tcp
		{Proto: "udp"},      // neither port nor range → skipped
	})
	require.True(t, s["8000-8100/tcp"])
	require.True(t, s["22/tcp"])
	require.Len(t, s, 2)
}

func TestFirewall_Apply_DetectErrors(t *testing.T) {
	// Without backend override, detectBackend falls back to Nftables when no hints.
	mock := newFWMock()
	fm := NewFirewallManager().WithSession(mock)
	changes := []Change{{Action: "add_port", After: map[string]string{"zone": "default", "port": "80/tcp"}}}
	_, err := fm.Apply(context.Background(), changes, false)
	// Nftables doesn't support port ops → failure expected.
	require.NoError(t, err)
}

func TestFirewall_Apply_ReloadError(t *testing.T) {
	mock := newFWMock()
	// Make the reload fail.
	mock.responses = append(mock.responses, fwResp{match: "firewall-cmd --reload", err: errors.New("reload fail")})
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	changes := []Change{{Action: "add_port", After: map[string]string{"zone": "public", "port": "80/tcp"}}}
	_, err := fm.Apply(context.Background(), changes, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reload")
}

func TestFirewall_Apply_PortOpFails(t *testing.T) {
	mock := newFWMock()
	mock.responses = append(mock.responses, fwResp{match: "firewall-cmd --permanent", err: errors.New("zone missing")})
	fm := NewFirewallManager().WithSession(mock).WithBackend(FirewallBackendFirewalld)
	changes := []Change{{Action: "add_port", After: map[string]string{"zone": "public", "port": "80/tcp"}}}
	res, _ := fm.Apply(context.Background(), changes, false)
	require.Len(t, res.Failed, 1)
}

func TestFirewall_CastVariants(t *testing.T) {
	// config.Firewall value + pointer + config.Linux value
	fw := config.Firewall{Enabled: true}
	got, err := castFirewall(fw)
	require.NoError(t, err)
	require.NotNil(t, got)

	got2, err := castFirewall(&fw)
	require.NoError(t, err)
	require.NotNil(t, got2)

	lin := config.Linux{Firewall: &fw}
	got3, err := castFirewall(lin)
	require.NoError(t, err)
	require.NotNil(t, got3)

	got4, err := castFirewall(nil)
	require.NoError(t, err)
	require.Nil(t, got4)

	var np *config.Linux
	got5, err := castFirewall(np)
	require.NoError(t, err)
	require.Nil(t, got5)
}
