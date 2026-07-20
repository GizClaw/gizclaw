package modelloader

import (
	"fmt"
	"strings"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaotts"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/minimaxtts"
	"github.com/GizClaw/minimax-go"
)

func registerTTSBySchema(cfg ConfigFile) ([]string, error) {
	// Parse schema to determine provider
	parts := strings.Split(cfg.Schema, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid schema: %s", cfg.Schema)
	}
	provider := parts[0]

	switch provider {
	case "doubao":
		return registerDoubaoTTS(cfg)
	case "minimax":
		return registerMinimaxTTS(cfg)
	default:
		return nil, fmt.Errorf("unknown TTS provider: %s", provider)
	}
}

func registerDoubaoTTS(cfg ConfigFile) ([]string, error) {
	if cfg.AppID == "" {
		return nil, fmt.Errorf("app_id is required for doubao TTS")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for doubao TTS")
	}

	// Create Doubao client
	client := doubaospeech.NewClient(cfg.AppID, doubaospeech.WithAPIKey(cfg.APIKey))

	config := doubaotts.SeedV2Config{Client: client}
	if cfg.DefaultParams != nil {
		if format, ok := cfg.DefaultParams["format"].(string); ok && format != "" {
			config.Format = format
		}
		if sampleRate, ok := cfg.DefaultParams["sample_rate"].(float64); ok && sampleRate > 0 {
			config.SampleRate = int(sampleRate)
		}
	}

	var names []string
	for _, v := range cfg.Voices {
		if v.Name == "" || v.VoiceID == "" {
			return nil, fmt.Errorf("voice entry missing name or voice_id")
		}

		// Use DoubaoTTSSeedV2 for all voices
		// The transformer will auto-detect resource ID based on voice suffix
		voiceConfig := config
		voiceConfig.Speaker = v.VoiceID
		tts, err := doubaotts.NewSeedV2(voiceConfig)
		if err != nil {
			return nil, fmt.Errorf("create transformer %q: %w", v.Name, err)
		}
		if err := transformers.Handle(v.Name, tts); err != nil {
			return nil, fmt.Errorf("register transformer %q: %w", v.Name, err)
		}
		names = append(names, v.Name)
	}
	return names, nil
}

func registerMinimaxTTS(cfg ConfigFile) ([]string, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for minimax TTS")
	}

	clientConfig := minimax.Config{APIKey: cfg.APIKey}
	if cfg.BaseURL != "" {
		clientConfig.BaseURL = cfg.BaseURL
	}

	client, err := minimax.NewClient(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("create minimax client: %w", err)
	}

	var names []string
	for _, v := range cfg.Voices {
		if v.Name == "" || v.VoiceID == "" {
			return nil, fmt.Errorf("voice entry missing name or voice_id")
		}

		tts, err := minimaxtts.New(minimaxtts.Config{Client: client, VoiceID: v.VoiceID, Model: cfg.Model})
		if err != nil {
			return nil, fmt.Errorf("create transformer %q: %w", v.Name, err)
		}
		if err := transformers.Handle(v.Name, tts); err != nil {
			return nil, fmt.Errorf("register transformer %q: %w", v.Name, err)
		}
		names = append(names, v.Name)
	}
	return names, nil
}
