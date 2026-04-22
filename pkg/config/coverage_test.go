package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---- loader edge cases -----------------------------------------------------

func TestLoadLinux_FileMissing(t *testing.T) {
	_, err := LoadLinux("/nonexistent/nope.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "read")
}

func TestLoadLinux_BadYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(p, []byte(":\n\tnot yaml"), 0o600))
	_, err := LoadLinux(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse")
}

func TestLoadEnv_EmptyPath(t *testing.T) {
	_, err := LoadEnv("", nil)
	require.Error(t, err)
}

func TestLoadEnv_BadYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	require.NoError(t, os.WriteFile(p, []byte("\t\tnope"), 0o600))
	_, err := LoadEnv(p, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse")
}

func TestLoadEnv_BadExtends(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`version: "1"
kind: Env
metadata:
  name: child
extends: ./missing.yaml
spec:
  linux:
    kind: Linux
`), 0o600))
	_, err := LoadEnv(p, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "extends")
}

func TestLoadEnv_RefRelativeMissing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`version: "1"
kind: Env
metadata:
  name: x
spec:
  linux:
    $ref: ./missing-linux.yaml
`), 0o600))
	_, err := LoadEnv(p, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "linux")
}

func TestLoadEnv_RefAbsolute(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "linux.yaml")
	require.NoError(t, os.WriteFile(lp, []byte("kind: Linux\n"), 0o600))
	env := filepath.Join(dir, "env.yaml")
	require.NoError(t, os.WriteFile(env, []byte(`version: "1"
kind: Env
metadata:
  name: x
spec:
  linux:
    $ref: `+lp+`
`), 0o600))
	e, err := LoadEnv(env, nil)
	require.NoError(t, err)
	require.NotNil(t, e.Spec.Linux.Value)
	require.Equal(t, "Linux", e.Spec.Linux.Value.Kind)
}

func TestLoadEnv_InlineLinux(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`version: "1"
kind: Env
metadata:
  name: inline-env
spec:
  linux:
    kind: Linux
    directories:
      - path: /opt/x
        mode: "0755"
`), 0o600))
	e, err := LoadEnv(p, nil)
	require.NoError(t, err)
	require.NotNil(t, e.Spec.Linux.Inline)
	require.NotNil(t, e.Spec.Linux.Value)
	require.Len(t, e.Spec.Linux.Value.Directories, 1)
}

func TestLoadEnv_InlineBadLinux(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	// Use a malformed inline linux (scalar where object expected).
	require.NoError(t, os.WriteFile(p, []byte(`version: "1"
kind: Env
metadata:
  name: bad
spec:
  linux: "stringvalue"
`), 0o600))
	_, err := LoadEnv(p, nil)
	require.Error(t, err)
}

func TestLoadEnv_ResolverPropagation(t *testing.T) {
	t.Setenv("PW_XZY", "hunter2")
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`version: "1"
kind: Env
metadata:
  name: res
spec:
  linux:
    kind: Linux
    users_groups:
      users:
        - name: alice
          password: "${env:PW_XZY}"
`), 0o600))
	r := NewResolver()
	e, err := LoadEnv(p, r)
	require.NoError(t, err)
	require.Equal(t, "hunter2", e.Spec.Linux.Value.UsersGroups.Users[0].Password)
}

func TestLoadEnv_ResolverErrorSurfaces(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`version: "1"
kind: Env
metadata:
  name: bad
spec:
  linux:
    kind: Linux
    users_groups:
      users:
        - name: alice
          password: "${env:NEVER_EVER_SET_PW}"
`), 0o600))
	r := NewResolver()
	_, err := LoadEnv(p, r)
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve password")
}

func TestLoadEnv_ValidationFails(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.yaml")
	// version != "1" triggers validator failure.
	require.NoError(t, os.WriteFile(p, []byte(`version: "2"
kind: Env
metadata:
  name: x
spec:
  linux:
    kind: Linux
`), 0o600))
	_, err := LoadEnv(p, nil)
	require.Error(t, err)
}

// mergeEnv: verify every branch — tags merge, hypervisor merge, databases
// append, hooks override.
func TestLoadEnv_MergeFullChain(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "linux.yaml")
	require.NoError(t, os.WriteFile(lp, []byte("kind: Linux\n"), 0o600))
	base := filepath.Join(dir, "base.yaml")
	require.NoError(t, os.WriteFile(base, []byte(`version: "1"
kind: Env
metadata:
  name: base
  domain: base.example.com
  description: base-desc
  tags:
    role: base
spec:
  linux:
    $ref: ./linux.yaml
  hypervisor:
    kind: proxmox
  networks:
    mgmt:
      cidr: 10.0.0.0/24
  storage_classes:
    ssd: {tier: gold}
  cluster:
    roles: []
  databases:
    - kind: postgres
hooks:
  on_apply_start:
    - type: notify
`), 0o600))
	child := filepath.Join(dir, "child.yaml")
	require.NoError(t, os.WriteFile(child, []byte(`version: "1"
kind: Env
metadata:
  name: child
  domain: child.example.com
  description: child-desc
  tags:
    env: uat
extends: ./base.yaml
spec:
  linux:
    $ref: ./linux.yaml
  hypervisor:
    pool: pool-a
  networks:
    svc:
      cidr: 10.1.0.0/24
  storage_classes:
    nvme: {tier: platinum}
  cluster:
    extra: true
  databases:
    - kind: oracle
hooks:
  on_apply_success:
    - type: slack
`), 0o600))
	e, err := LoadEnv(child, nil)
	require.NoError(t, err)
	require.Equal(t, "child", e.Metadata.Name)
	require.Equal(t, "child.example.com", e.Metadata.Domain)
	require.Equal(t, "child-desc", e.Metadata.Description)
	require.Equal(t, "base", e.Metadata.Tags["role"])
	require.Equal(t, "uat", e.Metadata.Tags["env"])
	require.Equal(t, "proxmox", e.Spec.Hypervisor["kind"])
	require.Equal(t, "pool-a", e.Spec.Hypervisor["pool"])
	require.Contains(t, e.Spec.Networks, "mgmt")
	require.Contains(t, e.Spec.Networks, "svc")
	require.Contains(t, e.Spec.StorageClasses, "ssd")
	require.Contains(t, e.Spec.StorageClasses, "nvme")
	require.Contains(t, e.Spec.Cluster, "roles")
	require.Contains(t, e.Spec.Cluster, "extra")
	require.Len(t, e.Spec.Databases, 2)
	// Child hooks override base hooks (non-nil replaces).
	require.NotNil(t, e.Hooks)
	require.Len(t, e.Hooks.OnApplySuccess, 1)
}

// Base has no tags map; child brings one — ensures mergeEnv allocates.
func TestLoadEnv_MergeAllocatesTags(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "linux.yaml")
	require.NoError(t, os.WriteFile(lp, []byte("kind: Linux\n"), 0o600))
	base := filepath.Join(dir, "base.yaml")
	require.NoError(t, os.WriteFile(base, []byte(`version: "1"
kind: Env
metadata:
  name: base
spec:
  linux:
    $ref: ./linux.yaml
`), 0o600))
	child := filepath.Join(dir, "c.yaml")
	require.NoError(t, os.WriteFile(child, []byte(`version: "1"
kind: Env
metadata:
  name: c
  tags:
    env: prod
extends: ./base.yaml
spec:
  linux:
    $ref: ./linux.yaml
`), 0o600))
	e, err := LoadEnv(child, nil)
	require.NoError(t, err)
	require.Equal(t, "prod", e.Metadata.Tags["env"])
}

// ---- resolver edge cases ---------------------------------------------------

type badVault struct{ err error }

func (b *badVault) Read(string) (string, error) { return "", b.err }

type badGen struct{ err error }

func (b *badGen) Generate(string) (string, error) { return "", b.err }

func TestResolver_VaultError(t *testing.T) {
	r := NewResolver()
	r.Vault = &badVault{err: errNotAvailable{}}
	_, err := r.Resolve("${vault:kv/x}")
	require.Error(t, err)
}

type errNotAvailable struct{}

func (errNotAvailable) Error() string { return "backend offline" }

func TestResolver_GenError(t *testing.T) {
	r := NewResolver()
	r.Gen = &badGen{err: errNotAvailable{}}
	_, err := r.Resolve("${gen:password}")
	require.Error(t, err)
}

func TestResolver_GenNotConfigured(t *testing.T) {
	r := NewResolver()
	_, err := r.Resolve("${gen:pw}")
	require.Error(t, err)
	require.Contains(t, err.Error(), "gen resolver")
}

func TestResolver_FileMissing(t *testing.T) {
	r := NewResolver()
	_, err := r.Resolve("${file:/nope/nope/nope}")
	require.Error(t, err)
}

func TestResolver_FileAbsoluteIgnoresBase(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.txt")
	require.NoError(t, os.WriteFile(p, []byte("z"), 0o600))
	r := NewResolver()
	r.BaseDir = "/some/other/dir"
	got, err := r.Resolve("${file:" + p + "}")
	require.NoError(t, err)
	require.Equal(t, "z", got)
}

func TestResolver_MultiplePlaceholdersInOneString(t *testing.T) {
	t.Setenv("A", "a")
	t.Setenv("B", "b")
	r := NewResolver()
	got, err := r.Resolve("${env:A}-${env:B}")
	require.NoError(t, err)
	require.Equal(t, "a-b", got)
}

// resolveLinuxSecrets nil-safety + error paths
func TestResolveLinuxSecrets_Nil(t *testing.T) {
	require.NoError(t, resolveLinuxSecrets(nil, NewResolver()))
}

func TestResolveLinuxSecrets_UserSSHKeyError(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		UsersGroups: &UsersGroups{
			Users: []User{{Name: "a", SSHKeys: []string{"${env:DOES_NOT_EXIST_XX}"}}},
		},
	}
	err := resolveLinuxSecrets(l, NewResolver())
	require.Error(t, err)
	require.Contains(t, err.Error(), "ssh_keys")
}

func TestResolveLinuxSecrets_MountCredsError(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		Mounts: []Mount{
			{Type: "cifs", Server: "nas", Share: "x", MountPoint: "/mnt/x", CredentialsVault: "${env:NEVERSET_XX}"},
		},
	}
	err := resolveLinuxSecrets(l, NewResolver())
	require.Error(t, err)
	require.Contains(t, err.Error(), "credentials_vault")
}

func TestResolveLinuxSecrets_AuthorizedKeyError(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		SSHConfig: &SSHConfig{
			AuthorizedKeys: map[string][]string{"root": {"${env:NEVERSET_YY}"}},
		},
	}
	err := resolveLinuxSecrets(l, NewResolver())
	require.Error(t, err)
	require.Contains(t, err.Error(), "authorized_keys")
}

// ---- validator cross-field checks -----------------------------------------

func TestValidate_DuplicateUser(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		UsersGroups: &UsersGroups{
			Groups: []Group{{Name: "dba"}},
			Users:  []User{{Name: "alice"}, {Name: "alice"}},
		},
	}
	err := Validate(l)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate user")
}

func TestValidate_UserUnknownGID(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		UsersGroups: &UsersGroups{
			Groups: []Group{{Name: "dba"}},
			Users:  []User{{Name: "alice", GID: "nobody"}},
		},
	}
	err := Validate(l)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown group")
}

func TestValidate_DuplicateHostsEntries(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		HostsEntries: []HostEntry{
			{IP: "10.0.0.1", Names: []string{"a"}},
			{IP: "10.0.0.1", Names: []string{"b"}},
		},
	}
	err := Validate(l)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate hosts")
}

func TestValidate_DuplicateServices(t *testing.T) {
	l := &Linux{
		Kind: "Linux",
		Services: []ServiceState{
			{Name: "chronyd"},
			{Name: "chronyd"},
		},
	}
	err := Validate(l)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate service")
}

// ---- ValidateEnv -----------------------------------------------------------

func TestValidateEnv_Nil(t *testing.T) {
	require.NoError(t, ValidateEnv(nil))
}

func TestValidateEnv_WithLinuxValue(t *testing.T) {
	e := &Env{
		Version: "1", Kind: "Env",
		Metadata: Metadata{Name: "ok"},
		Spec: EnvSpec{
			Linux: Ref[Linux]{Value: &Linux{Kind: "Linux"}},
		},
	}
	require.NoError(t, ValidateEnv(e))
}

func TestValidateEnv_InnerLinuxInvalid(t *testing.T) {
	e := &Env{
		Version: "1", Kind: "Env",
		Metadata: Metadata{Name: "ok"},
		Spec: EnvSpec{
			Linux: Ref[Linux]{Value: &Linux{
				Kind: "Linux",
				Directories: []Directory{
					{Path: "/a"}, {Path: "/a"},
				},
			}},
		},
	}
	err := ValidateEnv(e)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate directory")
}

func TestValidateEnv_TopLevelBad(t *testing.T) {
	e := &Env{
		Version: "2", Kind: "Env", // wrong version
		Metadata: Metadata{Name: "ok"},
		Spec:     EnvSpec{Linux: Ref[Linux]{Value: &Linux{Kind: "Linux"}}},
	}
	require.Error(t, ValidateEnv(e))
}

// ---- Linux type round-trips via YAML --------------------------------------

func TestLinux_RoundTripAllFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "full.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`kind: Linux
apiVersion: v1
disk_layout:
  root:
    device: /dev/sda
    vg_name: rootvg
    logical_volumes:
      - {name: lv_var, mount_point: /var, size: 10G, fs: xfs}
  additional:
    - device: /dev/sdb
      vg_name: datavg
      logical_volumes:
        - {name: lv_data, mount_point: /data, size: 100G, fs: ext4}
users_groups:
  groups: [{name: oinstall, gid: 1100}]
  users:
    - {name: oracle, uid: 1100, gid: oinstall, shell: /bin/bash}
directories:
  - {path: /opt/oracle, owner: oracle, group: oinstall, mode: "0755", recursive: true}
mounts:
  - {type: nfs, source: "nas:/x", mount_point: /mnt/x, options: [rw], persistent: true}
  - {type: tmpfs, mount_point: /mnt/ram}
packages:
  install: [htop]
  remove: [telnet]
  enabled_services: [chronyd]
  disabled_services: [iptables]
sysctl:
  - {key: net.ipv4.ip_forward, value: "1"}
sysctl_preset: oracle-db-19c
limits:
  - {user: oracle, type: soft, item: nofile, value: "65535"}
limits_preset: oracle
firewall:
  enabled: true
  zones:
    public:
      ports:
        - {name: ssh, port: 22, proto: tcp}
        - {port_range: "30000-31000", proto: udp}
      sources: ["10.0.0.0/8"]
      sources_from_network: mgmt
hosts_entries:
  - {ip: 10.0.0.1, names: [db01]}
services:
  - {name: chronyd, enabled: true, state: running}
ssh:
  authorized_keys:
    root: ["ssh-ed25519 AAA"]
  sshd_config:
    PermitRootLogin: "no"
selinux:
  mode: enforcing
  booleans:
    httpd_can_network_connect: true
`), 0o600))
	l, err := LoadLinux(p)
	require.NoError(t, err)
	require.NoError(t, Validate(l))
	require.NotNil(t, l.DiskLayout)
	require.NotNil(t, l.DiskLayout.Root)
	require.Equal(t, "/dev/sda", l.DiskLayout.Root.Device)
	require.Len(t, l.DiskLayout.Additional, 1)
	require.Equal(t, "datavg", l.DiskLayout.Additional[0].VGName)
	require.Len(t, l.Directories, 1)
	require.True(t, l.Directories[0].Recursive)
	require.Len(t, l.Mounts, 2)
	require.Equal(t, "tmpfs", l.Mounts[1].Type)
	require.NotNil(t, l.Packages)
	require.Contains(t, l.Packages.Install, "htop")
	require.Len(t, l.Sysctl, 1)
	require.Equal(t, "oracle-db-19c", l.SysctlPreset)
	require.Len(t, l.Limits, 1)
	require.NotNil(t, l.Firewall)
	require.True(t, l.Firewall.Enabled)
	require.Len(t, l.Firewall.Zones["public"].Ports, 2)
	require.Len(t, l.HostsEntries, 1)
	require.Len(t, l.Services, 1)
	require.NotNil(t, l.SSHConfig)
	require.Contains(t, l.SSHConfig.AuthorizedKeys, "root")
	require.Equal(t, "no", l.SSHConfig.SSHDConfig["PermitRootLogin"])
	require.NotNil(t, l.SELinux)
	require.Equal(t, "enforcing", l.SELinux.Mode)
	require.True(t, l.SELinux.Booleans["httpd_can_network_connect"])
}

func TestLinux_ValidatorRejectsBadSELinux(t *testing.T) {
	l := &Linux{Kind: "Linux", SELinux: &SELinuxConfig{Mode: "bogus"}}
	require.Error(t, Validate(l))
}

func TestLinux_ValidatorRejectsBadLimit(t *testing.T) {
	l := &Linux{Kind: "Linux", Limits: []LimitEntry{{User: "o", Type: "maybe", Item: "nofile", Value: "1"}}}
	require.Error(t, Validate(l))
}

func TestLinux_ValidatorRejectsBadMountType(t *testing.T) {
	l := &Linux{Kind: "Linux", Mounts: []Mount{{Type: "zfs", MountPoint: "/mnt/a"}}}
	require.Error(t, Validate(l))
}

func TestLinux_ValidatorRejectsRelativeMountPoint(t *testing.T) {
	l := &Linux{Kind: "Linux", Mounts: []Mount{{Type: "nfs", Source: "x:/y", MountPoint: "relative"}}}
	require.Error(t, Validate(l))
}

func TestLinux_ValidatorRejectsBadDirMode(t *testing.T) {
	l := &Linux{Kind: "Linux", Directories: []Directory{{Path: "/a", Mode: "0755x"}}}
	require.Error(t, Validate(l))
}

func TestLinux_ValidatorRejectsBadHostIP(t *testing.T) {
	l := &Linux{Kind: "Linux", HostsEntries: []HostEntry{{IP: "not-an-ip", Names: []string{"a"}}}}
	require.Error(t, Validate(l))
}

func TestLinux_ValidatorRejectsEmptyHostNames(t *testing.T) {
	l := &Linux{Kind: "Linux", HostsEntries: []HostEntry{{IP: "10.0.0.1"}}}
	require.Error(t, Validate(l))
}

func TestLinux_FirewallZonesArePlainMap(t *testing.T) {
	// Zones is map[string]FirewallZone without `dive` — struct validator
	// does not descend. This test documents that behaviour; cross-validation
	// of ports would have to be added explicitly in validateCrossFields.
	l := &Linux{Kind: "Linux", Firewall: &Firewall{Zones: map[string]FirewallZone{
		"p": {Ports: []PortRule{{Port: 22, Proto: "tcp"}}},
	}}}
	require.NoError(t, Validate(l))
}

func TestLinux_ValidatorRequiresServiceName(t *testing.T) {
	l := &Linux{Kind: "Linux", Services: []ServiceState{{Name: ""}}}
	require.Error(t, Validate(l))
}

func TestLinux_ValidatorRejectsUIDBelow1000(t *testing.T) {
	l := &Linux{Kind: "Linux", UsersGroups: &UsersGroups{
		Users: []User{{Name: "root2", UID: 500}},
	}}
	require.Error(t, Validate(l))
}

func TestLinux_AdditionalDiskRequiresLVs(t *testing.T) {
	l := &Linux{Kind: "Linux", DiskLayout: &DiskLayout{
		Additional: []AdditionalDisk{{Device: "/dev/sdb", VGName: "vg"}},
	}}
	require.Error(t, Validate(l))
}

func TestLinux_LVRequiresAbsoluteMountPoint(t *testing.T) {
	l := &Linux{Kind: "Linux", DiskLayout: &DiskLayout{
		Additional: []AdditionalDisk{{
			Device: "/dev/sdb", VGName: "vg",
			LogicalVolumes: []LogicalVolume{{Name: "lv", MountPoint: "var", Size: "1G", FS: "xfs"}},
		}},
	}}
	require.Error(t, Validate(l))
}
