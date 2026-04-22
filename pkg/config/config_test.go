package config

import "testing"

func TestConfig_ValidateEmpty(t *testing.T) {
	if err := Validate(&Linux{}); err != nil {
		t.Fatalf("empty Linux should be valid in scaffold, got %v", err)
	}
	if err := Validate(nil); err != nil {
		t.Fatalf("nil Linux should be valid in scaffold, got %v", err)
	}
}

func TestConfig_LoadEmpty(t *testing.T) {
	l, err := LoadLinux("")
	if err != nil {
		t.Fatalf("load empty path: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil Linux")
	}
}
