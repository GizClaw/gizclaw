package doubaotts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/audiostream"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

// SeedV2 is a TTS transformer using Doubao seed-tts-2.0 (大模型 TTS 2.0).
//
// Resource ID: seed-tts-2.0
//
// Speaker examples:
//   - zh_female_cancan (灿灿)
//   - zh_male_xiaoming (小明)
//   - zh_female_shuangkuaisisi_moon_bigtts (双快丝丝)
//
// Input type: text/plain
// Output type: audio/* (audio/ogg by default)
//
// EoS Handling:
//   - When receiving a text/plain EoS marker, finish synthesis, emit audio chunks, then emit audio/* EoS
//   - Non-text chunks are passed through unchanged
type SeedV2 struct {
	client      *doubaospeech.Client
	speaker     string
	resourceID  string
	format      string
	sampleRate  int
	bitRate     int
	speedRatio  float64
	volumeRatio float64
	pitchRatio  float64
	emotion     string
	language    string
}

var _ genx.Transformer = (*SeedV2)(nil)

// SeedV2Config contains immutable Seed V2 configuration.
type SeedV2Config struct {
	Client      *doubaospeech.Client
	Speaker     string
	ResourceID  string
	Format      string
	SampleRate  int
	BitRate     int
	SpeedRatio  float64
	VolumeRatio float64
	PitchRatio  float64
	Emotion     string
	Language    string
}

// NewSeedV2 creates a configured Seed V2 Transformer without opening a
// provider connection.
func NewSeedV2(config SeedV2Config) (*SeedV2, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("doubaotts: seed v2 client is required")
	}
	if strings.TrimSpace(config.Speaker) == "" {
		return nil, fmt.Errorf("doubaotts: seed v2 speaker is required")
	}
	return &SeedV2{
		client:      config.Client,
		speaker:     strings.TrimSpace(config.Speaker),
		resourceID:  firstString(config.ResourceID, doubaospeech.ResourceTTSV2),
		format:      firstString(config.Format, "ogg_opus"),
		sampleRate:  firstInt(config.SampleRate, 24000),
		bitRate:     config.BitRate,
		speedRatio:  firstFloat(config.SpeedRatio, 1),
		volumeRatio: firstFloat(config.VolumeRatio, 1),
		pitchRatio:  firstFloat(config.PitchRatio, 1),
		emotion:     config.Emotion,
		language:    config.Language,
	}, nil
}

// Transform converts Text chunks to audio Blob chunks.
// SeedV2 does not require connection setup, so it returns immediately.
// The context governs provider work and the invocation-local output lifetime.
func (t *SeedV2) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return streamkit.NewTTSStream(ctx, input, streamkit.OutputConfig{InitialCapacity: 100}, t.mimeType(), t.synthesize), nil
}

func (t *SeedV2) synthesize(ctx context.Context, text string, meta streamkit.TTSMeta, mimeType string, emit func([]byte) error) error {
	format := t.format
	if format == "ogg" {
		format = string(doubaospeech.FormatOGG)
	}

	req := &doubaospeech.TTSV2Request{
		Text:       text,
		Speaker:    t.speaker,
		ResourceID: t.resourceID,
		Format:     doubaospeech.AudioFormat(format),
		SampleRate: doubaospeech.SampleRate(t.sampleRate),
		BitRate:    t.bitRate,
		SpeechRate: ratioToRate(t.speedRatio),
		VolumeRate: ratioToRate(t.volumeRatio),
		PitchRate:  ratioToRate(t.pitchRatio),
		Emotion:    t.emotion,
		Language:   t.language,
	}

	normalizer := audiostream.NewNormalizer(mimeType)
	start := time.Now()
	firstAudio := false
	for chunk, err := range t.client.TTSV2.Stream(ctx, req) {
		if err != nil {
			return err
		}

		if chunk.Audio != nil && len(chunk.Audio) > 0 {
			audio := normalizer.Normalize(chunk.Audio)
			if ttsDebugEnabled() && !firstAudio && len(audio) > 0 {
				firstAudio = true
				slog.Info(
					"doubao tts: first audio",
					"stream_id", meta.StreamID,
					"name", meta.Name,
					"runes", utf8.RuneCountInString(text),
					"elapsed", time.Since(start),
					"bytes", len(audio),
					"text", ttsDebugPreview(text, 120),
				)
			}
			if err := emit(audio); err != nil {
				return err
			}
		}
	}
	audio := normalizer.Flush()
	if ttsDebugEnabled() && !firstAudio && len(audio) > 0 {
		slog.Info(
			"doubao tts: first audio",
			"stream_id", meta.StreamID,
			"name", meta.Name,
			"runes", utf8.RuneCountInString(text),
			"elapsed", time.Since(start),
			"bytes", len(audio),
			"text", ttsDebugPreview(text, 120),
		)
	}
	if err := emit(audio); err != nil {
		return err
	}
	return nil
}

func firstString(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func firstInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func firstFloat(value, fallback float64) float64 {
	if value != 0 {
		return value
	}
	return fallback
}

func ratioToRate(r float64) int {
	if r == 0 {
		return 0
	}
	return int((r - 1.0) * 100)
}

func (t *SeedV2) mimeType() string {
	switch t.format {
	case "mp3":
		return "audio/mpeg"
	case "ogg_opus":
		return "audio/ogg"
	case "pcm":
		return "audio/pcm"
	default:
		return "audio/ogg"
	}
}
