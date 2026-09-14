package apitypes

import (
	"fmt"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/runtimealias"
	"strings"
)

// ValidateSpeakerVoices checks shared Eino and Flowcraft speaker mappings.
func ValidateSpeakerVoices(voices *map[string]string) error {
	if voices == nil {
		return nil
	}
	for name, alias := range *voices {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "【】") {
			return fmt.Errorf("speaker_voices: invalid speaker name %q", name)
		}
		if err := runtimealias.Validate("speaker Voice alias", alias); err != nil {
			return fmt.Errorf("speaker_voices.%s: %w", name, err)
		}
	}
	return nil
}
