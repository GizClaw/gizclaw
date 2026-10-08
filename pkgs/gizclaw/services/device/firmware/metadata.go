package firmware

import (
	"encoding/json"
	"fmt"
	"regexp"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

var metadataKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// ValidateMetadataKey validates a key in the channel-independent metadata map.
func ValidateMetadataKey(key string) error {
	if !metadataKeyPattern.MatchString(key) {
		return fmt.Errorf("metadata key must contain 1–64 ASCII letters, digits, dots, underscores or hyphens and start with a letter or digit")
	}
	return nil
}

func normalizeMetadata(in *apitypes.FirmwareMetadata) (*apitypes.FirmwareMetadata, error) {
	if in == nil || len(*in) == 0 {
		return nil, nil
	}
	if len(*in) > 64 {
		return nil, fmt.Errorf("metadata must contain at most 64 entries")
	}
	out := make(apitypes.FirmwareMetadata, len(*in))
	for key, entry := range *in {
		if err := ValidateMetadataKey(key); err != nil {
			return nil, err
		}
		if !utf8.Valid(entry) {
			return nil, fmt.Errorf("metadata value must be valid UTF-8 JSON")
		}
		// Marshal RawMessage validates and compacts JSON without converting
		// numbers through float64. It also allocates an owned copy and applies
		// the same escaping as catalog storage and HTTP output.
		value, err := json.Marshal(entry)
		if err != nil {
			return nil, fmt.Errorf("metadata value must be valid JSON")
		}
		if len(value) > 65536 {
			return nil, fmt.Errorf("metadata JSON value must contain at most 65536 UTF-8 bytes")
		}
		out[key] = value
	}
	return &out, nil
}
