package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnv_NodeHostnames_NilEnv(t *testing.T) {
	var e *Env
	_, err := e.NodeHostnames()
	require.Error(t, err)
}

func TestEnv_NodeHostnames_NoHypervisor(t *testing.T) {
	e := &Env{}
	_, err := e.NodeHostnames()
	require.Error(t, err)
	require.Contains(t, err.Error(), "hypervisor")
}

func TestEnv_NodeHostnames_NoNodesKey(t *testing.T) {
	e := &Env{Spec: EnvSpec{Hypervisor: map[string]any{"kind": "proxmox"}}}
	_, err := e.NodeHostnames()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no nodes map")
}

func TestEnv_NodeHostnames_EmptyNodes(t *testing.T) {
	e := &Env{Spec: EnvSpec{Hypervisor: map[string]any{"nodes": map[string]any{}}}}
	_, err := e.NodeHostnames()
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty")
}

func TestEnv_NodeHostnames_InlineShape(t *testing.T) {
	e := &Env{Spec: EnvSpec{Hypervisor: map[string]any{
		"nodes": map[string]any{
			"node2.example": map[string]any{},
			"node1.example": map[string]any{},
			"node3.example": map[string]any{},
		},
	}}}
	got, err := e.NodeHostnames()
	require.NoError(t, err)
	require.Equal(t, []string{"node1.example", "node2.example", "node3.example"}, got)
}

// YAML decodes sub-maps as map[any]any in some cases — cover that branch.
func TestEnv_NodeHostnames_MapAnyShape(t *testing.T) {
	e := &Env{Spec: EnvSpec{Hypervisor: map[string]any{
		"nodes": map[any]any{
			"n1": map[string]any{"role": "db"},
			"n2": map[string]any{"role": "db"},
			42:   "ignored-non-string-key",
		},
	}}}
	got, err := e.NodeHostnames()
	require.NoError(t, err)
	require.Equal(t, []string{"n1", "n2"}, got)
}

func TestEnv_NodeHostnames_RefResolvedShape(t *testing.T) {
	e := &Env{Spec: EnvSpec{Hypervisor: map[string]any{
		"Value": map[string]any{
			"nodes": map[string]any{
				"rac-a": map[string]any{},
				"rac-b": map[string]any{},
			},
		},
	}}}
	got, err := e.NodeHostnames()
	require.NoError(t, err)
	require.Equal(t, []string{"rac-a", "rac-b"}, got)
}

func TestEnv_NodeHostnames_WrongTypeForNodes(t *testing.T) {
	e := &Env{Spec: EnvSpec{Hypervisor: map[string]any{"nodes": "not-a-map"}}}
	_, err := e.NodeHostnames()
	require.Error(t, err)
}
