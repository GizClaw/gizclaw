package chatroom

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

const Type = "chatroom"

const (
	defaultInputStreamID = "audio"
	transcriptLabel      = "transcript"
)

type Factory struct {
	Transformer genx.Transformer
}

type config struct {
	transformer       genx.Transformer
	transcriptEnabled bool
	asrModel          string
	inputMode         apitypes.WorkspaceInputMode
}

func (f Factory) NewAgent(_ context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	if spec.Workflow.Spec.Chatroom == nil {
		return nil, fmt.Errorf("chatroom: workflow spec.chatroom is required")
	}
	cfg := config{transformer: f.Transformer, inputMode: apitypes.WorkspaceInputModePushToTalk}
	if spec.Workflow.Spec.Chatroom.Transcript != nil {
		cfg.transcriptEnabled = boolValue(spec.Workflow.Spec.Chatroom.Transcript.Enabled)
		cfg.asrModel = stringValue(spec.Workflow.Spec.Chatroom.Transcript.AsrModel)
	}
	if spec.Workspace.Parameters != nil {
		typed, err := spec.Workspace.Parameters.AsChatRoomWorkspaceParameters()
		if err != nil {
			return nil, fmt.Errorf("chatroom: decode workspace parameters: %w", err)
		}
		if !typed.AgentType.Valid() {
			return nil, fmt.Errorf("chatroom: unsupported agent_type %q", typed.AgentType)
		}
		if typed.Mode != nil && !typed.Mode.Valid() {
			return nil, fmt.Errorf("chatroom: unsupported mode %q", *typed.Mode)
		}
		if typed.Input != nil && !typed.Input.Valid() {
			return nil, fmt.Errorf("chatroom: unsupported input %q", *typed.Input)
		}
		if typed.Input != nil {
			cfg.inputMode = *typed.Input
		}
		mergeWorkspaceTranscriptConfig(&cfg, typed)
	}
	if cfg.transcriptEnabled {
		if cfg.asrModel == "" {
			return nil, fmt.Errorf("chatroom: transcript.asr_model is required when transcript is enabled")
		}
		if cfg.transformer == nil {
			return nil, fmt.Errorf("chatroom: transformer is required when transcript is enabled")
		}
	}
	return agenthost.NewTransformerAgent(agent{cfg: cfg}), nil
}

type agent struct {
	cfg config
}

type asrInputTransport struct {
	builder         *genx.StreamBuilder
	onConsumerError func(error)

	mu          sync.Mutex
	terminal    bool
	terminalErr error
}

func newASRInputTransport(onConsumerError func(error)) *asrInputTransport {
	return &asrInputTransport{
		builder:         genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64),
		onConsumerError: onConsumerError,
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
	defer t.mu.Unlock()
	if t.terminal {
		return t.terminalErr
	}
	t.terminal = true
	t.terminalErr = t.builder.Done(genx.Usage{})
	return t.terminalErr
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
	defer t.mu.Unlock()
	if t.terminal {
		return false, nil
	}
	t.terminal = true
	t.terminalErr = err
	return true, t.builder.Abort(err)
}

func (t *asrInputTransport) status() (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.terminal, t.terminalErr
}

func (t *asrInputTransport) failure() error {
	_, err := t.status()
	return err
}

type asrInputView struct {
	source    genx.Stream
	transport *asrInputTransport
}

func (s *asrInputView) Next() (*genx.MessageChunk, error) {
	return s.source.Next()
}

func (s *asrInputView) Close() error {
	return nil
}

func (s *asrInputView) CloseWithError(err error) error {
	if err == nil || isStreamDone(err) {
		return nil
	}
	first, closeErr := s.transport.abort(err)
	if first && s.transport.onConsumerError != nil {
		s.transport.onConsumerError(err)
	}
	return closeErr
}

func (a agent) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	if input == nil {
		return nil, fmt.Errorf("chatroom: input stream is required")
	}
	builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	if a.cfg.transcriptEnabled {
		go a.transcribeInput(ctx, input, builder)
	} else {
		go forwardTextInput(ctx, input, builder)
	}
	return builder.Stream(), nil
}

func mergeWorkspaceTranscriptConfig(cfg *config, params apitypes.ChatRoomWorkspaceParameters) {
	if cfg == nil || params.Transcript == nil {
		return
	}
	if params.Transcript.Enabled != nil {
		cfg.transcriptEnabled = *params.Transcript.Enabled
	}
	if model := strings.TrimSpace(stringValue(params.Transcript.AsrModel)); model != "" {
		cfg.asrModel = model
	}
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

func (a agent) transcribeInput(ctx context.Context, input genx.Stream, output *genx.StreamBuilder) {
	defer input.Close()
	stopInputCancel := context.AfterFunc(ctx, func() {
		_ = input.CloseWithError(ctx.Err())
	})
	defer stopInputCancel()
	var asrInput *asrInputTransport
	var asr genx.Stream
	var readDone chan error
	var stopASRCancel func() bool
	defer func() {
		if stopASRCancel != nil {
			stopASRCancel()
		}
	}()
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
			_ = input.CloseWithError(err)
		})
		var err error
		asr, err = a.cfg.transformer.Transform(ctx, a.asrPattern(), asrInput.Stream())
		if err != nil {
			err = fmt.Errorf("chatroom: start ASR: %w", err)
			_ = asrInput.Abort(err)
			return err
		}
		asrStream := asr
		stopASRCancel = context.AfterFunc(ctx, func() {
			_ = asrStream.CloseWithError(ctx.Err())
		})
		done := make(chan error, 1)
		readDone = done
		go func() {
			defer asrStream.Close()
			err := readTranscript(ctx, asrStream, output, streamID)
			if err != nil && ctx.Err() == nil {
				_ = asrInput.Abort(err)
				_ = input.CloseWithError(err)
			}
			done <- err
		}()
		return nil
	}
	waitTranscript := func() error {
		if readDone == nil {
			return nil
		}
		done := readDone
		readDone = nil
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			if asr != nil {
				_ = asr.CloseWithError(ctx.Err())
			}
			<-done
			return ctx.Err()
		}
	}
	fail := func(err error) {
		if asrInput != nil {
			_ = asrInput.Abort(err)
		}
		if asr != nil {
			_ = asr.CloseWithError(err)
		}
		_ = output.Abort(err)
		_ = waitTranscript()
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
				fail(err)
				return
			}
			if err := waitTranscript(); err != nil {
				fail(err)
				return
			}
			_ = output.Done(genx.Usage{})
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
			fail(err)
			return
		}
		if chunk.IsEndOfStream() {
			if err := asrInput.Done(); err != nil {
				fail(err)
				return
			}
			if err := waitTranscript(); err != nil {
				fail(err)
				return
			}
			_ = output.Done(genx.Usage{})
			return
		}
	}
}

func (a agent) asrPattern() string {
	pattern := "model/" + a.cfg.asrModel
	if a.cfg.inputMode == apitypes.WorkspaceInputModeRealtime {
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

func boolValue(v *bool) bool {
	return v != nil && *v
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
