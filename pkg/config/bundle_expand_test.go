package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadLinux_BundleExpansion verifies that when pkg/presets is imported
// (which registers the bundle expander), LoadLinux expands bundle_preset
// into per-category *_preset fields, with explicit per-category presets
// always winning.
func TestLoadLinux_BundleExpansion(t *testing.T) {
	// Register a stub expander so this test does not depend on pkg/presets.
	old := bundleExpander
	t.Cleanup(func() { bundleExpander = old })
	RegisterBundleExpander(func(name string) (map[string]string, error) {
		if name == "test-bundle" {
			return map[string]string{
				"directories":  "bundle-dirs",
				"users_groups": "bundle-users",
				"packages":     "bundle-pkgs",
				"sysctl":       "bundle-sysctl",
				"limits":       "bundle-limits",
			}, nil
		}
		return nil, nil
	})

	dir := t.TempDir()
	p := filepath.Join(dir, "linux.yaml")
	content := []byte(`kind: Linux
bundle_preset: test-bundle
sysctl_preset: explicit-sysctl
`)
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := LoadLinux(p)
	if err != nil {
		t.Fatal(err)
	}
	if l.DirectoriesPreset != "bundle-dirs" {
		t.Errorf("DirectoriesPreset: want bundle-dirs, got %q", l.DirectoriesPreset)
	}
	if l.SysctlPreset != "explicit-sysctl" {
		t.Errorf("explicit SysctlPreset should win over bundle, got %q", l.SysctlPreset)
	}
	if l.LimitsPreset != "bundle-limits" {
		t.Errorf("LimitsPreset: want bundle-limits, got %q", l.LimitsPreset)
	}
}

func TestLoadLinux_BundleExpansion_NoHook(t *testing.T) {
	old := bundleExpander
	t.Cleanup(func() { bundleExpander = old })
	bundleExpander = nil

	dir := t.TempDir()
	p := filepath.Join(dir, "linux.yaml")
	if err := os.WriteFile(p, []byte("kind: Linux\nbundle_preset: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := LoadLinux(p)
	if err != nil {
		t.Fatal(err)
	}
	if l.BundlePreset != "x" {
		t.Errorf("bundle preset should remain unexpanded without hook, got %q", l.BundlePreset)
	}
}
