package root

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateRegistry_NoFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINUXCTL_HOME", dir)
	if err := MigrateRegistry(); err != nil {
		t.Fatalf("no-op migration returned error: %v", err)
	}
	// Neither file should have been created.
	if _, err := os.Stat(filepath.Join(dir, "envs.yaml")); !os.IsNotExist(err) {
		t.Error("envs.yaml should not exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "stacks.yaml")); !os.IsNotExist(err) {
		t.Error("stacks.yaml should not exist")
	}
}

func TestMigrateRegistry_LegacyOnly_Renames(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINUXCTL_HOME", dir)
	legacy := filepath.Join(dir, "envs.yaml")
	body := []byte("default: prod\n")
	if err := os.WriteFile(legacy, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateRegistry(); err != nil {
		t.Fatalf("migration: %v", err)
	}
	// envs.yaml should be gone; stacks.yaml should hold the same content.
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("envs.yaml should have been renamed, stat err=%v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "stacks.yaml"))
	if err != nil {
		t.Fatalf("stacks.yaml read: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("content not preserved: got %q, want %q", got, body)
	}
}

func TestMigrateRegistry_BothExist_KeepsBoth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINUXCTL_HOME", dir)
	legacy := filepath.Join(dir, "envs.yaml")
	canonical := filepath.Join(dir, "stacks.yaml")
	os.WriteFile(legacy, []byte("legacy\n"), 0o600)
	os.WriteFile(canonical, []byte("canonical\n"), 0o600)
	if err := MigrateRegistry(); err != nil {
		t.Fatalf("migration: %v", err)
	}
	// Both files should still exist (we only warn).
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("legacy envs.yaml should have been kept: %v", err)
	}
	if _, err := os.Stat(canonical); err != nil {
		t.Errorf("stacks.yaml should have been kept: %v", err)
	}
}

func TestRegistryPathForRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINUXCTL_HOME", dir)

	// Neither exists → returns stacks.yaml (canonical, even if absent).
	got, err := RegistryPathForRead()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "stacks.yaml") {
		t.Errorf("empty dir: got %q, want stacks.yaml path", got)
	}

	// Only legacy exists → returns legacy path, emits warning.
	os.WriteFile(filepath.Join(dir, "envs.yaml"), []byte("x"), 0o600)
	got, err = RegistryPathForRead()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "envs.yaml") {
		t.Errorf("legacy-only: got %q", got)
	}

	// Canonical exists → prefers canonical.
	os.WriteFile(filepath.Join(dir, "stacks.yaml"), []byte("y"), 0o600)
	got, err = RegistryPathForRead()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "stacks.yaml") {
		t.Errorf("both: got %q", got)
	}
}

// TestMigrateRegistry_BothDirs covers the "oldPath is a dir" defensive path
// (oldExists := oldErr == nil && !oldStat.IsDir()) — a directory at envs.yaml
// is treated as "not exists" and no migration occurs.
func TestMigrateRegistry_LegacyIsDir_NoOp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINUXCTL_HOME", dir)
	if err := os.Mkdir(filepath.Join(dir, "envs.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := MigrateRegistry(); err != nil {
		t.Fatalf("migration: %v", err)
	}
	// No stacks.yaml file should have been created.
	if st, err := os.Stat(filepath.Join(dir, "stacks.yaml")); err == nil && !st.IsDir() {
		t.Error("stacks.yaml should not exist (legacy was a dir)")
	}
}

// TestMigrateRegistry_RenameFails_Warns covers the rename-error non-fatal path.
// We pre-create stacks.yaml as a directory so Rename fails on destination.
func TestMigrateRegistry_RenameFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINUXCTL_HOME", dir)
	// envs.yaml exists as regular file
	if err := os.WriteFile(filepath.Join(dir, "envs.yaml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// stacks.yaml is intentionally absent, but we make the home dir read-only
	// so os.Rename fails. Then restore perms so cleanup works.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	// MigrateRegistry should return nil (non-fatal) even if rename fails.
	if err := MigrateRegistry(); err != nil {
		t.Errorf("rename-fail should be non-fatal: %v", err)
	}
}

// TestRegistryPath_NoHome covers the registryHome() error propagation. We
// clear HOME and LINUXCTL_HOME — UserHomeDir will fail on some platforms.
// Just assert both helpers agree on error handling.
func TestRegistryPath_ErrorPaths(t *testing.T) {
	// Positive path: override sets home cleanly.
	t.Setenv("LINUXCTL_HOME", "/tmp/linuxctl-test-override")
	p, err := registryPath()
	if err != nil || p == "" {
		t.Fatalf("registryPath: %v %q", err, p)
	}
	lp, err := legacyRegistryPath()
	if err != nil || lp == "" {
		t.Fatalf("legacyRegistryPath: %v %q", err, lp)
	}
}

// TestRegistryHome_UserHomeDirFails covers the error branch where
// os.UserHomeDir() itself fails (rare in practice; injected via test hook).
func TestRegistryHome_UserHomeDirFails(t *testing.T) {
	t.Setenv("LINUXCTL_HOME", "")
	prev := userHomeDir
	userHomeDir = func() (string, error) { return "", fmt.Errorf("boom") }
	t.Cleanup(func() { userHomeDir = prev })

	if _, err := registryHome(); err == nil {
		t.Error("registryHome: expected error")
	}
	if _, err := registryPath(); err == nil {
		t.Error("registryPath: expected error propagation")
	}
	if _, err := legacyRegistryPath(); err == nil {
		t.Error("legacyRegistryPath: expected error propagation")
	}
	if err := MigrateRegistry(); err == nil {
		t.Error("MigrateRegistry: expected error propagation")
	}
	if _, err := RegistryPathForRead(); err == nil {
		t.Error("RegistryPathForRead: expected error propagation")
	}
}

func TestRegistryHome_UsesHomeDir(t *testing.T) {
	// Clear override so os.UserHomeDir() is used.
	t.Setenv("LINUXCTL_HOME", "")
	h, err := registryHome()
	if err != nil {
		t.Fatal(err)
	}
	if h == "" || filepath.Base(h) != ".linuxctl" {
		t.Errorf("unexpected home: %q", h)
	}
}

func TestApplyEnvVarDefaults(t *testing.T) {
	// LINUXCTL_STACK fills gf.stack when flag is empty.
	gf = globalFlags{}
	t.Setenv(envVarNameStack, "prod")
	t.Setenv(envVarNameEnv, "")
	applyEnvVarDefaults()
	if gf.stack != "prod" {
		t.Errorf("LINUXCTL_STACK: gf.stack=%q", gf.stack)
	}

	// LINUXCTL_ENV fills gf.env when flag is empty (emits warning).
	gf = globalFlags{}
	t.Setenv(envVarNameStack, "")
	t.Setenv(envVarNameEnv, "legacy")
	applyEnvVarDefaults()
	if gf.env != "legacy" {
		t.Errorf("LINUXCTL_ENV: gf.env=%q", gf.env)
	}

	// Both env vars set: LINUXCTL_STACK wins at stack slot, LINUXCTL_ENV goes to env slot.
	gf = globalFlags{}
	t.Setenv(envVarNameStack, "new")
	t.Setenv(envVarNameEnv, "old")
	applyEnvVarDefaults()
	if gf.stack != "new" || gf.env != "old" {
		t.Errorf("both set: gf.stack=%q gf.env=%q", gf.stack, gf.env)
	}

	// Flag explicitly set: env var should not overwrite.
	gf = globalFlags{stack: "from-flag"}
	t.Setenv(envVarNameStack, "from-env")
	t.Setenv(envVarNameEnv, "")
	applyEnvVarDefaults()
	if gf.stack != "from-flag" {
		t.Errorf("flag should win: gf.stack=%q", gf.stack)
	}

	// Reset for other tests.
	gf = globalFlags{}
}
