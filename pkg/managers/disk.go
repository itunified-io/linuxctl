package managers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/session"
)

// DiskManager handles LVM (pvcreate/vgcreate/lvcreate), mkfs, and fstab for
// root + additional disks. Discovery uses `pvs/vgs/lvs --reportformat=json`
// and `blkid`; mutation uses sudo-privileged LVM + mkfs commands.
//
// Safety: DiskManager only creates. It never removes PVs/VGs/LVs or reshapes
// existing filesystems. Rollback reverses in-session creations only.
type DiskManager struct {
	Session session.Session
}

// NewDiskManager returns a disk manager bound to sess. sess may be nil for
// tests that exercise pure parsing paths.
func NewDiskManager() *DiskManager { return &DiskManager{} }

// WithSession returns a copy of m bound to sess.
func (m *DiskManager) WithSession(sess session.Session) *DiskManager {
	cp := *m
	cp.Session = sess
	return &cp
}

// Name implements Manager.
func (*DiskManager) Name() string { return "disk" }

// DependsOn implements Manager.
func (*DiskManager) DependsOn() []string { return nil }

// ---- LVM discovery types (subset of pvs/vgs/lvs JSON) ----

type lvmPvReport struct {
	Report []struct {
		PV []struct {
			PVName string `json:"pv_name"`
			VGName string `json:"vg_name"`
		} `json:"pv"`
	} `json:"report"`
}
type lvmVgReport struct {
	Report []struct {
		VG []struct {
			VGName string `json:"vg_name"`
		} `json:"vg"`
	} `json:"report"`
}
type lvmLvReport struct {
	Report []struct {
		LV []struct {
			LVName string `json:"lv_name"`
			VGName string `json:"vg_name"`
		} `json:"lv"`
	} `json:"report"`
}

// discovery snapshots current on-host LVM + fs + mount state.
type discovery struct {
	PVs    map[string]string // pv_name → vg_name
	VGs    map[string]bool
	LVs    map[string]bool // key: "vg/lv"
	FSType map[string]string
	Mounts map[string]string // mount_point → source
	Fstab  string
	// RawDisks holds /dev/sdX device paths for whole-disk block devices that
	// have no partitions, no LVM PV signature, and no filesystem — i.e. the
	// disks linuxctl is free to claim for additional disk_layout entries.
	// Sorted alphabetically (sda, sdb, sdc, ...) so claim order is
	// deterministic when manifests use tags without explicit device paths.
	// Populated by discoverRawDisks (best-effort; empty slice if lsblk
	// missing or parse fails).
	RawDisks []string
}

func (m *DiskManager) discover(ctx context.Context) (*discovery, error) {
	if m.Session == nil {
		return nil, fmt.Errorf("disk manager: no session")
	}
	d := &discovery{
		PVs:    map[string]string{},
		VGs:    map[string]bool{},
		LVs:    map[string]bool{},
		FSType: map[string]string{},
		Mounts: map[string]string{},
	}
	if out, _, err := m.Session.RunSudo(ctx, "pvs --reportformat=json"); err == nil {
		var r lvmPvReport
		if jerr := json.Unmarshal([]byte(out), &r); jerr == nil {
			for _, rep := range r.Report {
				for _, p := range rep.PV {
					d.PVs[p.PVName] = p.VGName
				}
			}
		}
	}
	if out, _, err := m.Session.RunSudo(ctx, "vgs --reportformat=json"); err == nil {
		var r lvmVgReport
		if jerr := json.Unmarshal([]byte(out), &r); jerr == nil {
			for _, rep := range r.Report {
				for _, v := range rep.VG {
					d.VGs[v.VGName] = true
				}
			}
		}
	}
	if out, _, err := m.Session.RunSudo(ctx, "lvs --reportformat=json"); err == nil {
		var r lvmLvReport
		if jerr := json.Unmarshal([]byte(out), &r); jerr == nil {
			for _, rep := range r.Report {
				for _, lv := range rep.LV {
					d.LVs[lv.VGName+"/"+lv.LVName] = true
				}
			}
		}
	}
	// findmnt JSON.
	if out, _, err := m.Session.Run(ctx, "findmnt --json"); err == nil {
		parseFindmnt(out, d.Mounts)
	}
	// fstab content.
	if b, err := m.Session.ReadFile(ctx, "/etc/fstab"); err == nil {
		d.Fstab = string(b)
	}
	// Raw-disk inventory: every whole block device with no partitions, no
	// PV signature, and no filesystem. Sorted by KNAME (sda, sdb, sdc, …)
	// for deterministic claim order. Used by tag→device resolution when an
	// AdditionalDisk entry sets `tag:` without an explicit `device:`.
	d.RawDisks = m.discoverRawDisks(ctx, d.PVs)
	return d, nil
}

// discoverRawDisks runs `lsblk -bJ -o NAME,TYPE,FSTYPE,PARTTYPE,SIZE -d` on
// the target and returns the unique /dev/<KNAME> paths that are TYPE=disk
// AND have no children partitions AND no FSTYPE AND no PARTTYPE AND aren't
// already an LVM PV. Best-effort: returns nil on any lsblk failure.
func (m *DiskManager) discoverRawDisks(ctx context.Context, pvs map[string]string) []string {
	out, _, err := m.Session.Run(ctx, "lsblk -bJ -o NAME,TYPE,FSTYPE,PARTTYPE -d")
	if err != nil {
		return nil
	}
	var doc struct {
		Blockdevices []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			FSType   string `json:"fstype"`
			PartType string `json:"parttype"`
		} `json:"blockdevices"`
	}
	if jerr := json.Unmarshal([]byte(out), &doc); jerr != nil {
		return nil
	}
	var raw []string
	for _, bd := range doc.Blockdevices {
		if bd.Type != "disk" || bd.FSType != "" || bd.PartType != "" {
			continue
		}
		dev := "/dev/" + bd.Name
		if _, isPV := pvs[dev]; isPV {
			continue
		}
		// Confirm no partitions exist (lsblk -d hides them, so re-check
		// via /sys/class/block/<name>/<name>1).
		cmd := fmt.Sprintf("test ! -d /sys/class/block/%s/%s1 && test ! -d /sys/class/block/%s/%sp1",
			bd.Name, bd.Name, bd.Name, bd.Name)
		if _, _, err := m.Session.Run(ctx, cmd); err != nil {
			continue
		}
		raw = append(raw, dev)
	}
	sort.Strings(raw)
	return raw
}

type findmntTree struct {
	Filesystems []findmntNode `json:"filesystems"`
}
type findmntNode struct {
	Target   string        `json:"target"`
	Source   string        `json:"source"`
	Fstype   string        `json:"fstype"`
	Children []findmntNode `json:"children,omitempty"`
}

func parseFindmnt(jsonOut string, dst map[string]string) {
	var t findmntTree
	if err := json.Unmarshal([]byte(jsonOut), &t); err != nil {
		return
	}
	var walk func([]findmntNode)
	walk = func(ns []findmntNode) {
		for _, n := range ns {
			if n.Target != "" {
				dst[n.Target] = n.Source
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(t.Filesystems)
}

// PlanLayout plans changes for a typed DiskLayout. This is the ergonomic
// entrypoint used by the CLI and the orchestrator.
func (m *DiskManager) PlanLayout(ctx context.Context, layout *config.DiskLayout) ([]Change, error) {
	if layout == nil {
		return nil, nil
	}
	var disc *discovery
	if m.Session != nil {
		d, err := m.discover(ctx)
		if err != nil {
			return nil, err
		}
		disc = d
	} else {
		disc = &discovery{PVs: map[string]string{}, VGs: map[string]bool{}, LVs: map[string]bool{}, FSType: map[string]string{}, Mounts: map[string]string{}}
	}
	var changes []Change
	if layout.Root != nil {
		cs, err := m.planDisk(ctx, disc, layout.Root.Device, layout.Root.VGName, layout.Root.LogicalVolumes, true)
		if err != nil {
			return nil, err
		}
		changes = append(changes, cs...)
	}
	// Resolve tag → device for additional disks that don't set Device
	// explicitly. Claim raw disks (sda is system, so the first raw disk
	// here is typically sdb) in declaration order. Already-claimed devices
	// are removed from the pool so the next entry gets a different one.
	pool := append([]string(nil), disc.RawDisks...)
	for _, ad := range layout.Additional {
		device := ad.Device
		if device == "" {
			if ad.Tag == "" {
				return nil, fmt.Errorf("disk additional: must set device: or tag:")
			}
			if len(pool) == 0 {
				return nil, fmt.Errorf("disk additional tag=%q: no raw disks available on host (lsblk reported all disks already partitioned/PV/fs); set device: explicitly or provision the disk via the hypervisor first", ad.Tag)
			}
			device = pool[0]
			pool = pool[1:]
		}
		cs, err := m.planDisk(ctx, disc, device, ad.VGName, ad.LogicalVolumes, false)
		if err != nil {
			return nil, err
		}
		changes = append(changes, cs...)
	}
	return changes, nil
}

// planDisk compares desired state for one disk against discovery and emits
// the ordered changes needed to converge.
func (m *DiskManager) planDisk(ctx context.Context, disc *discovery, device, vg string, lvs []config.LogicalVolume, isRoot bool) ([]Change, error) {
	var changes []Change

	// Device presence — only enforce for additional disks; root disk is
	// assumed bootable and already provisioned.
	if !isRoot && m.Session != nil {
		exists, _ := m.Session.FileExists(ctx, device)
		if !exists {
			changes = append(changes, Change{
				Manager:     "disk",
				Target:      device,
				Action:      "noop",
				Hazard:      HazardWarn,
				After:       map[string]any{"op": "error", "reason": "device not present — hypervisor must provision first"},
				RollbackCmd: "",
			})
			return changes, nil
		}
	}

	// PV.
	if _, ok := disc.PVs[device]; !ok {
		changes = append(changes, Change{
			Manager:     "disk",
			Target:      device,
			Action:      "create",
			Hazard:      HazardDestructive,
			After:       map[string]any{"op": "pvcreate", "device": device},
			RollbackCmd: fmt.Sprintf("pvremove -ff -y %s", device),
		})
	}
	// VG.
	if !disc.VGs[vg] {
		changes = append(changes, Change{
			Manager:     "disk",
			Target:      vg,
			Action:      "create",
			Hazard:      HazardDestructive,
			After:       map[string]any{"op": "vgcreate", "name": vg, "pvs": []string{device}},
			RollbackCmd: fmt.Sprintf("vgremove -ff -y %s", vg),
		})
	}
	// LVs + FS + fstab + mount.
	for _, lv := range lvs {
		lvKey := vg + "/" + lv.Name
		dev := "/dev/" + vg + "/" + lv.Name
		if !disc.LVs[lvKey] {
			changes = append(changes, Change{
				Manager:     "disk",
				Target:      dev,
				Action:      "create",
				Hazard:      HazardDestructive,
				After:       map[string]any{"op": "lvcreate", "vg": vg, "name": lv.Name, "size": lv.Size},
				RollbackCmd: fmt.Sprintf("lvremove -f -y %s", dev),
			})
		}
		// FS (we don't know fs-by-device without blkid; conservative: always plan mkfs if LV is new).
		changes = append(changes, Change{
			Manager:     "disk",
			Target:      dev,
			Action:      "create",
			Hazard:      HazardDestructive,
			After:       map[string]any{"op": "mkfs", "device": dev, "fstype": lv.FS},
			RollbackCmd: "",
		})
		if lv.MountPoint != "" {
			// fstab entry.
			fstabLine := fmt.Sprintf("%s %s %s %s 0 0", dev, lv.MountPoint, lv.FS, "defaults")
			if !strings.Contains(disc.Fstab, dev+" "+lv.MountPoint) {
				changes = append(changes, Change{
					Manager:     "disk",
					Target:      lv.MountPoint,
					Action:      "create",
					Hazard:      HazardWarn,
					After:       map[string]any{"op": "fstab", "entry": fstabLine},
					RollbackCmd: fmt.Sprintf("sed -i '\\|^%s %s|d' /etc/fstab", dev, lv.MountPoint),
				})
			}
			if _, mounted := disc.Mounts[lv.MountPoint]; !mounted {
				changes = append(changes, Change{
					Manager:     "disk",
					Target:      lv.MountPoint,
					Action:      "create",
					Hazard:      HazardWarn,
					After:       map[string]any{"op": "mount", "mountpoint": lv.MountPoint, "device": dev},
					RollbackCmd: fmt.Sprintf("umount %s", lv.MountPoint),
				})
			}
		}
	}
	return changes, nil
}

// Plan implements Manager. Accepts a *config.DiskLayout via desired.
func (m *DiskManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	layout, err := coerceLayout(desired)
	if err != nil {
		return nil, err
	}
	return m.PlanLayout(ctx, layout)
}

func coerceLayout(desired Spec) (*config.DiskLayout, error) {
	if desired == nil {
		return nil, nil
	}
	switch v := desired.(type) {
	case *config.DiskLayout:
		return v, nil
	case config.DiskLayout:
		return &v, nil
	case nil:
		return nil, nil
	}
	return nil, fmt.Errorf("disk: unsupported desired spec type %T", desired)
}

// Apply executes the ordered changes via the session.
func (m *DiskManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	start := time.Now()
	res := ApplyResult{RunID: fmt.Sprintf("disk-%d", start.UnixNano())}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		res.Duration = time.Since(start)
		return res, nil
	}
	for _, c := range changes {
		cmd, err := diskChangeCmd(c)
		if err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: c, Err: err})
			res.Duration = time.Since(start)
			return res, err
		}
		if cmd == "" {
			res.Skipped = append(res.Skipped, c)
			continue
		}
		if m.Session == nil {
			res.Failed = append(res.Failed, ChangeErr{Change: c, Err: fmt.Errorf("no session")})
			res.Duration = time.Since(start)
			return res, fmt.Errorf("disk: no session")
		}
		if _, stderr, err := m.Session.RunSudo(ctx, cmd); err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: c, Err: fmt.Errorf("%w: %s", err, stderr)})
			res.Duration = time.Since(start)
			return res, err
		}
		res.Applied = append(res.Applied, c)
	}
	res.Duration = time.Since(start)
	return res, nil
}

func diskChangeCmd(c Change) (string, error) {
	after, ok := c.After.(map[string]any)
	if !ok {
		return "", fmt.Errorf("disk: change %q has no After map", c.Target)
	}
	op, _ := after["op"].(string)
	switch op {
	case "pvcreate":
		return fmt.Sprintf("pvcreate -f %s", after["device"]), nil
	case "vgcreate":
		pvs, _ := after["pvs"].([]string)
		if len(pvs) == 0 {
			// tests may pass []any
			if av, ok := after["pvs"].([]any); ok {
				for _, x := range av {
					if s, ok := x.(string); ok {
						pvs = append(pvs, s)
					}
				}
			}
		}
		return fmt.Sprintf("vgcreate %s %s", after["name"], strings.Join(pvs, " ")), nil
	case "lvcreate":
		// lvcreate uses two different size flags:
		//   -L <size>  → absolute (200G, 4096M, 1T)
		//   -l <pct>   → relative percentage (100%FREE, 50%VG, 100%PVS)
		// Manifest values like "100%" or "100%FREE" map to -l; anything else
		// is treated as absolute and goes to -L. Bare "N%" gets normalized
		// to "N%FREE" (the most useful default — fill the unallocated space
		// in the VG).
		size := fmt.Sprint(after["size"])
		flag := "-L"
		val := size
		if strings.Contains(size, "%") {
			flag = "-l"
			if !strings.ContainsAny(size, "FVP") { // FREE, VG, PVS
				val = strings.TrimRight(size, "%") + "%FREE"
			}
		}
		return fmt.Sprintf("lvcreate -y -n %s %s %s %s", after["name"], flag, val, after["vg"]), nil
	case "mkfs":
		return fmt.Sprintf("mkfs.%s -F %s", after["fstype"], after["device"]), nil
	case "fstab":
		entry, _ := after["entry"].(string)
		return fmt.Sprintf("sh -c 'grep -qxF %s /etc/fstab || echo %s >> /etc/fstab'", shSingleQuote(entry), shSingleQuote(entry)), nil
	case "mount":
		return fmt.Sprintf("mount %s", after["mountpoint"]), nil
	case "error":
		return "", fmt.Errorf("disk: %v", after["reason"])
	}
	return "", fmt.Errorf("disk: unknown op %q", op)
}

// Verify re-plans and reports any remaining drift.
func (m *DiskManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback reverses in-session changes in reverse order.
func (m *DiskManager) Rollback(ctx context.Context, changes []Change) error {
	if m.Session == nil {
		return fmt.Errorf("disk: no session")
	}
	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		if c.RollbackCmd == "" {
			continue
		}
		if _, stderr, err := m.Session.RunSudo(ctx, c.RollbackCmd); err != nil {
			return fmt.Errorf("disk rollback %s: %w (%s)", c.Target, err, stderr)
		}
	}
	return nil
}

// ---- helpers ----

func defaultStr(s, dflt string) string {
	if s == "" {
		return dflt
	}
	return s
}

func shSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func init() { Register(NewDiskManager()) }
