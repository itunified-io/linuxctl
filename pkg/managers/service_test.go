package managers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// svcMockSession is a sudoRunner mock: tracks Run + RunSudo calls, scripted
// responses matched by substring.
type svcMockSession struct {
	keys      []string
	responses map[string]mockResponse
	cmds      []string
	sudoCmds  []string
}

func newSvcMock() *svcMockSession {
	return &svcMockSession{responses: map[string]mockResponse{}}
}

func (m *svcMockSession) on(keyContains, stdout string, err error) *svcMockSession {
	m.keys = append(m.keys, keyContains)
	m.responses[keyContains] = mockResponse{stdout: stdout, err: err}
	return m
}

func (m *svcMockSession) match(cmd string) mockResponse {
	for _, k := range m.keys {
		if strings.Contains(cmd, k) {
			return m.responses[k]
		}
	}
	return mockResponse{}
}

func (m *svcMockSession) Run(_ context.Context, cmd string) (string, string, error) {
	m.cmds = append(m.cmds, cmd)
	r := m.match(cmd)
	return r.stdout, r.stderr, r.err
}

func (m *svcMockSession) RunSudo(_ context.Context, cmd string) (string, string, error) {
	m.cmds = append(m.cmds, cmd)
	m.sudoCmds = append(m.sudoCmds, cmd)
	r := m.match(cmd)
	return r.stdout, r.stderr, r.err
}

func (m *svcMockSession) ran(sub string) bool {
	for _, c := range m.cmds {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func TestServiceManager_Plan_UnitMissing(t *testing.T) {
	ms := newSvcMock().
		on("is-enabled 'missing'", "not-found\n", fmt.Errorf("exit 1")).
		on("systemctl status 'missing'", "Unit missing.service could not be found.\n", nil).
		on("is-active 'missing'", "inactive\n", fmt.Errorf("exit 3"))
	s := NewServiceManager().WithSession(ms)

	changes, err := s.Plan(context.Background(), []config.ServiceState{
		{Name: "missing", Enabled: true, State: "running"},
	}, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 || changes[0].Action != "error" {
		t.Fatalf("want 1 error change, got %+v", changes)
	}
}

func TestServiceManager_Plan_AlreadyCorrect(t *testing.T) {
	ms := newSvcMock().
		on("is-enabled 'sshd'", "enabled\n", nil).
		on("is-active 'sshd'", "active\n", nil)
	s := NewServiceManager().WithSession(ms)

	changes, err := s.Plan(context.Background(), []config.ServiceState{
		{Name: "sshd", Enabled: true, State: "running"},
	}, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("want no drift, got %+v", changes)
	}
}

func TestServiceManager_Plan_EnableAndStart(t *testing.T) {
	ms := newSvcMock().
		on("is-enabled 'httpd'", "disabled\n", nil).
		on("is-active 'httpd'", "inactive\n", fmt.Errorf("exit 3"))
	s := NewServiceManager().WithSession(ms)

	changes, err := s.Plan(context.Background(), []config.ServiceState{
		{Name: "httpd", Enabled: true, State: "running"},
	}, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("want 2 changes, got %+v", changes)
	}
	// first change = enable, second = start
	op1 := changes[0].After.(serviceEnableOp).Op
	op2 := changes[1].After.(serviceStateOp).Op
	if op1 != "enable" || op2 != "start" {
		t.Errorf("want enable+start, got %s+%s", op1, op2)
	}
}

func TestServiceManager_Plan_DisableAndStop(t *testing.T) {
	ms := newSvcMock().
		on("is-enabled 'telnet'", "enabled\n", nil).
		on("is-active 'telnet'", "active\n", nil)
	s := NewServiceManager().WithSession(ms)

	changes, err := s.Plan(context.Background(), []config.ServiceState{
		{Name: "telnet", Enabled: false, State: "stopped"},
	}, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("want 2 changes, got %+v", changes)
	}
	if changes[0].After.(serviceEnableOp).Op != "disable" {
		t.Errorf("want disable, got %+v", changes[0])
	}
	if changes[1].After.(serviceStateOp).Op != "stop" {
		t.Errorf("want stop, got %+v", changes[1])
	}
}

func TestServiceManager_Apply_EnableAndStart(t *testing.T) {
	ms := newSvcMock()
	s := NewServiceManager().WithSession(ms)

	changes := []Change{
		{Manager: "service", Target: "service/nginx", Action: "update",
			Before: serviceEnableSnap{Name: "nginx", Enabled: false},
			After:  serviceEnableOp{Name: "nginx", Op: "enable"}},
		{Manager: "service", Target: "service/nginx", Action: "update",
			Before: serviceStateSnap{Name: "nginx", Active: false},
			After:  serviceStateOp{Name: "nginx", Op: "start"}},
	}
	res, err := s.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Applied) != 2 || len(res.Failed) != 0 {
		t.Fatalf("want 2 applied 0 failed, got %+v", res)
	}
	if !ms.ran("systemctl enable 'nginx'") {
		t.Errorf("expected systemctl enable; cmds=%v", ms.cmds)
	}
	if !ms.ran("systemctl start 'nginx'") {
		t.Errorf("expected systemctl start; cmds=%v", ms.cmds)
	}
}

func TestServiceManager_Apply_DryRun(t *testing.T) {
	ms := newSvcMock()
	s := NewServiceManager().WithSession(ms)
	changes := []Change{{Target: "service/x", Action: "update",
		After: serviceEnableOp{Name: "x", Op: "enable"}}}
	res, err := s.Apply(context.Background(), changes, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || len(ms.cmds) != 0 {
		t.Fatalf("dry-run should skip, got %+v, cmds=%v", res, ms.cmds)
	}
}

func TestServiceManager_Apply_NoSession(t *testing.T) {
	s := NewServiceManager()
	_, err := s.Apply(context.Background(), []Change{{}}, false)
	if err == nil {
		t.Error("want session-required error")
	}
}

func TestServiceManager_Apply_MaskedRefused(t *testing.T) {
	ms := newSvcMock()
	s := NewServiceManager().WithSession(ms)
	changes := []Change{{Target: "service/ntpd", Action: "update",
		After: serviceStateOp{Name: "ntpd", Op: "start", Masked: true}}}
	res, _ := s.Apply(context.Background(), changes, false)
	if len(res.Failed) != 1 {
		t.Fatalf("want 1 failure for masked unit, got %+v", res)
	}
}

func TestServiceManager_Apply_ErrorChangeFails(t *testing.T) {
	ms := newSvcMock()
	s := NewServiceManager().WithSession(ms)
	changes := []Change{{Target: "service/missing", Action: "error",
		After: config.ServiceState{Name: "missing"}}}
	res, _ := s.Apply(context.Background(), changes, false)
	if len(res.Failed) != 1 {
		t.Fatalf("want 1 failure for error change, got %+v", res)
	}
}

func TestServiceManager_Verify_NoDrift(t *testing.T) {
	ms := newSvcMock().
		on("is-enabled 'sshd'", "enabled\n", nil).
		on("is-active 'sshd'", "active\n", nil)
	s := NewServiceManager().WithSession(ms)
	vr, err := s.Verify(context.Background(), []config.ServiceState{{Name: "sshd", Enabled: true, State: "running"}})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !vr.OK {
		t.Errorf("want OK, got %+v", vr)
	}
}

func TestServiceManager_Rollback_ReversesEnable(t *testing.T) {
	ms := newSvcMock()
	s := NewServiceManager().WithSession(ms)
	changes := []Change{
		{Target: "service/nginx", Action: "update",
			Before: serviceEnableSnap{Name: "nginx", Enabled: false},
			After:  serviceEnableOp{Name: "nginx", Op: "enable"}},
		{Target: "service/nginx", Action: "update",
			Before: serviceStateSnap{Name: "nginx", Active: false},
			After:  serviceStateOp{Name: "nginx", Op: "start"}},
	}
	if err := s.Rollback(context.Background(), changes); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	// rollback reverses: stop (undo start), disable (undo enable)
	if !ms.ran("systemctl stop 'nginx'") {
		t.Errorf("expected stop; cmds=%v", ms.cmds)
	}
	if !ms.ran("systemctl disable 'nginx'") {
		t.Errorf("expected disable; cmds=%v", ms.cmds)
	}
}

func TestServiceManager_CastServices_Variants(t *testing.T) {
	if _, err := castServices(nil); err != nil {
		t.Errorf("nil: %v", err)
	}
	if _, err := castServices(&config.Linux{Services: []config.ServiceState{{Name: "x"}}}); err != nil {
		t.Errorf("pointer: %v", err)
	}
	if _, err := castServices(config.Linux{Services: []config.ServiceState{{Name: "x"}}}); err != nil {
		t.Errorf("value: %v", err)
	}
	if _, err := castServices("bad"); err == nil {
		t.Error("want error for unsupported type")
	}
}

func TestServiceManager_Plan_NoSession(t *testing.T) {
	s := NewServiceManager()
	_, err := s.Plan(context.Background(), []config.ServiceState{{Name: "x"}}, nil)
	if err == nil {
		t.Error("want session-required error")
	}
}

func TestServiceManager_Rollback_NoSession(t *testing.T) {
	s := NewServiceManager()
	if err := s.Rollback(context.Background(), []Change{{}}); err == nil {
		t.Error("want session-required error")
	}
}

func TestServiceManager_Rollback_StopWhenActive(t *testing.T) {
	ms := newSvcMock()
	s := NewServiceManager().WithSession(ms)
	changes := []Change{
		{Before: serviceEnableSnap{Name: "svc", Enabled: true}, After: serviceEnableOp{Name: "svc", Op: "disable"}},
		{Before: serviceStateSnap{Name: "svc", Active: true}, After: serviceStateOp{Name: "svc", Op: "stop"}},
		{Before: "not a snap"},
	}
	if err := s.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if !ms.ran("systemctl enable 'svc'") {
		t.Errorf("expected re-enable; cmds=%v", ms.cmds)
	}
	if !ms.ran("systemctl start 'svc'") {
		t.Errorf("expected re-start; cmds=%v", ms.cmds)
	}
}

func TestServiceManager_ApplyOne_MaskedEnable(t *testing.T) {
	ms := newSvcMock()
	s := NewServiceManager().WithSession(ms)
	changes := []Change{{After: serviceEnableOp{Name: "m", Op: "enable", Masked: true}}}
	res, _ := s.Apply(context.Background(), changes, false)
	if len(res.Failed) != 1 {
		t.Errorf("expected masked refusal")
	}
}

func TestServiceManager_ApplyOne_UnexpectedAfter(t *testing.T) {
	ms := newSvcMock()
	s := NewServiceManager().WithSession(ms)
	changes := []Change{{After: "random string"}}
	res, _ := s.Apply(context.Background(), changes, false)
	if len(res.Failed) != 1 {
		t.Errorf("expected failure")
	}
}

func TestServiceManager_Observe_MaskedUnit(t *testing.T) {
	ms := newSvcMock().
		on("is-enabled 'svc'", "masked\n", nil).
		on("is-active 'svc'", "inactive\n", nil)
	s := NewServiceManager().WithSession(ms)
	changes, err := s.Plan(context.Background(), []config.ServiceState{{Name: "svc", Enabled: true, State: "running"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Masked must still produce changes but they carry Masked=true.
	hasMasked := false
	for _, ch := range changes {
		if op, ok := ch.After.(serviceEnableOp); ok && op.Masked {
			hasMasked = true
		}
		if op, ok := ch.After.(serviceStateOp); ok && op.Masked {
			hasMasked = true
		}
	}
	if !hasMasked {
		t.Errorf("expected at least one Masked=true; got %+v", changes)
	}
}

func TestServiceManager_Observe_DisabledUnit(t *testing.T) {
	ms := newSvcMock().
		on("is-enabled 'svc'", "disabled\n", nil).
		on("is-active 'svc'", "inactive\n", nil)
	s := NewServiceManager().WithSession(ms)
	changes, err := s.Plan(context.Background(), []config.ServiceState{{Name: "svc", Enabled: true, State: "running"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) == 0 {
		t.Error("expected enable/start changes")
	}
}

func TestServiceManager_Apply_StateRetry(t *testing.T) {
	// First invocation of start fails, retry succeeds (mock always returns nil so fabricate with error once).
	ms := newSvcMock()
	ms.on("systemctl start 'svc'", "", errors.New("transient"))
	s := NewServiceManager().WithSession(ms)
	changes := []Change{{After: serviceStateOp{Name: "svc", Op: "start"}}}
	res, _ := s.Apply(context.Background(), changes, false)
	// Both attempts fail, so this should fail — but we want to exercise the retry path.
	if len(res.Failed) != 1 {
		t.Errorf("expected failure on both retries; got %+v", res)
	}
}
