package apply

import (
	"context"
	"errors"
	"testing"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/managers"
)

// recording stub that captures the Spec it was passed, to exercise desiredFor.
type specCaptureStub struct {
	name string
	got  managers.Spec
}

func (s *specCaptureStub) Name() string        { return s.name }
func (s *specCaptureStub) DependsOn() []string { return nil }
func (s *specCaptureStub) Plan(_ context.Context, sp managers.Spec, _ managers.State) ([]managers.Change, error) {
	s.got = sp
	return nil, nil
}
func (s *specCaptureStub) Apply(_ context.Context, _ []managers.Change, _ bool) (managers.ApplyResult, error) {
	return managers.ApplyResult{}, nil
}
func (s *specCaptureStub) Verify(_ context.Context, sp managers.Spec) (managers.VerifyResult, error) {
	s.got = sp
	return managers.VerifyResult{OK: true}, nil
}
func (s *specCaptureStub) Rollback(_ context.Context, _ []managers.Change) error { return nil }

func TestOrchestrator_WithLinux_ReturnsOrchestrator(t *testing.T) {
	l := &config.Linux{Kind: "Linux"}
	o := New(nil, nil, false).WithLinux(l)
	if o.Linux != l {
		t.Fatalf("expected linux to be set")
	}
}

func TestOrchestrator_DesiredFor_AllManagerNames(t *testing.T) {
	l := &config.Linux{
		Kind:        "Linux",
		DiskLayout:  &config.DiskLayout{},
		Mounts:      []config.Mount{{Type: "nfs", MountPoint: "/mnt/x", Source: "s:/y"}},
		UsersGroups: &config.UsersGroups{},
		Directories: []config.Directory{{Path: "/a"}},
		Packages:    &config.Packages{},
		Sysctl:      []config.SysctlEntry{{Key: "vm.swappiness", Value: "10"}},
	}
	stubs := []*specCaptureStub{
		{name: "disk"},
		{name: "mount"},
		{name: "user"},
		{name: "dir"},
		{name: "package"},
		{name: "sysctl"}, // falls through to default branch → full Linux
		{name: "unknown-manager"},
	}
	mgrs := make([]managers.Manager, len(stubs))
	for i, s := range stubs {
		mgrs[i] = s
	}
	o := New(nil, nil, false).WithLinux(l).WithManagers(mgrs)
	if _, err := o.Plan(context.Background()); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// disk → DiskLayout
	if stubs[0].got != l.DiskLayout {
		t.Errorf("disk spec mismatch")
	}
	// mount → Mounts slice (compare lengths — []Mount is not directly comparable)
	if got, ok := stubs[1].got.([]config.Mount); !ok || len(got) != 1 {
		t.Errorf("mount spec mismatch: %T", stubs[1].got)
	}
	// user → full Linux (so bundle_preset can be expanded by
	// UserManager.castUsersGroups → usersGroupsFromLinux). Fixes linuxctl#21.
	if stubs[2].got != l {
		t.Errorf("user spec mismatch: want full *Linux for bundle_preset, got %T", stubs[2].got)
	}
	// dir → full Linux (preset expansion). Fixes linuxctl#21.
	if stubs[3].got != l {
		t.Errorf("dir spec mismatch: want full *Linux for bundle_preset, got %T", stubs[3].got)
	}
	// package → full Linux (so bundle_preset can be expanded by
	// PackageManager.castPackages → packagesFromLinux). Fixes linuxctl#21.
	if stubs[4].got != l {
		t.Errorf("package spec mismatch: want full *Linux for bundle_preset, got %T", stubs[4].got)
	}
	// sysctl → []SysctlEntry (typed slice the manager expects directly).
	// Per #25, orchestrator now dispatches per-manager rather than passing
	// full *Linux as a fall-through default.
	if got, ok := stubs[5].got.([]config.SysctlEntry); !ok || len(got) != 1 {
		t.Errorf("sysctl spec mismatch: %T", stubs[5].got)
	}
	// unknown → nil (default branch). Avoids passing *config.Linux to
	// managers whose cast helpers reject it (e.g. NetworkManager).
	if stubs[6].got != nil {
		t.Errorf("unknown default-branch should receive nil; got %T", stubs[6].got)
	}
}

func TestOrchestrator_DesiredFor_NilLinux(t *testing.T) {
	s := &specCaptureStub{name: "sysctl"}
	o := New(nil, nil, false).WithManagers([]managers.Manager{s})
	if _, err := o.Plan(context.Background()); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if s.got != nil {
		t.Errorf("expected nil Spec when Linux is nil; got %T", s.got)
	}
}

// Plan error propagates with manager name.
type planErrStub struct {
	name string
	err  error
}

func (p *planErrStub) Name() string        { return p.name }
func (p *planErrStub) DependsOn() []string { return nil }
func (p *planErrStub) Plan(context.Context, managers.Spec, managers.State) ([]managers.Change, error) {
	return nil, p.err
}
func (p *planErrStub) Apply(context.Context, []managers.Change, bool) (managers.ApplyResult, error) {
	return managers.ApplyResult{}, nil
}
func (p *planErrStub) Verify(context.Context, managers.Spec) (managers.VerifyResult, error) {
	return managers.VerifyResult{OK: true}, nil
}
func (p *planErrStub) Rollback(context.Context, []managers.Change) error { return nil }

func TestOrchestrator_Plan_PropagatesError(t *testing.T) {
	a := newStub("disk", []managers.Change{{Manager: "disk"}})
	b := &planErrStub{name: "user", err: errors.New("plan-bomb")}
	o := New(nil, nil, false).WithManagers([]managers.Manager{a, b})
	_, err := o.Plan(context.Background())
	if err == nil {
		t.Fatal("expected plan error")
	}
	if msg := err.Error(); msg == "" || !contains(msg, "user") || !contains(msg, "plan-bomb") {
		t.Errorf("error should name manager + cause, got %q", msg)
	}
}

func TestOrchestrator_Apply_PlanErrorShortCircuits(t *testing.T) {
	b := &planErrStub{name: "user", err: errors.New("plan-bomb")}
	o := New(nil, nil, false).WithManagers([]managers.Manager{b})
	_, err := o.Apply(context.Background())
	if err == nil {
		t.Fatal("expected apply to fail on plan error")
	}
}

func TestOrchestrator_Diff_PlanError(t *testing.T) {
	b := &planErrStub{name: "user", err: errors.New("nope")}
	o := New(nil, nil, false).WithManagers([]managers.Manager{b})
	_, err := o.Diff(context.Background())
	if err == nil {
		t.Fatal("expected diff to fail on plan error")
	}
}

// Verify error path.
type verifyErrStub struct {
	name string
}

func (v *verifyErrStub) Name() string        { return v.name }
func (v *verifyErrStub) DependsOn() []string { return nil }
func (v *verifyErrStub) Plan(context.Context, managers.Spec, managers.State) ([]managers.Change, error) {
	return nil, nil
}
func (v *verifyErrStub) Apply(context.Context, []managers.Change, bool) (managers.ApplyResult, error) {
	return managers.ApplyResult{}, nil
}
func (v *verifyErrStub) Verify(context.Context, managers.Spec) (managers.VerifyResult, error) {
	return managers.VerifyResult{}, errors.New("verify-boom")
}
func (v *verifyErrStub) Rollback(context.Context, []managers.Change) error { return nil }

func TestOrchestrator_Verify_PropagatesError(t *testing.T) {
	o := New(nil, nil, false).WithManagers([]managers.Manager{&verifyErrStub{name: "disk"}})
	_, err := o.Verify(context.Background())
	if err == nil {
		t.Fatal("expected verify error")
	}
	if !contains(err.Error(), "disk") {
		t.Errorf("error should name manager: %v", err)
	}
}

// Rollback with a manager whose Rollback errors → aggregated error.
type rollbackErrStub struct {
	stubManager
}

func (r *rollbackErrStub) Rollback(context.Context, []managers.Change) error {
	r.calls = append(r.calls, "rollback")
	return errors.New("rb-boom")
}

func TestOrchestrator_Rollback_AggregatesErrors(t *testing.T) {
	a := &rollbackErrStub{stubManager: stubManager{
		name:    "disk",
		planRes: []managers.Change{{Manager: "disk"}},
	}}
	b := &rollbackErrStub{stubManager: stubManager{
		name:    "user",
		planRes: []managers.Change{{Manager: "user"}},
	}}
	o := New(nil, nil, false).WithManagers([]managers.Manager{a, b})
	if _, err := o.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	err := o.Rollback(context.Background())
	if err == nil {
		t.Fatal("expected aggregate rollback error")
	}
	msg := err.Error()
	if !contains(msg, "disk") || !contains(msg, "user") {
		t.Errorf("aggregate should mention both managers: %q", msg)
	}
}

func TestOrchestrator_Rollback_NoAppliedIsNoop(t *testing.T) {
	a := newStub("disk", nil)
	o := New(nil, nil, false).WithManagers([]managers.Manager{a})
	if err := o.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback on empty applied should be nil: %v", err)
	}
	for _, c := range a.calls {
		if c == "rollback" {
			t.Errorf("rollback should not be called when nothing applied")
		}
	}
}

// Continue-on-error path in Apply: failure recorded but not returned.
func TestOrchestrator_Apply_ContinueOnError_NoApplyRecorded(t *testing.T) {
	a := newStub("disk", []managers.Change{{Manager: "disk"}})
	a.applyErr = errors.New("boom")
	b := newStub("user", []managers.Change{{Manager: "user"}})
	o := New(nil, nil, false).WithManagers([]managers.Manager{a, b})
	o.ContinueOnError = true
	res, err := o.Apply(context.Background())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(res.Applied) != 1 {
		t.Errorf("expected exactly b applied; got %d", len(res.Applied))
	}
	// Rollback should only touch b (a never recorded applied changes).
	if err := o.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if len(a.rollback) != 0 {
		t.Errorf("a failed apply should not be rolled back")
	}
	if len(b.rollback) != 1 {
		t.Errorf("b should have been rolled back once")
	}
}

// resolveManagers: custom manager order is honoured even when defaultOrder
// has different preference (WithManagers wins unconditionally).
func TestOrchestrator_ResolveManagers_CustomOrderWins(t *testing.T) {
	a := newStub("service", nil) // "service" is last in defaultOrder
	b := newStub("disk", nil)    // "disk" is first in defaultOrder
	o := New(nil, nil, false).WithManagers([]managers.Manager{a, b})
	got := o.resolveManagers()
	if got[0].Name() != "service" || got[1].Name() != "disk" {
		t.Errorf("WithManagers order must be preserved; got %s,%s", got[0].Name(), got[1].Name())
	}
}

// Unregistered manager referenced implicitly: if we pass only managers not in
// defaultOrder, they still all run. The "missing manager" case is covered by
// resolveManagers silently skipping.
func TestOrchestrator_ResolveManagers_IncludesExtras(t *testing.T) {
	// This registry-backed test relies on init() registering at least one
	// manager. It executes the "append extras not in defaultOrder" branch
	// by ensuring the registry returns a complete slice.
	o := New(nil, nil, false)
	mgrs := o.resolveManagers()
	if len(mgrs) == 0 {
		t.Skip("no managers registered in init() (scaffold repo)")
	}
	seen := map[string]bool{}
	for _, m := range mgrs {
		if seen[m.Name()] {
			t.Errorf("manager %q listed twice", m.Name())
		}
		seen[m.Name()] = true
	}
}

// Plan skips managers that return zero changes (the `if len(cs) == 0 continue`
// branch is exercised here explicitly).
func TestOrchestrator_Plan_SkipsEmptyManagers(t *testing.T) {
	a := newStub("disk", nil)
	b := newStub("user", []managers.Change{{Manager: "user", Action: "delete"}})
	o := New(nil, nil, false).WithManagers([]managers.Manager{a, b})
	res, err := o.Plan(context.Background())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if _, ok := res.ByManager["disk"]; ok {
		t.Errorf("empty manager should not appear in ByManager")
	}
	if res.TotalDelete != 1 {
		t.Errorf("expected 1 delete, got %d", res.TotalDelete)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
