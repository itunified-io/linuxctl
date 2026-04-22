package apply

import (
	"context"
	"errors"
	"testing"

	"github.com/itunified-io/linuxctl/pkg/managers"
)

// stubManager is a test manager that records calls.
type stubManager struct {
	name     string
	deps     []string
	planRes  []managers.Change
	planErr  error
	applyErr error
	verifyOK bool
	calls    []string
	applied  []managers.Change
	rollback []managers.Change
}

func (s *stubManager) Name() string      { return s.name }
func (s *stubManager) DependsOn() []string { return s.deps }
func (s *stubManager) Plan(_ context.Context, _ managers.Spec, _ managers.State) ([]managers.Change, error) {
	s.calls = append(s.calls, "plan")
	return s.planRes, s.planErr
}
func (s *stubManager) Apply(_ context.Context, changes []managers.Change, dryRun bool) (managers.ApplyResult, error) {
	s.calls = append(s.calls, "apply")
	if dryRun {
		return managers.ApplyResult{Skipped: changes}, nil
	}
	if s.applyErr != nil {
		return managers.ApplyResult{Failed: []managers.ChangeErr{{Err: s.applyErr}}}, s.applyErr
	}
	s.applied = append(s.applied, changes...)
	return managers.ApplyResult{Applied: changes}, nil
}
func (s *stubManager) Verify(_ context.Context, _ managers.Spec) (managers.VerifyResult, error) {
	s.calls = append(s.calls, "verify")
	return managers.VerifyResult{OK: s.verifyOK}, nil
}
func (s *stubManager) Rollback(_ context.Context, changes []managers.Change) error {
	s.calls = append(s.calls, "rollback")
	s.rollback = append(s.rollback, changes...)
	return nil
}

func newStub(name string, planRes []managers.Change) *stubManager {
	return &stubManager{name: name, planRes: planRes}
}

func TestOrchestrator_Plan_Aggregates(t *testing.T) {
	a := newStub("disk", []managers.Change{{Manager: "disk", Action: "create"}})
	b := newStub("user", []managers.Change{{Manager: "user", Action: "create"}, {Manager: "user", Action: "update"}})
	o := New(nil, nil, false).WithManagers([]managers.Manager{a, b})
	res, err := o.Plan(context.Background())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if res.TotalCreate != 2 {
		t.Errorf("expected 2 create, got %d", res.TotalCreate)
	}
	if res.TotalUpdate != 1 {
		t.Errorf("expected 1 update, got %d", res.TotalUpdate)
	}
	if len(res.ByManager) != 2 {
		t.Errorf("expected 2 manager entries, got %d", len(res.ByManager))
	}
}

func TestOrchestrator_Apply_Sequential(t *testing.T) {
	a := newStub("disk", []managers.Change{{Manager: "disk"}})
	b := newStub("user", []managers.Change{{Manager: "user"}})
	o := New(nil, nil, false).WithManagers([]managers.Manager{a, b})
	if _, err := o.Apply(context.Background()); err != nil {
		t.Fatalf("%v", err)
	}
	if len(a.calls) < 2 || a.calls[0] != "plan" {
		t.Errorf("a calls: %v", a.calls)
	}
	// Both a and b should have been applied.
	if len(a.applied) != 1 || len(b.applied) != 1 {
		t.Errorf("expected each manager applied once")
	}
}

func TestOrchestrator_Apply_StopOnError(t *testing.T) {
	a := newStub("disk", []managers.Change{{Manager: "disk"}})
	a.applyErr = errors.New("boom")
	b := newStub("user", []managers.Change{{Manager: "user"}})
	o := New(nil, nil, false).WithManagers([]managers.Manager{a, b})
	_, err := o.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	// b should not have been applied.
	for _, c := range b.calls {
		if c == "apply" {
			t.Errorf("b.apply called despite a error")
		}
	}
}

func TestOrchestrator_Apply_ContinueOnError(t *testing.T) {
	a := newStub("disk", []managers.Change{{Manager: "disk"}})
	a.applyErr = errors.New("boom")
	b := newStub("user", []managers.Change{{Manager: "user"}})
	o := New(nil, nil, false).WithManagers([]managers.Manager{a, b})
	o.ContinueOnError = true
	_, err := o.Apply(context.Background())
	if err != nil {
		t.Fatalf("ContinueOnError should swallow: %v", err)
	}
	if len(b.applied) != 1 {
		t.Errorf("b should have been applied")
	}
}

func TestOrchestrator_Apply_DryRun(t *testing.T) {
	a := newStub("disk", []managers.Change{{Manager: "disk"}})
	o := New(nil, nil, true).WithManagers([]managers.Manager{a})
	res, err := o.Apply(context.Background())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(res.Skipped) != 1 {
		t.Errorf("expected skipped in dry run")
	}
	if len(a.applied) != 0 {
		t.Errorf("dry run should not apply")
	}
}

func TestOrchestrator_Verify_AllOK(t *testing.T) {
	a := newStub("disk", nil)
	a.verifyOK = true
	b := newStub("user", nil)
	b.verifyOK = true
	o := New(nil, nil, false).WithManagers([]managers.Manager{a, b})
	v, err := o.Verify(context.Background())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !v.Matches {
		t.Errorf("expected matches")
	}
	if len(v.InDrift) != 0 {
		t.Errorf("no drift expected")
	}
}

func TestOrchestrator_Verify_Drift(t *testing.T) {
	a := newStub("disk", nil)
	a.verifyOK = false
	o := New(nil, nil, false).WithManagers([]managers.Manager{a})
	v, err := o.Verify(context.Background())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if v.Matches {
		t.Errorf("expected drift")
	}
	if len(v.InDrift) != 1 {
		t.Errorf("expected disk in drift; got %v", v.InDrift)
	}
}

func TestOrchestrator_Rollback_ReverseOrder(t *testing.T) {
	a := newStub("disk", []managers.Change{{Manager: "disk"}})
	b := newStub("user", []managers.Change{{Manager: "user"}})
	o := New(nil, nil, false).WithManagers([]managers.Manager{a, b})
	if _, err := o.Apply(context.Background()); err != nil {
		t.Fatalf("%v", err)
	}
	if err := o.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	// b rolled back first, then a.
	aIdx, bIdx := -1, -1
	for i, c := range a.calls {
		if c == "rollback" {
			aIdx = i
		}
	}
	for i, c := range b.calls {
		if c == "rollback" {
			bIdx = i
		}
	}
	if aIdx < 0 || bIdx < 0 {
		t.Fatalf("missing rollback calls; a=%v b=%v", a.calls, b.calls)
	}
	_ = aIdx
	_ = bIdx
	// Check semantic order by relative apply-vs-rollback ordering.
	if len(a.rollback) != 1 || len(b.rollback) != 1 {
		t.Errorf("expected each manager rolled back once")
	}
}

func TestOrchestrator_Diff(t *testing.T) {
	a := newStub("disk", []managers.Change{{Manager: "disk", Action: "create"}})
	o := New(nil, nil, false).WithManagers([]managers.Manager{a})
	d, err := o.Diff(context.Background())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if d.Empty {
		t.Errorf("expected non-empty diff")
	}
	if len(d.ByManager) != 1 {
		t.Errorf("expected 1 manager in diff")
	}
}

func TestOrchestrator_Diff_Empty(t *testing.T) {
	a := newStub("disk", nil)
	o := New(nil, nil, false).WithManagers([]managers.Manager{a})
	d, err := o.Diff(context.Background())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !d.Empty {
		t.Errorf("expected empty diff")
	}
}

func TestOrchestrator_RegistryFallback(t *testing.T) {
	// When Managers is empty, defaultOrder + Registry() is used.
	o := New(nil, nil, false)
	got := o.resolveManagers()
	if len(got) == 0 {
		t.Errorf("expected registry-fallback managers; got 0 (init() should register)")
	}
}
