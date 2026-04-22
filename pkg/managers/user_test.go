package managers

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockSession records every command and returns scripted responses.
type mockSession struct {
	// responses maps a substring-key → (stdout, stderr, err). The first matching
	// key (in insertion order, tracked separately) wins.
	keys      []string
	responses map[string]mockResponse
	cmds      []string
}

type mockResponse struct {
	stdout string
	stderr string
	err    error
}

func newMockSession() *mockSession {
	return &mockSession{responses: map[string]mockResponse{}}
}

func (m *mockSession) on(keyContains string, stdout string, err error) *mockSession {
	m.keys = append(m.keys, keyContains)
	m.responses[keyContains] = mockResponse{stdout: stdout, err: err}
	return m
}

func (m *mockSession) Run(_ context.Context, cmd string) (string, string, error) {
	m.cmds = append(m.cmds, cmd)
	for _, k := range m.keys {
		if strings.Contains(cmd, k) {
			r := m.responses[k]
			return r.stdout, r.stderr, r.err
		}
	}
	return "", "", nil
}

func (m *mockSession) ranContaining(sub string) bool {
	for _, c := range m.cmds {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func TestUserManager_Plan_NewUser(t *testing.T) {
	ms := newMockSession().
		on("getent group 'admins'", "", fmt.Errorf("not found")).
		on("getent passwd 'alice'", "", fmt.Errorf("not found"))
	u := NewUserManager().WithSession(ms)

	spec := UsersGroupsSpec{
		Groups: []GroupSpec{{Name: "admins", GID: 2000}},
		Users:  []UserSpec{{Name: "alice", UID: 3000, GID: "admins", Shell: "/bin/bash"}},
	}
	changes, err := u.Plan(context.Background(), spec, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("want 2 changes, got %d: %+v", len(changes), changes)
	}
	if changes[0].Action != "create" || changes[0].Target != "group/admins" {
		t.Errorf("first change wrong: %+v", changes[0])
	}
	if changes[1].Action != "create" || changes[1].Target != "user/alice" {
		t.Errorf("second change wrong: %+v", changes[1])
	}
}

func TestUserManager_Plan_NoDrift(t *testing.T) {
	ms := newMockSession().
		on("getent group 'admins'", "admins:x:2000:", nil).
		on("getent passwd 'alice'", "alice:x:3000:2000::/home/alice:/bin/bash", nil).
		on("id -nG 'alice'", "admins", nil).
		on("authorized_keys", "", nil)
	u := NewUserManager().WithSession(ms)

	spec := UsersGroupsSpec{
		Groups: []GroupSpec{{Name: "admins", GID: 2000}},
		Users:  []UserSpec{{Name: "alice", UID: 3000, Home: "/home/alice", Shell: "/bin/bash", Groups: []string{"admins"}}},
	}
	changes, err := u.Plan(context.Background(), spec, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("want 0 changes (no drift), got %d: %+v", len(changes), changes)
	}
}

func TestUserManager_Plan_SSHKeyDrift(t *testing.T) {
	ms := newMockSession().
		on("getent passwd 'alice'", "alice:x:3000:3000::/home/alice:/bin/bash", nil).
		on("id -nG 'alice'", "alice", nil).
		on("authorized_keys", "ssh-ed25519 AAAAold user@old\n", nil)
	u := NewUserManager().WithSession(ms)

	spec := UsersGroupsSpec{
		Users: []UserSpec{{
			Name: "alice", UID: 3000, Home: "/home/alice", Shell: "/bin/bash",
			SSHKeys: []string{"ssh-ed25519 AAAAnew user@new"},
		}},
	}
	changes, err := u.Plan(context.Background(), spec, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 || changes[0].Action != "update" {
		t.Fatalf("want 1 update, got %+v", changes)
	}
}

func TestUserManager_Apply_CreateUser(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)

	changes := []Change{{
		ID: "user:alice", Manager: "user", Target: "user/alice", Action: "create",
		After: UserSpec{
			Name: "alice", UID: 3000, GID: "admins", Shell: "/bin/bash",
			Home: "/home/alice", SSHKeys: []string{"ssh-ed25519 AAAA user@host"},
		},
	}}
	res, err := u.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Applied) != 1 || len(res.Failed) != 0 {
		t.Fatalf("want 1 applied 0 failed, got %+v", res)
	}
	if !ms.ranContaining("useradd") {
		t.Error("expected useradd command")
	}
	if !ms.ranContaining("authorized_keys") {
		t.Error("expected authorized_keys write")
	}
}

func TestUserManager_Apply_DryRun(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	changes := []Change{{Target: "user/x", Action: "create", After: UserSpec{Name: "x"}}}
	res, err := u.Apply(context.Background(), changes, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || len(res.Applied) != 0 {
		t.Fatalf("dry-run should skip all: %+v", res)
	}
	if len(ms.cmds) != 0 {
		t.Errorf("dry-run should not execute commands: %v", ms.cmds)
	}
}

func TestUserManager_Apply_GroupAddBeforeUserAdd(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	// Provide user change first to verify reordering.
	changes := []Change{
		{Target: "user/bob", Action: "create", After: UserSpec{Name: "bob"}},
		{Target: "group/dev", Action: "create", After: GroupSpec{Name: "dev", GID: 5000}},
	}
	res, err := u.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failed) != 0 {
		t.Fatalf("unexpected failures: %+v", res.Failed)
	}
	// groupadd must appear before useradd.
	groupIdx, userIdx := -1, -1
	for i, c := range ms.cmds {
		if strings.Contains(c, "groupadd") && groupIdx == -1 {
			groupIdx = i
		}
		if strings.Contains(c, "useradd") && userIdx == -1 {
			userIdx = i
		}
	}
	if groupIdx == -1 || userIdx == -1 || groupIdx > userIdx {
		t.Errorf("groupadd must run before useradd; got cmds=%v", ms.cmds)
	}
}

func TestUserManager_Rollback_Create(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	changes := []Change{
		{Target: "group/dev", Action: "create", After: GroupSpec{Name: "dev"}},
		{Target: "user/bob", Action: "create", After: UserSpec{Name: "bob"}},
	}
	if err := u.Rollback(context.Background(), changes); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if !ms.ranContaining("userdel -r") {
		t.Error("expected userdel -r")
	}
	if !ms.ranContaining("groupdel") {
		t.Error("expected groupdel")
	}
}

func TestUserManager_Verify_Matches(t *testing.T) {
	ms := newMockSession().
		on("getent group 'admins'", "admins:x:2000:", nil)
	u := NewUserManager().WithSession(ms)

	spec := UsersGroupsSpec{Groups: []GroupSpec{{Name: "admins", GID: 2000}}}
	vr, err := u.Verify(context.Background(), spec)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !vr.OK || len(vr.Drift) != 0 {
		t.Errorf("want OK, no drift, got %+v", vr)
	}
}

func TestUserManager_CastUsersGroups_Unsupported(t *testing.T) {
	u := NewUserManager().WithSession(newMockSession())
	_, err := u.Plan(context.Background(), "not-a-spec", nil)
	if err == nil {
		t.Error("expected error for unsupported desired type")
	}
}

func TestUserManager_Apply_UpdateUser(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	changes := []Change{{
		Target: "user/alice", Action: "update",
		Before: currentUser{UID: 3000, Home: "/home/alice", Shell: "/bin/sh"},
		After: UserSpec{
			Name: "alice", GID: "admins", Groups: []string{"wheel"},
			Shell: "/bin/bash", Home: "/home/alice",
			SSHKeys:  []string{"ssh-ed25519 KEY user"},
			Password: "$6$rounds=5000$abc$def",
		},
	}}
	res, err := u.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failed) != 0 {
		t.Fatalf("expected no failures, got %+v", res.Failed)
	}
	wantSubs := []string{"usermod -g", "usermod -G", "usermod -s", "usermod -d", "chpasswd -e", "authorized_keys"}
	for _, s := range wantSubs {
		if !ms.ranContaining(s) {
			t.Errorf("expected to run %q, got %v", s, ms.cmds)
		}
	}
}

func TestUserManager_Apply_DeleteUser(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	changes := []Change{{Target: "user/alice", Action: "delete", After: UserSpec{Name: "alice"}}}
	res, err := u.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 1 {
		t.Fatalf("want 1 applied, got %+v", res)
	}
	if !ms.ranContaining("userdel -r") {
		t.Error("expected userdel -r")
	}
}

func TestUserManager_Apply_UpdateGroup(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	changes := []Change{{Target: "group/admins", Action: "update", After: GroupSpec{Name: "admins", GID: 2500}}}
	if _, err := u.Apply(context.Background(), changes, false); err != nil {
		t.Fatal(err)
	}
	if !ms.ranContaining("groupmod -g 2500") {
		t.Errorf("expected groupmod, got %v", ms.cmds)
	}
}

func TestUserManager_Apply_NoSession(t *testing.T) {
	u := NewUserManager()
	_, err := u.Apply(context.Background(), []Change{{Target: "user/x", Action: "create"}}, false)
	if err == nil {
		t.Error("want error without session")
	}
}

func TestUserManager_Rollback_UpdateRestoresBefore(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	changes := []Change{{
		Target: "user/alice", Action: "update",
		Before: UserSpec{Name: "alice", Shell: "/bin/sh"},
		After:  UserSpec{Name: "alice", Shell: "/bin/bash"},
	}, {
		Target: "group/admins", Action: "update",
		Before: GroupSpec{Name: "admins", GID: 2000},
		After:  GroupSpec{Name: "admins", GID: 2500},
	}}
	if err := u.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if !ms.ranContaining("usermod -s") {
		t.Error("expected usermod -s during rollback")
	}
	if !ms.ranContaining("groupmod -g 2000") {
		t.Errorf("expected groupmod -g 2000, got %v", ms.cmds)
	}
}

func TestUserManager_CastUsersGroups_PointerAndNil(t *testing.T) {
	if _, err := castUsersGroups(&UsersGroupsSpec{}); err != nil {
		t.Errorf("pointer form: %v", err)
	}
	var p *UsersGroupsSpec
	if _, err := castUsersGroups(p); err != nil {
		t.Errorf("nil pointer form: %v", err)
	}
	if _, err := castUsersGroups(nil); err != nil {
		t.Errorf("nil form: %v", err)
	}
}

func TestUserManager_Apply_UnknownTarget(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	changes := []Change{{Target: "unknown/x", Action: "create"}}
	res, err := u.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failed) != 1 {
		t.Errorf("expected 1 failure, got %+v", res)
	}
}

func TestUserManager_Rollback_NoSession(t *testing.T) {
	u := NewUserManager()
	if err := u.Rollback(context.Background(), []Change{{Target: "user/x", Action: "create"}}); err == nil {
		t.Error("want session-required")
	}
}

func TestUserManager_Rollback_UpdateSkipsNonMatchingBefore(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	// Before is not a UserSpec → the update rollback should skip silently.
	changes := []Change{{Target: "user/alice", Action: "update", Before: "string"}}
	if err := u.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if len(ms.cmds) != 0 {
		t.Errorf("should skip; got %v", ms.cmds)
	}
}

func TestUserManager_Rollback_UpdateNilBeforeSkipped(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	changes := []Change{{Target: "user/alice", Action: "update"}}
	if err := u.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if len(ms.cmds) != 0 {
		t.Errorf("should skip nil before; got %v", ms.cmds)
	}
}

func TestUserManager_ApplyUser_BadAfterType(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	err := u.applyUser(context.Background(), Change{Action: "create", Target: "user/x", After: "wrong"})
	if err == nil {
		t.Error("want error")
	}
}

func TestUserManager_ApplyGroup_BadAfterType(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	err := u.applyGroup(context.Background(), Change{Action: "create", Target: "group/x", After: "wrong"})
	if err == nil {
		t.Error("want error")
	}
}

func TestUserManager_ApplyGroup_UpdateZeroGIDNoop(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	ch := Change{Action: "update", Target: "group/x", After: GroupSpec{Name: "x", GID: 0}}
	if err := u.applyGroup(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	if len(ms.cmds) != 0 {
		t.Errorf("zero GID update should be no-op; got %v", ms.cmds)
	}
}

func TestUserManager_ApplyGroup_UnknownAction(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	err := u.applyGroup(context.Background(), Change{Action: "weird", After: GroupSpec{Name: "x"}})
	if err == nil {
		t.Error("want error")
	}
}

func TestUserManager_ApplyUser_UnknownAction(t *testing.T) {
	ms := newMockSession()
	u := NewUserManager().WithSession(ms)
	err := u.applyUser(context.Background(), Change{Action: "weird", After: UserSpec{Name: "x"}})
	if err == nil {
		t.Error("want error")
	}
}

func TestUserManager_Run_NoSession(t *testing.T) {
	u := NewUserManager()
	if err := u.run(context.Background(), "ls"); err == nil {
		t.Error("want error")
	}
	_, _, err := u.runOut(context.Background(), "ls")
	if err == nil {
		t.Error("want error")
	}
}

func TestUserManager_Run_ErrWithStderr(t *testing.T) {
	ms := newMockSession()
	ms.on("fail", "", fmt.Errorf("boom"))
	ms.responses["fail"] = mockResponse{stderr: "oh no", err: fmt.Errorf("boom")}
	u := NewUserManager().WithSession(ms)
	err := u.run(context.Background(), "fail")
	if err == nil || !strings.Contains(err.Error(), "oh no") {
		t.Errorf("want err with stderr, got %v", err)
	}
}

func TestUserDrift_AllFields(t *testing.T) {
	base := currentUser{UID: 1000, Home: "/home/a", Shell: "/bin/bash", Groups: []string{"g"}, SSHKeys: []string{"k1"}}
	cases := []struct {
		name  string
		spec  UserSpec
		drift bool
	}{
		{"no drift", UserSpec{UID: 1000, Home: "/home/a", Shell: "/bin/bash", Groups: []string{"g"}, SSHKeys: []string{"k1"}}, false},
		{"uid drift", UserSpec{UID: 2000}, true},
		{"home drift", UserSpec{Home: "/home/b"}, true},
		{"shell drift", UserSpec{Shell: "/bin/sh"}, true},
		{"groups drift", UserSpec{Groups: []string{"x"}}, true},
		{"keys drift", UserSpec{SSHKeys: []string{"k2"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := userDrift(tc.spec, base); got != tc.drift {
				t.Errorf("want %v, got %v", tc.drift, got)
			}
		})
	}
}

func TestSameStringSet(t *testing.T) {
	if !sameStringSet([]string{"a", "b"}, []string{"b", "a"}) {
		t.Error("want equal")
	}
	if sameStringSet([]string{"a"}, []string{"a", "b"}) {
		t.Error("want not equal")
	}
	if sameStringSet([]string{"a"}, []string{"b"}) {
		t.Error("want not equal")
	}
}

func TestUserManager_OrderKey_AllBranches(t *testing.T) {
	if orderKey(Change{Target: "group/a", Action: "create"}) != 0 {
		t.Error("group create")
	}
	if orderKey(Change{Target: "group/a", Action: "update"}) != 1 {
		t.Error("group update")
	}
	if orderKey(Change{Target: "user/a", Action: "create"}) != 2 {
		t.Error("user create")
	}
	if orderKey(Change{Target: "user/a", Action: "update"}) != 3 {
		t.Error("user update")
	}
	if orderKey(Change{Target: "user/a", Action: "delete"}) != 4 {
		t.Error("delete default")
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"plain":      `'plain'`,
		"with space": `'with space'`,
		"it's":       `'it'"'"'s'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
