package transformers

import (
	"context"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/minimax-go"
)

// MinimaxTTS is a TTS transformer using MiniMax text-to-speech API.
//
// Model: speech-02-hd (default)
//
// Input type: text/plain
// Output type: audio/* (audio/mpeg by default)
//
// EoS Handling:
//   - When receiving a text/plain EoS marker, finish synthesis, emit audio chunks, then emit audio/* EoS
//   - Non-text chunks are passed through unchanged
type MinimaxTTS struct {
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

var _ genx.Transformer = (*MinimaxTTS)(nil)

// MinimaxTTSOption is a functional option for MinimaxTTS.
type MinimaxTTSOption func(*MinimaxTTS)

// WithMinimaxTTSModel sets the model.
func WithMinimaxTTSModel(model string) MinimaxTTSOption {
	return func(t *MinimaxTTS) {
		t.model = model
	}
}

// WithMinimaxTTSSpeed sets the speech speed (0.5-2.0).
func WithMinimaxTTSSpeed(speed float64) MinimaxTTSOption {
	return func(t *MinimaxTTS) {
		t.speed = speed
	}
}

// WithMinimaxTTSVolume sets the volume (0-10).
func WithMinimaxTTSVolume(vol float64) MinimaxTTSOption {
	return func(t *MinimaxTTS) {
		t.vol = vol
	}
}

// WithMinimaxTTSPitch sets the pitch adjustment (-12 to 12).
func WithMinimaxTTSPitch(pitch int) MinimaxTTSOption {
	return func(t *MinimaxTTS) {
		t.pitch = pitch
	}
}

// WithMinimaxTTSEmotion sets the emotion.
// Options: happy, sad, angry, fearful, disgusted, surprised, neutral
func WithMinimaxTTSEmotion(emotion string) MinimaxTTSOption {
	return func(t *MinimaxTTS) {
		t.emotion = emotion
	}
}

// WithMinimaxTTSFormat sets the audio format.
// Options: mp3, pcm, flac, wav
func WithMinimaxTTSFormat(format string) MinimaxTTSOption {
	return func(t *MinimaxTTS) {
		t.format = format
	}
}

// WithMinimaxTTSSampleRate sets the sample rate.
// Options: 8000, 16000, 22050, 24000, 32000, 44100
func WithMinimaxTTSSampleRate(sampleRate int) MinimaxTTSOption {
	return func(t *MinimaxTTS) {
		t.sampleRate = sampleRate
	}
}

// WithMinimaxTTSBitrate sets the bitrate.
// Options: 32000, 64000, 128000, 256000
func WithMinimaxTTSBitrate(bitrate int) MinimaxTTSOption {
	return func(t *MinimaxTTS) {
		t.bitrate = bitrate
	}
}

// NewMinimaxTTS creates a new MinimaxTTS transformer.
//
// Parameters:
//   - client: MiniMax client
//   - voiceID: Voice identifier (e.g., "female-shaonv", "male-qn-qingse")
//   - opts: Optional configuration
func NewMinimaxTTS(client *minimax.Client, voiceID string, opts ...MinimaxTTSOption) *MinimaxTTS {
	t := &MinimaxTTS{
		client:     client,
		model:      "speech-2.6-hd",
		voiceID:    voiceID,
		speed:      1.0,
		vol:        1.0,
		format:     "mp3",
		sampleRate: 32000,
		bitrate:    128000,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// MinimaxTTSCtxKey is the context key for runtime options.
type minimaxTTSCtxKey struct{}

// MinimaxTTSCtxOptions are runtime options passed via context.
// TODO: Add fields as needed for runtime configuration.
type MinimaxTTSCtxOptions struct{}

// WithMinimaxTTSCtxOptions attaches runtime options to context.
func WithMinimaxTTSCtxOptions(ctx context.Context, opts MinimaxTTSCtxOptions) context.Context {
	return context.WithValue(ctx, minimaxTTSCtxKey{}, opts)
}

// Transform converts Text chunks to audio Blob chunks.
// MinimaxTTS does not require connection setup, so it returns immediately.
// The ctx is unused (no initialization needed); the goroutine lifetime
// is governed by the input Stream.
func (t *MinimaxTTS) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	output := newBufferStream(100)

	go runTTSTransform(ctx, input, output, t.mimeType(), t.synthesize)

	return output, nil
}

func (t *MinimaxTTS) synthesize(ctx context.Context, text string, meta ttsChunkMeta, mimeType string, output *bufferStream) error {
	speed := t.speed
	vol := t.vol
	pitch := t.pitch

	stream, err := t.client.Speech.OpenStream(ctx, minimax.SpeechStreamRequest{
		Model:   t.model,
		Text:    text,
		VoiceID: t.voiceID,
		Speed:   &speed,
		Vol:     &vol,
		Pitch:   &pitch,
	})
	if err != nil {
		return err
	}
	defer stream.Close()

	normalizer := newTTSAudioNormalizer(mimeType)
	for {
		chunk, err := stream.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if len(chunk.Audio) > 0 {
			if err := pushTTSAudioChunk(output, meta, mimeType, normalizer.Write(chunk.Audio)); err != nil {
				return err
			}
		}

		if chunk.Done {
			break
		}
	}
	if err := pushTTSAudioChunk(output, meta, mimeType, normalizer.Flush()); err != nil {
		return err
	}
	return nil
}

func (t *MinimaxTTS) mimeType() string {
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
