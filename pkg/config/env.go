// Package config contains the YAML contract consumed by linuxctl.
package config

import (
	"fmt"
	"sort"
)

// NodeHostnames returns the sorted hostname keys from an Env's Hypervisor spec.
//
// The Hypervisor map is opaque to linuxctl (owned by proxclt / dbx), so this
// helper walks the map defensively and tolerates the two shapes seen in the
// wild:
//
//	spec.hypervisor.nodes:           # inline form
//	  node1: { ... }
//	  node2: { ... }
//
//	spec.hypervisor.Value.Nodes:     # $ref-resolved form ($Ref[Hypervisor])
//	  node1: { ... }
//	  node2: { ... }
//
// Returns an error if hypervisor is missing or has no nodes — callers rely on
// this to surface misconfigured env files instead of silently doing nothing.
func (e *Env) NodeHostnames() ([]string, error) {
	if e == nil {
		return nil, fmt.Errorf("env is nil")
	}
	if e.Spec.Hypervisor == nil {
		return nil, fmt.Errorf("spec.hypervisor is not defined")
	}
	nodes := findNodesMap(e.Spec.Hypervisor)
	if nodes == nil {
		return nil, fmt.Errorf("spec.hypervisor has no nodes map")
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("spec.hypervisor.nodes is empty")
	}
	out := make([]string, 0, len(nodes))
	for k := range nodes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// findNodesMap probes the two supported shapes for the nodes map.
func findNodesMap(h map[string]any) map[string]any {
	if v, ok := h["nodes"]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
		if m, ok := v.(map[any]any); ok {
			out := make(map[string]any, len(m))
			for k, vv := range m {
				if ks, ok := k.(string); ok {
					out[ks] = vv
				}
			}
			return out
		}
	}
	// $ref-resolved form: spec.hypervisor.Value.Nodes
	if v, ok := h["Value"]; ok {
		if vm, ok := v.(map[string]any); ok {
			return findNodesMap(vm)
		}
	}
	return nil
}

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
