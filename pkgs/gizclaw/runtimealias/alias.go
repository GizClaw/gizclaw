// Package runtimealias owns the canonical syntax shared by RuntimeProfile
// bindings and resources that persist references to those bindings.
package runtimealias

import (
	"fmt"
	"regexp"
)

const maxBytes = 63

var pattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*(?:\.[a-z0-9]+(?:-[a-z0-9]+)*)*$`)

// Validate checks one complete RuntimeProfile alias without normalizing it.
func Validate(kind, value string) error {
	if len(value) == 0 || len(value) > maxBytes || !pattern.MatchString(value) {
		return fmt.Errorf("%s %q must be a RuntimeProfile alias with 1-63 bytes of dot-separated lowercase kebab-case segments", kind, value)
	}
	return nil
}
