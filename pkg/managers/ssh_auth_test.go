package managers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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
		on("__MISSING__", "__MISSING__\n", nil).
		on("id_ed25519.pub", "ssh-ed25519 NODE1 grid@n1\n", nil)
	n2 := newMockSession().
		on("__MISSING__", "__MISSING__\n", nil).
		on("id_ed25519.pub", "ssh-ed25519 NODE2 grid@n2\n", nil)

	sessions := map[string]SessionRunner{"n1.example": n1, "n2.example": n2}
	res, err := SetupClusterSSH(context.Background(), sessions, []string{"grid"})
	if err != nil {
		t.Fatalf("SetupClusterSSH: %v", err)
	}
	if res == nil || len(res.PerNode) != 2 {
		t.Fatalf("expected 2 nodes in result, got %+v", res)
	}
	for _, s := range []*mockSession{n1, n2} {
		if !s.ranContaining("ssh-keygen") {
			t.Error("expected ssh-keygen")
		}
		if !s.ranContaining("authorized_keys") {
			t.Error("expected authorized_keys install")
		}
		if !s.ranContaining("ssh-keyscan") {
			t.Error("expected ssh-keyscan")
		}
	}
}

func TestSetupClusterSSH_NoSessions(t *testing.T) {
	if _, err := SetupClusterSSH(context.Background(), nil, []string{"grid"}); err == nil {
		t.Error("expected error with no sessions")
	}
}

func TestSetupClusterSSH_NoUsers(t *testing.T) {
	ms := newMockSession()
	if _, err := SetupClusterSSH(context.Background(),
		map[string]SessionRunner{"n1": ms}, nil); err == nil {
		t.Error("expected error with no users")
	}
}

func TestSetupClusterSSH_TwoNodes_CrossAuth(t *testing.T) {
	// Each node returns its own unique pubkey. After exchange, each node's
	// authorized_keys write should include BOTH pubkeys.
	n1 := newMockSession().
		on("__MISSING__", "__MISSING__\n", nil).
		on("id_ed25519.pub", "ssh-ed25519 AAA grid@n1\n", nil)
	n2 := newMockSession().
		on("__MISSING__", "__MISSING__\n", nil).
		on("id_ed25519.pub", "ssh-ed25519 BBB grid@n2\n", nil)

	sessions := map[string]SessionRunner{"n1": n1, "n2": n2}
	res, err := SetupClusterSSH(context.Background(), sessions, []string{"grid"})
	if err != nil {
		t.Fatal(err)
	}
	// Each node's authorized_keys merge must reference BOTH pubkeys.
	for name, s := range map[string]*mockSession{"n1": n1, "n2": n2} {
		var foundA, foundB bool
		for _, c := range s.cmds {
			if strings.Contains(c, "authorized_keys") && strings.Contains(c, "AAA") {
				foundA = true
			}
			if strings.Contains(c, "authorized_keys") && strings.Contains(c, "BBB") {
				foundB = true
			}
		}
		if !foundA || !foundB {
			t.Errorf("%s: expected both pubkeys in authorized_keys write (A=%v B=%v)", name, foundA, foundB)
		}
	}
	if got := res.PerNode["n1"].AuthorizedKeys["grid"]; got < 2 {
		t.Errorf("n1.AuthorizedKeys[grid]=%d, want >=2", got)
	}
}

func TestSetupClusterSSH_KeyAlreadyExists(t *testing.T) {
	// Probe returns __EXISTS__ → no ssh-keygen invocation, GeneratedKeys stays empty.
	n1 := newMockSession().
		on("__EXISTS__", "__EXISTS__\n", nil).
		on("id_ed25519.pub", "ssh-ed25519 CACHED grid@n1\n", nil)
	res, err := SetupClusterSSH(context.Background(),
		map[string]SessionRunner{"n1": n1}, []string{"grid"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range n1.cmds {
		if strings.Contains(c, "ssh-keygen -t ed25519") {
			t.Errorf("did not expect keygen when key exists; ran: %s", c)
		}
	}
	if len(res.PerNode["n1"].GeneratedKeys) != 0 {
		t.Errorf("GeneratedKeys should be empty when key pre-existed: %+v", res.PerNode["n1"].GeneratedKeys)
	}
}

// failOnceSession returns an error on the first probe, then behaves normally.
type failOnceSession struct {
	*mockSession
	failKey string
	failed  bool
}

func (f *failOnceSession) Run(ctx context.Context, cmd string) (string, string, error) {
	if !f.failed && strings.Contains(cmd, f.failKey) {
		f.failed = true
		return "", "pipe closed", fmt.Errorf("session died")
	}
	return f.mockSession.Run(ctx, cmd)
}

func TestSetupClusterSSH_PartialFailure(t *testing.T) {
	// n1 dies on probe; n2 succeeds.
	bad := &failOnceSession{
		mockSession: newMockSession().
			on("__MISSING__", "__MISSING__\n", nil).
			on("id_ed25519.pub", "", nil),
		failKey: "test -f",
	}
	good := newMockSession().
		on("__MISSING__", "__MISSING__\n", nil).
		on("id_ed25519.pub", "ssh-ed25519 OK grid@n2\n", nil)

	res, err := SetupClusterSSH(context.Background(),
		map[string]SessionRunner{"n1": bad, "n2": good}, []string{"grid"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	// n2 must still have gone through authorized_keys merge.
	if !good.ranContaining("authorized_keys") {
		t.Error("expected n2 to complete despite n1 failure")
	}
}

func TestSetupClusterSSH_Concurrent(t *testing.T) {
	// Per-node phase-1 runs in parallel — assert by counting overlapping
	// inflight goroutines on each node's Run() via a shared barrier.
	var inflight, peak int64
	var barMu sync.Mutex
	mk := func(pub string) SessionRunner {
		return &barrierSession{
			mockSession: newMockSession().
				on("__MISSING__", "__MISSING__\n", nil).
				on("id_ed25519.pub", pub, nil),
			inflight: &inflight,
			peak:     &peak,
			mu:       &barMu,
		}
	}
	sessions := map[string]SessionRunner{
		"n1": mk("ssh-ed25519 A grid@n1\n"),
		"n2": mk("ssh-ed25519 B grid@n2\n"),
		"n3": mk("ssh-ed25519 C grid@n3\n"),
	}
	if _, err := SetupClusterSSH(context.Background(), sessions, []string{"grid"}); err != nil {
		t.Fatal(err)
	}
	barMu.Lock()
	observed := peak
	barMu.Unlock()
	if observed < 2 {
		t.Errorf("expected parallel execution (peak>=2), got peak=%d", observed)
	}
}

func TestSeedKnownHosts_FullPath(t *testing.T) {
	// Mock returns real keyscan output on the scan command, an existing entry
	// on the known_hosts read (so merge dedup path is exercised), and no error
	// on the write.
	ms := newMockSession().
		on("ssh-keyscan -t ed25519", "peer1 ssh-ed25519 AAA\npeer2 ssh-ed25519 BBB\npeer1 ssh-ed25519 AAA\n\n", nil).
		on("known_hosts", "peer1 ssh-ed25519 AAA\n", nil) // used for both cat + write paths; write ignores stdout
	n, err := seedKnownHosts(context.Background(), ms, []string{"grid"}, []string{"peer1", "peer2"})
	if err != nil {
		t.Fatalf("seedKnownHosts: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 unique peer entries, got %d", n)
	}
	// Must have run a write command.
	if !ms.ranContaining("known_hosts") || !ms.ranContaining("chmod 0644") {
		t.Errorf("expected known_hosts write + chmod; cmds=%v", ms.cmds)
	}
}

func TestSeedKnownHosts_NoPeers(t *testing.T) {
	ms := newMockSession()
	n, err := seedKnownHosts(context.Background(), ms, []string{"grid"}, nil)
	if err != nil || n != 0 {
		t.Errorf("want 0,nil; got %d,%v", n, err)
	}
	if len(ms.cmds) != 0 {
		t.Errorf("no peers → no commands; got %v", ms.cmds)
	}
}

func TestSeedKnownHosts_ScanError(t *testing.T) {
	ms := newMockSession().on("ssh-keyscan -t ed25519", "", fmt.Errorf("no route"))
	_, err := seedKnownHosts(context.Background(), ms, []string{"grid"}, []string{"p1"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestSeedKnownHosts_WriteError(t *testing.T) {
	// scan returns entries, but the tee/install write fails.
	// Order: ssh-keyscan first, install -d next (write-only), cat as fallthrough.
	ms := newMockSession().
		on("ssh-keyscan -t ed25519", "p1 ssh-ed25519 AAA\n", nil).
		on("install -d", "", fmt.Errorf("disk full")).
		on("cat ", "", nil)
	_, err := seedKnownHosts(context.Background(), ms, []string{"grid"}, []string{"p1"})
	if err == nil || !strings.Contains(err.Error(), "write known_hosts") {
		t.Errorf("expected write err, got %v", err)
	}
}

func TestMergeAuthorizedKeys_DedupAndMerge(t *testing.T) {
	// Existing authorized_keys already has KEY1. Bundle also includes KEY1
	// (de-dup) + a new KEY2.
	ms := newMockSession().
		on("authorized_keys", "ssh-ed25519 KEY1 existing\n# comment ignored\nssh-ed25519 KEY1 existing\n", nil)
	n, err := mergeAuthorizedKeys(context.Background(), ms, "grid",
		[]string{"ssh-ed25519 KEY1 existing", "ssh-ed25519 KEY2 new", ""})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 unique keys, got %d", n)
	}
}

func TestMergeAuthorizedKeys_ReadError(t *testing.T) {
	ms := newMockSession().on("cat ", "", fmt.Errorf("boom"))
	_, err := mergeAuthorizedKeys(context.Background(), ms, "grid", nil)
	if err == nil {
		t.Error("expected error")
	}
}

func TestMergeAuthorizedKeys_WriteError(t *testing.T) {
	// Order matters: the mock picks the first matching key. "install -d" only
	// appears in the write, not the read, so it selectively fails writes.
	ms := newMockSession().
		on("install -d", "", fmt.Errorf("ro fs")).
		on("cat ", "", nil)
	_, err := mergeAuthorizedKeys(context.Background(), ms, "grid",
		[]string{"ssh-ed25519 K x"})
	if err == nil {
		t.Error("expected write error")
	}
}

func TestEnsureKeypair_Root(t *testing.T) {
	// Root home branch. Match pub read before the generic probe key so it wins.
	ms := newMockSession().
		on("id_ed25519.pub", "ssh-ed25519 ROOTKEY root@host\n", nil).
		on("__MISSING__", "__MISSING__\n", nil)
	pub, _, err := ensureKeypair(context.Background(), ms, "root")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pub, "ROOTKEY") {
		t.Errorf("pub = %q", pub)
	}
	// Also assert we used /root/... path not /home/root/...
	if !ms.ranContaining("/root/.ssh/id_ed25519") {
		t.Errorf("expected /root/... path; cmds=%v", ms.cmds)
	}
}

func TestEnsureKeypair_ProbeError(t *testing.T) {
	ms := newMockSession().on("test -f", "", fmt.Errorf("ssh closed"))
	_, _, err := ensureKeypair(context.Background(), ms, "grid")
	if err == nil {
		t.Error("expected probe error")
	}
}

func TestEnsureKeypair_KeygenError(t *testing.T) {
	ms := newMockSession().
		on("__MISSING__", "__MISSING__\n", nil).
		on("ssh-keygen -t ed25519", "", fmt.Errorf("keygen broke"))
	_, _, err := ensureKeypair(context.Background(), ms, "grid")
	if err == nil || !strings.Contains(err.Error(), "keygen") {
		t.Errorf("expected keygen err, got %v", err)
	}
}

func TestEnsureKeypair_ReadPubError(t *testing.T) {
	ms := newMockSession().
		on("__EXISTS__", "__EXISTS__\n", nil).
		on("cat ", "", fmt.Errorf("no such file"))
	_, _, err := ensureKeypair(context.Background(), ms, "grid")
	if err == nil || !strings.Contains(err.Error(), "read pub") {
		t.Errorf("expected read-pub err, got %v", err)
	}
}

type barrierSession struct {
	*mockSession
	inflight *int64
	peak     *int64
	mu       *sync.Mutex
}

func (b *barrierSession) Run(ctx context.Context, cmd string) (string, string, error) {
	// Only measure during phase-1 (probe / pub read). Phase-2 is serialised.
	if strings.Contains(cmd, "test -f") || strings.Contains(cmd, "id_ed25519.pub") || strings.Contains(cmd, "ssh-keygen -t ed25519") {
		b.mu.Lock()
		*b.inflight++
		if *b.inflight > *b.peak {
			*b.peak = *b.inflight
		}
		b.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		b.mu.Lock()
		*b.inflight--
		b.mu.Unlock()
	}
	return b.mockSession.Run(ctx, cmd)
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

func TestSSHManager_ApplyAuthorizedKeys_BadType(t *testing.T) {
	ms := newMockSession()
	m := NewSSHAuthManager().WithSession(ms)
	err := m.applyAuthorizedKeys(context.Background(), Change{Target: "authorized_keys/alice", After: "wrong"})
	if err == nil {
		t.Error("expected err")
	}
}

func TestSSHManager_ApplySSHDConfig_BadType(t *testing.T) {
	ms := newMockSession()
	m := NewSSHAuthManager().WithSession(ms)
	err := m.applySSHDConfig(context.Background(), Change{After: "wrong"})
	if err == nil {
		t.Error("expected err")
	}
}

func TestSSHManager_ApplySSHDConfig_SSHDValidationFails(t *testing.T) {
	ms := newMockSession().on("sshd -t", "", fmt.Errorf("bad config"))
	m := NewSSHAuthManager().WithSession(ms)
	ch := Change{Target: "sshd_config/drop-in", After: map[string]string{"PasswordAuthentication": "no"}}
	err := m.applySSHDConfig(context.Background(), ch)
	if err == nil {
		t.Error("expected err")
	}
	// Should roll back: rm -f the drop-in.
	if !ms.ranContaining("rm -f") {
		t.Errorf("expected rm -f on sshd -t failure; got %v", ms.cmds)
	}
}

func TestSSHManager_ApplySSHDConfig_WriteFails(t *testing.T) {
	ms := newMockSession().on("install -d -m 0755", "", fmt.Errorf("ro fs"))
	m := NewSSHAuthManager().WithSession(ms)
	ch := Change{Target: "sshd_config/drop-in", After: map[string]string{"X": "y"}}
	err := m.applySSHDConfig(context.Background(), ch)
	if err == nil {
		t.Error("expected err")
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
