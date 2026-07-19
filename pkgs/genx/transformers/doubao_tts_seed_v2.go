package transformers

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// DoubaoTTSSeedV2 is a TTS transformer using Doubao seed-tts-2.0 (大模型 TTS 2.0).
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
type DoubaoTTSSeedV2 struct {
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

var _ genx.Transformer = (*DoubaoTTSSeedV2)(nil)

// DoubaoTTSSeedV2Option is a functional option for DoubaoTTSSeedV2.
type DoubaoTTSSeedV2Option func(*DoubaoTTSSeedV2)

// WithDoubaoTTSSeedV2Format sets the audio format (pcm, mp3, ogg_opus).
func WithDoubaoTTSSeedV2Format(format string) DoubaoTTSSeedV2Option {
	return func(t *DoubaoTTSSeedV2) {
		t.format = format
	}
}

// WithDoubaoTTSSeedV2SampleRate sets the sample rate (8000, 16000, 24000, 32000).
func WithDoubaoTTSSeedV2SampleRate(sampleRate int) DoubaoTTSSeedV2Option {
	return func(t *DoubaoTTSSeedV2) {
		t.sampleRate = sampleRate
	}
}

// WithDoubaoTTSSeedV2BitRate sets the bit rate for mp3 (32000, 64000, 128000).
func WithDoubaoTTSSeedV2BitRate(bitRate int) DoubaoTTSSeedV2Option {
	return func(t *DoubaoTTSSeedV2) {
		t.bitRate = bitRate
	}
}

// WithDoubaoTTSSeedV2Speed sets the speech speed ratio (0.2-3.0).
func WithDoubaoTTSSeedV2Speed(speedRatio float64) DoubaoTTSSeedV2Option {
	return func(t *DoubaoTTSSeedV2) {
		t.speedRatio = speedRatio
	}
}

// WithDoubaoTTSSeedV2Volume sets the volume ratio (0.1-3.0).
func WithDoubaoTTSSeedV2Volume(volumeRatio float64) DoubaoTTSSeedV2Option {
	return func(t *DoubaoTTSSeedV2) {
		t.volumeRatio = volumeRatio
	}
}

// WithDoubaoTTSSeedV2Pitch sets the pitch ratio (0.1-3.0).
func WithDoubaoTTSSeedV2Pitch(pitchRatio float64) DoubaoTTSSeedV2Option {
	return func(t *DoubaoTTSSeedV2) {
		t.pitchRatio = pitchRatio
	}
}

// WithDoubaoTTSSeedV2Emotion sets the emotion (happy, sad, angry, fear, hate, surprise).
func WithDoubaoTTSSeedV2Emotion(emotion string) DoubaoTTSSeedV2Option {
	return func(t *DoubaoTTSSeedV2) {
		t.emotion = emotion
	}
}

// WithDoubaoTTSSeedV2Language sets the language (zh, en, ja, etc.).
func WithDoubaoTTSSeedV2Language(language string) DoubaoTTSSeedV2Option {
	return func(t *DoubaoTTSSeedV2) {
		t.language = language
	}
}

// WithDoubaoTTSSeedV2ResourceID sets the Volcengine speech resource ID.
func WithDoubaoTTSSeedV2ResourceID(resourceID string) DoubaoTTSSeedV2Option {
	return func(t *DoubaoTTSSeedV2) {
		t.resourceID = strings.TrimSpace(resourceID)
	}
}

// NewDoubaoTTSSeedV2 creates a new DoubaoTTSSeedV2 transformer.
//
// Parameters:
//   - client: Doubao speech client
//   - speaker: Voice type (e.g., "zh_female_cancan")
//   - opts: Optional configuration
func NewDoubaoTTSSeedV2(client *doubaospeech.Client, speaker string, opts ...DoubaoTTSSeedV2Option) *DoubaoTTSSeedV2 {
	t := &DoubaoTTSSeedV2{
		client:      client,
		speaker:     speaker,
		resourceID:  doubaospeech.ResourceTTSV2,
		format:      "ogg_opus",
		sampleRate:  24000,
		speedRatio:  1.0,
		volumeRatio: 1.0,
		pitchRatio:  1.0,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// DoubaoTTSSeedV2CtxKey is the context key for runtime options.
type doubaoTTSSeedV2CtxKey struct{}

// DoubaoTTSSeedV2CtxOptions are runtime options passed via context.
// TODO: Add fields as needed for runtime configuration.
type DoubaoTTSSeedV2CtxOptions struct{}

// WithDoubaoTTSSeedV2CtxOptions attaches runtime options to context.
func WithDoubaoTTSSeedV2CtxOptions(ctx context.Context, opts DoubaoTTSSeedV2CtxOptions) context.Context {
	return context.WithValue(ctx, doubaoTTSSeedV2CtxKey{}, opts)
}

// Transform converts Text chunks to audio Blob chunks.
// DoubaoTTSSeedV2 does not require connection setup, so it returns immediately.
// The ctx is unused (no initialization needed); the goroutine lifetime
// is governed by the input Stream.
func (t *DoubaoTTSSeedV2) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	output := newBufferStream(100)

	go runTTSTransform(ctx, input, output, t.mimeType(), t.synthesize)

	return output, nil
}

func (t *DoubaoTTSSeedV2) synthesize(ctx context.Context, text string, meta ttsChunkMeta, mimeType string, output *bufferStream) error {
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

	normalizer := newTTSAudioNormalizer(mimeType)
	start := time.Now()
	firstAudio := false
	for chunk, err := range t.client.TTSV2.Stream(ctx, req) {
		if err != nil {
			return err
		}

		if chunk.Audio != nil && len(chunk.Audio) > 0 {
			audio := normalizer.Write(chunk.Audio)
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
			if err := pushTTSAudioChunk(output, meta, mimeType, audio); err != nil {
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
	if err := pushTTSAudioChunk(output, meta, mimeType, audio); err != nil {
		return err
	}
	return nil
}

func ratioToRate(r float64) int {
	if r == 0 {
		return 0
	}
	return int((r - 1.0) * 100)
}

func (t *DoubaoTTSSeedV2) mimeType() string {
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
