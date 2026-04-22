package root

import (
	"context"
	"fmt"
	"os"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/managers"
	"github.com/itunified-io/linuxctl/pkg/session"
)

// envPathFromArgs returns the first positional argument, or --env flag value,
// or the current directory's default "env.yaml".
func envPathFromArgs(args []string) string {
	if len(args) > 0 && args[0] != "" {
		return args[0]
	}
	if gf.env != "" {
		return gf.env
	}
	return "env.yaml"
}

// openSession returns a session for the current --host flag. Empty host →
// local session; otherwise a lazy SSH descriptor (not dialed). Exposed as a
// package-level var so tests can substitute a deterministic local session.
var openSession = openSessionReal

func openSessionReal() session.Session {
	if gf.host == "" || gf.host == "localhost" {
		return session.NewLocal()
	}
	u := os.Getenv("USER")
	if u == "" {
		u = "root"
	}
	return session.NewSSH(gf.host, u)
}

// loadLinux loads a linux.yaml from either an env.yaml or a direct linux.yaml
// file. The argument is a path; if it's an env.yaml, we follow spec.linux
// ($ref or inline) to the effective Linux manifest.
func loadLinux(path string) (*config.Linux, error) {
	if path == "" {
		return &config.Linux{}, nil
	}
	// Heuristic: try as env first, fall back to linux.
	env, err := config.LoadEnv(path, nil)
	if err == nil && env != nil && env.Spec.Linux.Value != nil {
		return env.Spec.Linux.Value, nil
	}
	return config.LoadLinux(path)
}

// printChanges writes a compact table of changes to stdout.
func printChanges(mgr string, changes []managers.Change) {
	if len(changes) == 0 {
		fmt.Printf("%s: no changes\n", mgr)
		return
	}
	fmt.Printf("%s: %d change(s)\n", mgr, len(changes))
	for _, c := range changes {
		fmt.Printf("  [%s] %s %s", c.Action, c.Target, hazardMark(c.Hazard))
		if m, ok := c.After.(map[string]any); ok {
			if op, ok := m["op"].(string); ok {
				fmt.Printf("  op=%s", op)
			}
		}
		fmt.Println()
	}
}

func hazardMark(h managers.HazardLevel) string {
	switch h {
	case managers.HazardDestructive:
		return "!!"
	case managers.HazardWarn:
		return "!"
	}
	return ""
}

// deadlineCtx returns a context with a sensible default timeout.
func deadlineCtx() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// bindSession best-effort attaches sess to m. If m exposes a WithSession
// method (any signature accepting a session.Session), we call it via type
// switch on the known concrete types. Unknown managers pass through.
func bindSession(m managers.Manager, sess session.Session) managers.Manager {
	switch v := m.(type) {
	case interface {
		WithSession(session.Session) *managers.DiskManager
	}:
		return v.WithSession(sess)
	case interface {
		WithSession(session.Session) *managers.MountManager
	}:
		return v.WithSession(sess)
	}
	// Try a broader reflective-style match via known signatures of other
	// managers. Since we can't import their concrete types without an import
	// cycle, we rely on the interface-based dispatch above plus fallthrough
	// for managers that wire their session through package-level globals or
	// managers.All().
	return m
}

// desiredFor returns the desired-state value a given manager expects. Mirrors
// pkg/apply.Orchestrator.desiredFor but without a dependency on that package.
func desiredFor(linux *config.Linux, name string) managers.Spec {
	if linux == nil {
		return nil
	}
	switch name {
	case "disk":
		return linux.DiskLayout
	case "mount":
		return linux.Mounts
	case "user":
		return usersGroupsSpec(linux.UsersGroups)
	case "dir":
		return linux.Directories
	case "package":
		return packagesSpec(linux.Packages)
	}
	return linux
}

// usersGroupsSpec converts the typed config.UsersGroups (nominal type) into
// the shape the managers package expects. Both structs are structurally
// identical but distinct nominal types; managers.castUsersGroups previously
// rejected config.*UsersGroups with "unsupported desired-state type".
// Fixes linuxctl#8.
func usersGroupsSpec(ug *config.UsersGroups) managers.UsersGroupsSpec {
	if ug == nil {
		return managers.UsersGroupsSpec{}
	}
	spec := managers.UsersGroupsSpec{}
	for _, g := range ug.Groups {
		spec.Groups = append(spec.Groups, managers.GroupSpec{Name: g.Name, GID: g.GID})
	}
	for _, u := range ug.Users {
		spec.Users = append(spec.Users, managers.UserSpec{
			Name:     u.Name,
			UID:      u.UID,
			GID:      u.GID,
			Groups:   u.Groups,
			Home:     u.Home,
			Shell:    u.Shell,
			SSHKeys:  u.SSHKeys,
			Password: u.Password,
		})
	}
	return spec
}

// packagesSpec converts config.Packages → managers.PackagesSpec. Fixes linuxctl#8.
func packagesSpec(p *config.Packages) managers.PackagesSpec {
	if p == nil {
		return managers.PackagesSpec{}
	}
	return managers.PackagesSpec{
		Install: p.Install,
		Remove:  p.Remove,
	}
}
