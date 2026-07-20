package modelloader

import (
	"fmt"
	"strings"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtimeduplex"
)

func registerRealtimeBySchema(cfg ConfigFile) ([]string, error) {
	parts := strings.Split(cfg.Schema, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid schema: %s", cfg.Schema)
	}
	provider := parts[0]
	subject := strings.Join(parts[1:], "/")
	if len(parts) > 2 {
		subject = strings.Join(parts[1:len(parts)-1], "/")
	}

	switch provider {
	case "doubao":
		switch normalizeRealtimeSchemaSubject(subject) {
		case "realtime":
			return registerDoubaoRealtime(cfg)
		case "realtime_duplex", "duplex_realtime":
			return registerDoubaoRealtimeDuplex(cfg)
		default:
			return nil, fmt.Errorf("unknown doubao realtime schema: %s", cfg.Schema)
		}
	default:
		return nil, fmt.Errorf("unknown realtime provider: %s", provider)
	}
}

func normalizeRealtimeSchemaSubject(subject string) string {
	subject = strings.ToLower(strings.TrimSpace(subject))
	subject = strings.ReplaceAll(subject, "-", "_")
	subject = strings.ReplaceAll(subject, "/", "_")
	return subject
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
	config := doubaorealtime.Config{Client: client, Mode: doubaorealtime.ModePushToTalk}
	if cfg.DefaultParams != nil {
		if model, ok := cfg.DefaultParams["model"].(string); ok {
			config.Model = model
		}
		parsedMode, err := doubaoRealtimeModeFromParams(cfg.DefaultParams)
		if err != nil {
			return nil, err
		}
		if parsedMode != "" {
			config.Mode = parsedMode
		}
		if dialogID := realtimeParamString(cfg.DefaultParams, "dialog_id"); dialogID != "" {
			config.DialogID = dialogID
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
		modelConfig := config
		if m.Voice != "" {
			modelConfig.Speaker = m.Voice
		}
		rt, err := doubaorealtime.New(modelConfig)
		if err != nil {
			return nil, fmt.Errorf("construct realtime transformer %q: %w", m.Name, err)
		}
		if err := transformers.Handle(m.Name, rt); err != nil {
			return nil, fmt.Errorf("register realtime transformer %q: %w", m.Name, err)
		}
		names = append(names, m.Name)
	}

	return names, nil
}

func registerDoubaoRealtimeDuplex(cfg ConfigFile) ([]string, error) {
	if cfg.AppID == "" {
		return nil, fmt.Errorf("app_id is required for doubao realtime duplex")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for doubao realtime duplex")
	}

	client := doubaospeech.NewClient(cfg.AppID, doubaospeech.WithAPIKey(cfg.APIKey))

	config := doubaorealtimeduplex.Config{Client: client}
	if cfg.DefaultParams != nil {
		if model, ok := cfg.DefaultParams["model"].(string); ok {
			config.Model = model
		}
		if dialogID := realtimeParamString(cfg.DefaultParams, "dialog_id"); dialogID != "" {
			config.SessionID = dialogID
		}
		if err := validateDoubaoRealtimeDuplexMode(cfg.DefaultParams); err != nil {
			return nil, err
		}
	}

	var names []string
	for _, m := range cfg.Models {
		if m.Name == "" {
			return nil, fmt.Errorf("realtime duplex model entry missing name")
		}

		modelConfig := config
		if m.Voice != "" {
			modelConfig.Speaker = m.Voice
		}
		rt, err := doubaorealtimeduplex.New(modelConfig)
		if err != nil {
			return nil, fmt.Errorf("construct realtime duplex transformer %q: %w", m.Name, err)
		}
		if err := transformers.Handle(m.Name, rt); err != nil {
			return nil, fmt.Errorf("register realtime duplex transformer %q: %w", m.Name, err)
		}
		names = append(names, m.Name)
	}

	return names, nil
}

func doubaoRealtimeModeFromParams(params map[string]any) (doubaorealtime.Mode, error) {
	for _, key := range []string{"mode", "input_mode", "input"} {
		value, ok := params[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "push-to-talk", "push_to_talk", "ptt", "default":
			return doubaorealtime.ModePushToTalk, nil
		case "realtime", "real-time", "real_time":
			return doubaorealtime.ModeRealtime, nil
		case "text":
			return doubaorealtime.ModeText, nil
		default:
			return "", fmt.Errorf("unsupported doubao realtime mode %q", value)
		}
	}
	return "", nil
}

func validateDoubaoRealtimeDuplexMode(params map[string]any) error {
	for _, key := range []string{"mode", "input_mode", "input"} {
		value, ok := params[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "realtime", "real-time", "real_time", "default":
			return nil
		default:
			return fmt.Errorf("doubao realtime duplex only supports realtime mode, got %q", value)
		}
	}
	return nil
}

func realtimeParamString(params map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := params[key].(string)
		if ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
