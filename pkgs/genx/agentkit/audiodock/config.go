package audiodock

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// VoiceRequest describes one model output route presented to VoiceResolver.
// Chunk is a defensive copy of the first text-bearing chunk for the route.
type VoiceRequest struct {
	StreamID string
	Role     genx.Role
	Name     string
	Label    string
	Chunk    *genx.MessageChunk
}

// VoiceResolver returns the TTS TransformerMux pattern for one output route.
// An empty pattern disables synthesis for that route.
type VoiceResolver func(context.Context, VoiceRequest) (string, error)

// Config declares one reusable Audio Dock.
type Config struct {
	// Agent is the required downstream text Transformer.
	Agent genx.Transformer
	// ASR optionally converts streaming audio input into text input for Agent.
	ASR genx.Transformer
	// TTS optionally synthesizes eligible Agent text output.
	TTS genx.TransformerMux
	// ResolveVoice selects a TTS pattern per output route.
	ResolveVoice VoiceResolver
}

func normalizeConfig(config Config) (Config, error) {
	if config.Agent == nil {
		return Config{}, fmt.Errorf("audiodock: Agent is required")
	}
	if config.TTS == nil && config.ResolveVoice != nil {
		return Config{}, fmt.Errorf("audiodock: ResolveVoice requires TTS")
	}
	if config.TTS != nil && config.ResolveVoice == nil {
		return Config{}, fmt.Errorf("audiodock: TTS requires ResolveVoice")
	}
	return config, nil
}

func resolveVoice(ctx context.Context, resolver VoiceResolver, chunk *genx.MessageChunk) (string, error) {
	request := VoiceRequest{Chunk: chunk.Clone(), Role: chunk.Role, Name: chunk.Name}
	if chunk.Ctrl != nil {
		request.StreamID = strings.TrimSpace(chunk.Ctrl.StreamID)
		request.Label = strings.TrimSpace(chunk.Ctrl.Label)
	}
	pattern, err := resolver(ctx, request)
	if err != nil {
		return "", fmt.Errorf("audiodock: resolve voice stream_id=%q name=%q: %w", request.StreamID, request.Name, err)
	}
	return strings.TrimSpace(pattern), nil
}
