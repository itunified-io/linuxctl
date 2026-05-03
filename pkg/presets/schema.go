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

// Bundle is a meta-preset that composes one preset per category. In
// addition to per-category preset references, a bundle may carry inline
// capabilities that do not fit the named-preset model (linuxctl#57):
//
//   - ReposEnable: dnf repository IDs to enable on the host (e.g.
//     ol9_codeready_builder for OL9 Oracle 19c builds).
//   - Files: literal file payloads to materialise (e.g. the
//     /usr/lib64/libpthread_nonshared.a stub used as a glibc-on-OL9
//     workaround for runInstaller's genclntsh link step).
//
// Both lists are merge-targets on config.Linux: explicit entries in the
// stack manifest are unioned with bundle-supplied entries; conflicts on
// the natural key (repo ID / file path) are resolved in favour of the
// explicit (manifest) entry.
type Bundle struct {
	DirectoriesPreset string     `yaml:"directories_preset,omitempty"`
	UsersGroupsPreset string     `yaml:"users_groups_preset,omitempty"`
	PackagesPreset    string     `yaml:"packages_preset,omitempty"`
	SysctlPreset      string     `yaml:"sysctl_preset,omitempty"`
	LimitsPreset      string     `yaml:"limits_preset,omitempty"`
	ReposEnable       []string   `yaml:"repos_enable,omitempty"`
	Files             []FileSpec `yaml:"files,omitempty"`
}

// FileSpec describes a single file payload owned by the file manager.
// Content is base64-encoded so binary payloads (empty ar archives, ELF
// stubs) survive YAML round-trips intact.
type FileSpec struct {
	Path       string `yaml:"path"`
	Mode       string `yaml:"mode,omitempty"`
	Owner      string `yaml:"owner,omitempty"`
	Group      string `yaml:"group,omitempty"`
	ContentB64 string `yaml:"content_b64"`
	// CreateOnly skips overwrite when the target already exists. Used for
	// stub files that must not clobber a real implementation if one was
	// installed by a later RPM.
	CreateOnly bool `yaml:"create_only,omitempty"`
}

// TierFunc is a callback supplied by the caller (usually wired to the license
// gate) that reports the user's active tier. Passing nil defaults to Community.
type TierFunc func() Tier
