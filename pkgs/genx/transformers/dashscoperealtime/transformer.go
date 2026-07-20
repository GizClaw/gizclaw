package dashscoperealtime

import (
	"fmt"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
)

// Config contains immutable DashScope realtime dependencies and session options.
type Config struct {
	Client            *dashscope.Client
	Model             string
	Voice             string
	Instructions      string
	Modalities        []string
	VAD               string
	Temperature       *float64
	MaxOutputTokens   *int
	EnableASR         *bool
	ASRModel          string
	TurnDetection     *dashscope.TurnDetection
	InputAudioFormat  string
	OutputAudioFormat string
}

// New constructs a DashScope realtime transformer without opening a WebSocket.
func New(config Config) (*Transformer, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("dashscope realtime: client is required")
	}
	config.Modalities = append([]string(nil), config.Modalities...)
	config.Temperature = cloneFloat64(config.Temperature)
	config.MaxOutputTokens = cloneInt(config.MaxOutputTokens)
	config.EnableASR = cloneBool(config.EnableASR)
	if config.TurnDetection != nil {
		turnDetection := *config.TurnDetection
		config.TurnDetection = &turnDetection
	}
	opts := make([]option, 0, 12)
	if config.Model != "" {
		opts = append(opts, withModel(config.Model))
	}
	if config.Voice != "" {
		opts = append(opts, withVoice(config.Voice))
	}
	if config.Instructions != "" {
		opts = append(opts, withInstructions(config.Instructions))
	}
	if config.Modalities != nil {
		opts = append(opts, withModalities(append([]string(nil), config.Modalities...)))
	}
	if config.VAD != "" {
		opts = append(opts, withVAD(config.VAD))
	}
	if config.Temperature != nil {
		opts = append(opts, withTemperature(*config.Temperature))
	}
	if config.MaxOutputTokens != nil {
		opts = append(opts, withMaxOutputTokens(*config.MaxOutputTokens))
	}
	if config.EnableASR != nil {
		opts = append(opts, withEnableASR(*config.EnableASR))
	}
	if config.ASRModel != "" {
		opts = append(opts, withASRModel(config.ASRModel))
	}
	if config.TurnDetection != nil {
		opts = append(opts, withTurnDetection(config.TurnDetection))
	}
	if config.InputAudioFormat != "" {
		opts = append(opts, withInputAudioFormat(config.InputAudioFormat))
	}
	if config.OutputAudioFormat != "" {
		opts = append(opts, withOutputAudioFormat(config.OutputAudioFormat))
	}
	return newTransformer(config.Client, opts...), nil
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
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
