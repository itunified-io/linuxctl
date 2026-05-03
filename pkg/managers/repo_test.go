package managers

import (
	"context"
	"strings"
	"testing"
)

// repolist output fixtures.
const (
	enabledOnlyAppstream = "repo id    repo name\nol9_appstream   Oracle Linux 9 (x86_64) AppStream\nol9_baseos      Oracle Linux 9 (x86_64) BaseOS\n"
	allReposCRBKnown     = "repo id    repo name\nol9_appstream             Oracle Linux 9 AppStream\nol9_baseos                Oracle Linux 9 BaseOS\nol9_codeready_builder     Oracle Linux 9 CodeReady Builder\n"
	allReposEnabledCRB   = "repo id    repo name\nol9_appstream             Oracle Linux 9 AppStream\nol9_baseos                Oracle Linux 9 BaseOS\nol9_codeready_builder     Oracle Linux 9 CodeReady Builder\n"
)

func newRepoMock() *fileMockSession {
	return newFileMock().
		on("repolist --enabled", enabledOnlyAppstream, nil).
		on("repolist --all", allReposCRBKnown, nil).
		on("config-manager --set-enabled", "", nil).
		on("config-manager --set-disabled", "", nil)
}

func TestRepoManager_Plan_EnableMissingRepo(t *testing.T) {
	m := NewRepoManager().WithSession(newRepoMock())
	desired := []string{"ol9_codeready_builder"}
	changes, err := m.Plan(context.Background(), Spec(desired), nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].Target != "ol9_codeready_builder" {
		t.Errorf("Target = %q, want ol9_codeready_builder", changes[0].Target)
	}
	if changes[0].Action != "update" {
		t.Errorf("Action = %q, want update", changes[0].Action)
	}
}

func TestRepoManager_Plan_AlreadyEnabled_NoChange(t *testing.T) {
	mock := newFileMock().
		on("repolist --enabled", allReposEnabledCRB, nil).
		on("repolist --all", allReposCRBKnown, nil)
	m := NewRepoManager().WithSession(mock)
	desired := []string{"ol9_codeready_builder"}
	changes, err := m.Plan(context.Background(), Spec(desired), nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes (already enabled), got %d", len(changes))
	}
}

func TestRepoManager_Plan_UnknownRepo_Errors(t *testing.T) {
	m := NewRepoManager().WithSession(newRepoMock())
	desired := []string{"nonexistent_repo"}
	_, err := m.Plan(context.Background(), Spec(desired), nil)
	if err == nil {
		t.Fatal("expected error for unknown repo, got nil")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error = %v, want 'not configured'", err)
	}
}

func TestRepoManager_Plan_EmptyDesired_NoChange(t *testing.T) {
	m := NewRepoManager().WithSession(newRepoMock())
	changes, err := m.Plan(context.Background(), Spec([]string{}), nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}
}

func TestRepoManager_Plan_RequiresSession(t *testing.T) {
	m := NewRepoManager()
	_, err := m.Plan(context.Background(), Spec([]string{"ol9_codeready_builder"}), nil)
	if err != ErrSessionRequired {
		t.Errorf("err = %v, want ErrSessionRequired", err)
	}
}

func TestRepoManager_Apply_RunsConfigManager(t *testing.T) {
	mock := newRepoMock()
	m := NewRepoManager().WithSession(mock)
	changes, err := m.Plan(context.Background(), Spec([]string{"ol9_codeready_builder"}), nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	res, err := m.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Applied) != 1 {
		t.Errorf("Applied = %d, want 1", len(res.Applied))
	}
	if !mock.ran("dnf config-manager --set-enabled 'ol9_codeready_builder'") {
		t.Errorf("expected dnf config-manager --set-enabled call, cmds = %v", mock.cmds)
	}
}

func TestRepoManager_Apply_DryRun_NoCommands(t *testing.T) {
	mock := newRepoMock()
	m := NewRepoManager().WithSession(mock)
	changes, _ := m.Plan(context.Background(), Spec([]string{"ol9_codeready_builder"}), nil)
	cmdsBefore := len(mock.cmds)
	res, err := m.Apply(context.Background(), changes, true)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Skipped) != 1 {
		t.Errorf("Skipped = %d, want 1", len(res.Skipped))
	}
	// Apply must not run any commands beyond what Plan ran.
	if len(mock.cmds) != cmdsBefore {
		t.Errorf("dry-run ran extra commands: %v", mock.cmds[cmdsBefore:])
	}
}

func TestRepoManager_Verify_OK_WhenNoDrift(t *testing.T) {
	mock := newFileMock().
		on("repolist --enabled", allReposEnabledCRB, nil).
		on("repolist --all", allReposCRBKnown, nil)
	m := NewRepoManager().WithSession(mock)
	res, err := m.Verify(context.Background(), Spec([]string{"ol9_codeready_builder"}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK {
		t.Errorf("Verify OK = false, want true; drift = %+v", res.Drift)
	}
}

func TestRepoManager_Rollback_DisablesRepo(t *testing.T) {
	mock := newRepoMock()
	m := NewRepoManager().WithSession(mock)
	changes, _ := m.Plan(context.Background(), Spec([]string{"ol9_codeready_builder"}), nil)
	if err := m.Rollback(context.Background(), changes); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if !mock.ran("dnf config-manager --set-disabled 'ol9_codeready_builder'") {
		t.Errorf("expected --set-disabled call, cmds = %v", mock.cmds)
	}
}

func TestRepoManager_Idempotency(t *testing.T) {
	// Two consecutive Plans against the post-Apply state must yield empty diffs.
	mock := newFileMock().
		on("repolist --enabled", allReposEnabledCRB, nil).
		on("repolist --all", allReposCRBKnown, nil)
	m := NewRepoManager().WithSession(mock)
	for i := 0; i < 2; i++ {
		changes, err := m.Plan(context.Background(), Spec([]string{"ol9_codeready_builder"}), nil)
		if err != nil {
			t.Fatalf("Plan #%d: %v", i, err)
		}
		if len(changes) != 0 {
			t.Errorf("Plan #%d: expected 0 changes (idempotent), got %d", i, len(changes))
		}
	}
}

func TestParseRepolist_StripsHeader(t *testing.T) {
	out := parseRepolist(allReposCRBKnown)
	if !out["ol9_appstream"] || !out["ol9_codeready_builder"] {
		t.Errorf("missing repo IDs: %+v", out)
	}
	// "repo" header line must NOT make it into the set.
	if out["repo"] {
		t.Errorf("header line leaked into result")
	}
}
