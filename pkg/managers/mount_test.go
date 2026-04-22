package managers

import (
	"context"
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
