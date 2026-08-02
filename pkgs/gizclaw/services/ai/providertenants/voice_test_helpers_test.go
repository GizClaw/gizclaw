package providertenants

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	voicecatalog "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type voiceFilters struct {
	source       *string
	providerKind *string
	providerID   *string
}

func providerData(kind apitypes.VoiceProviderKind, values map[string]any) *apitypes.VoiceProviderData {
	return voicecatalog.ProviderData(kind, values)
}

func stableVoiceName(kind apitypes.VoiceProviderKind, tenantID, providerVoiceID string) string {
	return voicecatalog.StableID(kind, tenantID, providerVoiceID)
}

func writeVoice(ctx context.Context, store kv.Store, voice apitypes.Voice, previous *apitypes.Voice) error {
	return voicecatalog.Write(ctx, store, voice, previous)
}

func getVoice(ctx context.Context, store kv.Store, id string) (apitypes.Voice, error) {
	return voicecatalog.Get(ctx, store, id)
}

func getVoiceByName(ctx context.Context, store kv.Store, name string) (apitypes.Voice, error) {
	escaped := strings.ReplaceAll(strings.ReplaceAll(name, "%", "%25"), ":", "%3A")
	id, err := store.Get(ctx, kv.Key{"by-name", escaped})
	if err != nil {
		return apitypes.Voice{}, err
	}
	return getVoice(ctx, store, string(id))
}

func decodeVoice(data []byte, out *apitypes.Voice) error {
	return voicecatalog.Decode(data, out)
}

func voiceProviderDataString(voice apitypes.Voice, key string) string {
	return voicecatalog.ProviderDataString(voice, key)
}

func matchesVoiceFilters(voice apitypes.Voice, filters voiceFilters) bool {
	if filters.source != nil && string(voice.Source) != *filters.source {
		return false
	}
	if filters.providerKind != nil && string(voice.Provider.Kind) != *filters.providerKind {
		return false
	}
	if filters.providerID != nil && string(voice.Provider.Id) != *filters.providerID {
		return false
	}
	return true
}

func providerDataString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func rawEqual(left, right *map[string]any) bool {
	return mapEqual(left, right)
}

func mapEqual(left, right *map[string]any) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}
