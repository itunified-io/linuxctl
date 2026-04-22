package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_ValidateEmpty(t *testing.T) {
	require.NoError(t, Validate(&Linux{}))
	require.NoError(t, Validate(nil))
}

func TestConfig_LoadEmpty(t *testing.T) {
	l, err := LoadLinux("")
	require.NoError(t, err)
	require.NotNil(t, l)
}

func TestConfig_LoadLinuxFixture(t *testing.T) {
	l, err := LoadLinux("testdata/linux.yaml")
	require.NoError(t, err)
	require.Equal(t, "Linux", l.Kind)
	require.Len(t, l.Directories, 2)
	require.NotNil(t, l.UsersGroups)
	require.Len(t, l.UsersGroups.Users, 1)
	require.Equal(t, "oracle", l.UsersGroups.Users[0].Name)
	require.NoError(t, Validate(l))
}

func TestConfig_LoadEnvFixture(t *testing.T) {
	env, err := LoadEnv("testdata/env.yaml", nil)
	require.NoError(t, err)
	require.Equal(t, "db01-uat", env.Metadata.Name)
	require.NotNil(t, env.Spec.Linux.Value)
	require.Len(t, env.Spec.Linux.Value.Directories, 2)
	require.Equal(t, "proxmox", env.Spec.Hypervisor["kind"])
}

func TestConfig_ValidateDuplicateDirs(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		Directories: []Directory{
			{Path: "/a"},
			{Path: "/a"},
		},
	}
	err := Validate(l)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate directory")
}

func TestConfig_ValidateDuplicateMounts(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		Mounts: []Mount{
			{Type: "nfs", MountPoint: "/mnt/x", Source: "nas:/x"},
			{Type: "nfs", MountPoint: "/mnt/x", Source: "nas:/y"},
		},
	}
	require.ErrorContains(t, Validate(l), "duplicate mount_point")
}

func TestConfig_ValidateUserRefsUnknownGroup(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		UsersGroups: &UsersGroups{
			Groups: []Group{{Name: "oinstall"}},
			Users: []User{
				{Name: "oracle", GID: "oinstall", Groups: []string{"dba"}},
			},
		},
	}
	require.ErrorContains(t, Validate(l), "unknown group \"dba\"")
}

func TestConfig_ValidateDuplicateGroup(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		UsersGroups: &UsersGroups{
			Groups: []Group{{Name: "g1"}, {Name: "g1"}},
		},
	}
	require.ErrorContains(t, Validate(l), "duplicate group")
}

func TestConfig_ValidateBadFSFails(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		DiskLayout: &DiskLayout{
			Additional: []AdditionalDisk{{
				Device: "/dev/sdb",
				VGName: "vg_data",
				LogicalVolumes: []LogicalVolume{
					{Name: "lv1", MountPoint: "/u01", Size: "10G", FS: "reiserfs"},
				},
			}},
		},
	}
	require.Error(t, Validate(l))
}

func TestResolver_EnvAndFile(t *testing.T) {
	t.Setenv("LINUXCTL_TEST_VAL", "hello")
	r := NewResolver()
	got, err := r.Resolve("x=${env:LINUXCTL_TEST_VAL}")
	require.NoError(t, err)
	require.Equal(t, "x=hello", got)

	dir := t.TempDir()
	f := filepath.Join(dir, "s.txt")
	require.NoError(t, os.WriteFile(f, []byte("secret\n"), 0o600))
	got, err = r.Resolve("p=${file:" + f + "}")
	require.NoError(t, err)
	require.Equal(t, "p=secret", got)
}

func TestResolver_UnsetEnvFails(t *testing.T) {
	r := NewResolver()
	_, err := r.Resolve("${env:LINUXCTL_MISSING_ZZ}")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not set")
}

func TestResolver_UnknownSchemeFails(t *testing.T) {
	r := NewResolver()
	_, err := r.Resolve("${wat:foo}")
	require.Error(t, err)
}

func TestResolver_Passthrough(t *testing.T) {
	r := NewResolver()
	got, err := r.Resolve("no placeholders")
	require.NoError(t, err)
	require.Equal(t, "no placeholders", got)
}

func TestResolver_VaultNotConfigured(t *testing.T) {
	r := NewResolver()
	_, err := r.Resolve("${vault:kv/foo}")
	assert.Error(t, err)
}

type fakeVault struct{ m map[string]string }

func (f *fakeVault) Read(p string) (string, error) { return f.m[p], nil }

func TestResolver_Vault(t *testing.T) {
	r := NewResolver()
	r.Vault = &fakeVault{m: map[string]string{"kv/pw": "topsecret"}}
	got, err := r.Resolve("${vault:kv/pw}")
	require.NoError(t, err)
	require.Equal(t, "topsecret", got)
}

func TestConfig_ResolveLinuxSecrets(t *testing.T) {
	t.Setenv("LX_PW", "supersecret")
	l := &Linux{
		Kind: "Linux",
		UsersGroups: &UsersGroups{
			Users: []User{{Name: "oracle", Password: "${env:LX_PW}"}},
		},
	}
	r := NewResolver()
	require.NoError(t, resolveLinuxSecrets(l, r))
	require.Equal(t, "supersecret", l.UsersGroups.Users[0].Password)
}

func TestConfig_LoadEnvWithExtends(t *testing.T) {
	env, err := LoadEnv("testdata/child-env.yaml", nil)
	require.NoError(t, err)
	// Child metadata should win.
	require.Equal(t, "child", env.Metadata.Name)
	// Tags should be merged.
	require.Equal(t, "base", env.Metadata.Tags["role"])
	require.Equal(t, "uat", env.Metadata.Tags["env"])
	// Hypervisor from base, networks from child.
	require.Equal(t, "proxmox", env.Spec.Hypervisor["kind"])
	require.Contains(t, env.Spec.Networks, "mgmt")
	require.NotNil(t, env.Spec.Linux.Value)
}

func TestConfig_ResolveMountCreds(t *testing.T) {
	t.Setenv("NAS_CREDS", "/etc/cifs.creds")
	l := &Linux{
		Kind: "Linux",
		Mounts: []Mount{
			{Type: "cifs", Server: "nas", Share: "x", MountPoint: "/mnt/x", CredentialsVault: "${env:NAS_CREDS}"},
		},
	}
	r := NewResolver()
	require.NoError(t, resolveLinuxSecrets(l, r))
	require.Equal(t, "/etc/cifs.creds", l.Mounts[0].CredentialsVault)
}

func TestConfig_ResolveAuthorizedKeys(t *testing.T) {
	t.Setenv("MY_KEY", "ssh-ed25519 AAA")
	l := &Linux{
		Kind: "Linux",
		SSHConfig: &SSHConfig{
			AuthorizedKeys: map[string][]string{
				"root": {"${env:MY_KEY}"},
			},
		},
	}
	r := NewResolver()
	require.NoError(t, resolveLinuxSecrets(l, r))
	require.Equal(t, "ssh-ed25519 AAA", l.SSHConfig.AuthorizedKeys["root"][0])
}

func TestResolver_FileRelative(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "s.txt"), []byte("v"), 0o600))
	r := NewResolver()
	r.BaseDir = dir
	got, err := r.Resolve("${file:s.txt}")
	require.NoError(t, err)
	require.Equal(t, "v", got)
}

func TestResolver_Ref(t *testing.T) {
	r := NewResolver()
	r.Refs["x"] = "abc"
	got, err := r.Resolve("${ref:x}")
	require.NoError(t, err)
	require.Equal(t, "abc", got)

	_, err = r.Resolve("${ref:missing}")
	require.Error(t, err)
}

type fakeGen struct{}

func (fakeGen) Generate(spec string) (string, error) { return "gen:" + spec, nil }

func TestResolver_Gen(t *testing.T) {
	r := NewResolver()
	r.Gen = fakeGen{}
	got, err := r.Resolve("${gen:password:16}")
	require.NoError(t, err)
	require.Equal(t, "gen:password:16", got)
}

func TestConfig_LoadEnvMissing(t *testing.T) {
	_, err := LoadEnv("testdata/does-not-exist.yaml", nil)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "read") || os.IsNotExist(err))
}
