package doubaorealtimeduplex

import (
	"encoding/json"
	"fmt"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
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
	opts := make([]option, 0, 14)
	if config.Speaker != "" {
		opts = append(opts, withSpeaker(config.Speaker))
	}
	if config.Format != "" {
		opts = append(opts, withFormat(config.Format))
	}
	if config.SampleRate != 0 {
		opts = append(opts, withSampleRate(config.SampleRate))
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
	if config.Model != "" {
		opts = append(opts, withModel(config.Model))
	}
	if config.SessionID != "" {
		opts = append(opts, withSessionID(config.SessionID))
	}
	if config.Instructions != "" {
		opts = append(opts, withInstructions(config.Instructions))
	}
	if config.OutputSpeed != nil {
		opts = append(opts, withOutputSpeed(*config.OutputSpeed))
	}
	if config.OutputLoudness != nil {
		opts = append(opts, withOutputLoudness(*config.OutputLoudness))
	}
	if config.Extension != nil {
		opts = append(opts, withExtension(config.Extension))
	}
	return newTransformer(config.Client, opts...), nil
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
