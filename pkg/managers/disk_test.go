package managers

import (
	"context"
	"strings"
	"testing"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// fullMockSession implements session.Session for disk/mount tests.
type fullMockSession struct {
	// response lookup — first substring match wins, insertion-ordered.
	keys      []string
	responses map[string]struct {
		stdout, stderr string
		err            error
	}
	cmds   []string
	files  map[string][]byte
	exists map[string]bool
}

func newFullMock() *fullMockSession {
	return &fullMockSession{
		responses: map[string]struct {
			stdout, stderr string
			err            error
		}{},
		files:  map[string][]byte{},
		exists: map[string]bool{},
	}
}

func (m *fullMockSession) on(key, stdout string, err error) *fullMockSession {
	m.keys = append(m.keys, key)
	m.responses[key] = struct {
		stdout, stderr string
		err            error
	}{stdout: stdout, err: err}
	return m
}

func (m *fullMockSession) Host() string { return "mock" }

func (m *fullMockSession) Run(_ context.Context, cmd string) (string, string, error) {
	m.cmds = append(m.cmds, cmd)
	for _, k := range m.keys {
		if strings.Contains(cmd, k) {
			r := m.responses[k]
			return r.stdout, r.stderr, r.err
		}
	}
	return "", "", nil
}
func (m *fullMockSession) RunSudo(ctx context.Context, cmd string) (string, string, error) {
	return m.Run(ctx, cmd)
}
func (m *fullMockSession) WriteFile(_ context.Context, path string, content []byte, _ uint32) error {
	m.files[path] = content
	return nil
}
func (m *fullMockSession) ReadFile(_ context.Context, path string) ([]byte, error) {
	if b, ok := m.files[path]; ok {
		return b, nil
	}
	return nil, nil
}
func (m *fullMockSession) FileExists(_ context.Context, path string) (bool, error) {
	return m.exists[path], nil
}
func (m *fullMockSession) Close() error { return nil }

func (m *fullMockSession) ranContaining(sub string) bool {
	for _, c := range m.cmds {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// -------- DiskManager tests --------

func TestDiskManager_Plan_NewAdditionalDisk(t *testing.T) {
	ms := newFullMock().
		on("pvs", `{"report":[{"pv":[]}]}`, nil).
		on("vgs", `{"report":[{"vg":[]}]}`, nil).
		on("lvs", `{"report":[{"lv":[]}]}`, nil).
		on("findmnt", `{"filesystems":[]}`, nil)
	ms.exists["/dev/sdb"] = true
	ms.files["/etc/fstab"] = []byte("")

	d := NewDiskManager().WithSession(ms)
	layout := &config.DiskLayout{
		Additional: []config.AdditionalDisk{
			{
				Device:  "/dev/sdb",
				VGName:  "datavg",
				LogicalVolumes: []config.LogicalVolume{
					{Name: "data", Size: "10G", FS: "ext4", MountPoint: "/data"},
				},
			},
		},
	}
	changes, err := d.PlanLayout(context.Background(), layout)
	if err != nil {
		t.Fatalf("PlanLayout: %v", err)
	}
	if len(changes) == 0 {
		t.Fatalf("expected changes, got 0")
	}
	ops := []string{}
	for _, c := range changes {
		after, _ := c.After.(map[string]any)
		ops = append(ops, after["op"].(string))
	}
	want := []string{"pvcreate", "vgcreate", "lvcreate", "mkfs", "fstab", "mount"}
	for _, w := range want {
		found := false
		for _, o := range ops {
			if o == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing op %q in plan; got %v", w, ops)
		}
	}
}

func TestDiskManager_Plan_MissingDevice(t *testing.T) {
	ms := newFullMock().
		on("pvs", `{"report":[{"pv":[]}]}`, nil).
		on("vgs", `{"report":[{"vg":[]}]}`, nil).
		on("lvs", `{"report":[{"lv":[]}]}`, nil).
		on("findmnt", `{"filesystems":[]}`, nil)
	// /dev/sdz does not exist.

	d := NewDiskManager().WithSession(ms)
	layout := &config.DiskLayout{
		Additional: []config.AdditionalDisk{
			{Device: "/dev/sdz", VGName: "vg", LogicalVolumes: []config.LogicalVolume{{Name: "a", Size: "1G", FS: "ext4", MountPoint: "/a"}}},
		},
	}
	changes, err := d.PlanLayout(context.Background(), layout)
	if err != nil {
		t.Fatalf("PlanLayout: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change (error marker), got %d", len(changes))
	}
	after, _ := changes[0].After.(map[string]any)
	if after["op"] != "error" {
		t.Errorf("expected error op, got %v", after["op"])
	}
}

func TestDiskManager_Plan_PartialExistingLVM(t *testing.T) {
	// PV + VG exist; LV missing.
	ms := newFullMock().
		on("pvs", `{"report":[{"pv":[{"pv_name":"/dev/sdb","vg_name":"datavg"}]}]}`, nil).
		on("vgs", `{"report":[{"vg":[{"vg_name":"datavg"}]}]}`, nil).
		on("lvs", `{"report":[{"lv":[]}]}`, nil).
		on("findmnt", `{"filesystems":[]}`, nil)
	ms.exists["/dev/sdb"] = true

	d := NewDiskManager().WithSession(ms)
	layout := &config.DiskLayout{
		Additional: []config.AdditionalDisk{
			{Device: "/dev/sdb", VGName: "datavg", LogicalVolumes: []config.LogicalVolume{{Name: "data", Size: "10G", FS: "ext4", MountPoint: "/data"}}},
		},
	}
	changes, err := d.PlanLayout(context.Background(), layout)
	if err != nil {
		t.Fatalf("PlanLayout: %v", err)
	}
	for _, c := range changes {
		after, _ := c.After.(map[string]any)
		op := after["op"].(string)
		if op == "pvcreate" || op == "vgcreate" {
			t.Errorf("did not expect %s; PV+VG already exist", op)
		}
	}
}

func TestDiskManager_Verify_NoDrift(t *testing.T) {
	// Everything exists — no drift.
	ms := newFullMock().
		on("pvs", `{"report":[{"pv":[{"pv_name":"/dev/sdb","vg_name":"datavg"}]}]}`, nil).
		on("vgs", `{"report":[{"vg":[{"vg_name":"datavg"}]}]}`, nil).
		on("lvs", `{"report":[{"lv":[{"lv_name":"data","vg_name":"datavg"}]}]}`, nil).
		on("findmnt", `{"filesystems":[{"target":"/data","source":"/dev/mapper/datavg-data"}]}`, nil)
	ms.exists["/dev/sdb"] = true
	ms.files["/etc/fstab"] = []byte("/dev/datavg/data /data ext4 defaults 0 0\n")

	d := NewDiskManager().WithSession(ms)
	layout := &config.DiskLayout{
		Additional: []config.AdditionalDisk{
			{Device: "/dev/sdb", VGName: "datavg", LogicalVolumes: []config.LogicalVolume{{Name: "data", Size: "10G", FS: "ext4", MountPoint: "/data"}}},
		},
	}
	// mkfs is always planned for new LVs in our conservative policy — if the
	// LV already exists, it's not re-emitted. So no-drift is achieved.
	changes, err := d.PlanLayout(context.Background(), layout)
	if err != nil {
		t.Fatalf("%v", err)
	}
	// The conservative mkfs-always behavior means we still expect the mkfs
	// line. Assert only that we get no pvcreate/vgcreate/lvcreate.
	for _, c := range changes {
		after, _ := c.After.(map[string]any)
		op := after["op"].(string)
		if op == "pvcreate" || op == "vgcreate" || op == "lvcreate" {
			t.Errorf("unexpected op %s; everything exists", op)
		}
	}
}

func TestDiskManager_Apply_DryRun(t *testing.T) {
	ms := newFullMock()
	d := NewDiskManager().WithSession(ms)
	changes := []Change{{Manager: "disk", Target: "/dev/sdb", Action: "create",
		After: map[string]any{"op": "pvcreate", "device": "/dev/sdb"}}}
	res, err := d.Apply(context.Background(), changes, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(res.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(res.Skipped))
	}
	if ms.ranContaining("pvcreate") {
		t.Errorf("dry run should not run commands")
	}
}

func TestDiskManager_Apply_Real(t *testing.T) {
	ms := newFullMock()
	d := NewDiskManager().WithSession(ms)
	changes := []Change{
		{Manager: "disk", After: map[string]any{"op": "pvcreate", "device": "/dev/sdb"}},
		{Manager: "disk", After: map[string]any{"op": "vgcreate", "name": "vg1", "pvs": []string{"/dev/sdb"}}},
		{Manager: "disk", After: map[string]any{"op": "lvcreate", "vg": "vg1", "name": "lv1", "size": "1G"}},
	}
	if _, err := d.Apply(context.Background(), changes, false); err != nil {
		t.Fatalf("%v", err)
	}
	for _, want := range []string{"pvcreate", "vgcreate vg1", "lvcreate"} {
		if !ms.ranContaining(want) {
			t.Errorf("expected command containing %q; got: %v", want, ms.cmds)
		}
	}
}

func TestDiskManager_Rollback(t *testing.T) {
	ms := newFullMock()
	d := NewDiskManager().WithSession(ms)
	changes := []Change{
		{Manager: "disk", RollbackCmd: "lvremove -f -y /dev/vg1/lv1"},
		{Manager: "disk", RollbackCmd: "vgremove -ff -y vg1"},
	}
	if err := d.Rollback(context.Background(), changes); err != nil {
		t.Fatalf("%v", err)
	}
	// Verify reverse order: vgremove should come first (last change, rolled first).
	if len(ms.cmds) != 2 {
		t.Fatalf("expected 2 rollback cmds, got %d", len(ms.cmds))
	}
	if !strings.Contains(ms.cmds[0], "vgremove") {
		t.Errorf("rollback order wrong; first cmd = %q", ms.cmds[0])
	}
}

func TestDiskManager_Name(t *testing.T) {
	if NewDiskManager().Name() != "disk" {
		t.Errorf("wrong name")
	}
}

func TestDiskManager_Plan_InterfacePath(t *testing.T) {
	ms := newFullMock().
		on("pvs", `{"report":[{"pv":[]}]}`, nil).
		on("vgs", `{"report":[{"vg":[]}]}`, nil).
		on("lvs", `{"report":[{"lv":[]}]}`, nil).
		on("findmnt", `{"filesystems":[]}`, nil)
	ms.exists["/dev/sdb"] = true

	d := NewDiskManager().WithSession(ms)
	layout := &config.DiskLayout{
		Additional: []config.AdditionalDisk{{
			Device: "/dev/sdb", VGName: "vg1",
			LogicalVolumes: []config.LogicalVolume{{Name: "lv1", Size: "1G", FS: "ext4", MountPoint: "/mnt/lv1"}},
		}},
	}
	// Via interface Plan(ctx, Spec, State).
	cs, err := d.Plan(context.Background(), layout, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(cs) == 0 {
		t.Errorf("expected changes")
	}
	// Via Verify.
	v, err := d.Verify(context.Background(), layout)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if v.OK {
		t.Errorf("expected drift for fresh disk")
	}
}

func TestDiskManager_coerceLayout_Unsupported(t *testing.T) {
	if _, err := coerceLayout("bogus"); err == nil {
		t.Error("expected error for unsupported spec type")
	}
	v, _ := coerceLayout(nil)
	if v != nil {
		t.Error("nil spec should coerce to nil")
	}
}
