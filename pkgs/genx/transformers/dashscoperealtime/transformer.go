package dashscoperealtime

import (
	"context"
	"fmt"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	legacy "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/agentkit"
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

// Transformer adapts DashScope realtime sessions to the GenX contract.
type Transformer struct {
	config      Config
	newDelegate func() genx.Transformer
}

var _ genx.Transformer = (*Transformer)(nil)

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
	return &Transformer{config: config}, nil
}

func (t *Transformer) delegate() genx.Transformer {
	if t.newDelegate != nil {
		return t.newDelegate()
	}
	config := t.config
	opts := make([]legacy.DashScopeRealtimeOption, 0, 12)
	if config.Model != "" {
		opts = append(opts, legacy.WithDashScopeRealtimeModel(config.Model))
	}
	if config.Voice != "" {
		opts = append(opts, legacy.WithDashScopeRealtimeVoice(config.Voice))
	}
	if config.Instructions != "" {
		opts = append(opts, legacy.WithDashScopeRealtimeInstructions(config.Instructions))
	}
	if config.Modalities != nil {
		opts = append(opts, legacy.WithDashScopeRealtimeModalities(append([]string(nil), config.Modalities...)))
	}
	if config.VAD != "" {
		opts = append(opts, legacy.WithDashScopeRealtimeVAD(config.VAD))
	}
	if config.Temperature != nil {
		opts = append(opts, legacy.WithDashScopeRealtimeTemperature(*config.Temperature))
	}
	if config.MaxOutputTokens != nil {
		opts = append(opts, legacy.WithDashScopeRealtimeMaxOutputTokens(*config.MaxOutputTokens))
	}
	if config.EnableASR != nil {
		opts = append(opts, legacy.WithDashScopeRealtimeEnableASR(*config.EnableASR))
	}
	if config.ASRModel != "" {
		opts = append(opts, legacy.WithDashScopeRealtimeASRModel(config.ASRModel))
	}
	if config.TurnDetection != nil {
		opts = append(opts, legacy.WithDashScopeRealtimeTurnDetection(config.TurnDetection))
	}
	if config.InputAudioFormat != "" {
		opts = append(opts, legacy.WithDashScopeRealtimeInputAudioFormat(config.InputAudioFormat))
	}
	if config.OutputAudioFormat != "" {
		opts = append(opts, legacy.WithDashScopeRealtimeOutputAudioFormat(config.OutputAudioFormat))
	}
	return legacy.NewDashScopeRealtime(config.Client, opts...)
}

// Transform starts one independent DashScope WebSocket for this invocation.
func (t *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t == nil || t.config.Client == nil && t.newDelegate == nil {
		return nil, fmt.Errorf("dashscope realtime: transformer is not initialized")
	}
	output, err := t.delegate().Transform(ctx, input)
	if err != nil {
		return nil, err
	}
	return agentkit.NewResponseStream(output)
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
