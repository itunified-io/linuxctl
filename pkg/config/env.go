// Package config contains the YAML contract consumed by linuxctl.
package config

// Env mirrors the top-level env.yaml manifest.
type Env struct {
	Kind       string            `yaml:"kind" validate:"required,eq=Env"`
	APIVersion string            `yaml:"apiVersion" validate:"required"`
	Name       string            `yaml:"name" validate:"required"`
	Tags       []string          `yaml:"tags,omitempty"`
	Hooks      map[string]string `yaml:"hooks,omitempty"`
}
