package chatroom

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const (
	defaultInputStreamID = "audio"
	transcriptLabel      = "transcript"
)

var (
	errASRInputConsumerClosed = errors.New("chatroom: ASR input consumer closed")
	errASROutputCompleted     = errors.New("chatroom: ASR output completed while input remained active")
)

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

func (t *asrInputTransport) consumerSawEOS() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.consumerEOS
}

func (t *asrInputTransport) closeConsumer() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.consumerClosed || (t.terminal && t.terminalErr != nil) {
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

type asrSession struct {
	input           *asrInputTransport
	output          genx.Stream
	readDone        chan error
	stopInputCancel func() bool

	expectedCompletion atomic.Bool
	routeCompletionOK  bool
	closeOutputOnce    sync.Once
}

func (s *asrSession) allowCompletion() {
	if s != nil {
		s.expectedCompletion.Store(true)
	}
}

func (s *asrSession) completionAllowed() bool {
	if s == nil {
		return false
	}
	return s.expectedCompletion.Load() || (s.routeCompletionOK && s.input.consumerSawEOS())
}

func (s *asrSession) wait(ctx context.Context) error {
	if s == nil || s.readDone == nil {
		return nil
	}
	select {
	case err := <-s.readDone:
		s.readDone = nil
		s.closeOutput(err)
		s.stopInputCancellation()
		return err
	case <-ctx.Done():
		s.closeOutput(ctx.Err())
		err := <-s.readDone
		s.readDone = nil
		s.stopInputCancellation()
		return errors.Join(ctx.Err(), err)
	}
}

func (s *asrSession) complete(ctx context.Context, allowWithoutRouteEOS bool) error {
	if s == nil {
		return nil
	}
	if allowWithoutRouteEOS {
		s.allowCompletion()
	}
	if err := s.input.Done(); err != nil {
		s.abort(err)
		return err
	}
	return s.wait(ctx)
}

func (s *asrSession) abort(err error) {
	if s == nil {
		return
	}
	_ = s.input.Abort(err)
	s.closeOutput(err)
	s.stopInputCancellation()
	if s.readDone != nil {
		<-s.readDone
		s.readDone = nil
	}
}

func (s *asrSession) closeOutput(err error) {
	if s == nil {
		return
	}
	s.closeOutputOnce.Do(func() {
		if s.output == nil {
			return
		}
		if err != nil {
			_ = s.output.CloseWithError(err)
			return
		}
		_ = s.output.Close()
	})
}

func (s *asrSession) stopInputCancellation() {
	if s != nil && s.stopInputCancel != nil {
		s.stopInputCancel()
		s.stopInputCancel = nil
	}
}

func (a *Transformer) transcribeInput(ctx context.Context, input genx.Stream, output *genx.StreamBuilder) {
	defer input.Close()
	stopInputCancel := context.AfterFunc(ctx, func() {
		_ = input.CloseWithError(ctx.Err())
	})
	defer stopInputCancel()
	var session *asrSession
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
	startASR := func() (*asrSession, error) {
		if session != nil {
			return session, nil
		}
		next := &asrSession{
			readDone:          make(chan error, 1),
			routeCompletionOK: a.config.InputMode == InputModePushToTalk,
		}
		var asrInput *asrInputTransport
		asrInput = newASRInputTransport(func(err error) {
			if err == nil {
				if next.completionAllowed() {
					return
				}
				err = errASRInputConsumerClosed
				_ = asrInput.Abort(err)
			}
			if ctx.Err() == nil {
				_ = input.CloseWithError(err)
			}
		})
		next.input = asrInput
		asrInputStream := asrInput
		next.stopInputCancel = context.AfterFunc(ctx, func() {
			_ = asrInputStream.Abort(ctx.Err())
		})
		asr, err := a.config.ASR.Transform(ctx, a.asrPattern(), asrInput.Stream())
		if err != nil {
			err = fmt.Errorf("chatroom: start ASR: %w", err)
			_ = asrInput.Abort(err)
			next.stopInputCancellation()
			return nil, err
		}
		if asr == nil {
			err := errors.New("chatroom: ASR output stream is required")
			_ = asrInput.Abort(err)
			next.stopInputCancellation()
			return nil, err
		}
		next.output = asr
		go func() {
			err := readTranscript(ctx, asr, output, streamID)
			if err == nil && !next.completionAllowed() && ctx.Err() == nil {
				err = errASROutputCompleted
			}
			if err != nil && ctx.Err() == nil {
				_ = asrInput.Abort(err)
				_ = input.CloseWithError(err)
			}
			next.readDone <- err
		}()
		session = next
		return next, nil
	}
	fail := func(err error) {
		if session != nil {
			session.abort(err)
			session = nil
		}
		_ = output.Abort(err)
	}
	finish := func() {
		if session != nil {
			if err := session.complete(ctx, true); err != nil {
				fail(err)
				return
			}
			session = nil
		}
		if err := flushText(); err != nil {
			fail(err)
			return
		}
		_ = output.Done(genx.Usage{})
	}

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
		if err != nil {
			if session != nil {
				if asrErr := session.input.failure(); asrErr != nil {
					fail(asrErr)
					return
				}
			}
			if !isStreamDone(err) {
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
		active, err := startASR()
		if err != nil {
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
		if err := active.input.Add(next); err != nil {
			fail(err)
			return
		}
		if a.config.InputMode == InputModePushToTalk && chunk.IsEndOfStream() {
			if err := active.complete(ctx, false); err != nil {
				fail(err)
				return
			}
			session = nil
		}
	}
}

func (a *Transformer) asrPattern() string {
	pattern := a.config.ASRPattern
	if a.config.InputMode == InputModeRealtime {
		separator := "?"
		if strings.Contains(pattern, "?") {
			separator = "&"
		}
		pattern += separator + "emit_interim=true"
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
