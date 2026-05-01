package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadLinux_HostsAsSequence — regression: existing []HostSpec form must
// continue to parse cleanly into Linux.Hosts (not Linux.HostsByName).
func TestLoadLinux_HostsAsSequence(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "linux.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`kind: Linux
hosts:
  - selector:
      role: [db]
    spec:
      packages:
        install: [chrony]
`), 0o644))
	l, err := LoadLinux(p)
	require.NoError(t, err)
	require.NotNil(t, l)
	assert.Len(t, l.Hosts, 1)
	assert.Equal(t, []string{"db"}, l.Hosts[0].Selector.Role)
	assert.Empty(t, l.HostsByName, "map field must be empty when sequence form is used")
}

// TestLoadLinux_HostsAsMap — proxctl-style: `hosts: <name>: {...}`. New form;
// canonical for stacks authored against the proxctl env.yaml convention
// (matches spec.hypervisor.nodes shape).
func TestLoadLinux_HostsAsMap(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "linux.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`kind: Linux
bundle_preset: dbx-host-ubuntu
hosts:
  dbx01:
    packages:
      install: [docker-ce, chrony, qemu-guest-agent]
    services:
      - name: docker
        enabled: true
        state: running
    sysctl:
      - key: vm.max_map_count
        value: "262144"
`), 0o644))
	l, err := LoadLinux(p)
	require.NoError(t, err)
	require.NotNil(t, l)
	assert.Empty(t, l.Hosts, "sequence field must be empty when map form is used")
	require.Contains(t, l.HostsByName, "dbx01")

	dbx01 := l.HostsByName["dbx01"]
	require.NotNil(t, dbx01.Packages)
	assert.Contains(t, dbx01.Packages.Install, "docker-ce")
	assert.Len(t, dbx01.Services, 1)
	assert.Equal(t, "docker", dbx01.Services[0].Name)
	require.Len(t, dbx01.Sysctl, 1)
	assert.Equal(t, "vm.max_map_count", dbx01.Sysctl[0].Key)
}

// TestLoadLinux_HostsAsMap_MultipleHosts — map form with multiple hostnames
// (typical 2-node cluster manifest).
func TestLoadLinux_HostsAsMap_MultipleHosts(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "linux.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`kind: Linux
hosts:
  ext3adm1:
    packages: { install: [chrony] }
  ext4adm1:
    packages: { install: [chrony, openssh-server] }
`), 0o644))
	l, err := LoadLinux(p)
	require.NoError(t, err)
	require.Len(t, l.HostsByName, 2)
	got := []string{}
	for k := range l.HostsByName {
		got = append(got, k)
	}
	assert.ElementsMatch(t, []string{"ext3adm1", "ext4adm1"}, got)
	assert.Len(t, l.HostsByName["ext4adm1"].Packages.Install, 2)
}

// TestEffectiveSpec_HostInMap — when --host matches a HostsByName key, the
// effective spec is the host's spec layered over top-level Linux fields.
func TestEffectiveSpec_HostInMap(t *testing.T) {
	l := &Linux{
		BundlePreset: "dbx-host-ubuntu", // top-level
		HostsByName: map[string]Spec{
			"dbx01": {
				Packages: &Packages{Install: []string{"docker-ce"}},
			},
		},
	}
	eff := l.EffectiveSpec("dbx01")
	require.NotNil(t, eff)
	require.NotNil(t, eff.Packages)
	assert.Contains(t, eff.Packages.Install, "docker-ce")
	assert.Equal(t, "dbx-host-ubuntu", eff.BundlePreset, "top-level fields propagate when not overridden")
}

// TestEffectiveSpec_HostNotInMap — when --host doesn't match any key, fall
// back to top-level Linux as the effective spec.
func TestEffectiveSpec_HostNotInMap(t *testing.T) {
	l := &Linux{
		Packages: &Packages{Install: []string{"chrony"}},
	}
	eff := l.EffectiveSpec("anyhost")
	require.NotNil(t, eff)
	require.NotNil(t, eff.Packages)
	assert.Equal(t, []string{"chrony"}, eff.Packages.Install)
}

// TestEffectiveSpec_TopLevelMergeOverlay — host-specific fields override
// top-level when both are set. Top-level fields not set in host carry through.
func TestEffectiveSpec_TopLevelMergeOverlay(t *testing.T) {
	l := &Linux{
		BundlePreset: "shared-bundle",                          // top-level
		Packages:     &Packages{Install: []string{"top-only"}}, // top-level
		HostsByName: map[string]Spec{
			"web01": {
				Packages: &Packages{Install: []string{"host-only"}}, // override
			},
		},
	}
	eff := l.EffectiveSpec("web01")
	require.NotNil(t, eff)
	assert.Equal(t, "shared-bundle", eff.BundlePreset, "top-level scalars carry through when host doesn't override")
	require.NotNil(t, eff.Packages)
	assert.Equal(t, []string{"host-only"}, eff.Packages.Install, "host overrides top-level slice (no merge)")
}

// TestEffectiveSpec_NilLinux — defensive: nil receiver returns nil.
func TestEffectiveSpec_NilLinux(t *testing.T) {
	var l *Linux
	assert.Nil(t, l.EffectiveSpec("any"))
}

// TestLoadLinux_HostsRejectsUnknownNodeKind — robustness: a `hosts:` value
// that's neither sequence nor mapping should fail with a clear error.
func TestLoadLinux_HostsRejectsUnknownNodeKind(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "linux.yaml")
	require.NoError(t, os.WriteFile(p, []byte(`kind: Linux
hosts: "this is a string"
`), 0o644))
	_, err := LoadLinux(p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hosts")
}
