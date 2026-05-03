package presets

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// tierRank maps a Tier to a numeric precedence for comparison.
func tierRank(t Tier) int {
	switch t {
	case TierCommunity:
		return 1
	case TierBusiness:
		return 2
	case TierEnterprise:
		return 3
	default:
		return 1
	}
}

// resolveTier returns the caller's active tier, defaulting to Community.
func resolveTier(fn TierFunc) Tier {
	if fn == nil {
		return TierCommunity
	}
	t := fn()
	if t == "" {
		return TierCommunity
	}
	return t
}

// Resolve loads a preset by name. Returns an error if unknown, ambiguous
// (same name in multiple categories), or if the caller's tier is below the
// preset's required tier. For ambiguous names, use ResolveCategory.
func Resolve(name string, tierFn TierFunc) (*Preset, error) {
	if name == "" {
		return nil, fmt.Errorf("preset: empty name")
	}
	idx, err := load()
	if err != nil {
		return nil, err
	}
	matches, ok := idx.byName[name]
	if !ok || len(matches) == 0 {
		return nil, fmt.Errorf("preset %q not found", name)
	}
	if len(matches) > 1 {
		cats := make([]string, 0, len(matches))
		for _, m := range matches {
			cats = append(cats, string(m.Metadata.Category))
		}
		return nil, fmt.Errorf("preset %q is ambiguous across categories %v; use ResolveCategory", name, cats)
	}
	p := matches[0]
	caller := resolveTier(tierFn)
	if tierRank(p.Metadata.Tier) > tierRank(caller) {
		return nil, fmt.Errorf("preset %q requires tier %q (active tier: %q)", name, p.Metadata.Tier, caller)
	}
	return p, nil
}

// ResolveCategory loads a preset by (category, name). This is the form the
// managers use, since each manager owns a single category.
func ResolveCategory(category, name string, tierFn TierFunc) (*Preset, error) {
	if name == "" {
		return nil, fmt.Errorf("preset: empty name")
	}
	idx, err := load()
	if err != nil {
		return nil, err
	}
	p, ok := idx.byKey[catKey(category, name)]
	if !ok {
		return nil, fmt.Errorf("preset %s/%s not found", category, name)
	}
	caller := resolveTier(tierFn)
	if tierRank(p.Metadata.Tier) > tierRank(caller) {
		return nil, fmt.Errorf("preset %s/%s requires tier %q (active tier: %q)", category, name, p.Metadata.Tier, caller)
	}
	return p, nil
}

// List returns metadata for all presets available at the caller's tier.
// Results are sorted by (category, name) for stable output.
func List(tierFn TierFunc) []PresetMeta {
	idx, err := load()
	if err != nil {
		return nil
	}
	caller := resolveTier(tierFn)
	out := make([]PresetMeta, 0, len(idx.meta))
	for _, m := range idx.meta {
		if tierRank(m.Tier) <= tierRank(caller) {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// BundleExpand resolves a bundle name and returns a map of category → child
// preset name. The caller can then call ResolveCategory(category, childName,
// ...) for each category.
func BundleExpand(name string, tierFn TierFunc) (map[string]string, error) {
	p, err := ResolveCategory("bundles", name, tierFn)
	if err != nil {
		return nil, err
	}
	if p.Kind != "Bundle" {
		return nil, fmt.Errorf("preset %q is not a Bundle (kind=%q)", name, p.Kind)
	}
	idx, _ := load()
	b, ok := idx.bundles[name]
	if !ok {
		return nil, fmt.Errorf("bundle %q: internal: not in bundle index", name)
	}
	out := map[string]string{}
	if b.DirectoriesPreset != "" {
		out["directories"] = b.DirectoriesPreset
	}
	if b.UsersGroupsPreset != "" {
		out["users_groups"] = b.UsersGroupsPreset
	}
	if b.PackagesPreset != "" {
		out["packages"] = b.PackagesPreset
	}
	if b.SysctlPreset != "" {
		out["sysctl"] = b.SysctlPreset
	}
	if b.LimitsPreset != "" {
		out["limits"] = b.LimitsPreset
	}
	return out, nil
}

// BundleInlineExpand returns the inline capabilities (repos_enable + files)
// declared on a bundle. Both lists may be empty / nil for bundles that do
// not use inline capabilities. Tier-gating mirrors BundleExpand.
//
// The Bundle struct already decodes both fields during load(); this helper
// converts the registry-internal FileSpec into the config-package shape so
// callers in pkg/config can consume the result without an import cycle.
func BundleInlineExpand(name string, tierFn TierFunc) ([]string, []config.FileSpec, error) {
	p, err := ResolveCategory("bundles", name, tierFn)
	if err != nil {
		return nil, nil, err
	}
	if p.Kind != "Bundle" {
		return nil, nil, fmt.Errorf("preset %q is not a Bundle (kind=%q)", name, p.Kind)
	}
	idx, _ := load()
	b, ok := idx.bundles[name]
	if !ok {
		return nil, nil, fmt.Errorf("bundle %q: internal: not in bundle index", name)
	}
	repos := append([]string(nil), b.ReposEnable...)
	files := make([]config.FileSpec, 0, len(b.Files))
	for _, f := range b.Files {
		files = append(files, config.FileSpec{
			Path:       f.Path,
			Mode:       f.Mode,
			Owner:      f.Owner,
			Group:      f.Group,
			ContentB64: f.ContentB64,
			CreateOnly: f.CreateOnly,
		})
	}
	return repos, files, nil
}

// ---- Typed spec decoders ---------------------------------------------------
//
// These helpers decode a preset's RawSpec into the concrete Go types the
// managers consume. They are thin wrappers over yaml.Marshal + Unmarshal so
// the category-specific schema in YAML stays the authoritative format.

// DirectoriesSpec returns the directories list from a directories preset.
func DirectoriesSpec(p *Preset) ([]config.Directory, error) {
	var shape struct {
		Directories []config.Directory `yaml:"directories"`
	}
	if err := reencode(p.RawSpec, &shape); err != nil {
		return nil, err
	}
	return shape.Directories, nil
}

// UsersGroupsSpec returns the users+groups block from a users_groups preset.
func UsersGroupsSpec(p *Preset) (*config.UsersGroups, error) {
	var shape struct {
		UsersGroups config.UsersGroups `yaml:"users_groups"`
	}
	if err := reencode(p.RawSpec, &shape); err != nil {
		return nil, err
	}
	return &shape.UsersGroups, nil
}

// PackagesSpec returns the packages block from a packages preset.
func PackagesSpec(p *Preset) (*config.Packages, error) {
	var shape struct {
		Packages config.Packages `yaml:"packages"`
	}
	if err := reencode(p.RawSpec, &shape); err != nil {
		return nil, err
	}
	return &shape.Packages, nil
}

// SysctlSpec returns the sysctl list from a sysctl preset.
func SysctlSpec(p *Preset) ([]config.SysctlEntry, error) {
	var shape struct {
		Sysctl []config.SysctlEntry `yaml:"sysctl"`
	}
	if err := reencode(p.RawSpec, &shape); err != nil {
		return nil, err
	}
	return shape.Sysctl, nil
}

// LimitsSpec returns the limits list from a limits preset.
func LimitsSpec(p *Preset) ([]config.LimitEntry, error) {
	var shape struct {
		Limits []config.LimitEntry `yaml:"limits"`
	}
	if err := reencode(p.RawSpec, &shape); err != nil {
		return nil, err
	}
	return shape.Limits, nil
}

func reencode(src any, dst any) error {
	b, err := yaml.Marshal(src)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, dst)
}
