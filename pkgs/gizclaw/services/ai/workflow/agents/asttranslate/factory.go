package asttranslate

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"golang.org/x/sync/errgroup"
)

const Type = "ast-translate"

// Give realtime input BOS a scheduling window to interrupt stale AST downlink
// before another queued assistant chunk reaches the peer.
const interruptibleAssistantChunkGrace = 160 * time.Millisecond

type Factory struct {
	Transformer genx.Transformer
}

func (f Factory) NewAgent(_ context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	if f.Transformer == nil {
		return nil, fmt.Errorf("asttranslate: transformer is required")
	}
	resolved, err := resolveConfig(spec)
	if err != nil {
		return nil, err
	}
	if resolved.ttsVoice == "" {
		return agenthost.NewTransformerAgent(interruptibleTransformer{Transformer: patternTransformer{Transformer: f.Transformer, Pattern: resolved.astPattern}}), nil
	}
	return agenthost.NewTransformerAgent(interruptibleTransformer{
		Transformer: externalVoiceTransformer{
			Transformer: f.Transformer,
			ASTPattern:  resolved.astPattern,
			TTSPattern:  voicePattern(resolved.ttsVoice),
		},
		keepActiveAfterTextEOS: true,
	}), nil
}

type patternTransformer struct {
	Transformer genx.Transformer
	Pattern     string
}

func (t patternTransformer) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	if t.Transformer == nil {
		return nil, fmt.Errorf("asttranslate: transformer is required")
	}
	return t.Transformer.Transform(ctx, t.Pattern, input)
}

type interruptibleTransformer struct {
	Transformer            genx.Transformer
	keepActiveAfterTextEOS bool
}

func (t interruptibleTransformer) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	if t.Transformer == nil {
		return nil, fmt.Errorf("asttranslate: transformer is required")
	}
	ctx, cancel := context.WithCancel(ctx)
	out := newInterruptibleOutput(t.keepActiveAfterTextEOS)
	observedInput := newObservedInputStream(ctx, input, out.interrupt)
	inner, err := t.Transformer.Transform(ctx, pattern, observedInput)
	if err != nil {
		cancel()
		observedInput.CloseWithError(err)
		return nil, err
	}
	go func() {
		defer cancel()
		defer inner.Close()
		for {
			if err := ctx.Err(); err != nil {
				out.closeWithError(err)
				return
			}
			chunk, err := inner.Next()
			if err != nil {
				if isStreamDone(err) {
					out.close()
					return
				}
				out.closeWithError(err)
				return
			}
			if chunk == nil {
				continue
			}
			if err := out.push(chunk.Clone()); err != nil {
				return
			}
		}
	}()
	return out, nil
}

type observedInputStream struct {
	source genx.Stream
	onBOS  func(string)

	mu       sync.Mutex
	cond     *sync.Cond
	queue    []*genx.MessageChunk
	closed   bool
	closeErr error
}

func newObservedInputStream(ctx context.Context, source genx.Stream, onBOS func(string)) *observedInputStream {
	stream := &observedInputStream{
		source: source,
		onBOS:  onBOS,
	}
	stream.cond = sync.NewCond(&stream.mu)
	go stream.copy(ctx)
	return stream
}

func (s *observedInputStream) Next() (*genx.MessageChunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.queue) == 0 {
		if s.closeErr != nil {
			return nil, s.closeErr
		}
		if s.closed {
			return nil, io.EOF
		}
		s.cond.Wait()
	}
	chunk := s.queue[0]
	s.queue = s.queue[1:]
	return chunk, nil
}

func (s *observedInputStream) Close() error {
	s.close()
	if s.source != nil {
		return s.source.Close()
	}
	return nil
}

func (s *observedInputStream) CloseWithError(err error) error {
	s.closeWithError(err)
	if s.source != nil {
		return s.source.CloseWithError(err)
	}
	return nil
}

func (s *observedInputStream) copy(ctx context.Context) {
	defer s.source.Close()
	for {
		if err := ctx.Err(); err != nil {
			s.closeWithError(err)
			return
		}
		chunk, err := s.source.Next()
		if err != nil {
			if isStreamDone(err) {
				s.close()
				return
			}
			s.closeWithError(err)
			return
		}
		if chunk == nil {
			continue
		}
		if chunk.IsBeginOfStream() && s.onBOS != nil {
			streamID := ""
			if chunk.Ctrl != nil {
				streamID = chunk.Ctrl.StreamID
			}
			s.onBOS(streamID)
		}
		if err := s.push(chunk); err != nil {
			return
		}
	}
}

func (s *observedInputStream) push(chunk *genx.MessageChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.closeErr != nil {
		return io.ErrClosedPipe
	}
	s.queue = append(s.queue, chunk)
	s.cond.Signal()
	return nil
}

func (s *observedInputStream) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.cond.Broadcast()
	}
}

func (s *observedInputStream) closeWithError(err error) {
	if err == nil {
		err = io.ErrClosedPipe
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closeErr == nil {
		s.closeErr = err
		s.closed = true
		s.cond.Broadcast()
	}
}

type interruptibleOutput struct {
	mu                     sync.Mutex
	cond                   *sync.Cond
	queue                  []*genx.MessageChunk
	closed                 bool
	closeErr               error
	active                 bool
	activeStream           string
	activeStreamKeys       map[string]map[string]struct{}
	blockedStream          map[string]bool
	keepActiveAfterTextEOS bool
}

func newInterruptibleOutput(keepActiveAfterTextEOS ...bool) *interruptibleOutput {
	out := &interruptibleOutput{
		activeStreamKeys: make(map[string]map[string]struct{}),
		blockedStream:    make(map[string]bool),
	}
	if len(keepActiveAfterTextEOS) > 0 {
		out.keepActiveAfterTextEOS = keepActiveAfterTextEOS[0]
	}
	out.cond = sync.NewCond(&out.mu)
	return out
}

func (s *interruptibleOutput) Next() (*genx.MessageChunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
retry:
	for len(s.queue) == 0 {
		if s.closeErr != nil {
			return nil, s.closeErr
		}
		if s.closed {
			return nil, io.EOF
		}
		s.cond.Wait()
	}
	if chunk := s.queue[0]; shouldGraceASTAssistantChunk(chunk) {
		s.mu.Unlock()
		time.Sleep(interruptibleAssistantChunkGrace)
		s.mu.Lock()
		if len(s.queue) == 0 || s.queue[0] != chunk {
			goto retry
		}
	}
	chunk := s.queue[0]
	s.queue = s.queue[1:]
	return chunk, nil
}

func (s *interruptibleOutput) Close() error {
	s.close()
	return nil
}

func (s *interruptibleOutput) CloseWithError(err error) error {
	s.closeWithError(err)
	return nil
}

func (s *interruptibleOutput) push(chunk *genx.MessageChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.closeErr != nil {
		return io.ErrClosedPipe
	}
	if isASTAssistantChunk(chunk) {
		streamID := astAssistantResponseStreamID(chunk.Ctrl.StreamID)
		if s.isBlockedStream(chunk.Ctrl.StreamID) {
			return nil
		}
		s.observeAssistantChunk(chunk, streamID)
	}
	s.queue = append(s.queue, chunk)
	s.cond.Signal()
	return nil
}

func (s *interruptibleOutput) interrupt(inputStreamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	streamID := strings.TrimSpace(s.activeStream)
	if streamID == "" {
		streamID = strings.TrimSpace(inputStreamID)
	}
	if streamID == "" {
		streamID = "audio"
	}
	s.blockedStream[streamID] = true
	s.active = false
	s.activeStream = ""
	delete(s.activeStreamKeys, streamID)
	s.queue = removeASTAssistantStreamChunks(s.queue, streamID)
	s.queue = append(astInterruptedChunks(streamID), s.queue...)
	s.cond.Broadcast()
}

func (s *interruptibleOutput) observeAssistantChunk(chunk *genx.MessageChunk, responseStreamID string) {
	if responseStreamID == "" {
		return
	}
	key := astAssistantActiveKey(chunk)
	if !chunk.Ctrl.EndOfStream {
		s.addActiveStreamKey(responseStreamID, key)
		if s.keepActiveAfterTextEOS && astAssistantChunkKind(chunk) == "text" {
			s.addActiveStreamKey(responseStreamID, astAssistantPendingAudioKey(chunk.Ctrl.StreamID))
		}
		s.active = true
		s.activeStream = responseStreamID
		return
	}
	s.removeActiveStreamKey(responseStreamID, key)
	if s.keepActiveAfterTextEOS && astAssistantChunkKind(chunk) == "audio" {
		s.removeActiveStreamKey(responseStreamID, astAssistantPendingAudioKey(chunk.Ctrl.StreamID))
	}
	if s.activeStream == responseStreamID && len(s.activeStreamKeys[responseStreamID]) == 0 {
		s.active = false
		s.activeStream = ""
	}
}

func (s *interruptibleOutput) addActiveStreamKey(responseStreamID, key string) {
	if key == "" {
		return
	}
	keys := s.activeStreamKeys[responseStreamID]
	if keys == nil {
		keys = make(map[string]struct{})
		s.activeStreamKeys[responseStreamID] = keys
	}
	keys[key] = struct{}{}
}

func (s *interruptibleOutput) removeActiveStreamKey(responseStreamID, key string) {
	if key == "" {
		return
	}
	keys := s.activeStreamKeys[responseStreamID]
	if keys == nil {
		return
	}
	delete(keys, key)
	if len(keys) == 0 {
		delete(s.activeStreamKeys, responseStreamID)
	}
}

func (s *interruptibleOutput) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.cond.Broadcast()
	}
}

func (s *interruptibleOutput) closeWithError(err error) {
	if err == nil {
		err = io.ErrClosedPipe
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closeErr == nil {
		s.closeErr = err
		s.closed = true
		s.cond.Broadcast()
	}
}

func (s *interruptibleOutput) isBlockedStream(streamID string) bool {
	streamID = strings.TrimSpace(streamID)
	if s.blockedStream[streamID] {
		return true
	}
	responseStreamID := astAssistantResponseStreamID(streamID)
	return responseStreamID != streamID && s.blockedStream[responseStreamID]
}

func isASTAssistantChunk(chunk *genx.MessageChunk) bool {
	return chunk != nil &&
		chunk.Role == genx.RoleModel &&
		chunk.Ctrl != nil &&
		chunk.Ctrl.Label == "assistant"
}

func shouldGraceASTAssistantChunk(chunk *genx.MessageChunk) bool {
	return isASTAssistantChunk(chunk) && !chunk.Ctrl.EndOfStream && chunk.Ctrl.Error == ""
}

func astAssistantActiveKey(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	kind := astAssistantChunkKind(chunk)
	if kind == "" {
		return ""
	}
	return kind + "\x00" + strings.TrimSpace(chunk.Ctrl.StreamID)
}

func astAssistantPendingAudioKey(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return ""
	}
	return "audio-pending\x00" + streamID
}

func astAssistantChunkKind(chunk *genx.MessageChunk) string {
	if chunk == nil {
		return ""
	}
	switch part := chunk.Part.(type) {
	case genx.Text:
		return "text"
	case *genx.Blob:
		if part != nil && strings.HasPrefix(baseMIME(part.MIMEType), "audio/") {
			return "audio"
		}
	}
	return ""
}

func removeASTAssistantStreamChunks(chunks []*genx.MessageChunk, streamID string) []*genx.MessageChunk {
	if len(chunks) == 0 {
		return chunks
	}
	out := chunks[:0]
	for _, chunk := range chunks {
		if isASTAssistantChunk(chunk) && astAssistantStreamIDMatches(chunk.Ctrl.StreamID, streamID) {
			continue
		}
		out = append(out, chunk)
	}
	return out
}

func astAssistantResponseStreamID(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	if i := strings.Index(streamID, ":ast:"); i >= 0 {
		return streamID[:i]
	}
	return streamID
}

func astAssistantStreamIDMatches(streamID, responseStreamID string) bool {
	streamID = strings.TrimSpace(streamID)
	responseStreamID = astAssistantResponseStreamID(responseStreamID)
	if responseStreamID == "" {
		return streamID == ""
	}
	return streamID == responseStreamID || strings.HasPrefix(streamID, responseStreamID+":ast:")
}

func astInterruptedChunks(streamID string) []*genx.MessageChunk {
	return []*genx.MessageChunk{
		{
			Role: genx.RoleModel,
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", EndOfStream: true, Error: "interrupted"},
		},
		{
			Role: genx.RoleModel,
			Part: &genx.Blob{MIMEType: "audio/opus"},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", EndOfStream: true, Error: "interrupted"},
		},
	}
}

type resolvedConfig struct {
	astPattern string
	ttsVoice   string
}

func resolvePattern(spec agenthost.Spec) (string, error) {
	resolved, err := resolveConfig(spec)
	if err != nil {
		return "", err
	}
	return resolved.astPattern, nil
}

func resolveConfig(spec agenthost.Spec) (resolvedConfig, error) {
	workflowSpec := spec.Workflow.Spec.AstTranslate
	if workflowSpec == nil {
		return resolvedConfig{}, fmt.Errorf("asttranslate: workflow spec.ast_translate is required")
	}
	model := strings.TrimSpace(workflowSpec.TranslationModel)
	params := workflowParams(*workflowSpec)
	ttsVoice := astTranslateTTSVoice(params)
	if spec.Workspace.Parameters != nil {
		typed, err := spec.Workspace.Parameters.AsASTTranslateWorkspaceParameters()
		if err != nil {
			return resolvedConfig{}, fmt.Errorf("asttranslate: decode workspace parameters: %w", err)
		}
		model = firstNonEmpty(ptrString(typed.TranslationModel), model)
		params = mergeWorkspaceParams(params, typed)
		ttsVoice = firstNonEmpty(astTranslateWorkspaceTTSVoice(typed), ttsVoice)
	}
	if model == "" {
		return resolvedConfig{}, fmt.Errorf("asttranslate: translation_model is required")
	}
	if ttsVoice != "" {
		params["mode"] = "s2t"
		delete(params, "tts_voice")
		delete(params, "speaker_id")
		delete(params, "is_custom_speaker")
		delete(params, "tts_resource_id")
		delete(params, "speech_rate")
	}
	if err := normalizeLanguagePair(params, true); err != nil {
		return resolvedConfig{}, err
	}
	return resolvedConfig{
		astPattern: appendPatternParams("model/"+strings.Trim(strings.TrimSpace(model), "/"), params),
		ttsVoice:   ttsVoice,
	}, nil
}

func workflowParams(ast apitypes.ASTTranslateWorkflowSpec) map[string]any {
	params := make(map[string]any)
	if ast.Mode != nil {
		setParam(params, "mode", string(*ast.Mode))
	}
	if ast.Voice != nil {
		mergeASTTranslateVoice(params, *ast.Voice)
	}
	setParam(params, "speaker_id", ast.SpeakerId)
	setParam(params, "is_custom_speaker", ast.IsCustomSpeaker)
	setParam(params, "tts_resource_id", ast.TtsResourceId)
	setParam(params, "speech_rate", ast.SpeechRate)
	setParam(params, "enable_source_language_detect", ast.EnableSourceLanguageDetect)
	setParam(params, "denoise", ast.Denoise)
	setParam(params, "resource_id", ast.ResourceId)
	if len(params) == 0 {
		return nil
	}
	return params
}

func mergeWorkspaceParams(params map[string]any, typed apitypes.ASTTranslateWorkspaceParameters) map[string]any {
	if params == nil {
		params = make(map[string]any)
	}
	if typed.Mode != nil {
		setParam(params, "mode", string(*typed.Mode))
	}
	if typed.Input != nil {
		setParam(params, "input", string(*typed.Input))
	}
	if typed.LangPair != nil {
		setParam(params, "lang_pair", *typed.LangPair)
	}
	if typed.Voice != nil {
		mergeASTTranslateVoice(params, *typed.Voice)
	}
	if typed.SpeakerId != nil {
		setParam(params, "speaker_id", *typed.SpeakerId)
	}
	if typed.IsCustomSpeaker != nil {
		setParam(params, "is_custom_speaker", *typed.IsCustomSpeaker)
	}
	if typed.TtsResourceId != nil {
		setParam(params, "tts_resource_id", *typed.TtsResourceId)
	}
	if typed.SpeechRate != nil {
		setParam(params, "speech_rate", *typed.SpeechRate)
	}
	if typed.EnableSourceLanguageDetect != nil {
		setParam(params, "enable_source_language_detect", *typed.EnableSourceLanguageDetect)
	}
	if typed.Denoise != nil {
		setParam(params, "denoise", *typed.Denoise)
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func mergeASTTranslateVoice(params map[string]any, value apitypes.ASTTranslateVoiceParameters) {
	if speaker, err := value.AsASTTranslateInternalSpeakerParameters(); err == nil {
		if strings.TrimSpace(speaker.SpeakerId) != "" {
			params["speaker_id"] = speaker.SpeakerId
			setParam(params, "is_custom_speaker", speaker.IsCustomSpeaker)
			setParam(params, "tts_resource_id", speaker.TtsResourceId)
			setParam(params, "speech_rate", speaker.SpeechRate)
			return
		}
	}
	voice, err := value.AsASTTranslateExternalVoiceParameters()
	if err != nil {
		return
	}
	if strings.TrimSpace(voice.TtsVoice) != "" {
		params["tts_voice"] = voice.TtsVoice
	}
}

func astTranslateTTSVoice(params map[string]any) string {
	if params == nil {
		return ""
	}
	if value, ok := paramString(params["tts_voice"]); ok {
		return value
	}
	return ""
}

func astTranslateWorkspaceTTSVoice(typed apitypes.ASTTranslateWorkspaceParameters) string {
	if typed.Voice == nil {
		return ""
	}
	voice, err := typed.Voice.AsASTTranslateExternalVoiceParameters()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(voice.TtsVoice)
}

type externalVoiceTransformer struct {
	Transformer genx.Transformer
	ASTPattern  string
	TTSPattern  string
}

func (t externalVoiceTransformer) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	if t.Transformer == nil {
		return nil, fmt.Errorf("asttranslate: transformer is required")
	}
	astOutput, err := t.Transformer.Transform(ctx, t.ASTPattern, input)
	if err != nil {
		return nil, err
	}
	ttsInput := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 128)
	ttsOutput, err := t.Transformer.Transform(ctx, t.TTSPattern, ttsInput.Stream())
	if err != nil {
		_ = astOutput.Close()
		_ = ttsInput.Abort(err)
		return nil, err
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 256)
	go t.run(ctx, astOutput, ttsInput, ttsOutput, output)
	return output.Stream(), nil
}

func (t externalVoiceTransformer) run(ctx context.Context, astOutput genx.Stream, ttsInput *genx.StreamBuilder, ttsOutput genx.Stream, output *genx.StreamBuilder) {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		err := forwardASTTranslateText(ctx, astOutput, ttsInput, output)
		if err != nil {
			_ = ttsInput.Abort(err)
			return err
		}
		return ttsInput.Done(genx.Usage{})
	})
	g.Go(func() error {
		return forwardASTTranslateTTS(ctx, ttsOutput, output)
	})
	if err := g.Wait(); err != nil {
		_ = output.Abort(err)
		return
	}
	_ = output.Done(genx.Usage{})
}

func forwardASTTranslateText(ctx context.Context, astOutput genx.Stream, ttsInput *genx.StreamBuilder, output *genx.StreamBuilder) error {
	defer astOutput.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := astOutput.Next()
		if err != nil {
			if isStreamDone(err) {
				return nil
			}
			return err
		}
		if chunk == nil {
			continue
		}
		if err := output.Add(chunk.Clone()); err != nil {
			return err
		}
		if isAssistantTextChunk(chunk) {
			if err := ttsInput.Add(chunk.Clone()); err != nil {
				return err
			}
		}
	}
}

func forwardASTTranslateTTS(ctx context.Context, ttsOutput genx.Stream, output *genx.StreamBuilder) error {
	defer ttsOutput.Close()
	decoders := map[string]*astTranslateOggOpusFrameDecoder{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := ttsOutput.Next()
		if err != nil {
			if isStreamDone(err) {
				return nil
			}
			return err
		}
		if chunk == nil {
			continue
		}
		chunk = chunk.Clone()
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || !strings.HasPrefix(baseMIME(blob.MIMEType), "audio/") {
			if err := output.Add(chunk); err != nil {
				return err
			}
			continue
		}
		if chunk.Ctrl == nil {
			chunk.Ctrl = &genx.StreamCtrl{}
		}
		chunk.Ctrl.Label = "assistant"
		streamID := chunk.Ctrl.StreamID
		switch baseMIME(blob.MIMEType) {
		case "audio/ogg", "application/ogg":
			decoder := decoders[streamID]
			if decoder == nil {
				decoder = newASTTranslateOggOpusFrameDecoder()
				decoders[streamID] = decoder
			}
			if len(blob.Data) > 0 {
				frames, err := decoder.Write(blob.Data)
				if err != nil {
					return fmt.Errorf("asttranslate: decode external TTS ogg opus: %w", err)
				}
				for _, frame := range frames {
					out := chunk.Clone()
					out.Part = &genx.Blob{MIMEType: "audio/opus", Data: frame}
					out.Ctrl.EndOfStream = false
					if err := output.Add(out); err != nil {
						return err
					}
				}
			}
			if chunk.IsEndOfStream() {
				if err := decoder.Close(); err != nil {
					return fmt.Errorf("asttranslate: decode external TTS ogg opus: %w", err)
				}
				delete(decoders, streamID)
				chunk.Part = &genx.Blob{MIMEType: "audio/opus"}
				if err := output.Add(chunk); err != nil {
					return err
				}
			}
		default:
			if err := output.Add(chunk); err != nil {
				return err
			}
		}
	}
}

func isAssistantTextChunk(chunk *genx.MessageChunk) bool {
	if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.Label != "assistant" {
		return false
	}
	_, ok := chunk.Part.(genx.Text)
	return ok
}

func isStreamDone(err error) bool {
	return err == nil || err == io.EOF || err == genx.ErrDone || agenthost.IsStreamDone(err)
}

func voicePattern(voice string) string {
	voice = strings.Trim(strings.TrimSpace(voice), "/")
	if voice == "" || strings.Contains(voice, "/") {
		return voice
	}
	return "voice/" + voice
}

func baseMIME(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}

type astTranslateOggOpusFrameDecoder struct {
	pending               []byte
	packet                []byte
	expectingContinuation bool
	currentPacketBOS      bool
}

func newASTTranslateOggOpusFrameDecoder() *astTranslateOggOpusFrameDecoder {
	return &astTranslateOggOpusFrameDecoder{}
}

func (d *astTranslateOggOpusFrameDecoder) Write(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	d.pending = append(d.pending, data...)
	var frames [][]byte
	for {
		page, ok, err := d.nextPage()
		if err != nil {
			return nil, err
		}
		if !ok {
			return frames, nil
		}
		pageFrames, err := d.consumePage(page)
		if err != nil {
			return nil, err
		}
		frames = append(frames, pageFrames...)
	}
}

func (d *astTranslateOggOpusFrameDecoder) Close() error {
	if len(d.pending) != 0 {
		return fmt.Errorf("truncated ogg page: %d pending bytes", len(d.pending))
	}
	if d.expectingContinuation || len(d.packet) != 0 {
		return fmt.Errorf("stream ended with unterminated ogg packet")
	}
	return nil
}

func (d *astTranslateOggOpusFrameDecoder) nextPage() (*ogg.Page, bool, error) {
	const oggPageHeaderSize = 27
	if len(d.pending) == 0 {
		return nil, false, nil
	}
	if len(d.pending) < oggPageHeaderSize {
		if len(d.pending) < len(ogg.CapturePattern) && !strings.HasPrefix(ogg.CapturePattern, string(d.pending)) {
			return nil, false, fmt.Errorf("invalid ogg capture pattern prefix %q", d.pending)
		}
		if len(d.pending) >= len(ogg.CapturePattern) && string(d.pending[:len(ogg.CapturePattern)]) != ogg.CapturePattern {
			return nil, false, fmt.Errorf("invalid ogg capture pattern prefix %q", d.pending)
		}
		return nil, false, nil
	}
	if string(d.pending[:4]) != ogg.CapturePattern {
		return nil, false, fmt.Errorf("invalid ogg capture pattern %q", d.pending[:4])
	}
	segmentCount := int(d.pending[26])
	headerLen := oggPageHeaderSize + segmentCount
	if len(d.pending) < headerLen {
		return nil, false, nil
	}
	payloadLen := 0
	for _, segment := range d.pending[oggPageHeaderSize:headerLen] {
		payloadLen += int(segment)
	}
	pageLen := headerLen + payloadLen
	if len(d.pending) < pageLen {
		return nil, false, nil
	}
	page, err := ogg.ParsePage(d.pending[:pageLen])
	if err != nil {
		return nil, false, err
	}
	d.pending = d.pending[pageLen:]
	return page, true, nil
}

func (d *astTranslateOggOpusFrameDecoder) consumePage(page *ogg.Page) ([][]byte, error) {
	if page == nil {
		return nil, fmt.Errorf("ogg page is nil")
	}
	if page.HasContinuation() {
		if !d.expectingContinuation {
			return nil, fmt.Errorf("unexpected ogg continuation page")
		}
	} else if d.expectingContinuation {
		return nil, fmt.Errorf("missing ogg continuation page")
	}

	var frames [][]byte
	payloadOffset := 0
	for segmentIndex, segment := range page.Segments {
		if !d.expectingContinuation && len(d.packet) == 0 {
			d.currentPacketBOS = page.HasBOS() && segmentIndex == 0
		}
		chunkLen := int(segment)
		if payloadOffset+chunkLen > len(page.Payload) {
			return nil, fmt.Errorf("ogg segment overflows payload")
		}
		if chunkLen > 0 {
			d.packet = append(d.packet, page.Payload[payloadOffset:payloadOffset+chunkLen]...)
		}
		payloadOffset += chunkLen
		if segment == 255 {
			d.expectingContinuation = true
			continue
		}
		packet := append([]byte(nil), d.packet...)
		d.packet = d.packet[:0]
		d.expectingContinuation = false
		d.currentPacketBOS = false
		if len(packet) == 0 || codecconv.IsOpusHeadPacket(packet) || codecconv.IsOpusTagsPacket(packet) {
			continue
		}
		frames = append(frames, packet)
	}
	if payloadOffset != len(page.Payload) {
		return nil, fmt.Errorf("ogg page has trailing payload")
	}
	return frames, nil
}

func normalizeLanguagePair(params map[string]any, required bool) error {
	if params == nil {
		if required {
			return fmt.Errorf("asttranslate: workspace lang_pair is required")
		}
		return nil
	}
	pair, _ := paramString(params["lang_pair"])
	source, target, auto, err := parseLanguagePair(pair)
	if err != nil {
		return fmt.Errorf("asttranslate: invalid lang_pair %q: %w", pair, err)
	}
	if source == "" || target == "" {
		if required {
			return fmt.Errorf("asttranslate: workspace lang_pair is required")
		}
		return nil
	}
	params["source_language"] = source
	params["target_language"] = target
	delete(params, "lang_pair")
	if auto {
		params["enable_source_language_detect"] = true
	}
	return nil
}

func parseLanguagePair(pair string) (source string, target string, auto bool, err error) {
	pair = strings.ToLower(strings.TrimSpace(pair))
	switch pair {
	case "":
		return "", "", false, nil
	case "auto":
		return "zhen", "zhen", true, nil
	}
	parts := strings.Split(pair, "/")
	if len(parts) != 2 {
		return "", "", false, fmt.Errorf("expected source/target or auto")
	}
	source = strings.TrimSpace(parts[0])
	target = strings.TrimSpace(parts[1])
	if source == "" || target == "" {
		return "", "", false, fmt.Errorf("source and target must be non-empty")
	}
	source = normalizeLanguageCode(source)
	target = normalizeLanguageCode(target)
	if source == "zhen" || target == "zhen" {
		return "", "", false, fmt.Errorf("zhen is only available through auto")
	}
	return source, target, false, nil
}

func normalizeLanguageCode(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "jp":
		return "ja"
	default:
		return strings.ToLower(strings.TrimSpace(language))
	}
}

func setParam(params map[string]any, key string, value any) {
	if params == nil {
		return
	}
	if text, ok := paramString(value); ok {
		params[key] = text
	}
}

func appendPatternParams(pattern string, params map[string]any) string {
	if len(params) == 0 {
		return pattern
	}
	base, rawQuery, _ := strings.Cut(strings.TrimSpace(pattern), "?")
	query, _ := url.ParseQuery(rawQuery)
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if text, ok := paramString(params[key]); ok {
			query.Set(key, text)
		}
	}
	encoded := query.Encode()
	if encoded == "" {
		return base
	}
	return base + "?" + encoded
}

func paramString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		return text, text != ""
	case *string:
		if typed == nil {
			return "", false
		}
		text := strings.TrimSpace(*typed)
		return text, text != ""
	case bool:
		return strconv.FormatBool(typed), true
	case *bool:
		if typed == nil {
			return "", false
		}
		return strconv.FormatBool(*typed), true
	case int:
		return strconv.Itoa(typed), true
	case *int:
		if typed == nil {
			return "", false
		}
		return strconv.Itoa(*typed), true
	case float64:
		if typed != float64(int(typed)) {
			return "", false
		}
		return strconv.Itoa(int(typed)), true
	default:
		return "", false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
