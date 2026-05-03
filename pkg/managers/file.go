package managers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/session"
)

// FileManager reconciles literal file payloads declared in
// config.Linux.Files (typically populated by a bundle preset; see
// linuxctl#57). The manager owns each declared path: it writes the
// decoded base64 content with the requested mode/owner/group, and on
// Rollback it either removes the file (if absent before) or restores
// the previous body.
type FileManager struct {
	sess session.Session
}

// NewFileManager returns a file manager with no session.
func NewFileManager() *FileManager { return &FileManager{} }

// WithSession binds sess for Apply / Verify / Rollback.
func (m *FileManager) WithSession(s session.Session) *FileManager {
	cp := *m
	cp.sess = s
	return &cp
}

// Name implements Manager.
func (*FileManager) Name() string { return "file" }

// DependsOn implements Manager. File payloads have no hard dependency on
// any other manager — they describe state, not behaviour.
func (*FileManager) DependsOn() []string { return nil }

func init() { Register(NewFileManager()) }

// ---- Plan -----------------------------------------------------------------

// fileBefore captures the existing file (if any) so Rollback can restore it.
type fileBefore struct {
	Existed bool
	Body    []byte
}

// fileApply is the After payload — the decoded body and its file metadata.
type fileApply struct {
	Spec config.FileSpec
	Body []byte
}

// Plan implements Manager.
func (m *FileManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	files := castFiles(desired)
	if len(files) == 0 {
		return nil, nil
	}
	if m.sess == nil {
		return nil, ErrSessionRequired
	}
	// Stable iteration order for deterministic plans.
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	var changes []Change
	for _, f := range files {
		body, err := decodeContent(f.ContentB64)
		if err != nil {
			return nil, fmt.Errorf("file.Plan: %s: %w", f.Path, err)
		}
		exists, err := m.sess.FileExists(ctx, f.Path)
		if err != nil {
			return nil, fmt.Errorf("file.Plan: stat %s: %w", f.Path, err)
		}
		var before fileBefore
		var existingBody []byte
		if exists {
			existingBody, err = m.sess.ReadFile(ctx, f.Path)
			if err != nil {
				return nil, fmt.Errorf("file.Plan: read %s: %w", f.Path, err)
			}
			before = fileBefore{Existed: true, Body: existingBody}
		}

		// CreateOnly: skip if the file already exists, regardless of
		// body drift. Used for stub files that must defer to a real
		// implementation if installed later.
		if f.CreateOnly && exists {
			continue
		}

		// Idempotency: if body hash + mode match the desired, skip.
		// (Owner/group are only verified at Apply time because reading
		// them through session.Session would require a stat helper that
		// is not yet on the interface.)
		if exists && hashEqual(existingBody, body) {
			// Same body — no content change. We currently do not
			// re-stat mode/owner/group at Plan time; Apply runs
			// chmod/chown unconditionally, which is itself idempotent.
			continue
		}

		action := "create"
		if exists {
			action = "update"
		}
		changes = append(changes, Change{
			ID:      "file:" + f.Path,
			Manager: "file",
			Target:  f.Path,
			Action:  action,
			Before:  before,
			After:   fileApply{Spec: f, Body: body},
			Hazard:  HazardWarn,
		})
	}
	return changes, nil
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *FileManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, ErrSessionRequired
	}
	for _, ch := range changes {
		a, ok := ch.After.(fileApply)
		if !ok {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("file.Apply: After is not fileApply (%T)", ch.After)})
			continue
		}
		mode, err := parseMode(a.Spec.Mode, 0o644)
		if err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: err})
			continue
		}
		if err := m.sess.WriteFile(ctx, a.Spec.Path, a.Body, mode); err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("write %s: %w", a.Spec.Path, err)})
			continue
		}
		// chown if either owner or group set.
		if a.Spec.Owner != "" || a.Spec.Group != "" {
			ownerSpec := a.Spec.Owner
			if a.Spec.Group != "" {
				ownerSpec = ownerSpec + ":" + a.Spec.Group
			}
			cmd := "chown " + shellQuoteOne(ownerSpec) + " " + shellQuoteOne(a.Spec.Path)
			if _, err := RunSudoAndCheck(ctx, m.sess, cmd); err != nil {
				res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("chown %s: %w", a.Spec.Path, err)})
				continue
			}
		}
		res.Applied = append(res.Applied, ch)
	}
	return res, nil
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *FileManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback restores the pre-Apply state captured in Change.Before.
func (m *FileManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return ErrSessionRequired
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		b, ok := ch.Before.(fileBefore)
		if !ok {
			continue
		}
		path, ok := ch.Target, true
		_ = ok
		if !b.Existed {
			_, _ = RunSudoAndCheck(ctx, m.sess, "rm -f "+shellQuoteOne(path))
			continue
		}
		if err := m.sess.WriteFile(ctx, path, b.Body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ---- helpers --------------------------------------------------------------

// decodeContent returns the decoded body of a FileSpec.ContentB64.
func decodeContent(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, nil
	}
	body, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("invalid content_b64: %w", err)
	}
	return body, nil
}

// parseMode parses a 4-digit octal mode string. Empty falls back to def.
func parseMode(s string, def uint32) (uint32, error) {
	if s == "" {
		return def, nil
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid mode %q: %w", s, err)
	}
	return uint32(v), nil
}

// hashEqual reports whether a + b have the same SHA-256 digest.
func hashEqual(a, b []byte) bool {
	ha := sha256.Sum256(a)
	hb := sha256.Sum256(b)
	return hex.EncodeToString(ha[:]) == hex.EncodeToString(hb[:])
}

// castFiles accepts the typed slice the runtime passes for the file manager.
func castFiles(desired Spec) []config.FileSpec {
	switch v := desired.(type) {
	case []config.FileSpec:
		return v
	case nil:
		return nil
	default:
		return nil
	}
}
