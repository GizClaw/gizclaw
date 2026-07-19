package doubaorealtime

import (
	"context"
	"encoding/json"
	"fmt"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	legacy "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/agentkit"
)

// Mode controls how client input boundaries are interpreted.
type Mode string

const (
	// ModePushToTalk treats each input stream as one user turn.
	ModePushToTalk Mode = "push_to_talk"
	// ModeRealtime continuously detects user turns.
	ModeRealtime Mode = "realtime"
	// ModeText sends text input directly to the dialogue model.
	ModeText Mode = "text"
)

// Config contains immutable Doubao realtime dependencies and session options.
type Config struct {
	Client            *doubaospeech.Client
	Speaker           string
	Format            string
	SampleRate        int
	Channels          int
	SpeechRate        *int
	LoudnessRate      *int
	InputFormat       string
	InputSampleRate   int
	InputChannels     int
	InputTranscode    *bool
	ASRExtra          *doubaospeech.RealtimeASRExtra
	TTSExtra          *doubaospeech.RealtimeTTSExtra
	BotName           string
	SystemRole        string
	VADWindow         int
	SpeakingStyle     string
	CharacterManifest string
	DialogID          string
	DialogExtra       *doubaospeech.RealtimeDialogExtra
	SearchAPIKey      string
	Model             string
	Mode              Mode
}

// Transformer adapts Doubao realtime dialogue to the GenX contract.
type Transformer struct {
	config      Config
	newDelegate func() genx.Transformer
}

var _ genx.Transformer = (*Transformer)(nil)

// New constructs a Doubao realtime transformer without opening a WebSocket.
func New(config Config) (*Transformer, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("doubao realtime: client is required")
	}
	config, err := cloneConfig(config)
	if err != nil {
		return nil, err
	}
	return &Transformer{config: config}, nil
}

func (t *Transformer) delegate() genx.Transformer {
	if t.newDelegate != nil {
		return t.newDelegate()
	}
	config := t.config
	opts := make([]legacy.DoubaoRealtimeOption, 0, 24)
	if config.Speaker != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeSpeaker(config.Speaker))
	}
	if config.Format != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeFormat(config.Format))
	}
	if config.SampleRate != 0 {
		opts = append(opts, legacy.WithDoubaoRealtimeSampleRate(config.SampleRate))
	}
	if config.Channels != 0 {
		opts = append(opts, legacy.WithDoubaoRealtimeChannels(config.Channels))
	}
	if config.SpeechRate != nil {
		opts = append(opts, legacy.WithDoubaoRealtimeSpeechRate(*config.SpeechRate))
	}
	if config.LoudnessRate != nil {
		opts = append(opts, legacy.WithDoubaoRealtimeLoudnessRate(*config.LoudnessRate))
	}
	if config.InputFormat != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeInputFormat(config.InputFormat))
	}
	if config.InputSampleRate != 0 {
		opts = append(opts, legacy.WithDoubaoRealtimeInputSampleRate(config.InputSampleRate))
	}
	if config.InputChannels != 0 {
		opts = append(opts, legacy.WithDoubaoRealtimeInputChannels(config.InputChannels))
	}
	if config.InputTranscode != nil {
		opts = append(opts, legacy.WithDoubaoRealtimeInputTranscode(*config.InputTranscode))
	}
	if config.ASRExtra != nil {
		opts = append(opts, legacy.WithDoubaoRealtimeASRExtra(*config.ASRExtra))
	}
	if config.TTSExtra != nil {
		opts = append(opts, legacy.WithDoubaoRealtimeTTSExtra(*config.TTSExtra))
	}
	if config.BotName != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeBotName(config.BotName))
	}
	if config.SystemRole != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeSystemRole(config.SystemRole))
	}
	if config.VADWindow != 0 {
		opts = append(opts, legacy.WithDoubaoRealtimeVADWindow(config.VADWindow))
	}
	if config.SpeakingStyle != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeSpeakingStyle(config.SpeakingStyle))
	}
	if config.CharacterManifest != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeCharacterManifest(config.CharacterManifest))
	}
	if config.DialogID != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeDialogID(config.DialogID))
	}
	if config.DialogExtra != nil {
		opts = append(opts, legacy.WithDoubaoRealtimeDialogExtra(*config.DialogExtra))
	}
	if config.SearchAPIKey != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeSearchAPIKey(config.SearchAPIKey))
	}
	if config.Model != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeModel(config.Model))
	}
	if config.Mode != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeMode(legacy.DoubaoRealtimeMode(config.Mode)))
	}
	return legacy.NewDoubaoRealtime(config.Client, opts...)
}

// Transform starts one independent provider session for this invocation.
func (t *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t == nil || t.config.Client == nil && t.newDelegate == nil {
		return nil, fmt.Errorf("doubao realtime: transformer is not initialized")
	}
	output, err := t.delegate().Transform(ctx, input)
	if err != nil {
		return nil, err
	}
	return agentkit.NewResponseStream(output)
}

func cloneConfig(config Config) (Config, error) {
	config.SpeechRate = cloneInt(config.SpeechRate)
	config.LoudnessRate = cloneInt(config.LoudnessRate)
	config.InputTranscode = cloneBool(config.InputTranscode)
	var err error
	config.ASRExtra, err = cloneJSON(config.ASRExtra)
	if err != nil {
		return Config{}, fmt.Errorf("doubao realtime: clone ASR config: %w", err)
	}
	config.TTSExtra, err = cloneJSON(config.TTSExtra)
	if err != nil {
		return Config{}, fmt.Errorf("doubao realtime: clone TTS config: %w", err)
	}
	config.DialogExtra, err = cloneJSON(config.DialogExtra)
	if err != nil {
		return Config{}, fmt.Errorf("doubao realtime: clone dialog config: %w", err)
	}
	return config, nil
}

func cloneJSON[T any](value *T) (*T, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var clone T
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
