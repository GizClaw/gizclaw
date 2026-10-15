package minimaxtts

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/audiostream"
	"github.com/GizClaw/minimax-go"
)

type bidiSession struct {
	ctx         context.Context
	cancel      context.CancelCauseFunc
	ws          *minimax.SpeechBidi
	done        chan struct{}
	pingDone    chan struct{}
	acks        chan string
	err         error // published by closing done
	transformer *Transformer
	mimeType    string
	emit        func([]byte) error
	timing      *synthesisTiming

	pendingSpace string // input-loop owned; whitespace-only provider events are discarded

	// The reader alone owns the encoder and container normalizer. A flush
	// seals one logical Ogg stream before the caller changes message identity.
	encoder    *oggOpusSegmentEncoder
	normalizer *audiostream.Normalizer
}

var _ streamkit.TTSStreamingSession = (*bidiSession)(nil)

func (t *Transformer) openBidi(ctx context.Context, meta streamkit.TTSMeta, emit func(context.Context, []byte) error) (streamkit.TTSStreamingSession, error) {
	meta.SegmentIndex = 1
	ctx, timing := newSynthesisTiming(ctx, meta, TransportBidi)
	ctx, cancel := context.WithCancelCause(ctx)
	slog.InfoContext(ctx, "minimax tts: open bidi", "model", t.model, "voice_id", t.voiceID, "language_boost", t.languageBoost)
	rate, bitrate := t.sampleRate, t.bitrate
	speed, volume, pitch := t.speed, t.vol, t.pitch
	format := t.format
	if format == FormatOggOpus {
		format, rate = providerFormatPCM, oggOpusSampleRate
	}
	ws, err := t.client.Speech.OpenBidi(ctx, minimax.SpeechBidiRequest{
		Model: t.model, VoiceID: t.voiceID, SessionID: meta.StreamID,
		Speed: &speed, Vol: &volume, Pitch: &pitch, Emotion: t.emotion,
		LanguageBoost: t.languageBoost,
		AudioSetting:  &minimax.SpeechAudioSetting{Format: format, SampleRate: &rate, Bitrate: &bitrate},
	})
	if err != nil {
		cancel(err)
		return nil, err
	}
	s := &bidiSession{
		ctx: ctx, cancel: cancel, ws: ws, done: make(chan struct{}), pingDone: make(chan struct{}),
		acks: make(chan string, 1), transformer: t, mimeType: t.mimeType(),
		timing: timing, normalizer: audiostream.NewNormalizer(t.mimeType()),
	}
	s.emit = func(data []byte) error {
		if len(data) > 0 {
			timing.mark(&timing.firstOutput)
		}
		return emit(ctx, data)
	}
	go s.read()
	go s.keepalive()
	return s, nil
}

func (s *bidiSession) SendText(ctx context.Context, text string) error {
	// This is a wire-size limit, not a sentence boundary. The provider
	// receives every rune and owns its cross-event sentence buffer.
	text = s.pendingSpace + text
	if strings.TrimSpace(text) == "" {
		if len(text) > 65536 {
			return fmt.Errorf("minimaxtts: pending bidi whitespace exceeds 64 KiB")
		}
		s.pendingSpace = text
		return nil
	}
	s.pendingSpace = ""
	runes := []rune(text)
	for offset := 0; offset < len(runes); offset += 10000 {
		if err := s.ws.SendText(ctx, string(runes[offset:min(offset+10000, len(runes))])); err != nil {
			return err
		}
	}
	return nil
}

func (s *bidiSession) Flush(ctx context.Context) error {
	s.pendingSpace = ""
	if err := s.ws.Flush(ctx); err != nil {
		return err
	}
	select {
	case event := <-s.acks:
		if event != "task_flushed" {
			return fmt.Errorf("minimaxtts: unexpected bidi acknowledgement %q", event)
		}
		return nil
	case <-s.done:
		if s.err != nil {
			return s.err
		}
		return io.ErrUnexpectedEOF
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (s *bidiSession) Finish(ctx context.Context) error {
	s.pendingSpace = ""
	if err := s.ws.Finish(ctx); err != nil {
		return err
	}
	select {
	case <-s.done:
		return s.err
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (s *bidiSession) Done() <-chan struct{} { return s.done }
func (s *bidiSession) Err() error            { return s.err }

func (s *bidiSession) Close() error {
	s.cancel(context.Canceled)
	err := s.ws.Close()
	<-s.done
	<-s.pingDone
	return err
}

func (s *bidiSession) keepalive() {
	defer close(s.pingDone)
	ticker := time.NewTicker(40 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// A delivery wait may delay pong processing. Keep this governed
			// by the session context so backpressure cannot trigger an
			// artificial short timeout while cancellation still releases IO.
			if err := s.ws.Ping(s.ctx); err != nil {
				s.cancel(err)
				_ = s.ws.Close()
				return
			}
		}
	}
}

func (s *bidiSession) read() {
	defer close(s.done)
	defer s.cancel(context.Canceled)
	defer s.ws.Close()
	defer func() {
		if s.encoder != nil {
			s.encoder.discard()
		}
	}()
	for {
		event, err := s.ws.Next(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				err = context.Cause(s.ctx)
			}
			s.err = err
			return
		}
		if event.UsageCharacters != nil {
			genx.RecordUsage(s.ctx, genx.UsageRecord{
				Provider: "minimax", Model: s.transformer.model, Modality: genx.UsageModalityText,
				Unit: genx.UsageUnitCharacter, Input: *event.UsageCharacters,
			})
		}
		if len(event.Audio) > 0 {
			s.timing.mark(&s.timing.firstProviderAudio)
			started := time.Now()
			if err := s.emitAudio(s.normalizer.Normalize(event.Audio)); err != nil {
				s.err = err
				return
			}
			s.timing.logFirst(s.ctx, time.Since(started))
		}
		if event.Event == "task_flushed" || event.Done {
			if err := s.finishAudio(); err != nil {
				s.err = err
				return
			}
		}
		if event.Done {
			return
		}
		if event.Event == "task_flushed" {
			select {
			case s.acks <- event.Event:
			case <-s.ctx.Done():
				s.err = context.Cause(s.ctx)
				return
			}
		}
	}
}

func (s *bidiSession) emitAudio(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if s.transformer.format != FormatOggOpus {
		return s.emit(data)
	}
	if s.encoder == nil {
		encoder, err := newOggOpusSegmentEncoder(s.emit, s.transformer.nextOggSerial())
		if err != nil {
			return err
		}
		s.encoder = encoder
	}
	return s.encoder.write(data)
}

func (s *bidiSession) finishAudio() error {
	if err := s.emitAudio(s.normalizer.Flush()); err != nil {
		return err
	}
	if s.encoder != nil {
		if err := s.encoder.finish(); err != nil {
			return err
		}
		s.encoder = nil
	}
	s.normalizer = audiostream.NewNormalizer(s.mimeType)
	return nil
}
