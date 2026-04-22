package config

import (
	"github.com/go-playground/validator/v10"
)

// sharedValidator is reused across calls — validator.New is non-trivial.
var sharedValidator = func() *validator.Validate {
	return validator.New(validator.WithRequiredStructEnabled())
}()

// Validate runs go-playground/validator on a decoded Linux manifest plus the
// cross-field uniqueness / referential checks.
// An empty manifest is considered valid (scaffold behaviour).
func Validate(l *Linux) error {
	if l == nil || l.Kind == "" {
		return nil
	}
	if err := sharedValidator.Struct(l); err != nil {
		return err
	}
	return validateCrossFields(l)
}

// ValidateEnv validates an Env plus its resolved linux layer.
func ValidateEnv(e *Env) error {
	if e == nil {
		return nil
	}
	if err := sharedValidator.Struct(e); err != nil {
		return err
	}
	if e.Spec.Linux.Value != nil {
		return Validate(e.Spec.Linux.Value)
	}
	return nil
}
