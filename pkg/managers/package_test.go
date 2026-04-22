package managers

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// osReleaseOL9 is a representative /etc/os-release from Oracle Linux 9.
const osReleaseOL9 = `NAME="Oracle Linux Server"
VERSION="9.3"
ID="ol"
ID_LIKE="fedora"
VARIANT="Server"
VARIANT_ID="server"
VERSION_ID="9.3"
`

const osReleaseUbuntu = `NAME="Ubuntu"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian
`

const osReleaseSLES = `NAME="SLES"
VERSION="15-SP5"
ID="sles"
ID_LIKE="suse"
`

func TestPackageManager_Plan_OL9_InstallPartial(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", osReleaseOL9, nil).
		on("command -v dnf", "", nil).
		on("rpm -q 'vim'", "", nil).                     // installed
		on("rpm -q 'htop'", "", fmt.Errorf("not found")). // missing
		on("rpm -q 'curl'", "", nil)                     // installed
	p := NewPackageManager().WithSession(ms)

	spec := PackagesSpec{Install: []string{"vim", "htop", "curl"}}
	changes, err := p.Plan(context.Background(), spec, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("want 1 change (htop), got %d: %+v", len(changes), changes)
	}
	if changes[0].Target != "pkg/htop" || changes[0].Action != "create" {
		t.Errorf("unexpected change: %+v", changes[0])
	}
}

func TestPackageManager_Plan_Ubuntu(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", osReleaseUbuntu, nil).
		on("dpkg-query", "", fmt.Errorf("not installed"))
	p := NewPackageManager().WithSession(ms)

	spec := PackagesSpec{Install: []string{"nginx"}}
	changes, err := p.Plan(context.Background(), spec, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 || changes[0].Target != "pkg/nginx" {
		t.Fatalf("want 1 change for nginx, got %+v", changes)
	}
	if p.family != familyDEB || p.tool != "apt-get" {
		t.Errorf("distro detection wrong: family=%s tool=%s", p.family, p.tool)
	}
}

func TestPackageManager_Plan_SLES(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", osReleaseSLES, nil).
		on("rpm -q", "", fmt.Errorf("not installed"))
	p := NewPackageManager().WithSession(ms)

	_, err := p.Plan(context.Background(), PackagesSpec{Install: []string{"tmux"}}, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if p.family != familyZYPPER || p.tool != "zypper" {
		t.Errorf("distro detection wrong: family=%s tool=%s", p.family, p.tool)
	}
}

func TestPackageManager_Plan_EmptySpec(t *testing.T) {
	p := NewPackageManager() // no session required
	changes, err := p.Plan(context.Background(), PackagesSpec{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if changes != nil {
		t.Errorf("empty spec should produce no changes, got %+v", changes)
	}
}

func TestPackageManager_Apply_BatchInstall(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", osReleaseOL9, nil).
		on("command -v dnf", "", nil)
	p := NewPackageManager().WithSession(ms)

	changes := []Change{
		{Target: "pkg/htop", Action: "create", After: "htop"},
		{Target: "pkg/vim", Action: "create", After: "vim"},
	}
	res, err := p.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Applied) != 2 || len(res.Failed) != 0 {
		t.Fatalf("want 2 applied, got %+v", res)
	}
	// One batched install command covering both packages.
	found := false
	for _, c := range ms.cmds {
		if strings.Contains(c, "dnf install -y") && strings.Contains(c, "htop") && strings.Contains(c, "vim") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected batched dnf install, got commands: %v", ms.cmds)
	}
}

func TestPackageManager_Apply_DryRun(t *testing.T) {
	ms := newMockSession()
	p := NewPackageManager().WithSession(ms)
	changes := []Change{{Target: "pkg/x", Action: "create", After: "x"}}
	res, err := p.Apply(context.Background(), changes, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || len(ms.cmds) != 0 {
		t.Errorf("dry-run should not execute: %+v cmds=%v", res, ms.cmds)
	}
}

func TestPackageManager_Apply_Remove(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", osReleaseUbuntu, nil)
	p := NewPackageManager().WithSession(ms)

	changes := []Change{{Target: "pkg/nginx", Action: "delete", Before: "nginx"}}
	res, err := p.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 1 {
		t.Fatalf("want 1 applied, got %+v", res)
	}
	if !ms.ranContaining("apt-get -y purge nginx") && !ms.ranContaining("apt-get -y purge 'nginx'") {
		t.Errorf("expected apt-get purge, got %v", ms.cmds)
	}
}

func TestPackageManager_LockRetry(t *testing.T) {
	ms := newMockSession()
	// First two attempts fail with "locked", third succeeds.
	attempt := 0
	ms.keys = append(ms.keys, "cat /etc/os-release", "command -v dnf", "dnf install")
	ms.responses["cat /etc/os-release"] = mockResponse{stdout: osReleaseOL9}
	ms.responses["command -v dnf"] = mockResponse{}
	// Custom Run wrapper: override with closure via shadow struct.
	wrap := &retryingSession{inner: ms, onInstall: func() (string, string, error) {
		attempt++
		if attempt < 3 {
			return "", "dnf: could not get lock /var/lib/rpm/.rpm.lock", fmt.Errorf("lock")
		}
		return "", "", nil
	}}
	p := NewPackageManager().WithSession(wrap)

	changes := []Change{{Target: "pkg/htop", Action: "create", After: "htop"}}
	res, err := p.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if attempt != 3 {
		t.Errorf("expected 3 attempts, got %d", attempt)
	}
	if len(res.Applied) != 1 {
		t.Errorf("want 1 applied after retry, got %+v", res)
	}
}

// retryingSession lets a test inject per-command behavior on top of mockSession.
type retryingSession struct {
	inner     *mockSession
	onInstall func() (string, string, error)
}

func (r *retryingSession) Run(ctx context.Context, cmd string) (string, string, error) {
	if strings.Contains(cmd, "dnf install") {
		return r.onInstall()
	}
	return r.inner.Run(ctx, cmd)
}

func TestPackageManager_Verify_NoDrift(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", osReleaseOL9, nil).
		on("command -v dnf", "", nil).
		on("rpm -q 'vim'", "", nil) // installed
	p := NewPackageManager().WithSession(ms)
	vr, err := p.Verify(context.Background(), PackagesSpec{Install: []string{"vim"}})
	if err != nil {
		t.Fatal(err)
	}
	if !vr.OK {
		t.Errorf("expected no drift, got %+v", vr)
	}
}

func TestPackageManager_Rollback_Create(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", osReleaseOL9, nil).
		on("command -v dnf", "", nil)
	p := NewPackageManager().WithSession(ms)
	changes := []Change{{Target: "pkg/htop", Action: "create", After: "htop"}}
	if err := p.Rollback(context.Background(), changes); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if !ms.ranContaining("dnf remove -y") {
		t.Errorf("expected dnf remove during rollback, got %v", ms.cmds)
	}
}

func TestParseOSRelease(t *testing.T) {
	kv := parseOSRelease(osReleaseOL9)
	if kv["ID"] != "ol" {
		t.Errorf("ID = %q, want ol", kv["ID"])
	}
	if kv["ID_LIKE"] != "fedora" {
		t.Errorf("ID_LIKE = %q, want fedora", kv["ID_LIKE"])
	}
}

func TestPackageManager_Plan_Remove(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", osReleaseOL9, nil).
		on("command -v dnf", "", nil).
		on("rpm -q 'oldpkg'", "", nil) // installed → will be removed
	p := NewPackageManager().WithSession(ms)
	changes, err := p.Plan(context.Background(), PackagesSpec{Remove: []string{"oldpkg"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Action != "delete" {
		t.Fatalf("expected 1 delete change, got %+v", changes)
	}
}

func TestPackageManager_DetectDistro_Unsupported(t *testing.T) {
	ms := newMockSession().on("cat /etc/os-release", `ID=plan9`, nil)
	p := NewPackageManager().WithSession(ms)
	_, err := p.Plan(context.Background(), PackagesSpec{Install: []string{"x"}}, nil)
	if err == nil {
		t.Error("expected unsupported-distro error")
	}
}

func TestPackageManager_CastPackages(t *testing.T) {
	if _, err := castPackages(&PackagesSpec{Install: []string{"x"}}); err != nil {
		t.Errorf("pointer form: %v", err)
	}
	if _, err := castPackages(nil); err != nil {
		t.Errorf("nil form: %v", err)
	}
	if _, err := castPackages(42); err == nil {
		t.Error("wrong type must fail")
	}
}

func TestPackageManager_Rollback_Delete(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", osReleaseUbuntu, nil)
	p := NewPackageManager().WithSession(ms)
	changes := []Change{{Target: "pkg/nginx", Action: "delete", Before: "nginx"}}
	if err := p.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if !ms.ranContaining("apt-get -y install") {
		t.Errorf("expected apt-get install during rollback, got %v", ms.cmds)
	}
}

func TestPackageManager_NoSession(t *testing.T) {
	p := NewPackageManager()
	_, err := p.Plan(context.Background(), PackagesSpec{Install: []string{"x"}}, nil)
	if err == nil {
		t.Error("expected error without session")
	}
	_, err = p.Apply(context.Background(), []Change{{Action: "create", After: "x"}}, false)
	if err == nil {
		t.Error("expected error without session")
	}
}

func TestIsLockError(t *testing.T) {
	cases := map[string]bool{
		"":                                                  false,
		"E: Could not get lock /var/lib/dpkg/lock-frontend": true,
		"Another app is currently holding the yum lock":     true,
		"permission denied":                                 false,
	}
	for in, want := range cases {
		if got := isLockError(in); got != want {
			t.Errorf("isLockError(%q) = %v, want %v", in, got, want)
		}
	}
}
