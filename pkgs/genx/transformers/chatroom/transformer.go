package chatroom

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const (
	defaultInputStreamID = "audio"
	transcriptLabel      = "transcript"
)

var errASRInputConsumerClosed = errors.New("chatroom: ASR input consumer closed")

// InputMode controls whether ASR emits interim transcripts.
type InputMode string

const (
	InputModePushToTalk InputMode = "push-to-talk"
	InputModeRealtime   InputMode = "realtime"
)

// Config contains provider-neutral dependencies for one Chatroom Transformer.
type Config struct {
	ASR               genx.TransformerMux
	TranscriptEnabled bool
	ASRPattern        string
	InputMode         InputMode
}

// Transformer handles input routes and optional transcript streaming without
// importing GizClaw workspace or workflow contracts.
type Transformer struct {
	config Config
}

// New creates a reusable Chatroom Transformer without opening provider
// connections. Provider sessions remain invocation-local in ASR.
func New(config Config) (*Transformer, error) {
	config.ASRPattern = strings.TrimSpace(config.ASRPattern)
	if config.InputMode == "" {
		config.InputMode = InputModePushToTalk
	}
	if config.InputMode != InputModePushToTalk && config.InputMode != InputModeRealtime {
		return nil, fmt.Errorf("chatroom: unsupported input mode %q", config.InputMode)
	}
	if config.TranscriptEnabled {
		if config.ASRPattern == "" {
			return nil, fmt.Errorf("chatroom: transcript.asr_model is required when transcript is enabled")
		}
		if config.ASR == nil {
			return nil, fmt.Errorf("chatroom: transformer is required when transcript is enabled")
		}
	}
	return &Transformer{config: config}, nil
}

type asrInputTransport struct {
	builder         *genx.StreamBuilder
	onConsumerClose func(error)

	mu             sync.Mutex
	terminal       bool
	terminalErr    error
	completing     chan struct{}
	consumerEOS    bool
	consumerClosed bool
}

func newASRInputTransport(onConsumerClose func(error)) *asrInputTransport {
	return &asrInputTransport{
		builder:         genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64),
		onConsumerClose: onConsumerClose,
	}
}

func (t *asrInputTransport) Stream() genx.Stream {
	return &asrInputView{source: t.builder.Stream(), transport: t}
}

func (t *asrInputTransport) Add(chunks ...*genx.MessageChunk) error {
	terminal, terminalErr := t.status()
	if terminal {
		if terminalErr != nil {
			return terminalErr
		}
		return genx.ErrDone
	}
	if err := t.builder.Add(chunks...); err != nil {
		if terminalErr := t.failure(); terminalErr != nil {
			return terminalErr
		}
		return err
	}
	return nil
}

func (t *asrInputTransport) Done() error {
	t.mu.Lock()
	if t.terminal {
		err := t.terminalErr
		t.mu.Unlock()
		return err
	}
	if t.completing != nil {
		completing := t.completing
		t.mu.Unlock()
		<-completing
		return t.failure()
	}
	completing := make(chan struct{})
	t.completing = completing
	t.mu.Unlock()

	completionErr := t.builder.Done(genx.Usage{})

	t.mu.Lock()
	if !t.terminal {
		t.terminal = true
		t.terminalErr = completionErr
	}
	err := t.terminalErr
	t.completing = nil
	close(completing)
	t.mu.Unlock()
	return err
}

func (t *asrInputTransport) Abort(err error) error {
	_, closeErr := t.abort(err)
	return closeErr
}

func (t *asrInputTransport) abort(err error) (bool, error) {
	if err == nil {
		err = io.ErrClosedPipe
	}
	t.mu.Lock()
	if t.terminal {
		t.mu.Unlock()
		return false, nil
	}
	t.terminal = true
	t.terminalErr = err
	t.mu.Unlock()
	return true, t.builder.Abort(err)
}

func (t *asrInputTransport) status() (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.terminal || t.completing != nil, t.terminalErr
}

func (t *asrInputTransport) failure() error {
	_, err := t.status()
	return err
}

func (t *asrInputTransport) markConsumerEOS() {
	t.mu.Lock()
	t.consumerEOS = true
	t.mu.Unlock()
}

func (t *asrInputTransport) closeConsumer() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.terminal || t.consumerEOS || t.consumerClosed {
		return false
	}
	t.consumerClosed = true
	return true
}

type asrInputView struct {
	source    genx.Stream
	transport *asrInputTransport
}

func (s *asrInputView) Next() (*genx.MessageChunk, error) {
	chunk, err := s.source.Next()
	if chunk != nil && chunk.IsEndOfStream() {
		s.transport.markConsumerEOS()
	}
	return chunk, err
}

func (s *asrInputView) Close() error {
	if s.transport.closeConsumer() && s.transport.onConsumerClose != nil {
		s.transport.onConsumerClose(nil)
	}
	return nil
}

func (s *asrInputView) CloseWithError(err error) error {
	if err == nil || isStreamDone(err) {
		return s.Close()
	}
	first, closeErr := s.transport.abort(err)
	if first && s.transport.onConsumerClose != nil {
		s.transport.onConsumerClose(err)
	}
	return closeErr
}

func (a *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if input == nil {
		return nil, fmt.Errorf("chatroom: input stream is required")
	}
	builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	if a.config.TranscriptEnabled {
		go a.transcribeInput(ctx, input, builder)
	} else {
		go forwardTextInput(ctx, input, builder)
	}
	return builder.Stream(), nil
}

func forwardTextInput(ctx context.Context, input genx.Stream, builder *genx.StreamBuilder) {
	defer input.Close()
	streamID := defaultInputStreamID
	textOpen := false
	textStreamID := ""
	flushText := func() error {
		if !textOpen {
			return nil
		}
		if err := builder.Add(textChunk(textStreamID, "", true)); err != nil {
			return err
		}
		textOpen = false
		textStreamID = ""
		return nil
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = builder.Abort(err)
			return
		}
		chunk, err := input.Next()
		switch {
		case err == nil:
			if chunk == nil {
				continue
			}
			nextStreamID := streamID
			if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.StreamID) != "" {
				nextStreamID = strings.TrimSpace(chunk.Ctrl.StreamID)
			}
			if textOpen && textStreamID != "" && nextStreamID != textStreamID {
				if err := flushText(); err != nil {
					_ = builder.Abort(err)
					return
				}
			}
			streamID = nextStreamID
			text, ok := chunk.Part.(genx.Text)
			if ok && text != "" {
				textOpen = true
				textStreamID = streamID
				if err := builder.Add(textChunk(streamID, string(text), false)); err != nil {
					_ = builder.Abort(err)
					return
				}
			}
			if chunk.IsEndOfStream() && ok {
				if err := flushText(); err != nil {
					_ = builder.Abort(err)
					return
				}
			}
			continue
		case isStreamDone(err):
			if err := flushText(); err != nil {
				_ = builder.Abort(err)
				return
			}
			_ = builder.Done(genx.Usage{})
			return
		default:
			_ = builder.Abort(err)
			return
		}
	}
}

func (a *Transformer) transcribeInput(ctx context.Context, input genx.Stream, output *genx.StreamBuilder) {
	defer input.Close()
	stopInputCancel := context.AfterFunc(ctx, func() {
		_ = input.CloseWithError(ctx.Err())
	})
	defer stopInputCancel()
	var asrInput *asrInputTransport
	var asr genx.Stream
	var readDone chan error
	var stopASRInputCancel func() bool
	defer func() {
		if stopASRInputCancel != nil {
			stopASRInputCancel()
		}
	}()
	var closeASROnce sync.Once
	streamID := &lockedString{value: defaultInputStreamID}
	textOpen := false
	textStreamID := ""
	flushText := func() error {
		if !textOpen {
			return nil
		}
		if err := output.Add(textChunk(textStreamID, "", true)); err != nil {
			return err
		}
		textOpen = false
		textStreamID = ""
		return nil
	}
	startASR := func() error {
		if readDone != nil {
			return nil
		}
		asrInput = newASRInputTransport(func(err error) {
			if err == nil {
				_ = asrInput.Abort(errASRInputConsumerClosed)
				_ = input.Close()
				return
			}
			_ = input.CloseWithError(err)
		})
		asrInputStream := asrInput
		stopASRInputCancel = context.AfterFunc(ctx, func() {
			_ = asrInputStream.Abort(ctx.Err())
		})
		var err error
		asr, err = a.config.ASR.Transform(ctx, a.asrPattern(), asrInput.Stream())
		if err != nil {
			err = fmt.Errorf("chatroom: start ASR: %w", err)
			_ = asrInput.Abort(err)
			return err
		}
		asrStream := asr
		done := make(chan error, 1)
		readDone = done
		go func() {
			err := readTranscript(ctx, asrStream, output, streamID)
			if err != nil && ctx.Err() == nil {
				_ = asrInput.Abort(err)
				_ = input.CloseWithError(err)
			}
			done <- err
		}()
		return nil
	}
	drainTranscript := func() {
		if readDone == nil {
			return
		}
		done := readDone
		readDone = nil
		<-done
	}
	closeASR := func(err error) {
		closeASROnce.Do(func() {
			if asr == nil {
				return
			}
			if err != nil {
				_ = asr.CloseWithError(err)
				return
			}
			_ = asr.Close()
		})
	}
	waitTranscript := func() error {
		if readDone == nil {
			return nil
		}
		done := readDone
		readDone = nil
		select {
		case err := <-done:
			closeASR(err)
			return err
		case <-ctx.Done():
			closeASR(ctx.Err())
			<-done
			return ctx.Err()
		}
	}
	fail := func(err error) {
		if asrInput != nil {
			_ = asrInput.Abort(err)
		}
		closeASR(err)
		_ = output.Abort(err)
		drainTranscript()
	}
	finish := func() {
		if err := waitTranscript(); err != nil {
			fail(err)
			return
		}
		_ = output.Done(genx.Usage{})
	}

	audioSeen := false
	for {
		if err := ctx.Err(); err != nil {
			fail(err)
			return
		}
		chunk, err := input.Next()
		if ctxErr := ctx.Err(); ctxErr != nil {
			fail(ctxErr)
			return
		}
		if asrInput != nil {
			if asrErr := asrInput.failure(); asrErr != nil {
				if errors.Is(asrErr, errASRInputConsumerClosed) {
					finish()
					return
				}
				fail(asrErr)
				return
			}
		}
		if err != nil {
			if !isStreamDone(err) {
				fail(err)
				return
			}
			if err := flushText(); err != nil {
				fail(err)
				return
			}
			if !audioSeen {
				_ = output.Done(genx.Usage{})
				return
			}
			if err := asrInput.Done(); err != nil {
				if errors.Is(err, errASRInputConsumerClosed) {
					finish()
					return
				}
				fail(err)
				return
			}
			finish()
			return
		}
		if chunk == nil {
			continue
		}
		nextStreamID := streamID.Get()
		if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.StreamID) != "" {
			nextStreamID = strings.TrimSpace(chunk.Ctrl.StreamID)
		}
		if textOpen && textStreamID != "" && nextStreamID != textStreamID {
			if err := flushText(); err != nil {
				_ = output.Abort(err)
				return
			}
		}
		streamID.Set(nextStreamID)
		if text, ok := chunk.Part.(genx.Text); ok {
			if text != "" {
				textOpen = true
				textStreamID = streamID.Get()
				if err := output.Add(textChunk(streamID.Get(), string(text), false)); err != nil {
					_ = output.Abort(err)
					return
				}
			}
			if chunk.IsEndOfStream() {
				if err := flushText(); err != nil {
					_ = output.Abort(err)
					return
				}
			}
			continue
		}
		if !isAudioChunk(chunk) {
			continue
		}
		audioSeen = true
		if err := startASR(); err != nil {
			fail(err)
			return
		}
		next := chunk.Clone()
		if next.Ctrl == nil {
			next.Ctrl = &genx.StreamCtrl{}
		}
		if strings.TrimSpace(next.Ctrl.StreamID) == "" {
			next.Ctrl.StreamID = streamID.Get()
		}
		if err := asrInput.Add(next); err != nil {
			if errors.Is(err, errASRInputConsumerClosed) {
				finish()
				return
			}
			fail(err)
			return
		}
		if chunk.IsEndOfStream() {
			if err := asrInput.Done(); err != nil {
				if errors.Is(err, errASRInputConsumerClosed) {
					finish()
					return
				}
				fail(err)
				return
			}
			finish()
			return
		}
	}
}

func (a *Transformer) asrPattern() string {
	pattern := a.config.ASRPattern
	if a.config.InputMode == InputModeRealtime {
		pattern += "?emit_interim=true"
	}
	return pattern
}

func readTranscript(ctx context.Context, asr genx.Stream, output *genx.StreamBuilder, streamID *lockedString) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := asr.Next()
		if err != nil {
			if isStreamDone(err) {
				return nil
			}
			return fmt.Errorf("chatroom: read ASR: %w", err)
		}
		if chunk == nil {
			continue
		}
		next := normalizeASRTranscriptChunk(chunk, streamID.Get())
		if next == nil {
			continue
		}
		if err := output.Add(next); err != nil {
			return err
		}
	}
}

func normalizeASRTranscriptChunk(chunk *genx.MessageChunk, fallbackStreamID string) *genx.MessageChunk {
	if chunk == nil {
		return nil
	}
	next := chunk.Clone()
	if next.Ctrl == nil {
		next.Ctrl = &genx.StreamCtrl{}
	}
	if strings.TrimSpace(next.Ctrl.StreamID) == "" {
		next.Ctrl.StreamID = strings.TrimSpace(fallbackStreamID)
	}
	if strings.TrimSpace(next.Ctrl.StreamID) == "" {
		next.Ctrl.StreamID = defaultInputStreamID
	}
	if next.Role == "" {
		next.Role = genx.RoleUser
	}
	if strings.TrimSpace(next.Name) == "" {
		next.Name = transcriptLabel
	}
	if strings.TrimSpace(next.Ctrl.Label) == "" {
		next.Ctrl.Label = transcriptLabel
	}
	if strings.TrimSpace(next.Ctrl.Label) == genx.HistoryUserAudioLabel {
		next.Role = genx.RoleUser
		if strings.TrimSpace(next.Name) == "" {
			next.Name = transcriptLabel
		}
		return next
	}
	if next.IsBeginOfStream() {
		return next
	}
	text, hasText := next.Part.(genx.Text)
	if hasText && text != "" {
		return next
	}
	if next.IsEndOfStream() {
		if !hasText {
			next.Part = genx.Text("")
		}
		return next
	}
	return nil
}

func textChunk(streamID, text string, eos bool) *genx.MessageChunk {
	if strings.TrimSpace(streamID) == "" {
		streamID = defaultInputStreamID
	}
	return &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: transcriptLabel,
		Part: genx.Text(text),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: transcriptLabel, EndOfStream: eos},
	}
}

func isAudioChunk(chunk *genx.MessageChunk) bool {
	if chunk == nil {
		return false
	}
	blob, ok := chunk.Part.(*genx.Blob)
	return ok && strings.HasPrefix(baseMIME(blob.MIMEType), "audio/")
}

func baseMIME(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}

func isStreamDone(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone)
}

type lockedString struct {
	mu    sync.RWMutex
	value string
}

func (s *lockedString) Set(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value = value
}

func (s *lockedString) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}
