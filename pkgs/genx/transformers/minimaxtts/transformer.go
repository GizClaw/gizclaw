package minimaxtts

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/internal/streamkit"
	"github.com/GizClaw/minimax-go"
)

// Transformer is a TTS transformer using MiniMax text-to-speech API.
//
// Model: speech-2.6-hd (default)
//
// Input type: text/plain
// Output type: audio/* (audio/mpeg by default)
//
// EoS Handling:
//   - When receiving a text/plain EoS marker, finish synthesis, emit audio chunks, then emit audio/* EoS
//   - Non-text chunks are passed through unchanged
type Transformer struct {
	client     *minimax.Client
	model      string
	voiceID    string
	speed      float64
	vol        float64
	pitch      int
	emotion    string
	format     string
	sampleRate int
	bitrate    int
}

var _ genx.Transformer = (*Transformer)(nil)

// Config contains immutable MiniMax TTS configuration. Pointer numeric fields
// distinguish explicit zero values from defaults.
type Config struct {
	Client     *minimax.Client
	VoiceID    string
	Model      string
	Speed      *float64
	Volume     *float64
	Pitch      *int
	Emotion    string
	Format     string
	SampleRate int
	BitRate    int
}

// New creates a configured MiniMax Transformer without opening a provider
// connection.
func New(config Config) (*Transformer, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("minimaxtts: client is required")
	}
	if strings.TrimSpace(config.VoiceID) == "" {
		return nil, fmt.Errorf("minimaxtts: voice ID is required")
	}
	return newTransformer(config), nil
}

func newTransformer(config Config) *Transformer {
	return &Transformer{
		client:     config.Client,
		model:      stringDefault(config.Model, "speech-2.6-hd"),
		voiceID:    strings.TrimSpace(config.VoiceID),
		speed:      floatDefault(config.Speed, 1),
		vol:        floatDefault(config.Volume, 1),
		pitch:      intDefault(config.Pitch, 0),
		emotion:    config.Emotion,
		format:     stringDefault(config.Format, "mp3"),
		sampleRate: positiveDefault(config.SampleRate, 32000),
		bitrate:    positiveDefault(config.BitRate, 128000),
	}
}

// Transform converts Text chunks to audio Blob chunks.
// Transformer does not require connection setup, so it returns immediately.
// The context governs provider work and the invocation-local output lifetime.
func (t *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return streamkit.NewTTSStream(ctx, input, streamkit.OutputConfig{InitialCapacity: 100}, t.mimeType(), t.synthesize), nil
}

func (t *Transformer) synthesize(ctx context.Context, text string, _ streamkit.TTSMeta, mimeType string, emit func([]byte) error) error {
	speed := t.speed
	vol := t.vol
	pitch := t.pitch
	sampleRate := t.sampleRate
	bitrate := t.bitrate

	stream, err := t.client.Speech.OpenWebSocket(ctx, minimax.SpeechWebSocketRequest{
		Model:   t.model,
		Text:    text,
		VoiceID: t.voiceID,
		Speed:   &speed,
		Vol:     &vol,
		Pitch:   &pitch,
		Emotion: t.emotion,
		AudioSetting: &minimax.SpeechAudioSetting{
			Format:     t.format,
			SampleRate: &sampleRate,
			Bitrate:    &bitrate,
		},
	})
	if err != nil {
		return err
	}
	defer stream.Close()

	normalizer := codecconv.NewTTSAudioNormalizer(mimeType)
	for {
		chunk, nextErr := stream.Next(ctx)
		if nextErr != nil {
			if nextErr == io.EOF {
				break
			}
			return nextErr
		}
		if len(chunk.Audio) > 0 {
			if err := emit(normalizer.Write(chunk.Audio)); err != nil {
				return err
			}
		}
		if chunk.Done {
			break
		}
	}
	if err := emit(normalizer.Flush()); err != nil {
		return err
	}
	return nil
}

func stringDefault(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func floatDefault(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

func intDefault(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func positiveDefault(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func (t *Transformer) mimeType() string {
	switch t.format {
	case "mp3":
		return "audio/mpeg"
	case "pcm":
		return "audio/pcm"
	case "flac":
		return "audio/flac"
	case "wav":
		return "audio/wav"
	default:
		return "audio/mpeg"
	}
}
