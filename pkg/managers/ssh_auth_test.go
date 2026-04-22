package managers

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/itunified-io/linuxctl/pkg/config"
)

func TestSSHManager_Plan_AuthorizedKeysDrift(t *testing.T) {
	ms := newMockSession().
		on("/home/ec2-user/.ssh/authorized_keys", "ssh-ed25519 OLD user@old\n", nil)
	m := NewSSHAuthManager().WithSession(ms)

	cfg := &config.SSHConfig{
		AuthorizedKeys: map[string][]string{
			"ec2-user": {"ssh-ed25519 NEW user@new"},
		},
	}
	changes, err := m.Plan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 || changes[0].Action != "update" || changes[0].Target != "authorized_keys/ec2-user" {
		t.Fatalf("want 1 update for ec2-user, got %+v", changes)
	}
}

func TestSSHManager_Plan_AuthorizedKeysCreateWhenEmpty(t *testing.T) {
	ms := newMockSession().
		on("authorized_keys", "", nil)
	m := NewSSHAuthManager().WithSession(ms)

	cfg := &config.SSHConfig{
		AuthorizedKeys: map[string][]string{"bob": {"ssh-ed25519 KEY x"}},
	}
	changes, err := m.Plan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 || changes[0].Action != "create" {
		t.Fatalf("want 1 create, got %+v", changes)
	}
}

func TestSSHManager_Plan_NoDrift(t *testing.T) {
	ms := newMockSession().
		on("authorized_keys", "ssh-ed25519 KEY x\n", nil)
	m := NewSSHAuthManager().WithSession(ms)
	cfg := &config.SSHConfig{AuthorizedKeys: map[string][]string{"bob": {"ssh-ed25519 KEY x"}}}
	changes, err := m.Plan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no drift, got %+v", changes)
	}
}

func TestSSHManager_Plan_SSHDConfigDrift(t *testing.T) {
	ms := newMockSession().
		on("authorized_keys", "", nil).
		on(sshdDropInPath, "", nil)
	m := NewSSHAuthManager().WithSession(ms)
	cfg := &config.SSHConfig{
		SSHDConfig: map[string]string{"PermitRootLogin": "no", "PasswordAuthentication": "no"},
	}
	changes, err := m.Plan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 || changes[0].Target != "sshd_config/drop-in" || changes[0].Action != "create" {
		t.Fatalf("want 1 create for drop-in, got %+v", changes)
	}
}

func TestSSHManager_Plan_SSHDConfigMatches(t *testing.T) {
	existing := "# BEGIN linuxctl\nPasswordAuthentication no\nPermitRootLogin no\n# END linuxctl\n"
	ms := newMockSession().on(sshdDropInPath, existing, nil)
	m := NewSSHAuthManager().WithSession(ms)
	cfg := &config.SSHConfig{SSHDConfig: map[string]string{"PermitRootLogin": "no", "PasswordAuthentication": "no"}}
	changes, err := m.Plan(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no drift; got %+v", changes)
	}
}

func TestSSHManager_Apply_AuthorizedKeys(t *testing.T) {
	ms := newMockSession()
	m := NewSSHAuthManager().WithSession(ms)
	ch := []Change{{
		Target: "authorized_keys/alice",
		Action: "update",
		After:  []string{"ssh-ed25519 KEY1 a", "ssh-ed25519 KEY2 b"},
	}}
	res, err := m.Apply(context.Background(), ch, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 1 {
		t.Fatalf("want 1 applied, got %+v", res)
	}
	if !ms.ranContaining("authorized_keys") || !ms.ranContaining("chmod 0600") {
		t.Errorf("expected authorized_keys install, got %v", ms.cmds)
	}
}

func TestSSHManager_Apply_SSHDConfigValid(t *testing.T) {
	ms := newMockSession().on("sshd -t", "", nil)
	m := NewSSHAuthManager().WithSession(ms)
	ch := []Change{{
		Target: "sshd_config/drop-in",
		Action: "create",
		After:  map[string]string{"PermitRootLogin": "no"},
	}}
	res, err := m.Apply(context.Background(), ch, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 1 {
		t.Fatalf("want 1 applied, got %+v", res)
	}
	if !ms.ranContaining("sshd -t") {
		t.Error("expected sshd -t validation")
	}
	if !ms.ranContaining("systemctl reload") {
		t.Error("expected systemctl reload sshd")
	}
}

func TestSSHManager_Apply_SSHDConfigValidationFails(t *testing.T) {
	ms := newMockSession().on("sshd -t", "", fmt.Errorf("bad config"))
	m := NewSSHAuthManager().WithSession(ms)
	ch := []Change{{
		Target: "sshd_config/drop-in", Action: "create",
		After: map[string]string{"BogusDirective": "1"},
	}}
	res, err := m.Apply(context.Background(), ch, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failed) != 1 {
		t.Fatalf("expected 1 failure, got %+v", res)
	}
	// Drop-in must have been removed after validation failure.
	removed := false
	for _, c := range ms.cmds {
		if strings.Contains(c, "rm -f") && strings.Contains(c, sshdDropInPath) {
			removed = true
		}
	}
	if !removed {
		t.Errorf("expected drop-in removal after sshd -t failure, got %v", ms.cmds)
	}
}

func TestSSHManager_Apply_DryRun(t *testing.T) {
	ms := newMockSession()
	m := NewSSHAuthManager().WithSession(ms)
	ch := []Change{{Target: "authorized_keys/x", Action: "create", After: []string{"k"}}}
	res, err := m.Apply(context.Background(), ch, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || len(res.Applied) != 0 {
		t.Fatalf("dry-run should skip: %+v", res)
	}
	if len(ms.cmds) != 0 {
		t.Errorf("dry-run should not exec: %v", ms.cmds)
	}
}

func TestSSHManager_Apply_NoSession(t *testing.T) {
	m := NewSSHAuthManager()
	_, err := m.Apply(context.Background(), []Change{{Target: "authorized_keys/x", Action: "create"}}, false)
	if err == nil {
		t.Error("expected error without session")
	}
}

func TestSSHManager_Apply_UnknownTarget(t *testing.T) {
	ms := newMockSession()
	m := NewSSHAuthManager().WithSession(ms)
	res, err := m.Apply(context.Background(),
		[]Change{{Target: "bogus/x", Action: "create"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failed) != 1 {
		t.Errorf("expected failure, got %+v", res)
	}
}

func TestSSHManager_Rollback_AuthorizedKeys(t *testing.T) {
	ms := newMockSession()
	m := NewSSHAuthManager().WithSession(ms)
	ch := []Change{{
		Target: "authorized_keys/alice", Action: "update",
		Before: []string{"ssh-ed25519 OLD a"},
		After:  []string{"ssh-ed25519 NEW a"},
	}}
	if err := m.Rollback(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	if !ms.ranContaining("authorized_keys") {
		t.Error("expected authorized_keys restore")
	}
}

func TestSSHManager_Rollback_SSHDConfigRemovesDropIn(t *testing.T) {
	ms := newMockSession()
	m := NewSSHAuthManager().WithSession(ms)
	ch := []Change{{Target: "sshd_config/drop-in", Action: "update", Before: map[string]string{}, After: map[string]string{"X": "y"}}}
	if err := m.Rollback(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	if !ms.ranContaining("rm -f") {
		t.Errorf("expected drop-in removal, got %v", ms.cmds)
	}
}

func TestSSHManager_Verify(t *testing.T) {
	ms := newMockSession().on("authorized_keys", "ssh-ed25519 K u\n", nil)
	m := NewSSHAuthManager().WithSession(ms)
	cfg := &config.SSHConfig{AuthorizedKeys: map[string][]string{"bob": {"ssh-ed25519 K u"}}}
	vr, err := m.Verify(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !vr.OK {
		t.Errorf("expected OK, got %+v", vr)
	}
}

func TestSSHManager_CastFromLinux(t *testing.T) {
	lx := &config.Linux{SSHConfig: &config.SSHConfig{AuthorizedKeys: map[string][]string{"bob": {"k"}}}}
	ms := newMockSession().on("authorized_keys", "", nil)
	m := NewSSHAuthManager().WithSession(ms)
	changes, err := m.Plan(context.Background(), lx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Errorf("expected 1 create, got %+v", changes)
	}
}

func TestSSHManager_CastUnsupported(t *testing.T) {
	m := NewSSHAuthManager().WithSession(newMockSession())
	_, err := m.Plan(context.Background(), "bogus", nil)
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestSSHManager_Name(t *testing.T) {
	if NewSSHAuthManager().Name() != "ssh" {
		t.Errorf("unexpected name")
	}
}

func TestRenderSSHDDropIn_Deterministic(t *testing.T) {
	a := renderSSHDDropIn(map[string]string{"A": "1", "B": "2"})
	b := renderSSHDDropIn(map[string]string{"B": "2", "A": "1"})
	if a != b {
		t.Errorf("expected deterministic output:\n%s\n---\n%s", a, b)
	}
	if !strings.Contains(a, "# BEGIN linuxctl") || !strings.Contains(a, "# END linuxctl") {
		t.Errorf("expected markers: %s", a)
	}
}

func TestSetupClusterSSH(t *testing.T) {
	n1 := newMockSession().
		on("id_ed25519.pub", "ssh-ed25519 NODE1 grid@n1\n", nil)
	n2 := newMockSession().
		on("id_ed25519.pub", "ssh-ed25519 NODE2 grid@n2\n", nil)

	sessions := map[string]SessionRunner{"n1.example": n1, "n2.example": n2}
	if err := SetupClusterSSH(context.Background(), sessions, []string{"grid"}); err != nil {
		t.Fatalf("SetupClusterSSH: %v", err)
	}
	// Both nodes should have run keygen + auth + keyscan.
	for _, s := range []*mockSession{n1, n2} {
		if !s.ranContaining("ssh-keygen") {
			t.Error("expected ssh-keygen")
		}
		if !s.ranContaining("authorized_keys") {
			t.Error("expected authorized_keys append")
		}
		if !s.ranContaining("ssh-keyscan") {
			t.Error("expected ssh-keyscan")
		}
	}
}

func TestSetupClusterSSH_NoSessions(t *testing.T) {
	if err := SetupClusterSSH(context.Background(), nil, []string{"grid"}); err == nil {
		t.Error("expected error with no sessions")
	}
}

func TestSetupClusterSSH_NoUsers(t *testing.T) {
	ms := newMockSession()
	if err := SetupClusterSSH(context.Background(),
		map[string]SessionRunner{"n1": ms}, nil); err == nil {
		t.Error("expected error with no users")
	}
}

func TestSSHManager_Rollback_NoSession(t *testing.T) {
	m := NewSSHAuthManager()
	if err := m.Rollback(context.Background(), []Change{{}}); err == nil {
		t.Error("want error")
	}
}

func TestSSHManager_Rollback_RestoresAuthKeys(t *testing.T) {
	ms := newMockSession()
	m := NewSSHAuthManager().WithSession(ms)
	changes := []Change{{
		Target: "authorized_keys/grid",
		Before: []string{"ssh-ed25519 AAA old"},
		After:  []string{"ssh-ed25519 AAA new"},
	}}
	if err := m.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if !ms.ranContaining("authorized_keys") {
		t.Error("expected authorized_keys restore")
	}
}

func TestSSHManager_Rollback_NilBeforeSkipped(t *testing.T) {
	ms := newMockSession()
	m := NewSSHAuthManager().WithSession(ms)
	changes := []Change{{Target: "authorized_keys/alice"}}
	if err := m.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if len(ms.cmds) != 0 {
		t.Errorf("should not run cmds; got %v", ms.cmds)
	}
}

func TestSSHManager_Rollback_SSHDDropInRemove(t *testing.T) {
	ms := newMockSession()
	m := NewSSHAuthManager().WithSession(ms)
	changes := []Change{{Target: "sshd_config/drop-in", After: map[string]string{"PasswordAuthentication": "no"}}}
	if err := m.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if !ms.ranContaining("rm -f") {
		t.Errorf("expected rm -f; got %v", ms.cmds)
	}
}

func TestSSHManager_ReadAuthorizedKeys_NoSession(t *testing.T) {
	m := NewSSHAuthManager()
	keys, err := m.readAuthorizedKeys(context.Background(), "alice")
	if err != nil || keys != nil {
		t.Errorf("no session → nil,nil; got (%v,%v)", keys, err)
	}
}

func TestSSHManager_ReadAuthorizedKeys_Root(t *testing.T) {
	ms := newMockSession().on("/root/.ssh/authorized_keys", "ssh-ed25519 AAA root@host\n# comment\n", nil)
	m := NewSSHAuthManager().WithSession(ms)
	keys, err := m.readAuthorizedKeys(context.Background(), "root")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key (comments stripped), got %v", keys)
	}
}

func TestSSHManager_ReadSSHDDropIn_NoSession(t *testing.T) {
	m := NewSSHAuthManager()
	out, err := m.readSSHDDropIn(context.Background())
	if err != nil || len(out) != 0 {
		t.Errorf("no session → empty,nil; got (%v,%v)", out, err)
	}
}

func TestSSHManager_Run_NoSession(t *testing.T) {
	m := NewSSHAuthManager()
	if err := m.run(context.Background(), "ls"); err == nil {
		t.Error("want err")
	}
}

func TestSSHManager_Run_ErrWithStderr(t *testing.T) {
	ms := newMockSession()
	ms.on("boom", "", fmt.Errorf("x"))
	ms.responses["boom"] = mockResponse{stderr: "oh no", err: fmt.Errorf("x")}
	m := NewSSHAuthManager().WithSession(ms)
	err := m.run(context.Background(), "boom")
	if err == nil || !strings.Contains(err.Error(), "oh no") {
		t.Errorf("want err with stderr, got %v", err)
	}
}

func TestSSHManager_CastVariants(t *testing.T) {
	m := NewSSHAuthManager().WithSession(newMockSession())
	cfg := config.SSHConfig{}
	if _, err := m.Plan(context.Background(), cfg, nil); err != nil {
		t.Error(err)
	}
	if _, err := m.Plan(context.Background(), &cfg, nil); err != nil {
		t.Error(err)
	}
	if _, err := m.Plan(context.Background(), nil, nil); err != nil {
		t.Error(err)
	}
	if _, err := m.Plan(context.Background(), config.Linux{SSHConfig: &cfg}, nil); err != nil {
		t.Error(err)
	}
	var np *config.Linux
	if _, err := m.Plan(context.Background(), np, nil); err != nil {
		t.Error(err)
	}
}
