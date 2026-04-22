package root

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Registry-file paths. The canonical name is `stacks.yaml`; `envs.yaml` is a
// deprecated alias kept for one release (#17). Exposed as package-level vars
// so tests can redirect the home directory via overrideHome().
var (
	stacksYAMLName = "stacks.yaml"
	envsYAMLName   = "envs.yaml" // deprecated alias; remove next release
)

// registryHome returns the per-user linuxctl config directory
// (~/.linuxctl). Honors the LINUXCTL_HOME env var for testing.
func registryHome() (string, error) {
	if p := os.Getenv("LINUXCTL_HOME"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".linuxctl"), nil
}

// registryPath returns the canonical path to the stack registry file. If the
// legacy envs.yaml is present and stacks.yaml is not, it is auto-migrated by
// MigrateRegistry() (called from startup).
func registryPath() (string, error) {
	h, err := registryHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, stacksYAMLName), nil
}

// legacyRegistryPath returns the deprecated envs.yaml path.
func legacyRegistryPath() (string, error) {
	h, err := registryHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, envsYAMLName), nil
}

// MigrateRegistry auto-migrates ~/.linuxctl/envs.yaml → ~/.linuxctl/stacks.yaml
// on startup if:
//   - envs.yaml exists, AND
//   - stacks.yaml does NOT exist
//
// In that case envs.yaml is renamed to stacks.yaml (preserving contents bit-for-bit)
// and a one-time warning is printed to stderr. If both exist, stacks.yaml wins,
// envs.yaml is left alone, and a warning is emitted on every startup until the
// user removes envs.yaml. Returns nil if no migration is needed.
//
// This is called from NewRootCmd so every invocation has the registry in its
// canonical location. Fixes #17 with a one-release deprecation window.
func MigrateRegistry() error {
	newPath, err := registryPath()
	if err != nil {
		return err
	}
	oldPath, err := legacyRegistryPath()
	if err != nil {
		return err
	}

	oldStat, oldErr := os.Stat(oldPath)
	newStat, newErr := os.Stat(newPath)
	oldExists := oldErr == nil && !oldStat.IsDir()
	newExists := newErr == nil && !newStat.IsDir()

	switch {
	case !oldExists:
		// Nothing to migrate.
		return nil
	case oldExists && !newExists:
		// Rename in place (atomic within the same dir).
		if err := os.Rename(oldPath, newPath); err != nil {
			// Non-fatal: emit warning and carry on using newPath if we can.
			fmt.Fprintf(os.Stderr,
				"warning: could not migrate %s → %s: %v (continuing; please migrate manually)\n",
				oldPath, newPath, err)
			return nil
		}
		fmt.Fprintf(os.Stderr,
			"notice: migrated %s → %s (env→stack rename, #17). envs.yaml is deprecated and will not be read next release.\n",
			oldPath, newPath)
		return nil
	case oldExists && newExists:
		fmt.Fprintf(os.Stderr,
			"warning: both %s and %s exist; stacks.yaml wins. Please delete envs.yaml (deprecated, will be removed next release).\n",
			oldPath, newPath)
		return nil
	}
	return nil
}

// RegistryPathForRead returns the path the stack registry should be loaded
// from. Callers should normally invoke MigrateRegistry() first; this helper
// additionally falls back to legacyRegistryPath() if stacks.yaml is absent but
// envs.yaml exists (defense-in-depth for the deprecation window).
func RegistryPathForRead() (string, error) {
	newPath, err := registryPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(newPath); err == nil {
		return newPath, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	oldPath, err := legacyRegistryPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(oldPath); err == nil {
		fmt.Fprintf(os.Stderr,
			"warning: reading deprecated %s; rename to %s (see #17)\n", oldPath, newPath)
		return oldPath, nil
	}
	return newPath, nil
}
