package doubaotts

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/audiostream"
)

// ICLV2 is a TTS transformer using Doubao seed-icl-2.0 (声音复刻 2.0).
//
// Resource ID: seed-icl-2.0
//
// Speaker: Custom cloned voice ID starting with "S_" (e.g., "S_xxxxxx")
//
// Input type: text/plain
// Output type: audio/* (audio/ogg by default)
//
// EoS Handling:
//   - When receiving a text/plain EoS marker, finish synthesis, emit audio chunks, then emit audio/* EoS
//   - Non-text chunks are passed through unchanged
type ICLV2 struct {
	client      *doubaospeech.Client
	speaker     string
	format      string
	sampleRate  int
	bitRate     int
	speedRatio  float64
	volumeRatio float64
	pitchRatio  float64
	emotion     string
	language    string
}

var _ genx.Transformer = (*ICLV2)(nil)

// ICLV2Config contains immutable voice-clone configuration.
type ICLV2Config struct {
	Client      *doubaospeech.Client
	Speaker     string
	Format      string
	SampleRate  int
	BitRate     int
	SpeedRatio  float64
	VolumeRatio float64
	PitchRatio  float64
	Emotion     string
	Language    string
}

// NewICLV2 creates a configured voice-clone Transformer without opening a
// provider connection.
func NewICLV2(config ICLV2Config) (*ICLV2, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("doubaotts: icl v2 client is required")
	}
	if strings.TrimSpace(config.Speaker) == "" {
		return nil, fmt.Errorf("doubaotts: icl v2 speaker is required")
	}
	return &ICLV2{
		client:      config.Client,
		speaker:     strings.TrimSpace(config.Speaker),
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
// ICLV2 does not require connection setup, so it returns immediately.
// The context governs provider work and the invocation-local output lifetime.
func (t *ICLV2) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return streamkit.NewTTSStream(ctx, input, streamkit.OutputConfig{InitialCapacity: 100}, t.mimeType(), t.synthesize), nil
}

func (t *ICLV2) synthesize(ctx context.Context, text string, _ streamkit.TTSMeta, mimeType string, emit func([]byte) error) error {
	format := t.format
	if format == "ogg" {
		format = string(doubaospeech.FormatOGG)
	}

	req := &doubaospeech.TTSV2Request{
		Text:       text,
		Speaker:    t.speaker,
		ResourceID: doubaospeech.ResourceVoiceCloneV2,
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
	for chunk, err := range t.client.TTSV2.Stream(ctx, req) {
		if err != nil {
			return err
		}

		if chunk.Audio != nil && len(chunk.Audio) > 0 {
			if err := emit(normalizer.Normalize(chunk.Audio)); err != nil {
				return err
			}
		}
	}
	if err := emit(normalizer.Flush()); err != nil {
		return err
	}
	return nil
}

func (t *ICLV2) mimeType() string {
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
