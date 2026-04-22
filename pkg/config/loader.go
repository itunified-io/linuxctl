package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadLinux reads and decodes a linux.yaml from disk without validation.
// Returns an empty Linux when the path is empty.
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
