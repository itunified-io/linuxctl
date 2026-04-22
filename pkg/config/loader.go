package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadLinux reads and decodes a linux.yaml from disk without validation.
// An empty path returns an empty Linux (scaffold behaviour).
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
