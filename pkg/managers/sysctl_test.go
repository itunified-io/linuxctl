package managers

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// fileMockSession is a full session.Session mock for sysctl + limits tests.
type fileMockSession struct {
	keys      []string
	responses map[string]mockResponse
	cmds      []string
	files     map[string][]byte
	writes    map[string][]byte // most-recent written content per path
	exists    map[string]bool
}

func newFileMock() *fileMockSession {
	return &fileMockSession{
		responses: map[string]mockResponse{},
		files:     map[string][]byte{},
		writes:    map[string][]byte{},
		exists:    map[string]bool{},
	}
}

func (m *fileMockSession) on(keyContains, stdout string, err error) *fileMockSession {
	m.keys = append(m.keys, keyContains)
	m.responses[keyContains] = mockResponse{stdout: stdout, err: err}
	return m
}

func (m *fileMockSession) withFile(path, content string) *fileMockSession {
	m.files[path] = []byte(content)
	m.exists[path] = true
	return m
}

func (m *fileMockSession) match(cmd string) mockResponse {
	for _, k := range m.keys {
		if strings.Contains(cmd, k) {
			return m.responses[k]
		}
	}
	return mockResponse{}
}

func (m *fileMockSession) Host() string { return "mock" }
func (m *fileMockSession) Close() error { return nil }

func (m *fileMockSession) Run(_ context.Context, cmd string) (string, string, error) {
	m.cmds = append(m.cmds, cmd)
	r := m.match(cmd)
	return r.stdout, r.stderr, r.err
}

func (m *fileMockSession) RunSudo(ctx context.Context, cmd string) (string, string, error) {
	return m.Run(ctx, cmd)
}

func (m *fileMockSession) WriteFile(_ context.Context, path string, content []byte, _ uint32) error {
	cp := append([]byte(nil), content...)
	m.files[path] = cp
	m.writes[path] = cp
	m.exists[path] = true
	return nil
}

func (m *fileMockSession) ReadFile(_ context.Context, path string) ([]byte, error) {
	if b, ok := m.files[path]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("no such file: %s", path)
}

func (m *fileMockSession) FileExists(_ context.Context, path string) (bool, error) {
	return m.exists[path], nil
}

func (m *fileMockSession) ran(sub string) bool {
	for _, c := range m.cmds {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func TestSysctl_PresetExpansion(t *testing.T) {
	entries := presetSysctl("oracle-19c")
	if len(entries) < 8 {
		t.Fatalf("oracle-19c preset too small: %d entries", len(entries))
	}
	keys := map[string]bool{}
	for _, e := range entries {
		keys[e.Key] = true
	}
	for _, req := range []string{"kernel.sem", "fs.aio-max-nr", "fs.file-max", "kernel.shmmax"} {
		if !keys[req] {
			t.Errorf("preset missing %q", req)
		}
	}
	if got := presetSysctl("pg-16"); got != nil {
		t.Errorf("pg-16 should stub to nil, got %d entries", len(got))
	}
	if got := presetSysctl("unknown"); got != nil {
		t.Errorf("unknown should be nil, got %v", got)
	}
	if got := presetSysctl(""); got != nil {
		t.Errorf("empty preset should be nil")
	}
}

func TestSysctl_MergePresetExplicit(t *testing.T) {
	preset := []config.SysctlEntry{
		{Key: "vm.swappiness", Value: "10"},
		{Key: "fs.file-max", Value: "6815744"},
	}
	explicit := []config.SysctlEntry{
		{Key: "vm.swappiness", Value: "1"}, // override
		{Key: "net.ipv4.ip_forward", Value: "1"},
	}
	merged := mergeSysctl(explicit, preset)
	if len(merged) != 3 {
		t.Fatalf("want 3, got %d: %+v", len(merged), merged)
	}
	want := map[string]string{"vm.swappiness": "1", "fs.file-max": "6815744", "net.ipv4.ip_forward": "1"}
	for _, e := range merged {
		if want[e.Key] != e.Value {
			t.Errorf("key %s: want %q got %q", e.Key, want[e.Key], e.Value)
		}
	}
}

func TestSysctl_Plan_NoDesired(t *testing.T) {
	ms := newFileMock()
	s := NewSysctlManager().WithSession(ms)
	changes, err := s.Plan(context.Background(), &config.Linux{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("want no changes for empty spec, got %+v", changes)
	}
}

func TestSysctl_Plan_FileDrift(t *testing.T) {
	ms := newFileMock().
		withFile(SysctlManagedPath, "# old\nvm.swappiness = 60\n").
		on("sysctl -n 'vm.swappiness'", "60\n", nil)
	s := NewSysctlManager().WithSession(ms)

	changes, err := s.Plan(context.Background(), &config.Linux{
		Sysctl: []config.SysctlEntry{{Key: "vm.swappiness", Value: "10"}},
	}, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 || changes[0].Action != "update" {
		t.Fatalf("want 1 update, got %+v", changes)
	}
	a := changes[0].After.(sysctlApply)
	if !strings.Contains(a.Body, "vm.swappiness = 10") {
		t.Errorf("body missing desired value: %q", a.Body)
	}
}

func TestSysctl_Plan_LiveDriftOnly(t *testing.T) {
	// File already correct but live kernel out of sync.
	body := renderSysctl([]config.SysctlEntry{{Key: "vm.swappiness", Value: "10"}})
	ms := newFileMock().
		withFile(SysctlManagedPath, body).
		on("sysctl -n 'vm.swappiness'", "60\n", nil) // live mismatch
	s := NewSysctlManager().WithSession(ms)

	changes, _ := s.Plan(context.Background(), &config.Linux{
		Sysctl: []config.SysctlEntry{{Key: "vm.swappiness", Value: "10"}},
	}, nil)
	if len(changes) != 1 {
		t.Fatalf("want 1 change due to live drift, got %+v", changes)
	}
}

func TestSysctl_Plan_NoDrift(t *testing.T) {
	body := renderSysctl([]config.SysctlEntry{{Key: "vm.swappiness", Value: "10"}})
	ms := newFileMock().
		withFile(SysctlManagedPath, body).
		on("sysctl -n 'vm.swappiness'", "10\n", nil)
	s := NewSysctlManager().WithSession(ms)
	changes, _ := s.Plan(context.Background(), &config.Linux{
		Sysctl: []config.SysctlEntry{{Key: "vm.swappiness", Value: "10"}},
	}, nil)
	if len(changes) != 0 {
		t.Errorf("want no drift, got %+v", changes)
	}
}

func TestSysctl_Apply_WritesFileAndReloads(t *testing.T) {
	ms := newFileMock()
	s := NewSysctlManager().WithSession(ms)

	entries := []config.SysctlEntry{{Key: "fs.file-max", Value: "6815744"}}
	changes := []Change{{
		Target: SysctlManagedPath, Action: "update",
		Before: sysctlSnap{Body: ""},
		After:  sysctlApply{Body: renderSysctl(entries), Entries: entries},
	}}
	res, err := s.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 1 || len(res.Failed) != 0 {
		t.Fatalf("want 1 applied, got %+v", res)
	}
	body, ok := ms.writes[SysctlManagedPath]
	if !ok {
		t.Fatal("WriteFile was not called for managed path")
	}
	if !strings.Contains(string(body), "fs.file-max = 6815744") {
		t.Errorf("written body missing entry: %s", body)
	}
	if !ms.ran("sysctl -p") {
		t.Errorf("expected sysctl -p; cmds=%v", ms.cmds)
	}
}

func TestSysctl_Apply_DryRun(t *testing.T) {
	ms := newFileMock()
	s := NewSysctlManager().WithSession(ms)
	changes := []Change{{Action: "update", After: sysctlApply{Body: "x"}}}
	res, _ := s.Apply(context.Background(), changes, true)
	if len(res.Skipped) != 1 {
		t.Fatalf("want 1 skipped, got %+v", res)
	}
	if len(ms.writes) != 0 {
		t.Errorf("dry-run should not write files")
	}
}

func TestSysctl_Apply_NoSession(t *testing.T) {
	s := NewSysctlManager()
	_, err := s.Apply(context.Background(), []Change{{Action: "update", After: sysctlApply{}}}, false)
	if err == nil {
		t.Error("want session-required error")
	}
}

func TestSysctl_Rollback_RestoresPrevious(t *testing.T) {
	ms := newFileMock().withFile(SysctlManagedPath, "new\n")
	s := NewSysctlManager().WithSession(ms)
	changes := []Change{{Action: "update",
		Before: sysctlSnap{Body: "old = 1\n"},
		After:  sysctlApply{Body: "new = 2\n"}}}
	if err := s.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if string(ms.writes[SysctlManagedPath]) != "old = 1\n" {
		t.Errorf("rollback did not restore old body; got %q", ms.writes[SysctlManagedPath])
	}
}

func TestSysctl_Rollback_NoPreviousRemovesFile(t *testing.T) {
	ms := newFileMock()
	s := NewSysctlManager().WithSession(ms)
	changes := []Change{{Action: "update",
		Before: sysctlSnap{Body: ""},
		After:  sysctlApply{Body: "x = 1\n"}}}
	if err := s.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if !ms.ran("rm -f '" + SysctlManagedPath + "'") {
		t.Errorf("expected rm -f of managed path; cmds=%v", ms.cmds)
	}
}

func TestSysctl_Verify(t *testing.T) {
	body := renderSysctl([]config.SysctlEntry{{Key: "vm.swappiness", Value: "10"}})
	ms := newFileMock().
		withFile(SysctlManagedPath, body).
		on("sysctl -n 'vm.swappiness'", "10", nil)
	s := NewSysctlManager().WithSession(ms)
	vr, err := s.Verify(context.Background(), &config.Linux{
		Sysctl: []config.SysctlEntry{{Key: "vm.swappiness", Value: "10"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !vr.OK {
		t.Errorf("want OK, got drift %+v", vr.Drift)
	}
}

func TestSysctl_CastLinuxForSysctl_Variants(t *testing.T) {
	if _, err := castLinuxForSysctl(nil); err != nil {
		t.Error(err)
	}
	if _, err := castLinuxForSysctl([]config.SysctlEntry{{Key: "a", Value: "1"}}); err != nil {
		t.Error(err)
	}
	if _, err := castLinuxForSysctl(config.Linux{}); err != nil {
		t.Error(err)
	}
	if _, err := castLinuxForSysctl("bad"); err == nil {
		t.Error("want error for bad type")
	}
}

func TestSysctl_ParseFile(t *testing.T) {
	body := "# comment\nfoo = 1\nbar=2\n\n; another\nbaz\n"
	m := parseSysctlFile(body)
	if m["foo"] != "1" || m["bar"] != "2" {
		t.Errorf("parse failed: %+v", m)
	}
	if _, ok := m["baz"]; ok {
		t.Error("malformed line should not be present")
	}
}

func TestSysctl_Rollback_NoSession(t *testing.T) {
	s := NewSysctlManager()
	if err := s.Rollback(context.Background(), []Change{{}}); err == nil {
		t.Error("want session-required")
	}
}

func TestSysctl_Rollback_SkipsBadBefore(t *testing.T) {
	ms := newFileMock()
	s := NewSysctlManager().WithSession(ms)
	changes := []Change{{Action: "update", Before: "not a sysctlSnap"}}
	if err := s.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if len(ms.writes) != 0 {
		t.Error("should not write")
	}
}

func TestSysctl_Apply_WrongAfterType(t *testing.T) {
	ms := newFileMock()
	s := NewSysctlManager().WithSession(ms)
	res, _ := s.Apply(context.Background(), []Change{{Action: "update", After: "bad"}}, false)
	if len(res.Failed) != 1 {
		t.Errorf("want 1 failed")
	}
}

func TestSysctl_Plan_UnknownPreset(t *testing.T) {
	ms := newFileMock()
	s := NewSysctlManager().WithSession(ms)
	changes, err := s.Plan(context.Background(), &config.Linux{SysctlPreset: "bogus-preset"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("bogus preset → no changes; got %+v", changes)
	}
}

func TestSysctl_CastNilLinuxPointer(t *testing.T) {
	var l *config.Linux
	got, err := castLinuxForSysctl(l)
	if err != nil || got == nil {
		t.Errorf("nil → empty Linux; got (%v,%v)", got, err)
	}
}

func TestSysctl_WriteAndReload_ReloadFails(t *testing.T) {
	ms := newFileMock().on("sysctl -p", "", fmt.Errorf("bad key"))
	ms.responses["sysctl -p"] = mockResponse{stderr: "kernel.bad: No such file", err: fmt.Errorf("bad key")}
	s := NewSysctlManager().WithSession(ms)
	changes := []Change{{Action: "update", After: sysctlApply{Body: "x=1\n"}}}
	res, _ := s.Apply(context.Background(), changes, false)
	if len(res.Failed) != 1 {
		t.Errorf("expected failure; got %+v", res)
	}
}

func TestSysctl_Plan_PresetMerge(t *testing.T) {
	ms := newFileMock()
	for _, k := range []string{"fs.aio-max-nr", "fs.file-max", "kernel.panic_on_oops",
		"kernel.sem", "kernel.shmall", "kernel.shmmax", "kernel.shmmni",
		"net.core.rmem_max", "net.core.wmem_max", "vm.swappiness"} {
		ms.on("sysctl -n '"+k+"'", "x", nil) // live mismatch → triggers Change
	}
	s := NewSysctlManager().WithSession(ms)
	changes, err := s.Plan(context.Background(), &config.Linux{SysctlPreset: "oracle-19c"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("want 1 change, got %+v", changes)
	}
	a := changes[0].After.(sysctlApply)
	if len(a.Entries) < 8 {
		t.Errorf("preset expansion too small: %d", len(a.Entries))
	}
}
