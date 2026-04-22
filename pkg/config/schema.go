package config

import "github.com/go-playground/validator/v10"

// Validate runs go-playground/validator on a decoded Linux manifest.
// An empty manifest is considered valid (scaffold behaviour).
func Validate(l *Linux) error {
	if l == nil || l.Kind == "" {
		return nil
	}
	v := validator.New(validator.WithRequiredStructEnabled())
	return v.Struct(l)
}
