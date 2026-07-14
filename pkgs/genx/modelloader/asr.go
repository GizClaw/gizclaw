package modelloader

import (
	"fmt"
	"slices"
	"strings"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
)

func registerASRBySchema(cfg ConfigFile) ([]string, error) {
	// Parse schema to determine provider
	parts := strings.Split(cfg.Schema, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid schema: %s", cfg.Schema)
	}
	provider := parts[0]

	switch provider {
	case "doubao":
		return registerDoubaoASR(cfg)
	default:
		return nil, fmt.Errorf("unknown ASR provider: %s", provider)
	}
}

func registerDoubaoASR(cfg ConfigFile) ([]string, error) {
	if cfg.AppID == "" {
		return nil, fmt.Errorf("app_id is required for doubao ASR")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for doubao ASR")
	}

	// Extract default params
	var opts []transformers.DoubaoASRSAUCOption
	if cfg.DefaultParams != nil {
		if language, ok := cfg.DefaultParams["language"].(string); ok {
			opts = append(opts, transformers.WithDoubaoASRSAUCLanguage(language))
		}
	}

	var names []string

	// Register ASR models from Models field (reusing Entry struct)
	for _, m := range cfg.Models {
		if m.Name == "" {
			return nil, fmt.Errorf("asr model entry missing name")
		}
		resourceID := m.ResourceID
		if resourceID == "" {
			resourceID = doubaospeech.ResourceASRStream
		}

		// Create ASR transformer with the resource options
		client := doubaospeech.NewClient(
			cfg.AppID,
			doubaospeech.WithAPIKey(cfg.APIKey),
			doubaospeech.WithResourceID(resourceID),
		)
		modelOpts := append(slices.Clone(opts), transformers.WithDoubaoASRSAUCResourceID(resourceID))
		asr := transformers.NewDoubaoASRSAUC(client, modelOpts...)
		if err := transformers.Handle(m.Name, asr); err != nil {
			return nil, fmt.Errorf("register transformer %q: %w", m.Name, err)
		}
		names = append(names, m.Name)
	}

	return names, nil
}
