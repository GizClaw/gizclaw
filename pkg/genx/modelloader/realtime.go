package modelloader

import (
	"fmt"
	"strings"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkg/genx/transformers"
)

func registerRealtimeBySchema(cfg ConfigFile) ([]string, error) {
	// Parse schema to determine provider
	parts := strings.Split(cfg.Schema, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid schema: %s", cfg.Schema)
	}
	provider := parts[0]

	switch provider {
	case "doubao":
		return registerDoubaoRealtime(cfg)
	default:
		return nil, fmt.Errorf("unknown realtime provider: %s", provider)
	}
}

func registerDoubaoRealtime(cfg ConfigFile) ([]string, error) {
	if cfg.AppID == "" {
		return nil, fmt.Errorf("app_id is required for doubao realtime")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for doubao realtime")
	}

	// Create Doubao client
	client := doubaospeech.NewClient(cfg.AppID, doubaospeech.WithAPIKey(cfg.APIKey))

	// Extract default params
	var defaultOpts []transformers.DoubaoRealtimeOption
	if cfg.DefaultParams != nil {
		if model, ok := cfg.DefaultParams["model"].(string); ok {
			defaultOpts = append(defaultOpts, transformers.WithDoubaoRealtimeModel(model))
		}
	}

	var names []string

	// Register realtime models from Models field
	// Each model has a name and voice
	for _, m := range cfg.Models {
		if m.Name == "" {
			return nil, fmt.Errorf("realtime model entry missing name")
		}

		// Build options for this model
		opts := make([]transformers.DoubaoRealtimeOption, len(defaultOpts))
		copy(opts, defaultOpts)

		// Use Voice field for speaker
		if m.Voice != "" {
			opts = append(opts, transformers.WithDoubaoRealtimeSpeaker(m.Voice))
		}

		// Create realtime transformer
		rt := transformers.NewDoubaoRealtime(client, opts...)
		if err := transformers.Handle(m.Name, rt); err != nil {
			return nil, fmt.Errorf("register realtime transformer %q: %w", m.Name, err)
		}
		names = append(names, m.Name)
	}

	return names, nil
}
