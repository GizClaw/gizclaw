package minimaxtts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/audiostream"
	"github.com/GizClaw/minimax-go"
)

const (
	// FormatOggOpus selects locally encoded Ogg/Opus output. MiniMax does not
	// offer an Opus container, so the Transformer requests raw PCM at an
	// Opus-compatible sample rate and encodes each synthesized segment itself.
	FormatOggOpus = "ogg_opus"

	defaultOggOpusSampleRate = 24000
	providerFormatPCM        = "pcm"
)

// Transformer is a TTS transformer using MiniMax text-to-speech API.
//
// Model: speech-2.6-hd (default)
//
// Input type: text/plain
// Output type: audio/* (audio/mpeg by default)
//
// Format "ogg_opus" (alias "ogg") is not a MiniMax provider format: the
// Transformer requests PCM from MiniMax and encodes Ogg/Opus locally so the
// output matches what Volc voices return for audio/ogg. PCM is requested at the
// configured sample rate when Opus supports it (8, 16, or 24 kHz); other
// MiniMax rates fall back to 24 kHz mono.
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
		format:     normalizeFormat(config.Format),
		sampleRate: positiveDefault(config.SampleRate, 32000),
		bitrate:    positiveDefault(config.BitRate, 128000),
	}
}

func normalizeFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "":
		return "mp3"
	case "ogg", FormatOggOpus:
		return FormatOggOpus
	default:
		return format
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
	providerFormat := t.format
	var encoder *oggOpusSegmentEncoder
	if t.format == FormatOggOpus {
		providerFormat = providerFormatPCM
		sampleRate = t.oggOpusSampleRate()
		var err error
		encoder, err = newOggOpusSegmentEncoder(sampleRate, emit)
		if err != nil {
			return err
		}
		defer encoder.discard()
	}

	stream, err := t.client.Speech.OpenWebSocket(ctx, minimax.SpeechWebSocketRequest{
		Model:   t.model,
		Text:    text,
		VoiceID: t.voiceID,
		Speed:   &speed,
		Vol:     &vol,
		Pitch:   &pitch,
		Emotion: t.emotion,
		AudioSetting: &minimax.SpeechAudioSetting{
			Format:     providerFormat,
			SampleRate: &sampleRate,
			Bitrate:    &bitrate,
		},
	})
	if err != nil {
		return err
	}
	defer stream.Close()

	emitAudio := emit
	if encoder != nil {
		emitAudio = encoder.write
	}
	normalizer := audiostream.NewNormalizer(mimeType)
	for {
		chunk, nextErr := stream.Next(ctx)
		if nextErr != nil {
			if nextErr == io.EOF {
				break
			}
			return nextErr
		}
		if len(chunk.Audio) > 0 {
			if err := emitAudio(normalizer.Normalize(chunk.Audio)); err != nil {
				return err
			}
		}
		if chunk.Done {
			break
		}
	}
	if err := emitAudio(normalizer.Flush()); err != nil {
		return err
	}
	if encoder != nil {
		return encoder.finish()
	}
	return nil
}

// oggOpusSampleRate picks the PCM sample rate requested from MiniMax when the
// output is Ogg/Opus. Opus only encodes 8/12/16/24/48 kHz and MiniMax only
// offers 8/16/22.05/24/32/44.1 kHz, so the intersection is honored and every
// other configured rate falls back to 24 kHz.
func (t *Transformer) oggOpusSampleRate() int {
	switch t.sampleRate {
	case 8000, 16000, 24000:
		return t.sampleRate
	default:
		return defaultOggOpusSampleRate
	}
}

// oggOpusSegmentEncoder turns one synthesized PCM16LE mono segment into a
// self-contained Ogg/Opus stream and forwards encoded bytes incrementally.
type oggOpusSegmentEncoder struct {
	buffer  bytes.Buffer
	encoder *codecconv.PCMToOggOpusEncoder
	emit    func([]byte) error
	wrote   bool
	done    bool
}

func newOggOpusSegmentEncoder(sampleRate int, emit func([]byte) error) (*oggOpusSegmentEncoder, error) {
	segment := &oggOpusSegmentEncoder{emit: emit}
	encoder, err := codecconv.NewPCMToOggOpusEncoder(&segment.buffer, sampleRate, 1, opus.ApplicationVoIP)
	if err != nil {
		return nil, fmt.Errorf("minimaxtts: create ogg opus encoder: %w", err)
	}
	segment.encoder = encoder
	return segment, nil
}

func (s *oggOpusSegmentEncoder) write(pcm []byte) error {
	if s == nil || len(pcm) == 0 {
		return nil
	}
	if _, err := s.encoder.Write(pcm); err != nil {
		return fmt.Errorf("minimaxtts: encode ogg opus: %w", err)
	}
	s.wrote = true
	return s.flush()
}

// finish closes the Ogg stream for the segment and forwards the trailing
// page. A segment without any PCM emits nothing so an empty provider result
// does not produce a dangling Ogg header.
func (s *oggOpusSegmentEncoder) finish() error {
	if s == nil || s.done {
		return nil
	}
	s.done = true
	if !s.wrote {
		return nil
	}
	if err := s.encoder.Close(); err != nil {
		return fmt.Errorf("minimaxtts: finish ogg opus: %w", err)
	}
	return s.flush()
}

func (s *oggOpusSegmentEncoder) discard() {
	if s == nil || s.done {
		return
	}
	s.done = true
	_ = s.encoder.Close()
}

func (s *oggOpusSegmentEncoder) flush() error {
	if s.buffer.Len() == 0 {
		return nil
	}
	data := append([]byte(nil), s.buffer.Bytes()...)
	s.buffer.Reset()
	return s.emit(data)
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
	case FormatOggOpus:
		return "audio/ogg"
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
