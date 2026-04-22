package managers

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/itunified-io/linuxctl/pkg/config"
)

func TestSELinuxManager_Plan_ModeChangeEnforcing(t *testing.T) {
	ms := newMockSession().
		on("/etc/selinux/config", "permissive\n", nil).
		on("getenforce", "Permissive\n", nil)
	m := NewSELinuxManager().WithSession(ms)
	cfg := &config.SELinuxConfig{Mode: "enforcing"}
	changes, err := m.Plan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(changes) != 1 || changes[0].Target != "selinux/mode" || changes[0].After.(string) != "enforcing" {
		t.Fatalf("want 1 mode change to enforcing, got %+v", changes)
	}
}

func TestSELinuxManager_Plan_NoDrift(t *testing.T) {
	ms := newMockSession().
		on("/etc/selinux/config", "enforcing\n", nil).
		on("getsebool", "httpd_can_network_connect --> on\n", nil)
	m := NewSELinuxManager().WithSession(ms)
	cfg := &config.SELinuxConfig{
		Mode:     "enforcing",
		Booleans: map[string]bool{"httpd_can_network_connect": true},
	}
	changes, err := m.Plan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no drift, got %+v", changes)
	}
}

func TestSELinuxManager_Plan_BooleanDrift(t *testing.T) {
	ms := newMockSession().
		on("/etc/selinux/config", "enforcing\n", nil).
		on("getsebool", "httpd_can_network_connect --> off\n", nil)
	m := NewSELinuxManager().WithSession(ms)
	cfg := &config.SELinuxConfig{Booleans: map[string]bool{"httpd_can_network_connect": true}}
	changes, err := m.Plan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(changes) != 1 || changes[0].Target != "selinux/bool/httpd_can_network_connect" {
		t.Fatalf("want 1 boolean change, got %+v", changes)
	}
	if changes[0].After.(bool) != true {
		t.Errorf("want After=true, got %v", changes[0].After)
	}
}

func TestSELinuxManager_Plan_DisabledHazard(t *testing.T) {
	ms := newMockSession().on("/etc/selinux/config", "enforcing\n", nil)
	m := NewSELinuxManager().WithSession(ms)
	cfg := &config.SELinuxConfig{Mode: "disabled"}
	changes, err := m.Plan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("want 1 change, got %+v", changes)
	}
	if changes[0].Hazard != HazardDestructive {
		t.Errorf("want destructive hazard for disabled, got %v", changes[0].Hazard)
	}
	if changes[0].RollbackCmd == "" {
		t.Errorf("want reboot rollback hint, got empty")
	}
}

func TestSELinuxManager_Plan_FromLinux(t *testing.T) {
	ms := newMockSession().on("/etc/selinux/config", "permissive\n", nil)
	m := NewSELinuxManager().WithSession(ms)
	lx := &config.Linux{SELinux: &config.SELinuxConfig{Mode: "enforcing"}}
	changes, err := m.Plan(context.Background(), lx, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %+v", changes)
	}
}

func TestSELinuxManager_Apply_ModePermissiveToEnforcing(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := []Change{{
		Target: "selinux/mode", Action: "update",
		Before: "permissive", After: "enforcing",
	}}
	res, err := m.Apply(context.Background(), ch, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 1 {
		t.Fatalf("want applied, got %+v", res)
	}
	if !ms.ranContaining("setenforce 1") {
		t.Errorf("expected setenforce 1, got %v", ms.cmds)
	}
	if !ms.ranContaining("sed -i") {
		t.Errorf("expected config file write, got %v", ms.cmds)
	}
}

func TestSELinuxManager_Apply_ModeDisabledNoSetenforce(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := []Change{{Target: "selinux/mode", Action: "update", Before: "enforcing", After: "disabled"}}
	_, err := m.Apply(context.Background(), ch, false)
	if err != nil {
		t.Fatal(err)
	}
	// We must NOT have called setenforce — disabled requires reboot.
	for _, c := range ms.cmds {
		if c == "setenforce 1" || c == "setenforce 0" {
			t.Errorf("setenforce should not be called for disabled: %v", ms.cmds)
		}
	}
	if !ms.ranContaining("sed -i") {
		t.Error("expected config file write")
	}
}

func TestSELinuxManager_Apply_Boolean(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := []Change{{Target: "selinux/bool/nis_enabled", Action: "update", After: true}}
	if _, err := m.Apply(context.Background(), ch, false); err != nil {
		t.Fatal(err)
	}
	if !ms.ranContaining("setsebool -P 'nis_enabled' on") {
		t.Errorf("expected setsebool, got %v", ms.cmds)
	}
}

func TestSELinuxManager_Apply_BooleanOff(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := []Change{{Target: "selinux/bool/nis_enabled", Action: "update", After: false}}
	if _, err := m.Apply(context.Background(), ch, false); err != nil {
		t.Fatal(err)
	}
	if !ms.ranContaining("setsebool -P 'nis_enabled' off") {
		t.Errorf("expected setsebool off, got %v", ms.cmds)
	}
}

func TestSELinuxManager_Apply_DryRun(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := []Change{{Target: "selinux/mode", After: "enforcing"}}
	res, err := m.Apply(context.Background(), ch, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 {
		t.Errorf("want skipped, got %+v", res)
	}
	if len(ms.cmds) != 0 {
		t.Errorf("dry-run should not exec: %v", ms.cmds)
	}
}

func TestSELinuxManager_Apply_NoSession(t *testing.T) {
	m := NewSELinuxManager()
	_, err := m.Apply(context.Background(), []Change{{Target: "selinux/mode"}}, false)
	if err == nil {
		t.Error("expected error without session")
	}
}

func TestSELinuxManager_Apply_UnknownTarget(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	res, err := m.Apply(context.Background(),
		[]Change{{Target: "bogus", After: "x"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failed) != 1 {
		t.Errorf("expected failure, got %+v", res)
	}
}

func TestSELinuxManager_Rollback_Boolean(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := []Change{{Target: "selinux/bool/nis_enabled", Before: false, After: true}}
	if err := m.Rollback(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	// Rollback should set it back to off.
	if !ms.ranContaining("setsebool -P 'nis_enabled' off") {
		t.Errorf("expected rollback to off, got %v", ms.cmds)
	}
}

func TestSELinuxManager_Rollback_Mode(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := []Change{{Target: "selinux/mode", Before: "permissive", After: "enforcing"}}
	if err := m.Rollback(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	if !ms.ranContaining("setenforce 0") {
		t.Errorf("expected setenforce 0 to rollback to permissive, got %v", ms.cmds)
	}
}

func TestSELinuxManager_Verify(t *testing.T) {
	ms := newMockSession().on("/etc/selinux/config", "enforcing\n", nil)
	m := NewSELinuxManager().WithSession(ms)
	cfg := &config.SELinuxConfig{Mode: "enforcing"}
	vr, err := m.Verify(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !vr.OK {
		t.Errorf("expected OK, got %+v", vr)
	}
}

func TestSELinuxManager_Name(t *testing.T) {
	if NewSELinuxManager().Name() != "selinux" {
		t.Error("unexpected name")
	}
}

func TestSELinuxManager_CastUnsupported(t *testing.T) {
	m := NewSELinuxManager().WithSession(newMockSession())
	_, err := m.Plan(context.Background(), "bogus", nil)
	if err == nil {
		t.Error("expected error for unsupported spec")
	}
}

func TestSELinuxManager_ReadMode_FallbackToGetenforce(t *testing.T) {
	ms := newMockSession().
		on("getenforce", "Enforcing\n", nil)
	m := NewSELinuxManager().WithSession(ms)
	mode, err := m.readMode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mode != "enforcing" {
		t.Errorf("want enforcing from fallback, got %q", mode)
	}
}

func TestSELinuxManager_ReadMode_FallbackPermissiveDisabled(t *testing.T) {
	ms := newMockSession().on("getenforce", "Permissive\n", nil)
	m := NewSELinuxManager().WithSession(ms)
	mode, _ := m.readMode(context.Background())
	if mode != "permissive" {
		t.Errorf("want permissive, got %q", mode)
	}
	ms2 := newMockSession().on("getenforce", "Disabled\n", nil)
	m2 := NewSELinuxManager().WithSession(ms2)
	mode2, _ := m2.readMode(context.Background())
	if mode2 != "disabled" {
		t.Errorf("want disabled, got %q", mode2)
	}
}

func TestSELinuxManager_ReadMode_NoSession(t *testing.T) {
	m := NewSELinuxManager()
	mode, err := m.readMode(context.Background())
	if err != nil || mode != "" {
		t.Errorf("no session → empty, nil: got (%q, %v)", mode, err)
	}
}

func TestSELinuxManager_ReadBoolean_NoSession(t *testing.T) {
	m := NewSELinuxManager()
	v, err := m.readBoolean(context.Background(), "httpd_can_network_connect")
	if err != nil || v {
		t.Errorf("no session → false, nil: got (%v, %v)", v, err)
	}
}

func TestSELinuxManager_ReadBoolean_On(t *testing.T) {
	ms := newMockSession().on("getsebool", "httpd_can_network_connect --> on\n", nil)
	m := NewSELinuxManager().WithSession(ms)
	v, _ := m.readBoolean(context.Background(), "httpd_can_network_connect")
	if !v {
		t.Error("expected true")
	}
}

func TestSELinuxManager_ApplyMode_Permissive(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := Change{Target: "selinux/mode", Before: "enforcing", After: "permissive"}
	if err := m.applyMode(context.Background(), &ch); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range ms.cmds {
		if strings.Contains(c, "setenforce 0") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected setenforce 0; got %v", ms.cmds)
	}
}

func TestSELinuxManager_ApplyBoolean_Off(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := Change{Target: "selinux/bool/httpd_can_network_connect", After: false}
	if err := m.applyBoolean(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(ms.cmds, " | ")
	if !strings.Contains(joined, "setsebool -P") || !strings.Contains(joined, "off") {
		t.Errorf("expected setsebool off; got %v", ms.cmds)
	}
}

func TestSELinuxManager_ApplyMode_SetenforceFails(t *testing.T) {
	ms := newMockSession().on("setenforce", "", fmt.Errorf("not permitted"))
	m := NewSELinuxManager().WithSession(ms)
	ch := Change{Target: "selinux/mode", Before: "permissive", After: "enforcing"}
	if err := m.applyMode(context.Background(), &ch); err == nil {
		t.Error("expected err")
	}
}

func TestSELinuxManager_ApplyMode_FromDisabled(t *testing.T) {
	// Transition from disabled → enforcing: no setenforce, only sed.
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := Change{Target: "selinux/mode", Before: "disabled", After: "enforcing"}
	if err := m.applyMode(context.Background(), &ch); err != nil {
		t.Fatal(err)
	}
	// No setenforce call.
	for _, c := range ms.cmds {
		if strings.Contains(c, "setenforce") {
			t.Errorf("setenforce should not run when transitioning from disabled; got %v", ms.cmds)
		}
	}
}

func TestSELinuxManager_ApplyMode_BadType(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := Change{After: 42}
	if err := m.applyMode(context.Background(), &ch); err == nil {
		t.Error("want error")
	}
}

func TestSELinuxManager_ApplyBoolean_BadType(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	if err := m.applyBoolean(context.Background(), Change{After: "x"}); err == nil {
		t.Error("want error")
	}
}

func TestSELinuxManager_Rollback_NoSession(t *testing.T) {
	m := NewSELinuxManager()
	if err := m.Rollback(context.Background(), []Change{{}}); err == nil {
		t.Error("want err")
	}
}

func TestSELinuxManager_Rollback_SkipsNilBefore(t *testing.T) {
	ms := newMockSession()
	m := NewSELinuxManager().WithSession(ms)
	ch := []Change{{Target: "selinux/mode", After: "enforcing"}}
	if err := m.Rollback(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	if len(ms.cmds) != 0 {
		t.Errorf("should not run any cmds; got %v", ms.cmds)
	}
}

func TestSELinuxManager_Run_NoSession(t *testing.T) {
	m := NewSELinuxManager()
	if err := m.run(context.Background(), "ls"); err == nil {
		t.Error("expected err")
	}
}

func TestSELinuxManager_Run_ErrWithStderr(t *testing.T) {
	ms := newMockSession()
	ms.on("failing-cmd", "", fmt.Errorf("exit 1"))
	ms.responses["failing-cmd"] = mockResponse{stderr: "nope", err: fmt.Errorf("exit 1")}
	m := NewSELinuxManager().WithSession(ms)
	err := m.run(context.Background(), "failing-cmd")
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("want err with stderr, got %v", err)
	}
}

func TestSELinuxManager_CastValueSELinuxConfig(t *testing.T) {
	got, err := castSELinuxConfig(config.SELinuxConfig{Mode: "permissive"})
	if err != nil || got == nil {
		t.Errorf("value form: got (%v, %v)", got, err)
	}
}

func TestSELinuxManager_CastNilLinuxPointer(t *testing.T) {
	var l *config.Linux
	got, err := castSELinuxConfig(l)
	if err != nil || got != nil {
		t.Errorf("nil *Linux → nil,nil; got (%v,%v)", got, err)
	}
}

func TestSELinuxManager_CastVariants(t *testing.T) {
	m := NewSELinuxManager().WithSession(newMockSession())
	_, err := m.Plan(context.Background(), config.Linux{SELinux: &config.SELinuxConfig{Mode: "permissive"}}, nil)
	if err != nil {
		t.Error(err)
	}
	_, err = m.Plan(context.Background(), &config.Linux{SELinux: &config.SELinuxConfig{Mode: "permissive"}}, nil)
	if err != nil {
		t.Error(err)
	}
	changes, err := m.Plan(context.Background(), nil, nil)
	if err != nil {
		t.Error(err)
	}
	if changes != nil {
		t.Error("nil spec → nil changes")
	}
}
