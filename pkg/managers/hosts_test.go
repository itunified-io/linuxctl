package managers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// hostsMockSession is an in-memory session stub for hosts tests.
type hostsMockSession struct {
	content  []byte
	writes   map[string][]byte
	readErr  error
	writeErr error
}

func newHostsMock(initial string) *hostsMockSession {
	return &hostsMockSession{content: []byte(initial), writes: map[string][]byte{}}
}

func (*hostsMockSession) Host() string { return "mock" }
func (*hostsMockSession) Close() error { return nil }
func (m *hostsMockSession) Run(_ context.Context, _ string) (string, string, error) {
	return "", "", nil
}
func (m *hostsMockSession) RunSudo(_ context.Context, _ string) (string, string, error) {
	return "", "", nil
}
func (m *hostsMockSession) WriteFile(_ context.Context, path string, content []byte, _ uint32) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.writes[path] = content
	if path == hostsPath {
		m.content = content
	}
	return nil
}
func (m *hostsMockSession) ReadFile(_ context.Context, path string) ([]byte, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	if path == hostsPath {
		return m.content, nil
	}
	return nil, nil
}
func (m *hostsMockSession) FileExists(_ context.Context, path string) (bool, error) {
	if path == hostsPath {
		return m.content != nil, nil
	}
	return false, nil
}

// ---- Tests ----------------------------------------------------------------

func TestHosts_Registered(t *testing.T) {
	require.NotNil(t, Lookup("hosts"))
}

func TestHosts_InterfaceCompliance(t *testing.T) {
	var _ Manager = NewHostsManager()
}

func TestHosts_PlanFreshFileCreatesBlock(t *testing.T) {
	initial := "127.0.0.1 localhost\n::1 localhost ip6-localhost\n"
	mock := newHostsMock(initial)
	hm := NewHostsManager().WithSession(mock)
	desired := []config.HostEntry{
		{IP: "10.0.0.10", Names: []string{"rac1.example.com", "rac1"}},
	}
	changes, err := hm.Plan(context.Background(), desired, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "update", changes[0].Action)
	after := changes[0].After.(string)
	require.Contains(t, after, "10.0.0.10")
	require.Contains(t, after, "rac1")
}

func TestHosts_PlanNoChangeReturnsEmpty(t *testing.T) {
	initial := "127.0.0.1 localhost\n" +
		hostsBeginMarker + "\n" +
		"10.0.0.10  rac1.example.com rac1\n" +
		hostsEndMarker + "\n"
	mock := newHostsMock(initial)
	hm := NewHostsManager().WithSession(mock)
	desired := []config.HostEntry{
		{IP: "10.0.0.10", Names: []string{"rac1.example.com", "rac1"}},
	}
	changes, err := hm.Plan(context.Background(), desired, nil)
	require.NoError(t, err)
	require.Empty(t, changes)
}

func TestHosts_PlanExistingBlockNeedsUpdate(t *testing.T) {
	initial := "127.0.0.1 localhost\n" +
		hostsBeginMarker + "\n" +
		"10.0.0.10  rac1\n" +
		hostsEndMarker + "\n"
	mock := newHostsMock(initial)
	hm := NewHostsManager().WithSession(mock)
	desired := []config.HostEntry{
		{IP: "10.0.0.10", Names: []string{"rac1.example.com", "rac1"}},
		{IP: "10.0.0.11", Names: []string{"rac2.example.com", "rac2"}},
	}
	changes, err := hm.Plan(context.Background(), desired, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "update", changes[0].Action)
}

func TestHosts_PlanConflictWarns(t *testing.T) {
	// An operator-managed entry OUTSIDE the block claims a desired name.
	initial := "127.0.0.1 localhost\n192.168.1.5 rac1\n"
	mock := newHostsMock(initial)
	hm := NewHostsManager().WithSession(mock)
	desired := []config.HostEntry{
		{IP: "10.0.0.10", Names: []string{"rac1.example.com", "rac1"}},
	}
	changes, err := hm.Plan(context.Background(), desired, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, HazardWarn, changes[0].Hazard)
}

func TestHosts_Apply(t *testing.T) {
	initial := "127.0.0.1 localhost\n"
	mock := newHostsMock(initial)
	hm := NewHostsManager().WithSession(mock)
	desired := []config.HostEntry{
		{IP: "10.0.0.10", Names: []string{"rac1.example.com", "rac1"}},
	}
	changes, err := hm.Plan(context.Background(), desired, nil)
	require.NoError(t, err)
	res, err := hm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	require.Len(t, res.Applied, 1)
	written := string(mock.writes[hostsPath])
	require.Contains(t, written, "127.0.0.1 localhost")
	require.Contains(t, written, hostsBeginMarker)
	require.Contains(t, written, hostsEndMarker)
	require.Contains(t, written, "10.0.0.10")
}

func TestHosts_ApplyPreservesOutsideLines(t *testing.T) {
	initial := "127.0.0.1 localhost\n" +
		"# custom entry\n" +
		"192.168.1.1 router\n" +
		hostsBeginMarker + "\n" +
		"10.0.0.10 old\n" +
		hostsEndMarker + "\n" +
		"192.168.1.2 printer\n"
	mock := newHostsMock(initial)
	hm := NewHostsManager().WithSession(mock)
	desired := []config.HostEntry{
		{IP: "10.0.0.10", Names: []string{"rac1"}},
	}
	changes, err := hm.Plan(context.Background(), desired, nil)
	require.NoError(t, err)
	_, err = hm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	written := string(mock.writes[hostsPath])
	require.Contains(t, written, "# custom entry")
	require.Contains(t, written, "192.168.1.1 router")
	require.Contains(t, written, "192.168.1.2 printer")
	require.Contains(t, written, "rac1")
	require.NotContains(t, written, " old")
}

func TestHosts_ApplyDryRun(t *testing.T) {
	mock := newHostsMock("")
	hm := NewHostsManager().WithSession(mock)
	changes := []Change{{Action: "update", After: "10.0.0.1 host1\n"}}
	res, err := hm.Apply(context.Background(), changes, true)
	require.NoError(t, err)
	require.Len(t, res.Skipped, 1)
	require.Empty(t, mock.writes)
}

func TestHosts_VerifyOK(t *testing.T) {
	initial := hostsBeginMarker + "\n10.0.0.10  rac1\n" + hostsEndMarker + "\n"
	mock := newHostsMock(initial)
	hm := NewHostsManager().WithSession(mock)
	desired := []config.HostEntry{{IP: "10.0.0.10", Names: []string{"rac1"}}}
	res, err := hm.Verify(context.Background(), desired)
	require.NoError(t, err)
	require.True(t, res.OK)
}

func TestHosts_Rollback(t *testing.T) {
	initial := "127.0.0.1 localhost\n" +
		hostsBeginMarker + "\n" +
		"10.0.0.10  new\n" +
		hostsEndMarker + "\n"
	mock := newHostsMock(initial)
	hm := NewHostsManager().WithSession(mock)
	changes := []Change{
		{Action: "update", Before: "10.0.0.10  old\n", After: "10.0.0.10  new\n"},
	}
	require.NoError(t, hm.Rollback(context.Background(), changes))
	written := string(mock.writes[hostsPath])
	require.Contains(t, written, "old")
	require.NotContains(t, written, "  new\n")
}

func TestHosts_NoSessionReturnsError(t *testing.T) {
	hm := NewHostsManager()
	_, err := hm.Plan(context.Background(), []config.HostEntry{{IP: "1.2.3.4", Names: []string{"x"}}}, nil)
	require.ErrorIs(t, err, ErrSessionRequired)
}

func TestHosts_CastFromLinux(t *testing.T) {
	mock := newHostsMock("")
	hm := NewHostsManager().WithSession(mock)
	l := &config.Linux{HostsEntries: []config.HostEntry{{IP: "10.0.0.1", Names: []string{"x"}}}}
	changes, err := hm.Plan(context.Background(), l, nil)
	require.NoError(t, err)
	require.Len(t, changes, 1)
}

func TestHosts_CastUnsupported(t *testing.T) {
	mock := newHostsMock("")
	hm := NewHostsManager().WithSession(mock)
	_, err := hm.Plan(context.Background(), 42, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported")
}

func TestHosts_ExtractBlockMissing(t *testing.T) {
	body := "127.0.0.1 localhost\n"
	block, begin, end := extractBlock(body)
	require.Equal(t, "", block)
	require.Equal(t, -1, begin)
	require.Equal(t, -1, end)
}

func TestHosts_MergeRendersMarkers(t *testing.T) {
	merged := mergeHosts([]string{"127.0.0.1 localhost"}, "10.0.0.1 foo\n")
	require.Contains(t, merged, hostsBeginMarker)
	require.Contains(t, merged, hostsEndMarker)
	require.True(t, strings.HasSuffix(merged, hostsEndMarker+"\n"))
}

func TestHosts_ApplyNoSession(t *testing.T) {
	hm := NewHostsManager()
	_, err := hm.Apply(context.Background(), []Change{{Action: "update", After: ""}}, false)
	require.ErrorIs(t, err, ErrSessionRequired)
}

func TestHosts_ApplyBadAction(t *testing.T) {
	mock := newHostsMock("")
	hm := NewHostsManager().WithSession(mock)
	changes := []Change{{Action: "create", After: "x"}}
	res, err := hm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	require.Len(t, res.Failed, 1)
}

func TestHosts_ApplyWrongAfterType(t *testing.T) {
	mock := newHostsMock("")
	hm := NewHostsManager().WithSession(mock)
	changes := []Change{{Action: "update", After: 42}}
	res, err := hm.Apply(context.Background(), changes, false)
	require.NoError(t, err)
	require.Len(t, res.Failed, 1)
}

func TestHosts_ApplyReadError(t *testing.T) {
	mock := newHostsMock("")
	mock.readErr = errors.New("io err")
	hm := NewHostsManager().WithSession(mock)
	changes := []Change{{Action: "update", After: "10.0.0.1 x\n"}}
	res, _ := hm.Apply(context.Background(), changes, false)
	require.Len(t, res.Failed, 1)
}

func TestHosts_ApplyWriteError(t *testing.T) {
	mock := newHostsMock("127.0.0.1 localhost\n")
	mock.writeErr = errors.New("disk full")
	hm := NewHostsManager().WithSession(mock)
	changes := []Change{{Action: "update", After: "10.0.0.1 x\n"}}
	res, _ := hm.Apply(context.Background(), changes, false)
	require.Len(t, res.Failed, 1)
}

func TestHosts_RollbackNoSession(t *testing.T) {
	hm := NewHostsManager()
	err := hm.Rollback(context.Background(), []Change{{Action: "update"}})
	require.ErrorIs(t, err, ErrSessionRequired)
}

func TestHosts_RollbackSkipsNonUpdateAndBadBefore(t *testing.T) {
	mock := newHostsMock("127.0.0.1 localhost\n")
	hm := NewHostsManager().WithSession(mock)
	changes := []Change{
		{Action: "create"},                  // skipped (not update)
		{Action: "update", Before: 42},      // skipped (not string)
	}
	require.NoError(t, hm.Rollback(context.Background(), changes))
	require.Empty(t, mock.writes)
}

func TestHosts_RollbackReadError(t *testing.T) {
	mock := newHostsMock("")
	mock.readErr = errors.New("io err")
	hm := NewHostsManager().WithSession(mock)
	err := hm.Rollback(context.Background(), []Change{{Action: "update", Before: "x\n"}})
	require.Error(t, err)
}

func TestHosts_RollbackWriteError(t *testing.T) {
	mock := newHostsMock("127.0.0.1 localhost\n")
	mock.writeErr = errors.New("ro fs")
	hm := NewHostsManager().WithSession(mock)
	err := hm.Rollback(context.Background(), []Change{{Action: "update", Before: "x\n"}})
	require.Error(t, err)
}

func TestHosts_CastValueLinux(t *testing.T) {
	got, err := castHostEntries(config.Linux{HostsEntries: []config.HostEntry{{IP: "1.2.3.4", Names: []string{"x"}}}})
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestHosts_CastNilLinuxPointer(t *testing.T) {
	var l *config.Linux
	got, err := castHostEntries(l)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestHosts_CastSliceDirect(t *testing.T) {
	got, err := castHostEntries([]config.HostEntry{{IP: "1.2.3.4", Names: []string{"x"}}})
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestHosts_Verify_PlanError(t *testing.T) {
	mock := newHostsMock("")
	hm := NewHostsManager().WithSession(mock)
	_, err := hm.Verify(context.Background(), 42)
	require.Error(t, err)
}
