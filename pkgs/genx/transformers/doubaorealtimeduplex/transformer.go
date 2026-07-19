package doubaorealtimeduplex

import (
	"context"
	"encoding/json"
	"fmt"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	legacy "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/agentkit"
)

// Config contains immutable Doubao realtime Duplex dependencies and options.
// It does not configure tools or function-call execution.
type Config struct {
	Client          *doubaospeech.Client
	Speaker         string
	Format          string
	SampleRate      int
	InputFormat     string
	InputSampleRate int
	InputChannels   int
	InputTranscode  *bool
	Model           string
	SessionID       string
	Instructions    string
	OutputSpeed     *int
	OutputLoudness  *int
	Extension       *doubaospeech.RealtimeDuplexExtension
}

// Transformer adapts the Doubao realtime Duplex API to the GenX contract.
type Transformer struct {
	config      Config
	newDelegate func() genx.Transformer
}

var _ genx.Transformer = (*Transformer)(nil)

// New constructs a Duplex transformer without opening a WebSocket.
func New(config Config) (*Transformer, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("doubao realtime duplex: client is required")
	}
	config.InputTranscode = cloneBool(config.InputTranscode)
	config.OutputSpeed = cloneInt(config.OutputSpeed)
	config.OutputLoudness = cloneInt(config.OutputLoudness)
	if config.Extension != nil {
		extension, err := cloneExtension(config.Extension)
		if err != nil {
			return nil, err
		}
		config.Extension = extension
	}
	return &Transformer{config: config}, nil
}

func (t *Transformer) delegate() genx.Transformer {
	if t.newDelegate != nil {
		return t.newDelegate()
	}
	config := t.config
	opts := make([]legacy.DoubaoRealtimeDuplexOption, 0, 14)
	if config.Speaker != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexSpeaker(config.Speaker))
	}
	if config.Format != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexFormat(config.Format))
	}
	if config.SampleRate != 0 {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexSampleRate(config.SampleRate))
	}
	if config.InputFormat != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexInputFormat(config.InputFormat))
	}
	if config.InputSampleRate != 0 {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexInputSampleRate(config.InputSampleRate))
	}
	if config.InputChannels != 0 {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexInputChannels(config.InputChannels))
	}
	if config.InputTranscode != nil {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexInputTranscode(*config.InputTranscode))
	}
	if config.Model != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexModel(config.Model))
	}
	if config.SessionID != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexSessionID(config.SessionID))
	}
	if config.Instructions != "" {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexInstructions(config.Instructions))
	}
	if config.OutputSpeed != nil {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexOutputSpeed(*config.OutputSpeed))
	}
	if config.OutputLoudness != nil {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexOutputLoudness(*config.OutputLoudness))
	}
	if config.Extension != nil {
		opts = append(opts, legacy.WithDoubaoRealtimeDuplexExtension(config.Extension))
	}
	return legacy.NewDoubaoRealtimeDuplexRealtime(config.Client, opts...)
}

func cloneExtension(extension *doubaospeech.RealtimeDuplexExtension) (*doubaospeech.RealtimeDuplexExtension, error) {
	data, err := json.Marshal(extension)
	if err != nil {
		return nil, fmt.Errorf("doubao realtime duplex: encode extension: %w", err)
	}
	var clone doubaospeech.RealtimeDuplexExtension
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("doubao realtime duplex: decode extension: %w", err)
	}
	return &clone, nil
}

// Transform starts one independent Duplex WebSocket for this invocation.
func (t *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t == nil || t.config.Client == nil && t.newDelegate == nil {
		return nil, fmt.Errorf("doubao realtime duplex: transformer is not initialized")
	}
	output, err := t.delegate().Transform(ctx, input)
	if err != nil {
		return nil, err
	}
	return agentkit.NewResponseStream(output)
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
