package providertenants

import (
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
)

func validateResourceID(id string) error {
	return customid.ValidateResourceID(id)
}

func validateResourceReference(field, id string) error {
	if err := customid.ValidateResourceID(id); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}
