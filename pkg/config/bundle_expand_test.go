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

// TestLoadLinux_BundleInlineExpansion exercises the linuxctl#57 inline
// capabilities (repos_enable + files): bundle-supplied entries union with
// explicit manifest entries, with explicit winning on path collision.
func TestLoadLinux_BundleInlineExpansion(t *testing.T) {
	old := bundleInlineExpander
	t.Cleanup(func() { bundleInlineExpander = old })
	RegisterBundleInlineExpander(func(name string) ([]string, []FileSpec, error) {
		if name == "ol-bundle" {
			return []string{"ol9_codeready_builder"},
				[]FileSpec{{
					Path:       "/usr/lib64/libpthread_nonshared.a",
					Mode:       "0644",
					Owner:      "root",
					Group:      "root",
					ContentB64: "ITxhcmNoPgo=",
					CreateOnly: true,
				}}, nil
		}
		return nil, nil, nil
	})

	dir := t.TempDir()
	p := filepath.Join(dir, "linux.yaml")
	content := []byte(`kind: Linux
bundle_preset: ol-bundle
repos_enable:
  - explicit_repo
files:
  - path: /etc/explicit
    mode: "0600"
    content_b64: aGVsbG8=
`)
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := LoadLinux(p)
	if err != nil {
		t.Fatal(err)
	}
	// Union: explicit first, bundle entries appended.
	if len(l.ReposEnable) != 2 {
		t.Fatalf("ReposEnable = %v", l.ReposEnable)
	}
	if l.ReposEnable[0] != "explicit_repo" || l.ReposEnable[1] != "ol9_codeready_builder" {
		t.Errorf("ordering wrong: %v", l.ReposEnable)
	}
	if len(l.Files) != 2 {
		t.Fatalf("Files = %v", l.Files)
	}
	// Explicit file appears first in manifest order.
	if l.Files[0].Path != "/etc/explicit" {
		t.Errorf("explicit file should be first, got %q", l.Files[0].Path)
	}
	// Bundle file (libpthread stub) appears second.
	if l.Files[1].Path != "/usr/lib64/libpthread_nonshared.a" || !l.Files[1].CreateOnly {
		t.Errorf("bundle file: %+v", l.Files[1])
	}
}

func TestLoadLinux_BundleInlineExpansion_ExplicitWinsOnPath(t *testing.T) {
	old := bundleInlineExpander
	t.Cleanup(func() { bundleInlineExpander = old })
	RegisterBundleInlineExpander(func(name string) ([]string, []FileSpec, error) {
		return []string{"r1"}, []FileSpec{{Path: "/x", ContentB64: "YnVuZGxl"}}, nil
	})
	dir := t.TempDir()
	p := filepath.Join(dir, "linux.yaml")
	content := []byte(`kind: Linux
bundle_preset: any
repos_enable:
  - r1
files:
  - path: /x
    content_b64: ZXhwbGljaXQ=
`)
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := LoadLinux(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.ReposEnable) != 1 {
		t.Errorf("dedup repos failed: %v", l.ReposEnable)
	}
	if len(l.Files) != 1 || l.Files[0].ContentB64 != "ZXhwbGljaXQ=" {
		t.Errorf("explicit file should win on path collision, got %+v", l.Files)
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
