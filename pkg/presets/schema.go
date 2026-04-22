// Package presets provides the embedded conventions library for linuxctl.
//
// Presets are shipped as YAML files under pkg/presets/data/ and embedded into
// the linuxctl binary via go:embed. Each preset declares a named, reusable
// fragment of desired state for one of five categories (directories,
// users_groups, packages, sysctl, limits) plus a "Bundle" meta-preset that
// composes one preset per category.
//
// Merge precedence (lowest → highest): bundle_preset < individual *_preset
// fields < explicit entries in the stack's linux.yaml. See plan 033.
package presets

// Tier mirrors pkg/license.Tier without a circular import.
type Tier string

const (
	// TierCommunity is the default / free tier. All shipped Phase-1 presets
	// are at this tier.
	TierCommunity Tier = "community"
	// TierBusiness unlocks hardened-cis and other enterprise-leaning presets.
	TierBusiness Tier = "business"
	// TierEnterprise is the highest tier; reserved for future presets.
	TierEnterprise Tier = "enterprise"
)

// APIVersion is the current preset schema identifier.
const APIVersion = "linuxctl.itunified.io/preset/v1"

// PresetMeta is the common metadata block on every preset + bundle.
type PresetMeta struct {
	Name     string `yaml:"name"`
	Category string `yaml:"category"`
	Tier     Tier   `yaml:"tier"`
	Source   string `yaml:"source,omitempty"`
	Version  string `yaml:"version,omitempty"`
}

// Preset is a single named fragment of desired state. The Spec field is
// decoded category-specifically by the registry (the raw decoded YAML map
// is retained in RawSpec for rendering / show commands).
type Preset struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   PresetMeta `yaml:"metadata"`
	// RawSpec holds the unparsed spec map — category-specific typed views
	// are decoded on demand by the registry helpers.
	RawSpec map[string]any `yaml:"spec"`
}

// Bundle is a meta-preset that composes one preset per category.
type Bundle struct {
	DirectoriesPreset string `yaml:"directories_preset,omitempty"`
	UsersGroupsPreset string `yaml:"users_groups_preset,omitempty"`
	PackagesPreset    string `yaml:"packages_preset,omitempty"`
	SysctlPreset      string `yaml:"sysctl_preset,omitempty"`
	LimitsPreset      string `yaml:"limits_preset,omitempty"`
}

// TierFunc is a callback supplied by the caller (usually wired to the license
// gate) that reports the user's active tier. Passing nil defaults to Community.
type TierFunc func() Tier
