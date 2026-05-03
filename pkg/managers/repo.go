package managers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/itunified-io/linuxctl/pkg/session"
)

// RepoManager reconciles dnf-managed repository enablement state.
//
// The manager only enables repositories declared in the manifest's
// `repos_enable:` list (typically populated by a bundle preset — see
// linuxctl#57). Disabling repositories is a one-way Rollback action,
// performed only on the IDs this run enabled.
type RepoManager struct {
	sess session.Session
}

// NewRepoManager returns a repo manager with no session.
func NewRepoManager() *RepoManager { return &RepoManager{} }

// WithSession binds sess for Apply / Verify / Rollback.
func (m *RepoManager) WithSession(s session.Session) *RepoManager {
	cp := *m
	cp.sess = s
	return &cp
}

// Name implements Manager.
func (*RepoManager) Name() string { return "repo" }

// DependsOn implements Manager. Repos must be enabled before package install
// resolution but after a working dnf binary (which the base image provides).
func (*RepoManager) DependsOn() []string { return nil }

func init() { Register(NewRepoManager()) }

// ---- Plan -----------------------------------------------------------------

// repoPlanPayload pairs the desired repo ID with its observed enabled state
// at plan time, captured in Change.Before so Rollback can reverse exactly
// what Apply enabled.
type repoPlanPayload struct {
	ID            string
	WasEnabledBefore bool
}

// Plan implements Manager.
func (m *RepoManager) Plan(ctx context.Context, desired Spec, _ State) ([]Change, error) {
	repos := castReposEnable(desired)
	if len(repos) == 0 {
		return nil, nil
	}
	if m.sess == nil {
		return nil, ErrSessionRequired
	}
	enabled, err := m.listEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("repo.Plan: list enabled repos: %w", err)
	}
	known, err := m.listAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("repo.Plan: list all repos: %w", err)
	}
	var changes []Change
	seen := map[string]bool{}
	for _, id := range repos {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if enabled[id] {
			continue // already in desired state
		}
		if !known[id] {
			return nil, fmt.Errorf("repo.Plan: repo %q is not configured on this host", id)
		}
		changes = append(changes, Change{
			ID:      "repo:" + id,
			Manager: "repo",
			Target:  id,
			Action:  "update",
			Before:  repoPlanPayload{ID: id, WasEnabledBefore: false},
			After:   repoPlanPayload{ID: id, WasEnabledBefore: false},
			Hazard:  HazardWarn,
		})
	}
	return changes, nil
}

// ---- Apply ----------------------------------------------------------------

// Apply implements Manager.
func (m *RepoManager) Apply(ctx context.Context, changes []Change, dryRun bool) (ApplyResult, error) {
	res := ApplyResult{}
	if dryRun {
		res.Skipped = append(res.Skipped, changes...)
		return res, nil
	}
	if m.sess == nil {
		return res, ErrSessionRequired
	}
	for _, ch := range changes {
		p, ok := ch.After.(repoPlanPayload)
		if !ok {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("repo.Apply: After is not repoPlanPayload (%T)", ch.After)})
			continue
		}
		cmd := "dnf config-manager --set-enabled " + shellQuoteOne(p.ID)
		if _, err := RunSudoAndCheck(ctx, m.sess, cmd); err != nil {
			res.Failed = append(res.Failed, ChangeErr{Change: ch, Err: fmt.Errorf("enable %s: %w", p.ID, err)})
			continue
		}
		res.Applied = append(res.Applied, ch)
	}
	return res, nil
}

// ---- Verify + Rollback ----------------------------------------------------

// Verify implements Manager.
func (m *RepoManager) Verify(ctx context.Context, desired Spec) (VerifyResult, error) {
	changes, err := m.Plan(ctx, desired, nil)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{OK: len(changes) == 0, Drift: changes}, nil
}

// Rollback disables exactly the repo IDs this run enabled (Before.WasEnabledBefore == false).
func (m *RepoManager) Rollback(ctx context.Context, changes []Change) error {
	if m.sess == nil {
		return ErrSessionRequired
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		b, ok := ch.Before.(repoPlanPayload)
		if !ok {
			continue
		}
		if b.WasEnabledBefore {
			continue // we did not enable it; leave alone
		}
		_, _ = RunSudoAndCheck(ctx, m.sess, "dnf config-manager --set-disabled "+shellQuoteOne(b.ID))
	}
	return nil
}

// ---- helpers --------------------------------------------------------------

// listEnabled returns a set of currently-enabled repo IDs.
func (m *RepoManager) listEnabled(ctx context.Context) (map[string]bool, error) {
	out, _, err := m.sess.Run(ctx, "dnf -q repolist --enabled")
	if err != nil {
		return nil, err
	}
	return parseRepolist(out), nil
}

// listAll returns the set of all configured repo IDs (enabled or disabled).
func (m *RepoManager) listAll(ctx context.Context) (map[string]bool, error) {
	out, _, err := m.sess.Run(ctx, "dnf -q repolist --all")
	if err != nil {
		return nil, err
	}
	return parseRepolist(out), nil
}

// parseRepolist extracts repo IDs from `dnf repolist` output. The first
// whitespace-delimited token of each non-header line is the repo ID.
// "repo id" and "repo name" header lines are skipped.
func parseRepolist(out string) map[string]bool {
	res := map[string]bool{}
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		// Skip the dnf table header.
		if strings.HasPrefix(strings.ToLower(ln), "repo id") {
			continue
		}
		fields := strings.Fields(ln)
		if len(fields) == 0 {
			continue
		}
		id := fields[0]
		if id == "Last" || strings.HasPrefix(id, "metadata") {
			continue
		}
		res[id] = true
	}
	return res
}

// castReposEnable accepts the slice form (manifest-driven) directly. The
// orchestrator passes desiredFor("repo") which returns []string from
// config.Linux.ReposEnable.
func castReposEnable(desired Spec) []string {
	switch v := desired.(type) {
	case []string:
		out := append([]string(nil), v...)
		sort.Strings(out)
		return out
	case nil:
		return nil
	default:
		return nil
	}
}
