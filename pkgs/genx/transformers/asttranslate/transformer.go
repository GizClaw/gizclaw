package asttranslate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
)

// Give realtime input BOS a scheduling window to interrupt stale AST text
// before another queued assistant event reaches the peer. Audio is already
// paced at its frame duration and must not pay this delay per frame.
const interruptibleAssistantChunkGrace = 160 * time.Millisecond

// observedInputQueueCapacity keeps a stalled AST provider from accumulating
// unbounded peer audio while still allowing a short scheduling window for BOS
// to interrupt stale translated output.
const observedInputQueueCapacity = 256

// Config configures an AST Translate Transformer. Model is a RuntimeProfile
// model alias; Params contain provider-supported AST parameters such as
// lang_pair, mode, input, and the internal-speaker fields. ExternalVoice asks
// AudioDock to synthesize translated text using the supplied voice pattern.
type Config struct {
	Transformer   genx.TransformerMux
	Model         string
	Params        map[string]any
	ExternalVoice string
}

// New creates a reusable AST Translate Transformer. It accepts text or audio
// GenX streams and preserves the interrupted-stream contract for each call.
func New(config Config) (genx.Transformer, error) {
	if config.Transformer == nil {
		return nil, fmt.Errorf("asttranslate: transformer is required")
	}
	model := strings.Trim(strings.TrimSpace(config.Model), "/")
	if model == "" {
		return nil, fmt.Errorf("asttranslate: translation_model is required")
	}
	params := cloneParams(config.Params)
	voice := strings.TrimSpace(config.ExternalVoice)
	if voice != "" {
		params["mode"] = "s2t"
		delete(params, "tts_voice")
		delete(params, "speaker_id")
		delete(params, "is_custom_speaker")
		delete(params, "tts_resource_id")
		delete(params, "speech_rate")
	}
	if err := normalizeLanguagePair(params, true); err != nil {
		return nil, err
	}
	pattern := appendPatternParams("model/"+model, params)
	if voice == "" {
		return interruptibleTransformer{Transformer: patternTransformer{Transformer: config.Transformer, Pattern: pattern}}, nil
	}
	return interruptibleTransformer{
		Transformer: externalVoiceTransformer{
			Transformer: config.Transformer,
			ASTPattern:  pattern,
			TTSPattern:  voicePattern(voice),
		},
		keepActiveAfterTextEOS: true,
	}, nil
}

func cloneParams(params map[string]any) map[string]any {
	if len(params) == 0 {
		return make(map[string]any)
	}
	cloned := make(map[string]any, len(params))
	maps.Copy(cloned, params)
	return cloned
}

type patternTransformer struct {
	Transformer genx.TransformerMux
	Pattern     string
}

func (t patternTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t.Transformer == nil {
		return nil, fmt.Errorf("asttranslate: transformer is required")
	}
	return t.Transformer.Transform(ctx, t.Pattern, input)
}

type interruptibleTransformer struct {
	Transformer            genx.Transformer
	keepActiveAfterTextEOS bool
}

func (t interruptibleTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t.Transformer == nil {
		return nil, fmt.Errorf("asttranslate: transformer is required")
	}
	if input == nil {
		return nil, fmt.Errorf("asttranslate: input stream is required")
	}
	ctx, cancel := context.WithCancel(ctx)
	out := newInterruptibleOutput(t.keepActiveAfterTextEOS)
	observedInput := newObservedInputStream(ctx, input, out.interrupt)
	inner, err := t.Transformer.Transform(ctx, observedInput)
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
	stopCtx  func() bool
}

func newObservedInputStream(ctx context.Context, source genx.Stream, onBOS func(string)) *observedInputStream {
	stream := &observedInputStream{
		source: source,
		onBOS:  onBOS,
	}
	stream.cond = sync.NewCond(&stream.mu)
	stream.stopCtx = context.AfterFunc(ctx, func() {
		_ = stream.CloseWithError(ctx.Err())
	})
	go stream.copy(ctx)
	return stream
}

func (s *observedInputStream) Next() (*genx.MessageChunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if s.closeErr != nil {
			return nil, s.closeErr
		}
		if len(s.queue) != 0 {
			chunk := s.queue[0]
			s.queue = s.queue[1:]
			s.cond.Signal()
			return chunk, nil
		}
		if s.closed {
			return nil, io.EOF
		}
		s.cond.Wait()
	}
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
	defer s.stopCtx()
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
	for len(s.queue) >= observedInputQueueCapacity && !s.closed && s.closeErr == nil {
		s.cond.Wait()
	}
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
		s.queue = nil
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
	if !isASTAssistantChunk(chunk) || chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != "" {
		return false
	}
	blob, isBlob := chunk.Part.(*genx.Blob)
	return !isBlob || blob == nil || !strings.HasPrefix(baseMIME(blob.MIMEType), "audio/")
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
	if responseID, _, ok := strings.Cut(streamID, ":ast:"); ok {
		return responseID
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

type externalVoiceTransformer struct {
	Transformer genx.TransformerMux
	ASTPattern  string
	TTSPattern  string
}

func (t externalVoiceTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t.Transformer == nil {
		return nil, fmt.Errorf("asttranslate: transformer is required")
	}
	dock, err := audiodock.New(audiodock.Config{
		Agent: patternTransformer{Transformer: t.Transformer, Pattern: t.ASTPattern},
		TTS:   astVoiceTransformerMux{Transformer: t.Transformer},
		ResolveVoice: func(context.Context, audiodock.VoiceRequest) (string, error) {
			return t.TTSPattern, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return dock.Transform(ctx, input)
}

// astVoiceTransformerMux keeps the AST product boundary's Ogg/Opus packet
// normalization while Audio Dock owns text/TTS composition and route EOS.
type astVoiceTransformerMux struct {
	Transformer genx.TransformerMux
}

func (t astVoiceTransformerMux) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	raw, err := t.Transformer.Transform(ctx, pattern, input)
	if err != nil {
		return nil, err
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 256)
	go func() {
		if err := forwardASTTranslateTTS(ctx, raw, output); err != nil {
			_ = output.Abort(err)
			return
		}
		_ = output.Done(genx.Usage{})
	}()
	return output.Stream(), nil
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

func isStreamDone(err error) bool {
	return err == nil || errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone)
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
