package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// BundleExpander is an optional hook that pkg/presets wires into the loader
// to expand `bundle_preset` into per-category *_preset fields. The loader
// stays free of a pkg/presets import to avoid a cycle; pkg/presets calls
// RegisterBundleExpander() in its init().
type BundleExpander func(name string) (map[string]string, error)

// BundleInlineExpander returns inline-capability data carried by a bundle
// (repos_enable + files). Optional companion to BundleExpander introduced
// in linuxctl#57. Loader gracefully tolerates a nil expander (older
// pkg/presets versions).
type BundleInlineExpander func(name string) (repos []string, files []FileSpec, err error)

var (
	bundleExpander       BundleExpander
	bundleInlineExpander BundleInlineExpander
)

// RegisterBundleExpander wires an expander. pkg/presets calls this from init.
func RegisterBundleExpander(fn BundleExpander) { bundleExpander = fn }

// RegisterBundleInlineExpander wires the inline-capability expander.
func RegisterBundleInlineExpander(fn BundleInlineExpander) { bundleInlineExpander = fn }

// expandBundleOnLinux fills the per-category *_preset fields from the named
// bundle, but only for fields the user left empty. Explicit per-category
// presets always win over the bundle.
func expandBundleOnLinux(l *Linux) error {
	if l == nil || l.BundlePreset == "" {
		return nil
	}
	if bundleExpander != nil {
		children, err := bundleExpander(l.BundlePreset)
		if err != nil {
			return fmt.Errorf("bundle %q: %w", l.BundlePreset, err)
		}
		if l.DirectoriesPreset == "" {
			l.DirectoriesPreset = children["directories"]
		}
		if l.UsersGroupsPreset == "" {
			l.UsersGroupsPreset = children["users_groups"]
		}
		if l.PackagesPreset == "" {
			l.PackagesPreset = children["packages"]
		}
		if l.SysctlPreset == "" {
			l.SysctlPreset = children["sysctl"]
		}
		if l.LimitsPreset == "" {
			l.LimitsPreset = children["limits"]
		}
	}
	// Inline capabilities (linuxctl#57). Explicit manifest entries are
	// retained; bundle entries are unioned in. Repo IDs dedup on string
	// equality; FileSpec entries dedup on absolute path with explicit
	// winning on collision.
	if bundleInlineExpander != nil {
		repos, files, err := bundleInlineExpander(l.BundlePreset)
		if err != nil {
			return fmt.Errorf("bundle %q (inline): %w", l.BundlePreset, err)
		}
		l.ReposEnable = mergeReposEnable(l.ReposEnable, repos)
		l.Files = mergeFiles(l.Files, files)
	}
	return nil
}

// mergeReposEnable returns the union of explicit + bundle repo IDs,
// preserving explicit-first ordering and removing duplicates.
func mergeReposEnable(explicit, bundle []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(explicit)+len(bundle))
	for _, r := range explicit {
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	for _, r := range bundle {
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// mergeFiles returns the union of explicit + bundle file specs keyed by
// absolute path; explicit wins on collision.
func mergeFiles(explicit, bundle []FileSpec) []FileSpec {
	byPath := map[string]FileSpec{}
	order := []string{}
	for _, f := range explicit {
		if _, dup := byPath[f.Path]; !dup {
			order = append(order, f.Path)
		}
		byPath[f.Path] = f
	}
	for _, f := range bundle {
		if _, has := byPath[f.Path]; has {
			continue
		}
		byPath[f.Path] = f
		order = append(order, f.Path)
	}
	out := make([]FileSpec, 0, len(order))
	for _, p := range order {
		out = append(out, byPath[p])
	}
	return out
}

// LoadLinux reads and decodes a linux.yaml from disk without validation.
// If the manifest declares `bundle_preset:`, the bundle is expanded into
// the per-category *_preset fields (user overrides always win). An empty
// path returns an empty Linux (scaffold behaviour).
func LoadLinux(path string) (*Linux, error) {
	if path == "" {
		return &Linux{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var l Linux
	if err := yaml.Unmarshal(b, &l); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := expandBundleOnLinux(&l); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &l, nil
}

// LoadEnv reads an env.yaml, resolves $ref on spec.linux (relative to the
// env file's directory), applies `extends` base merging, resolves secret
// placeholders through the supplied Resolver, and validates. A nil Resolver
// skips secret resolution.
func LoadEnv(path string, r *Resolver) (*Env, error) {
	if path == "" {
		return nil, fmt.Errorf("env path is required")
	}
	env, err := readEnv(path)
	if err != nil {
		return nil, err
	}

	// Apply base env via `extends` (one level; recursion handles chains).
	if env.Extends != "" {
		basePath := env.Extends
		if !filepath.IsAbs(basePath) {
			basePath = filepath.Join(filepath.Dir(path), basePath)
		}
		base, err := LoadEnv(basePath, r)
		if err != nil {
			return nil, fmt.Errorf("load extends %s: %w", basePath, err)
		}
		mergeEnv(base, env)
		env = base
	}

	// Resolve spec.linux $ref / inline.
	if env.Spec.Linux.Ref != "" {
		refPath := env.Spec.Linux.Ref
		if !filepath.IsAbs(refPath) {
			refPath = filepath.Join(filepath.Dir(path), refPath)
		}
		l, err := LoadLinux(refPath)
		if err != nil {
			return nil, fmt.Errorf("resolve linux $ref %s: %w", refPath, err)
		}
		env.Spec.Linux.Value = l
	} else if env.Spec.Linux.Inline != nil {
		env.Spec.Linux.Value = env.Spec.Linux.Inline
	}

	// Expand bundle_preset on the resolved Linux manifest (if any).
	if env.Spec.Linux.Value != nil {
		if err := expandBundleOnLinux(env.Spec.Linux.Value); err != nil {
			return nil, err
		}
	}

	// Resolve secret placeholders.
	if r != nil && env.Spec.Linux.Value != nil {
		if err := resolveLinuxSecrets(env.Spec.Linux.Value, r); err != nil {
			return nil, err
		}
	}

	if err := ValidateEnv(env); err != nil {
		return nil, err
	}
	return env, nil
}

// readEnv decodes an env.yaml and normalises the spec.linux node into a
// Ref[Linux] (either $ref or inline).
func readEnv(path string) (*Env, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var raw struct {
		Version  string   `yaml:"version"`
		Kind     string   `yaml:"kind"`
		Metadata Metadata `yaml:"metadata"`
		Extends  string   `yaml:"extends,omitempty"`
		Spec     struct {
			Linux          yaml.Node        `yaml:"linux"`
			Hypervisor     map[string]any   `yaml:"hypervisor,omitempty"`
			Networks       map[string]any   `yaml:"networks,omitempty"`
			StorageClasses map[string]any   `yaml:"storage_classes,omitempty"`
			Cluster        map[string]any   `yaml:"cluster,omitempty"`
			Databases      []map[string]any `yaml:"databases,omitempty"`
		} `yaml:"spec"`
		Hooks *Hooks `yaml:"hooks,omitempty"`
	}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	env := &Env{
		Version:  raw.Version,
		Kind:     raw.Kind,
		Metadata: raw.Metadata,
		Extends:  raw.Extends,
		Spec: EnvSpec{
			Hypervisor:     raw.Spec.Hypervisor,
			Networks:       raw.Spec.Networks,
			StorageClasses: raw.Spec.StorageClasses,
			Cluster:        raw.Spec.Cluster,
			Databases:      raw.Spec.Databases,
		},
		Hooks: raw.Hooks,
	}

	ln := raw.Spec.Linux
	if ln.Kind != 0 {
		var refHolder struct {
			Ref string `yaml:"$ref"`
		}
		if err := ln.Decode(&refHolder); err == nil && refHolder.Ref != "" {
			env.Spec.Linux = Ref[Linux]{Ref: refHolder.Ref}
		} else {
			var inline Linux
			if err := ln.Decode(&inline); err != nil {
				return nil, fmt.Errorf("decode inline linux: %w", err)
			}
			env.Spec.Linux = Ref[Linux]{Inline: &inline}
		}
	}
	return env, nil
}

// mergeEnv overlays `over` onto `base` in place.
func mergeEnv(base, over *Env) {
	if over.Version != "" {
		base.Version = over.Version
	}
	if over.Kind != "" {
		base.Kind = over.Kind
	}
	if over.Metadata.Name != "" {
		base.Metadata.Name = over.Metadata.Name
	}
	if over.Metadata.Domain != "" {
		base.Metadata.Domain = over.Metadata.Domain
	}
	if over.Metadata.Description != "" {
		base.Metadata.Description = over.Metadata.Description
	}
	if base.Metadata.Tags == nil && len(over.Metadata.Tags) > 0 {
		base.Metadata.Tags = map[string]string{}
	}
	for k, v := range over.Metadata.Tags {
		base.Metadata.Tags[k] = v
	}
	if over.Spec.Linux.Ref != "" || over.Spec.Linux.Inline != nil || over.Spec.Linux.Value != nil {
		base.Spec.Linux = over.Spec.Linux
	}
	for k, v := range over.Spec.Hypervisor {
		if base.Spec.Hypervisor == nil {
			base.Spec.Hypervisor = map[string]any{}
		}
		base.Spec.Hypervisor[k] = v
	}
	for k, v := range over.Spec.Networks {
		if base.Spec.Networks == nil {
			base.Spec.Networks = map[string]any{}
		}
		base.Spec.Networks[k] = v
	}
	for k, v := range over.Spec.StorageClasses {
		if base.Spec.StorageClasses == nil {
			base.Spec.StorageClasses = map[string]any{}
		}
		base.Spec.StorageClasses[k] = v
	}
	for k, v := range over.Spec.Cluster {
		if base.Spec.Cluster == nil {
			base.Spec.Cluster = map[string]any{}
		}
		base.Spec.Cluster[k] = v
	}
	if len(over.Spec.Databases) > 0 {
		base.Spec.Databases = append(base.Spec.Databases, over.Spec.Databases...)
	}
	if over.Hooks != nil {
		base.Hooks = over.Hooks
	}
}
