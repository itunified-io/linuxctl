package managers

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/itunified-io/linuxctl/pkg/config"
)

func TestMountManager_PlanCIFS_Fresh(t *testing.T) {
	ms := newFullMock().on("findmnt", `{"filesystems":[]}`, nil)
	ms.files["/etc/fstab"] = []byte("")

	m := NewMountManager().WithSession(ms)
	mounts := []config.Mount{{
		Type:             "cifs",
		Server:           "nas01",
		Share:            "backup",
		MountPoint:       "/mnt/backup",
		Options:          []string{"ro", "vers=3.0"},
		CredentialsVault: "vault:kv/cifs#backup",
		Persistent:       true,
	}}
	changes, err := m.PlanMounts(context.Background(), mounts)
	if err != nil {
		t.Fatalf("%v", err)
	}
	ops := collectOps(changes)
	wants := []string{"cifs_credentials", "cifs_mount", "fstab"}
	for _, w := range wants {
		if !containsStr(ops, w) {
			t.Errorf("missing op %q; got %v", w, ops)
		}
	}
}

func TestMountManager_PlanCIFS_AlreadyMounted(t *testing.T) {
	ms := newFullMock().on("findmnt",
		`{"filesystems":[{"target":"/mnt/backup","source":"//nas01/backup"}]}`, nil)
	ms.files["/etc/fstab"] = []byte("//nas01/backup /mnt/backup cifs credentials=/etc/cifs-utils/credentials/nas01-backup 0 0\n")

	m := NewMountManager().WithSession(ms)
	mounts := []config.Mount{{Type: "cifs", Server: "nas01", Share: "backup", MountPoint: "/mnt/backup", Persistent: true}}
	changes, err := m.PlanMounts(context.Background(), mounts)
	if err != nil {
		t.Fatalf("%v", err)
	}
	ops := collectOps(changes)
	// Credentials change is always planned (manager doesn't know vault state).
	// But no mount / no fstab change.
	if containsStr(ops, "cifs_mount") {
		t.Errorf("should not re-mount; ops=%v", ops)
	}
	if containsStr(ops, "fstab") {
		t.Errorf("fstab already present; ops=%v", ops)
	}
}

func TestMountManager_PlanNFS(t *testing.T) {
	ms := newFullMock().on("findmnt", `{"filesystems":[]}`, nil)
	m := NewMountManager().WithSession(ms)
	mounts := []config.Mount{{Type: "nfs", Server: "nas01", Share: "/export/data", MountPoint: "/data", Persistent: true}}
	changes, err := m.PlanMounts(context.Background(), mounts)
	if err != nil {
		t.Fatalf("%v", err)
	}
	ops := collectOps(changes)
	if !containsStr(ops, "nfs_mount") || !containsStr(ops, "fstab") {
		t.Errorf("expected nfs_mount + fstab; got %v", ops)
	}
}

func TestMountManager_PlanBind(t *testing.T) {
	ms := newFullMock().on("findmnt", `{"filesystems":[]}`, nil)
	m := NewMountManager().WithSession(ms)
	mounts := []config.Mount{{Type: "bind", Source: "/src", MountPoint: "/dst"}}
	changes, err := m.PlanMounts(context.Background(), mounts)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !containsStr(collectOps(changes), "bind_mount") {
		t.Errorf("expected bind_mount; got %v", changes)
	}
}

func TestMountManager_PlanTmpfs(t *testing.T) {
	ms := newFullMock().on("findmnt", `{"filesystems":[]}`, nil)
	m := NewMountManager().WithSession(ms)
	mounts := []config.Mount{{Type: "tmpfs", MountPoint: "/run/cache", Options: []string{"size=100M"}, Persistent: true}}
	changes, err := m.PlanMounts(context.Background(), mounts)
	if err != nil {
		t.Fatalf("%v", err)
	}
	ops := collectOps(changes)
	if !containsStr(ops, "tmpfs_mount") || !containsStr(ops, "fstab") {
		t.Errorf("expected tmpfs_mount + fstab; got %v", ops)
	}
}

func TestMountManager_Plan_UnknownType(t *testing.T) {
	m := NewMountManager().WithSession(newFullMock())
	_, err := m.PlanMounts(context.Background(), []config.Mount{{Type: "xfs-nfs4.1", MountPoint: "/x"}})
	if err == nil {
		t.Errorf("expected error for unknown type")
	}
}

func TestMountManager_Apply_DryRun(t *testing.T) {
	ms := newFullMock()
	m := NewMountManager().WithSession(ms)
	changes := []Change{{Manager: "mount", After: map[string]any{"op": "bind_mount", "source": "/a", "mountpoint": "/b"}}}
	res, err := m.Apply(context.Background(), changes, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(res.Skipped) != 1 {
		t.Errorf("expected 1 skipped")
	}
}

func TestMountManager_Apply_Bind(t *testing.T) {
	ms := newFullMock()
	m := NewMountManager().WithSession(ms)
	changes := []Change{{Manager: "mount", After: map[string]any{"op": "bind_mount", "source": "/a", "mountpoint": "/b"}}}
	if _, err := m.Apply(context.Background(), changes, false); err != nil {
		t.Fatalf("%v", err)
	}
	if !ms.ranContaining("mount --bind") {
		t.Errorf("expected bind mount cmd; got %v", ms.cmds)
	}
}

func TestMountManager_Rollback(t *testing.T) {
	ms := newFullMock()
	m := NewMountManager().WithSession(ms)
	changes := []Change{
		{RollbackCmd: "rm -f /etc/cifs-utils/credentials/nas01-backup"},
		{RollbackCmd: "umount /mnt/backup"},
	}
	if err := m.Rollback(context.Background(), changes); err != nil {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(ms.cmds[0], "umount") {
		t.Errorf("rollback order: first cmd should be umount; got %q", ms.cmds[0])
	}
}

func TestMountManager_Name(t *testing.T) {
	if NewMountManager().Name() != "mount" {
		t.Errorf("wrong name")
	}
}

func TestMountManager_Plan_InterfacePath(t *testing.T) {
	ms := newFullMock().on("findmnt", `{"filesystems":[]}`, nil)
	m := NewMountManager().WithSession(ms)
	mounts := []config.Mount{{Type: "bind", Source: "/a", MountPoint: "/b"}}
	cs, err := m.Plan(context.Background(), mounts, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(cs) == 0 {
		t.Error("expected changes")
	}
	v, err := m.Verify(context.Background(), mounts)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if v.OK {
		t.Error("expected drift")
	}
}

func TestMountManager_coerceMounts_Unsupported(t *testing.T) {
	if _, err := coerceMounts("bogus"); err == nil {
		t.Error("expected error")
	}
	v, _ := coerceMounts(nil)
	if v != nil {
		t.Error("nil → nil")
	}
}

func TestMountManager_DependsOn(t *testing.T) {
	deps := NewMountManager().DependsOn()
	if len(deps) != 1 || deps[0] != "disk" {
		t.Errorf("expected [disk]; got %v", deps)
	}
}

func TestBuildMountCmd_Table(t *testing.T) {
	cases := []struct {
		name        string
		after       map[string]any
		wantSubs    []string
		wantMkdir   string
		wantErr     bool
	}{
		{
			name: "cifs_no_opts",
			after: map[string]any{
				"op": "cifs_mount", "source": "//srv/s", "mountpoint": "/mnt/s",
				"credentials": "/etc/cifs/creds",
			},
			wantSubs:  []string{"mount -t cifs", "credentials=/etc/cifs/creds", "//srv/s", "/mnt/s"},
			wantMkdir: "/mnt/s",
		},
		{
			name: "cifs_with_opts",
			after: map[string]any{
				"op": "cifs_mount", "source": "//srv/s", "mountpoint": "/mnt/s",
				"options": "ro,vers=3.0", "credentials": "/etc/cifs/creds",
			},
			wantSubs:  []string{"mount -t cifs", "ro,vers=3.0,credentials=/etc/cifs/creds"},
			wantMkdir: "/mnt/s",
		},
		{
			name: "nfs_default_opts",
			after: map[string]any{
				"op": "nfs_mount", "source": "srv:/ex", "mountpoint": "/data",
			},
			wantSubs:  []string{"mount -t nfs", "'defaults'", "'srv:/ex'", "'/data'"},
			wantMkdir: "/data",
		},
		{
			name: "nfs_with_opts",
			after: map[string]any{
				"op": "nfs_mount", "source": "srv:/ex", "mountpoint": "/data",
				"options": "ro,soft",
			},
			wantSubs:  []string{"mount -t nfs", "ro,soft"},
			wantMkdir: "/data",
		},
		{
			name: "bind",
			after: map[string]any{
				"op": "bind_mount", "source": "/src", "mountpoint": "/dst",
			},
			wantSubs:  []string{"mount --bind", "'/src'", "'/dst'"},
			wantMkdir: "/dst",
		},
		{
			name: "tmpfs_no_opts",
			after: map[string]any{
				"op": "tmpfs_mount", "mountpoint": "/run/cache",
			},
			wantSubs:  []string{"mount -t tmpfs", "'defaults'", "tmpfs", "/run/cache"},
			wantMkdir: "/run/cache",
		},
		{
			name: "tmpfs_with_opts",
			after: map[string]any{
				"op": "tmpfs_mount", "mountpoint": "/run/cache", "options": "size=100M",
			},
			wantSubs:  []string{"mount -t tmpfs", "size=100M"},
			wantMkdir: "/run/cache",
		},
		{
			name: "fstab_persistent",
			after: map[string]any{
				"op": "fstab", "entry": "//srv/s /mnt/s cifs defaults 0 0",
			},
			wantSubs:  []string{"grep -qxF", "/etc/fstab", "//srv/s /mnt/s"},
			wantMkdir: "",
		},
		{
			name:    "unknown_op",
			after:   map[string]any{"op": "something"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, mkdir, err := buildMountCmd(tc.after)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if mkdir != tc.wantMkdir {
				t.Errorf("mkdir: want %q got %q", tc.wantMkdir, mkdir)
			}
			for _, s := range tc.wantSubs {
				if !strings.Contains(cmd, s) {
					t.Errorf("cmd %q missing %q", cmd, s)
				}
			}
		})
	}
}

func TestMountManager_ApplyOne_MissingAfter(t *testing.T) {
	ms := newFullMock()
	m := NewMountManager().WithSession(ms)
	err := m.applyOne(context.Background(), Change{After: "not a map"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMountManager_ApplyOne_UnknownOp(t *testing.T) {
	ms := newFullMock()
	m := NewMountManager().WithSession(ms)
	err := m.applyOne(context.Background(), Change{After: map[string]any{"op": "mystery"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMountManager_ApplyOne_CIFSCredentials_EmptySkipped(t *testing.T) {
	ms := newFullMock()
	m := NewMountManager().WithSession(ms)
	err := m.applyOne(context.Background(), Change{After: map[string]any{
		"op": "cifs_credentials", "path": "/etc/cifs/creds",
	}})
	if err != nil {
		t.Fatalf("should silently skip: %v", err)
	}
}

func TestMountManager_Apply_NoSession(t *testing.T) {
	m := NewMountManager()
	_, err := m.Apply(context.Background(), []Change{{After: map[string]any{"op": "bind_mount"}}}, false)
	if err == nil {
		t.Fatal("expected error without session")
	}
}

func TestMountManager_Rollback_NoSession(t *testing.T) {
	m := NewMountManager()
	err := m.Rollback(context.Background(), []Change{{RollbackCmd: "foo"}})
	if err == nil {
		t.Fatal("expected error without session")
	}
}

func TestMountManager_PlanBind_Persistent(t *testing.T) {
	ms := newFullMock().on("findmnt", `{"filesystems":[]}`, nil)
	m := NewMountManager().WithSession(ms)
	mounts := []config.Mount{{Type: "bind", Source: "/src", MountPoint: "/dst", Persistent: true}}
	changes, err := m.PlanMounts(context.Background(), mounts)
	if err != nil {
		t.Fatal(err)
	}
	ops := collectOps(changes)
	if !containsStr(ops, "bind_mount") || !containsStr(ops, "fstab") {
		t.Errorf("expected bind+fstab; got %v", ops)
	}
}

func TestMountManager_PlanBind_FstabAlreadyPresent(t *testing.T) {
	ms := newFullMock().on("findmnt", `{"filesystems":[]}`, nil)
	ms.files["/etc/fstab"] = []byte("/src /dst none bind 0 0\n")
	m := NewMountManager().WithSession(ms)
	mounts := []config.Mount{{Type: "bind", Source: "/src", MountPoint: "/dst", Persistent: true}}
	changes, err := m.PlanMounts(context.Background(), mounts)
	if err != nil {
		t.Fatal(err)
	}
	ops := collectOps(changes)
	if containsStr(ops, "fstab") {
		t.Errorf("fstab should not be planned; ops=%v", ops)
	}
}

func TestMountManager_Verify_PlanError(t *testing.T) {
	m := NewMountManager().WithSession(newFullMock())
	vr, err := m.Verify(context.Background(), "bogus")
	if err == nil {
		t.Errorf("expected err; got vr=%+v", vr)
	}
}

func TestMountManager_ApplyOne_MkdirFails(t *testing.T) {
	ms := newFullMock().on("mkdir -p", "", fmt.Errorf("denied"))
	m := NewMountManager().WithSession(ms)
	err := m.applyOne(context.Background(), Change{After: map[string]any{"op": "bind_mount", "source": "/a", "mountpoint": "/b"}})
	if err == nil {
		t.Fatal("expected err")
	}
}

func TestMountManager_ApplyOne_MountCmdFails(t *testing.T) {
	ms := newFullMock().on("mount --bind", "", fmt.Errorf("EBUSY"))
	m := NewMountManager().WithSession(ms)
	err := m.applyOne(context.Background(), Change{After: map[string]any{"op": "bind_mount", "source": "/a", "mountpoint": "/b"}})
	if err == nil {
		t.Fatal("expected err")
	}
}

func TestMountManager_ApplyOne_CIFSCredentials_Full(t *testing.T) {
	ms := newFullMock()
	m := NewMountManager().WithSession(ms)
	err := m.applyOne(context.Background(), Change{After: map[string]any{
		"op": "cifs_credentials", "path": "/etc/cifs/creds",
		"username": "u", "password": "p",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ms.files["/etc/cifs/creds"]; !ok {
		t.Error("expected credentials written")
	}
}

func TestMountManager_ApplyOne_CIFSCredentials_MkdirFails(t *testing.T) {
	ms := newFullMock().on("mkdir -p", "", fmt.Errorf("denied"))
	m := NewMountManager().WithSession(ms)
	err := m.applyOne(context.Background(), Change{After: map[string]any{
		"op": "cifs_credentials", "path": "/etc/cifs/creds",
		"username": "u", "password": "p",
	}})
	if err == nil {
		t.Fatal("expected err")
	}
}

func TestMountManager_Apply_FailureStopsEarly(t *testing.T) {
	ms := newFullMock().on("mount --bind", "", fmt.Errorf("boom"))
	m := NewMountManager().WithSession(ms)
	changes := []Change{{After: map[string]any{"op": "bind_mount", "source": "/a", "mountpoint": "/b"}}}
	res, err := m.Apply(context.Background(), changes, false)
	if err == nil || len(res.Failed) != 1 {
		t.Errorf("expected err + 1 failed; got err=%v, res=%+v", err, res)
	}
}

func TestMountManager_Rollback_CmdFails(t *testing.T) {
	ms := newFullMock().on("umount", "", fmt.Errorf("busy"))
	m := NewMountManager().WithSession(ms)
	changes := []Change{{Target: "/a", RollbackCmd: "umount /a"}}
	if err := m.Rollback(context.Background(), changes); err == nil {
		t.Fatal("expected err")
	}
}

func TestMountManager_coerceMounts_PointerForms(t *testing.T) {
	mounts := []config.Mount{{Type: "bind", Source: "/a", MountPoint: "/b"}}
	got, err := coerceMounts(&mounts)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("want 1, got %d", len(got))
	}
	var np *[]config.Mount
	got2, err := coerceMounts(np)
	if err != nil || got2 != nil {
		t.Errorf("nil pointer: want nil,nil; got (%v,%v)", got2, err)
	}
}

// -------- helpers --------

func collectOps(changes []Change) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		if m, ok := c.After.(map[string]any); ok {
			if op, ok := m["op"].(string); ok {
				out = append(out, op)
			}
		}
	}
	return out
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
