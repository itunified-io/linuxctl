package managers

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/session"
)

// MountManager handles CIFS / NFS / bind mounts. For CIFS, credentials are
// written to /etc/cifs-utils/credentials/<tag> with mode 0600.
type MountManager struct {
	Session session.Session
	// Vault optionally resolves CIFS credentials from a Vault path declared
	// via `credentials_vault:` in the Mount manifest. The expected schema
	// at the resolved path is `{"username": "<u>", "password": "<p>"}`.
	// If nil, manifests with credentials_vault and no inline username/password
	// produce an apply-time error pointing the operator at this gap.
	Vault config.VaultReader
}

// NewMountManager returns a mount manager.
func NewMountManager() *MountManager { return &MountManager{} }

// WithSession returns a copy bound to sess.
func (m *MountManager) WithSession(sess session.Session) *MountManager {
	cp := *m
	cp.Session = sess
	return &cp
}

// Name implements Manager.
func (*MountManager) Name() string { return "mount" }

// DependsOn implements Manager.
func (*MountManager) DependsOn() []string { return []string{"disk"} }

// PlanMounts is the typed planning entrypoint.
func (m *MountManager) PlanMounts(ctx context.Context, mounts []config.Mount) ([]Change, error) {
	currentMounts := map[string]string{}
	fstab := ""
	if m.Session != nil {
		if out, _, err := m.Session.Run(ctx, "findmnt --json"); err == nil {
			parseFindmnt(out, currentMounts)
		}
		if b, err := m.Session.ReadFile(ctx, "/etc/fstab"); err == nil {
			fstab = string(b)
		}
	}
	var changes []Change
	for _, mnt := range mounts {
		cs, err := planMount(mnt, currentMounts, fstab)
		if err != nil {
			return nil, err
		}
		changes = append(changes, cs...)
	}
	return changes, nil
}

func planMount(mnt config.Mount, current map[string]string, fstab string) ([]Change, error) {
	switch mnt.Type {
	case "cifs":
		return planCIFS(mnt, current, fstab), nil
	case "nfs":
		return planNFS(mnt, current, fstab), nil
	case "bind":
		return planBind(mnt, current, fstab), nil
	case "tmpfs":
		return planTmpfs(mnt, current, fstab), nil
	}
	return nil, fmt.Errorf("mount: unknown type %q", mnt.Type)
}

// credsFileForMount derives a deterministic credentials-file path from the
// vault reference or CIFS source. Mode is always 0600.
func credsFileForMount(mnt config.Mount) string {
	tag := sanitizeTag(mnt.Server + "-" + mnt.Share)
	return "/etc/cifs-utils/credentials/" + tag
}

func planCIFS(mnt config.Mount, current map[string]string, fstab string) []Change {
	var changes []Change
	credFile := credsFileForMount(mnt)
	source := fmt.Sprintf("//%s/%s", mnt.Server, mnt.Share)
	optsStr := strings.Join(mnt.Options, ",")

	changes = append(changes, Change{
		Manager:     "mount",
		Target:      credFile,
		Action:      "create",
		Hazard:      HazardNone,
		After:       map[string]any{"op": "cifs_credentials", "path": credFile, "vault": mnt.CredentialsVault},
		RollbackCmd: fmt.Sprintf("rm -f %s", credFile),
	})

	if existing, ok := current[mnt.MountPoint]; !ok || existing != source {
		changes = append(changes, Change{
			Manager:     "mount",
			Target:      mnt.MountPoint,
			Action:      "create",
			Hazard:      HazardWarn,
			After:       map[string]any{"op": "cifs_mount", "source": source, "mountpoint": mnt.MountPoint, "options": optsStr, "credentials": credFile},
			RollbackCmd: fmt.Sprintf("umount %s", mnt.MountPoint),
		})
	}
	if mnt.Persistent {
		line := fmt.Sprintf("%s %s cifs %s 0 0", source, mnt.MountPoint, combineOpts(optsStr, "credentials="+credFile))
		if !strings.Contains(fstab, source+" "+mnt.MountPoint) {
			changes = append(changes, Change{
				Manager:     "mount",
				Target:      mnt.MountPoint,
				Action:      "create",
				Hazard:      HazardWarn,
				After:       map[string]any{"op": "fstab", "entry": line, "source": source, "mountpoint": mnt.MountPoint},
				RollbackCmd: fmt.Sprintf("sed -i '\\|^%s %s|d' /etc/fstab", source, mnt.MountPoint),
			})
		}
	}
	return changes
}

func planNFS(mnt config.Mount, current map[string]string, fstab string) []Change {
	var changes []Change
	source := fmt.Sprintf("%s:%s", mnt.Server, mnt.Share)
	optsStr := strings.Join(mnt.Options, ",")
	if existing, ok := current[mnt.MountPoint]; !ok || existing != source {
		changes = append(changes, Change{
			Manager:     "mount",
			Target:      mnt.MountPoint,
			Action:      "create",
			Hazard:      HazardWarn,
			After:       map[string]any{"op": "nfs_mount", "source": source, "mountpoint": mnt.MountPoint, "options": optsStr},
			RollbackCmd: fmt.Sprintf("umount %s", mnt.MountPoint),
		})
	}
	if mnt.Persistent {
		line := fmt.Sprintf("%s %s nfs %s 0 0", source, mnt.MountPoint, defaultStr(optsStr, "defaults"))
		if !strings.Contains(fstab, source+" "+mnt.MountPoint) {
			changes = append(changes, Change{
				Manager:     "mount",
				Target:      mnt.MountPoint,
				Action:      "create",
				Hazard:      HazardWarn,
				After:       map[string]any{"op": "fstab", "entry": line, "source": source, "mountpoint": mnt.MountPoint},
				RollbackCmd: fmt.Sprintf("sed -i '\\|^%s %s|d' /etc/fstab", source, mnt.MountPoint),
			})
		}
	}
	return changes
}

func planBind(mnt config.Mount, current map[string]string, fstab string) []Change {
	var changes []Change
	if existing, ok := current[mnt.MountPoint]; !ok || existing != mnt.Source {
		changes = append(changes, Change{
			Manager:     "mount",
			Target:      mnt.MountPoint,
			Action:      "create",
			Hazard:      HazardWarn,
			After:       map[string]any{"op": "bind_mount", "source": mnt.Source, "mountpoint": mnt.MountPoint},
			RollbackCmd: fmt.Sprintf("umount %s", mnt.MountPoint),
		})
	}
	if mnt.Persistent {
		line := fmt.Sprintf("%s %s none bind 0 0", mnt.Source, mnt.MountPoint)
		if !strings.Contains(fstab, mnt.Source+" "+mnt.MountPoint) {
			changes = append(changes, Change{
				Manager:     "mount",
				Target:      mnt.MountPoint,
				Action:      "create",
				Hazard:      HazardWarn,
				After:       map[string]any{"op": "fstab", "entry": line, "source": mnt.Source, "mountpoint": mnt.MountPoint},
				RollbackCmd: fmt.Sprintf("sed -i '\\|^%s %s|d' /etc/fstab", mnt.Source, mnt.MountPoint),
			})
		}
	}
	return changes
}

func planTmpfs(mnt config.Mount, current map[string]string, fstab string) []Change {
	var changes []Change
	optsStr := strings.Join(mnt.Options, ",")
	if _, ok := current[mnt.MountPoint]; !ok {
		changes = append(changes, Change{
			Manager:     "mount",
			Target:      mnt.MountPoint,
			Action:      "create",
			Hazard:      HazardWarn,
			After:       map[string]any{"op": "tmpfs_mount", "mountpoint": mnt.MountPoint, "options": optsStr},
			RollbackCmd: fmt.Sprintf("umount %s", mnt.MountPoint),
		})
	}
	if mnt.Persistent {
		line := fmt.Sprintf("tmpfs %s tmpfs %s 0 0", mnt.MountPoint, defaultStr(optsStr, "defaults"))
		if !strings.Contains(fstab, "tmpfs "+mnt.MountPoint) {
			changes = append(changes, Change{
				Manager:     "mount",
				Target:      mnt.MountPoint,
				Action:      "create",
				Hazard:      HazardWarn,
				After:       map[string]any{"op": "fstab", "entry": line, "source": "tmpfs", "mountpoint": mnt.MountPoint},
				RollbackCmd: fmt.Sprintf("sed -i '\\|^tmpfs %s|d' /etc/fstab", mnt.MountPoint),
			})
		}
	}
	return changes
}

func combineOpts(parts ...string) string {
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return "defaults"
	}
	return strings.Join(out, ",")
}

func sanitizeTag(s string) string {
	r := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	return r.Replace(strings.Trim(s, "-"))
}

// Plan implements Manager. desired is a []config.Mount.
func (m *MountManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	mounts, err := coerceMounts(desired)
	if err != nil {
		return nil, err
	}
	return m.PlanMounts(ctx, mounts)
}

func coerceMounts(desired Spec) ([]config.Mount, error) {
	if desired == nil {
		return nil, nil
	}
	switch v := desired.(type) {
	case []config.Mount:
		return v, nil
	case *[]config.Mount:
		if v == nil {
			return nil, nil
		}
		return *v, nil
	}
	return nil, fmt.Errorf("mount: unsupported desired spec type %T", desired)
}

// Apply executes planned mount changes.
func (m *MountManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	start := time.Now()
	res := ApplyResult{RunID: fmt.Sprintf("mount-%d", start.UnixNano())}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		res.Duration = time.Since(start)
		return res, nil
	}
	if m.Session == nil {
		return res, fmt.Errorf("mount: no session")
	}
	for _, c := range changes {
		if err := m.applyOne(ctx, c); err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: c, Err: err})
			res.Duration = time.Since(start)
			return res, err
		}
		res.Applied = append(res.Applied, c)
	}
	res.Duration = time.Since(start)
	return res, nil
}

// buildMountCmd is a pure function that assembles the shell command for a
// single mount Change's After map. It returns the command plus an optional
// parent-dir mkdir target. Returning ("", "", err) signals an unknown op.
// It does not handle the "cifs_credentials" op (that path writes a file).
func buildMountCmd(after map[string]any) (cmd, mkdir string, err error) {
	op, _ := after["op"].(string)
	switch op {
	case "cifs_mount":
		source, _ := after["source"].(string)
		mp, _ := after["mountpoint"].(string)
		opts, _ := after["options"].(string)
		cred, _ := after["credentials"].(string)
		mountOpts := combineOpts(opts, "credentials="+cred)
		return fmt.Sprintf("mount -t cifs -o %s %s %s", shSingleQuote(mountOpts), shSingleQuote(source), shSingleQuote(mp)), mp, nil
	case "nfs_mount":
		source, _ := after["source"].(string)
		mp, _ := after["mountpoint"].(string)
		opts, _ := after["options"].(string)
		return fmt.Sprintf("mount -t nfs -o %s %s %s", shSingleQuote(defaultStr(opts, "defaults")), shSingleQuote(source), shSingleQuote(mp)), mp, nil
	case "tmpfs_mount":
		mp, _ := after["mountpoint"].(string)
		opts, _ := after["options"].(string)
		return fmt.Sprintf("mount -t tmpfs -o %s tmpfs %s", shSingleQuote(defaultStr(opts, "defaults")), shSingleQuote(mp)), mp, nil
	case "bind_mount":
		source, _ := after["source"].(string)
		mp, _ := after["mountpoint"].(string)
		return fmt.Sprintf("mount --bind %s %s", shSingleQuote(source), shSingleQuote(mp)), mp, nil
	case "fstab":
		entry, _ := after["entry"].(string)
		return fmt.Sprintf("sh -c 'grep -qxF %s /etc/fstab || echo %s >> /etc/fstab'", shSingleQuote(entry), shSingleQuote(entry)), "", nil
	}
	return "", "", fmt.Errorf("mount: unknown op %q", op)
}

func (m *MountManager) applyOne(ctx context.Context, c Change) error {
	after, ok := c.After.(map[string]any)
	if !ok {
		return fmt.Errorf("mount: missing After map")
	}
	op, _ := after["op"].(string)
	if op == "cifs_credentials" {
		path, _ := after["path"].(string)
		user, _ := after["username"].(string)
		pass, _ := after["password"].(string)
		// Fall through to Vault if username/password not pre-populated and
		// `vault:` carries a path. The expected schema at the path is a
		// kv with `username` and `password` keys.
		if (user == "" || pass == "") {
			vaultPath, _ := after["vault"].(string)
			if vaultPath == "" {
				return fmt.Errorf("mount: cifs_credentials %s: no inline username/password and no credentials_vault — set one", path)
			}
			vault := m.Vault
			if vault == nil {
				// Lazy default: build a Vault HTTP reader from env vars.
				// Lets MountManager work with the registry pattern without
				// requiring runtime-side wiring.
				vault = config.NewHTTPVaultReader()
			}
			u, err := vault.Read(vaultPath + "#username")
			if err != nil {
				return fmt.Errorf("mount: cifs_credentials %s: read username from %s: %w", path, vaultPath, err)
			}
			p, err := vault.Read(vaultPath + "#password")
			if err != nil {
				return fmt.Errorf("mount: cifs_credentials %s: read password from %s: %w", path, vaultPath, err)
			}
			user, pass = u, p
		}
		if _, _, err := m.Session.RunSudo(ctx, "mkdir -p "+shSingleQuote(filepath.Dir(path))); err != nil {
			return err
		}
		content := fmt.Sprintf("username=%s\npassword=%s\n", user, pass)
		return m.Session.WriteFile(ctx, path, []byte(content), 0o600)
	}
	cmd, mkdir, err := buildMountCmd(after)
	if err != nil {
		return err
	}
	if mkdir != "" {
		if _, _, err := m.Session.RunSudo(ctx, "mkdir -p "+shSingleQuote(mkdir)); err != nil {
			return err
		}
	}
	_, stderr, err := m.Session.RunSudo(ctx, cmd)
	if err != nil {
		return fmt.Errorf("%w: %s", err, stderr)
	}
	return nil
}

// Verify replans and reports drift.
func (m *MountManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback reverses in-session changes.
func (m *MountManager) Rollback(ctx context.Context, changes []Change) error {
	if m.Session == nil {
		return fmt.Errorf("mount: no session")
	}
	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		if c.RollbackCmd == "" {
			continue
		}
		if _, stderr, err := m.Session.RunSudo(ctx, c.RollbackCmd); err != nil {
			return fmt.Errorf("mount rollback %s: %w (%s)", c.Target, err, stderr)
		}
	}
	return nil
}

func init() { Register(NewMountManager()) }
