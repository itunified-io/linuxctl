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

func TestPackageManager_DetectDistro_RHEL7_YumFallback(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", `ID="rhel"
VERSION_ID="7.9"
`, nil).
		on("command -v dnf", "", fmt.Errorf("not found")) // no dnf → yum
	p := NewPackageManager().WithSession(ms)
	if err := p.detectDistro(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.tool != "yum" {
		t.Errorf("want yum, got %q", p.tool)
	}
}

func TestPackageManager_DetectDistro_SLES(t *testing.T) {
	ms := newMockSession().on("cat /etc/os-release", `ID="sles"
`, nil)
	p := NewPackageManager().WithSession(ms)
	if err := p.detectDistro(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.tool != "zypper" {
		t.Errorf("want zypper, got %q", p.tool)
	}
}

func TestPackageManager_DetectDistro_FromIDLike(t *testing.T) {
	// Primary ID is unknown but ID_LIKE lists ubuntu.
	ms := newMockSession().on("cat /etc/os-release", `ID="linuxmint"
ID_LIKE="ubuntu debian"
`, nil)
	p := NewPackageManager().WithSession(ms)
	if err := p.detectDistro(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.tool != "apt-get" {
		t.Errorf("want apt-get, got %q", p.tool)
	}
}

func TestPackageManager_DetectDistro_Rocky(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", `ID="rocky"
ID_LIKE="rhel fedora centos"
`, nil).
		on("command -v dnf", "", nil)
	p := NewPackageManager().WithSession(ms)
	if err := p.detectDistro(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.family != familyRPM {
		t.Errorf("want familyRPM, got %q", p.family)
	}
}

func TestPackageManager_Rollback_NoSession(t *testing.T) {
	p := NewPackageManager()
	err := p.Rollback(context.Background(), []Change{{Action: "create", After: "x"}})
	if err == nil {
		t.Fatal("expected err")
	}
}

func TestPackageManager_Rollback_DetectFails(t *testing.T) {
	ms := newMockSession().on("cat /etc/os-release", "ID=plan9\n", nil)
	p := NewPackageManager().WithSession(ms)
	err := p.Rollback(context.Background(), []Change{{Action: "create", After: "x"}})
	if err == nil {
		t.Fatal("expected err")
	}
}

func TestPackageManager_Apply_UnknownAction(t *testing.T) {
	ms := newMockSession().
		on("cat /etc/os-release", osReleaseOL9, nil).
		on("command -v dnf", "", nil)
	p := NewPackageManager().WithSession(ms)
	res, _ := p.Apply(context.Background(), []Change{{Action: "weird", After: "x"}}, false)
	if len(res.Failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(res.Failed))
	}
}

func TestPackageManager_Apply_EmptyChanges(t *testing.T) {
	ms := newMockSession()
	p := NewPackageManager().WithSession(ms)
	res, err := p.Apply(context.Background(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 0 {
		t.Error("expected no applied")
	}
}

func TestPackageManager_RunPackageOp_Zypper(t *testing.T) {
	ms := newMockSession()
	p := NewPackageManager().WithSession(ms)
	p.family = familyZYPPER
	p.tool = "zypper"
	if err := p.runPackageOp(context.Background(), "install", []string{"nginx"}); err != nil {
		t.Fatal(err)
	}
	if !ms.ranContaining("zypper --non-interactive install") {
		t.Errorf("expected zypper install; got %v", ms.cmds)
	}
	ms2 := newMockSession()
	p2 := NewPackageManager().WithSession(ms2)
	p2.family = familyZYPPER
	p2.tool = "zypper"
	if err := p2.runPackageOp(context.Background(), "remove", []string{"nginx"}); err != nil {
		t.Fatal(err)
	}
	if !ms2.ranContaining("remove --no-confirm") {
		t.Errorf("expected zypper remove; got %v", ms2.cmds)
	}
}

func TestPackageManager_RunPackageOp_UnknownFamily(t *testing.T) {
	ms := newMockSession()
	p := NewPackageManager().WithSession(ms)
	p.family = "weird"
	err := p.runPackageOp(context.Background(), "install", []string{"x"})
	if err == nil {
		t.Error("expected err")
	}
}

func TestPackageManager_RunPackageOp_EmptyNames(t *testing.T) {
	ms := newMockSession()
	p := NewPackageManager().WithSession(ms)
	p.family = familyRPM
	p.tool = "dnf"
	if err := p.runPackageOp(context.Background(), "install", nil); err != nil {
		t.Fatal(err)
	}
	if len(ms.cmds) != 0 {
		t.Error("should not run cmd")
	}
}

func TestPackageManager_RunPackageOp_FatalNonLockError(t *testing.T) {
	ms := newMockSession().on("dnf install", "", fmt.Errorf("kaboom"))
	ms.responses["dnf install"] = mockResponse{stderr: "permission denied", err: fmt.Errorf("kaboom")}
	p := NewPackageManager().WithSession(ms)
	p.family = familyRPM
	p.tool = "dnf"
	err := p.runPackageOp(context.Background(), "install", []string{"x"})
	if err == nil {
		t.Error("expected err")
	}
	// Should have only attempted once (not a lock error).
	if len(ms.cmds) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(ms.cmds))
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
