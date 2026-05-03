package root

import (
	"context"
	"fmt"
	"os"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/managers"
	"github.com/itunified-io/linuxctl/pkg/session"
)

// stackPathFromArgs returns the first positional argument, or --stack flag
// value (preferred), or --env flag value (deprecated alias), or the current
// directory's default "env.yaml". If both --stack and --env are set, --stack
// wins and a warning is printed to stderr.
func stackPathFromArgs(args []string) string {
	if len(args) > 0 && args[0] != "" {
		return args[0]
	}
	if gf.stack != "" {
		if gf.env != "" && gf.env != gf.stack {
			fmt.Fprintln(os.Stderr, "warning: both --stack and --env set; --stack wins (--env is deprecated)")
		}
		return gf.stack
	}
	if gf.env != "" {
		fmt.Fprintln(os.Stderr, "warning: --env is deprecated; use --stack instead")
		return gf.env
	}
	return "env.yaml"
}

// envPathFromArgs is a deprecated shim that forwards to stackPathFromArgs.
// Kept because internal callers and tests still reference this name; remove
// in a follow-up once the call sites are migrated.
func envPathFromArgs(args []string) string { return stackPathFromArgs(args) }

// openSession returns a session for the current --host flag. Empty host →
// local session; otherwise a lazy SSH descriptor (not dialed). Exposed as a
// package-level var so tests can substitute a deterministic local session.
var openSession = openSessionReal

func openSessionReal() session.Session {
	if gf.host == "" || gf.host == "localhost" {
		return session.NewLocal()
	}
	u := gf.sshUser
	if u == "" {
		u = os.Getenv("USER")
	}
	if u == "" {
		u = "root"
	}
	port := gf.sshPort
	if port == 0 {
		port = 22
	}
	// Dial the SSH transport so managers can run commands immediately.
	// session.NewSSH alone returns a non-dialed descriptor; managers fail
	// with "ssh: not connected (call Dial first)" on use (linuxctl#25).
	s, err := session.NewSSHDial(session.Opts{
		Host:    gf.host,
		Port:    port,
		User:    u,
		KeyFile: gf.sshKey,
	})
	if err != nil {
		// Fall back to a non-dialed descriptor; manager calls will surface
		// the error with manager context. We could panic here, but keeping
		// the lazy descriptor lets `--dry-run` paths that don't actually
		// run remote commands still work.
		fmt.Fprintf(os.Stderr, "warn: ssh dial %s@%s:%d failed: %v\n", u, gf.host, port, err)
		return session.NewSSH(gf.host, u)
	}
	return s
}

// loadLinux loads a linux.yaml from either an env.yaml or a direct linux.yaml
// file. The argument is a path; if it's an env.yaml, we follow spec.linux
// ($ref or inline) to the effective Linux manifest.
func loadLinux(path string) (*config.Linux, error) {
	if path == "" {
		return &config.Linux{}, nil
	}
	// Build a Vault-backed resolver so ${vault:…} placeholders in
	// linux.yaml (e.g. user.ssh_keys, user.password, mount credentials)
	// resolve at load time instead of getting written verbatim into
	// authorized_keys / etc/sudoers.d / shadow / cifs creds files.
	resolver := config.NewResolver()
	resolver.Vault = config.NewHTTPVaultReader()
	// Heuristic: try as env first, fall back to linux.
	env, err := config.LoadEnv(path, resolver)
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
	if sess == nil {
		return m
	}
	// Mirror the orchestrator's bindSession (pkg/apply/orchestrator.go) so
	// every per-subsystem CLI command (`linuxctl hosts apply`, `linuxctl
	// firewall apply`, …) gets a session-bound manager. Without this, single-
	// manager invocations bypass session wiring and fail at Plan/Apply with
	// session-required errors (linuxctl#51).
	switch v := m.(type) {
	case *managers.DiskManager:
		// Honor --reformat-filesystems on every disk-manager invocation
		// (single-manager CLI + orchestrator alike). linuxctl#52.
		return v.WithSession(sess).WithReformatFilesystems(gf.reformatFilesystems)
	case *managers.MountManager:
		return v.WithSession(sess)
	case *managers.UserManager:
		return v.WithSession(sess)
	case *managers.PackageManager:
		return v.WithSession(sess)
	case *managers.DirManager:
		return v.WithSession(sess)
	case *managers.LimitsManager:
		return v.WithSession(sess)
	case *managers.SELinuxManager:
		return v.WithSession(sess)
	case *managers.SysctlManager:
		return v.WithSession(sess)
	case *managers.ServiceManager:
		return v.WithSession(sess)
	case *managers.FirewallManager:
		return v.WithSession(sess)
	case *managers.HostsManager:
		return v.WithSession(sess)
	case *managers.NetworkManager:
		return v.WithSession(sess)
	case *managers.SSHAuthManager:
		return v.WithSession(sess)
	case *managers.RepoManager:
		return v.WithSession(sess)
	case *managers.FileManager:
		return v.WithSession(sess)
	}
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
	case "hosts":
		// HostsManager.castHostEntries accepts both *config.Linux and
		// []config.HostEntry; pass the slice directly so an empty
		// hosts_entries: produces a clean no-op plan rather than the
		// whole Linux struct dragging in unrelated fields. linuxctl#51.
		return linux.HostsEntries
	case "firewall":
		// FirewallManager.castFirewall accepts *config.Firewall directly.
		// Pass the typed pointer (may be nil → no-op plan). linuxctl#51.
		return linux.Firewall
	case "repo":
		// RepoManager consumes a []string of dnf repo IDs. linuxctl#57.
		return linux.ReposEnable
	case "file":
		// FileManager consumes []config.FileSpec literal payloads. linuxctl#57.
		return linux.Files
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
			Sudo:     u.Sudo,
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
