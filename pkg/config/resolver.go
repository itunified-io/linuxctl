package config

import "fmt"

// Resolver resolves ${vault:…} / ${ref:…} expressions against a lookup set.
// Mirrors the proxclt resolver so a shared package can be extracted later.
type Resolver struct{}

// NewResolver returns a scaffold resolver.
func NewResolver() *Resolver { return &Resolver{} }

// Resolve replaces placeholders in-place. Scaffold only — not implemented.
func (r *Resolver) Resolve(input string) (string, error) {
	_ = input
	return "", fmt.Errorf("resolver: not implemented")
}
