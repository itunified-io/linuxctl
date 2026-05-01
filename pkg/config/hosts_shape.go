package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// linuxYAML is a private alias used solely to defeat the recursive
// UnmarshalYAML lookup; without it, yaml.Node.Decode(&Linux{}) would
// re-enter Linux.UnmarshalYAML and stack-overflow.
type linuxYAML Linux

// UnmarshalYAML implements custom decoding so the `hosts:` key accepts both
// the legacy `[]HostSpec` (selector array) and the proxctl-style
// `map[string]Spec` (hostname-keyed map). See issue #48 + linuxctl#48 for
// rationale.
func (l *Linux) UnmarshalYAML(node *yaml.Node) error {
	// Decode all non-`hosts` fields via the alias (which has no UnmarshalYAML
	// defined). We strip `hosts:` from the node tree first.
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("linux.yaml root must be a mapping (got kind %d)", node.Kind)
	}

	// Walk the mapping, separating `hosts:` from everything else.
	var hostsNode *yaml.Node
	rest := &yaml.Node{Kind: yaml.MappingNode, Tag: node.Tag}
	for i := 0; i < len(node.Content); i += 2 {
		key, val := node.Content[i], node.Content[i+1]
		if key.Value == "hosts" {
			hostsNode = val
			continue
		}
		rest.Content = append(rest.Content, key, val)
	}

	// Decode the rest of the manifest via the alias type (no custom unmarshal).
	var inner linuxYAML
	if err := rest.Decode(&inner); err != nil {
		return fmt.Errorf("decode linux.yaml: %w", err)
	}
	*l = Linux(inner)

	// Now dispatch on hosts: shape.
	if hostsNode == nil {
		return nil
	}
	switch hostsNode.Kind {
	case yaml.SequenceNode:
		var legacy []HostSpec
		if err := hostsNode.Decode(&legacy); err != nil {
			return fmt.Errorf("decode hosts (sequence form): %w", err)
		}
		l.Hosts = legacy
	case yaml.MappingNode:
		byName := map[string]Spec{}
		// Walk pairs manually so we get hostname keys and decode each value
		// as a Spec (which is alias for Linux — but Linux has UnmarshalYAML,
		// so use the alias to avoid recursion).
		for i := 0; i < len(hostsNode.Content); i += 2 {
			k, v := hostsNode.Content[i], hostsNode.Content[i+1]
			if k.Kind != yaml.ScalarNode {
				return fmt.Errorf("hosts map key must be a scalar hostname (got kind %d)", k.Kind)
			}
			var s linuxYAML
			if err := v.Decode(&s); err != nil {
				return fmt.Errorf("decode hosts[%q]: %w", k.Value, err)
			}
			byName[k.Value] = Spec(s)
		}
		l.HostsByName = byName
	default:
		return fmt.Errorf("hosts: must be a sequence (selector form) or mapping (proxctl-style); got kind %d", hostsNode.Kind)
	}
	return nil
}

// EffectiveSpec returns the resolved Spec for a given hostname. When
// `HostsByName` contains an entry for the hostname, the host's spec is
// layered over the top-level Linux fields (host wins on overlap). When
// no host-specific entry exists, the top-level Linux is returned as-is.
//
// On a nil receiver, returns nil. On an empty hostname or empty map,
// returns the top-level Linux unchanged.
func (l *Linux) EffectiveSpec(hostname string) *Spec {
	if l == nil {
		return nil
	}
	host, ok := l.HostsByName[hostname]
	if !ok {
		// No host-specific entry — return top-level Linux as the spec.
		out := Spec(*l)
		return &out
	}
	// Layer host on top of top-level. Start from a copy of the top-level
	// Linux, then overwrite with non-zero host fields.
	merged := Spec(*l)
	overlaySpecOnto(&merged, &host)
	return &merged
}

// overlaySpecOnto applies `src` non-zero fields onto `dst` in place. Slices
// and pointers from `src` REPLACE `dst` values rather than merging — this
// matches operator expectation for explicit per-host overrides (no
// surprise concatenation).
func overlaySpecOnto(dst *Spec, src *Spec) {
	if src.Kind != "" {
		dst.Kind = src.Kind
	}
	if src.APIVersion != "" {
		dst.APIVersion = src.APIVersion
	}
	if src.DiskLayout != nil {
		dst.DiskLayout = src.DiskLayout
	}
	if src.UsersGroups != nil {
		dst.UsersGroups = src.UsersGroups
	}
	if len(src.Directories) > 0 {
		dst.Directories = src.Directories
	}
	if len(src.Mounts) > 0 {
		dst.Mounts = src.Mounts
	}
	if src.Packages != nil {
		dst.Packages = src.Packages
	}
	if len(src.Sysctl) > 0 {
		dst.Sysctl = src.Sysctl
	}
	if src.SysctlPreset != "" {
		dst.SysctlPreset = src.SysctlPreset
	}
	if len(src.Limits) > 0 {
		dst.Limits = src.Limits
	}
	if src.LimitsPreset != "" {
		dst.LimitsPreset = src.LimitsPreset
	}
	if src.DirectoriesPreset != "" {
		dst.DirectoriesPreset = src.DirectoriesPreset
	}
	if src.UsersGroupsPreset != "" {
		dst.UsersGroupsPreset = src.UsersGroupsPreset
	}
	if src.PackagesPreset != "" {
		dst.PackagesPreset = src.PackagesPreset
	}
	if src.BundlePreset != "" {
		dst.BundlePreset = src.BundlePreset
	}
	if src.Firewall != nil {
		dst.Firewall = src.Firewall
	}
	if len(src.HostsEntries) > 0 {
		dst.HostsEntries = src.HostsEntries
	}
	if len(src.Services) > 0 {
		dst.Services = src.Services
	}
	if src.SSHConfig != nil {
		dst.SSHConfig = src.SSHConfig
	}
	if src.SELinux != nil {
		dst.SELinux = src.SELinux
	}
	// HostsByName + Hosts are intentionally NOT carried through — those are
	// manifest-shape concerns, not per-host spec data.
}
