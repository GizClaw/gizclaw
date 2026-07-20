package customid

import (
	"fmt"
	"strings"
)

const (
	MinLength = 8
	MaxLength = 48
	Pattern   = "^[a-z][a-z0-9._-]{6,46}[a-z0-9]$"
)

// Validate checks client-supplied custom resource IDs. It does not normalize,
// trim, lowercase, or slugify the input.
func Validate(id string) error {
	if id == "" {
		return fmt.Errorf("custom id is required")
	}
	if len(id) < MinLength {
		return fmt.Errorf("custom id must be at least %d characters", MinLength)
	}
	if len(id) > MaxLength {
		return fmt.Errorf("custom id must be at most %d characters", MaxLength)
	}
	if !isLower(id[0]) {
		return fmt.Errorf("custom id must start with a lowercase ASCII letter")
	}
	last := id[len(id)-1]
	if !isLower(last) && !isDigit(last) {
		return fmt.Errorf("custom id must end with a lowercase ASCII letter or digit")
	}
	for i := 1; i < len(id)-1; i++ {
		c := id[i]
		if isLower(c) || isDigit(c) || c == '.' || c == '_' || c == '-' {
			continue
		}
		return fmt.Errorf("custom id may contain only lowercase ASCII letters, digits, '.', '_', and '-'")
	}
	return nil
}

func ValidateField(field, id string) error {
	if err := Validate(id); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}

// ValidateRegistrationTokenName accepts canonical custom IDs and the scoped
// app:<bundle-id> form reserved for RegistrationToken resources.
func ValidateRegistrationTokenName(name string) error {
	const appPrefix = "app:"
	if !strings.HasPrefix(name, appPrefix) {
		return Validate(name)
	}
	if err := Validate(strings.TrimPrefix(name, appPrefix)); err != nil {
		return fmt.Errorf("registration token app name: %w", err)
	}
	return nil
}

func isLower(c byte) bool {
	return c >= 'a' && c <= 'z'
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
