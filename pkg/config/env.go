// Package config contains the YAML contract consumed by linuxctl.
package config

// Env mirrors the top-level env.yaml manifest. linuxctl types the Linux layer
// in detail; Hypervisor / Networks / Cluster stay opaque because they are
// owned by proxclt / dbx.
type Env struct {
	Version  string   `yaml:"version" validate:"required,eq=1"`
	Kind     string   `yaml:"kind" validate:"required,eq=Env"`
	Metadata Metadata `yaml:"metadata" validate:"required"`
	Extends  string   `yaml:"extends,omitempty"`
	Spec     EnvSpec  `yaml:"spec" validate:"required"`
	Hooks    *Hooks   `yaml:"hooks,omitempty"`
}

// Metadata describes the env identity.
type Metadata struct {
	Name        string            `yaml:"name" validate:"required,hostname_rfc1123"`
	Domain      string            `yaml:"domain,omitempty"`
	Tags        map[string]string `yaml:"tags,omitempty"`
	Description string            `yaml:"description,omitempty"`
}

// EnvSpec holds the layered subsystems. Only Linux is typed by linuxctl.
type EnvSpec struct {
	Linux          Ref[Linux]       `yaml:"linux" validate:"required"`
	Hypervisor     map[string]any   `yaml:"hypervisor,omitempty"`
	Networks       map[string]any   `yaml:"networks,omitempty"`
	StorageClasses map[string]any   `yaml:"storage_classes,omitempty"`
	Cluster        map[string]any   `yaml:"cluster,omitempty"`
	Databases      []map[string]any `yaml:"databases,omitempty"`
}

// Ref is either an inline value or a $ref to a file. After loader.Resolve,
// Value points at the effective value.
type Ref[T any] struct {
	Ref    string `yaml:"$ref,omitempty"`
	Value  *T     `yaml:"-"`
	Inline *T     `yaml:"-"`
}

// Hooks are subset-mirror of proxclt hooks — shape preserved for cross-tool
// compatibility.
type Hooks struct {
	OnApplyStart   []Hook `yaml:"on_apply_start,omitempty"`
	OnApplySuccess []Hook `yaml:"on_apply_success,omitempty"`
	OnApplyFailure []Hook `yaml:"on_apply_failure,omitempty"`
}

// Hook is a single hook definition.
type Hook struct {
	Type   string         `yaml:"type" validate:"required"`
	Params map[string]any `yaml:",inline"`
}
