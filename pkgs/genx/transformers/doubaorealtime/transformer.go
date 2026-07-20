package doubaorealtime

import (
	"encoding/json"
	"fmt"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
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

// New constructs a Doubao realtime transformer without opening a WebSocket.
func New(config Config) (*Transformer, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("doubao realtime: client is required")
	}
	config, err := cloneConfig(config)
	if err != nil {
		return nil, err
	}
	opts := make([]option, 0, 24)
	if config.Speaker != "" {
		opts = append(opts, withSpeaker(config.Speaker))
	}
	if config.Format != "" {
		opts = append(opts, withFormat(config.Format))
	}
	if config.SampleRate != 0 {
		opts = append(opts, withSampleRate(config.SampleRate))
	}
	if config.Channels != 0 {
		opts = append(opts, withChannels(config.Channels))
	}
	if config.SpeechRate != nil {
		opts = append(opts, withSpeechRate(*config.SpeechRate))
	}
	if config.LoudnessRate != nil {
		opts = append(opts, withLoudnessRate(*config.LoudnessRate))
	}
	if config.InputFormat != "" {
		opts = append(opts, withInputFormat(config.InputFormat))
	}
	if config.InputSampleRate != 0 {
		opts = append(opts, withInputSampleRate(config.InputSampleRate))
	}
	if config.InputChannels != 0 {
		opts = append(opts, withInputChannels(config.InputChannels))
	}
	if config.InputTranscode != nil {
		opts = append(opts, withInputTranscode(*config.InputTranscode))
	}
	if config.ASRExtra != nil {
		opts = append(opts, withASRExtra(*config.ASRExtra))
	}
	if config.TTSExtra != nil {
		opts = append(opts, withTTSExtra(*config.TTSExtra))
	}
	if config.BotName != "" {
		opts = append(opts, withBotName(config.BotName))
	}
	if config.SystemRole != "" {
		opts = append(opts, withSystemRole(config.SystemRole))
	}
	if config.VADWindow != 0 {
		opts = append(opts, withVADWindow(config.VADWindow))
	}
	if config.SpeakingStyle != "" {
		opts = append(opts, withSpeakingStyle(config.SpeakingStyle))
	}
	if config.CharacterManifest != "" {
		opts = append(opts, withCharacterManifest(config.CharacterManifest))
	}
	if config.DialogID != "" {
		opts = append(opts, withDialogID(config.DialogID))
	}
	if config.DialogExtra != nil {
		opts = append(opts, withDialogExtra(*config.DialogExtra))
	}
	if config.SearchAPIKey != "" {
		opts = append(opts, withSearchAPIKey(config.SearchAPIKey))
	}
	if config.Model != "" {
		opts = append(opts, withModel(config.Model))
	}
	if config.Mode != "" {
		opts = append(opts, withMode(config.Mode))
	}
	return newTransformer(config.Client, opts...), nil
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
