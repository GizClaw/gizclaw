package asttranslate

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestParseLanguagePairRejectsZhenForms(t *testing.T) {
	for _, pair := range []string{"zhen", "zhen/zhen", "zh/zhen", "zhen/en"} {
		if _, _, _, err := parseLanguagePair(pair); err == nil {
			t.Fatalf("parseLanguagePair(%q) succeeded, want error", pair)
		}
	}
	source, target, auto, err := parseLanguagePair("auto")
	if err != nil {
		t.Fatalf("parseLanguagePair(auto) error = %v", err)
	}
	if source != "zhen" || target != "zhen" || !auto {
		t.Fatalf("parseLanguagePair(auto) = %q, %q, %v", source, target, auto)
	}
	source, target, auto, err = parseLanguagePair("zh/en")
	if err != nil {
		t.Fatalf("parseLanguagePair(zh/en) error = %v", err)
	}
	if source != "zh" || target != "en" || auto {
		t.Fatalf("parseLanguagePair(zh/en) = %q, %q, %v", source, target, auto)
	}
	source, target, auto, err = parseLanguagePair("zh/jp")
	if err != nil {
		t.Fatalf("parseLanguagePair(zh/jp) error = %v", err)
	}
	if source != "zh" || target != "ja" || auto {
		t.Fatalf("parseLanguagePair(zh/jp) = %q, %q, %v", source, target, auto)
	}
}

func TestNewBuildsAliasPatternAndExternalVoiceMode(t *testing.T) {
	transformer, err := New(Config{
		Transformer: &scriptedTransformer{},
		Model:       "runtime-ast",
		Params:      map[string]any{"lang_pair": "zh/jp", "mode": "s2s", "speaker_id": "speaker-a"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	native, ok := transformer.(interruptibleTransformer)
	if !ok {
		t.Fatalf("New() = %T, want interruptibleTransformer", transformer)
	}
	pattern, ok := native.Transformer.(patternTransformer)
	if !ok || pattern.Pattern != "model/runtime-ast?mode=s2s&source_language=zh&speaker_id=speaker-a&target_language=ja" {
		t.Fatalf("native pattern = %#v", native.Transformer)
	}

	transformer, err = New(Config{
		Transformer:   &scriptedTransformer{},
		Model:         "runtime-ast",
		Params:        map[string]any{"lang_pair": "auto", "mode": "s2s", "speaker_id": "speaker-a"},
		ExternalVoice: "voice-a",
	})
	if err != nil {
		t.Fatalf("New() external voice error = %v", err)
	}
	external, ok := transformer.(interruptibleTransformer)
	if !ok || !external.keepActiveAfterTextEOS {
		t.Fatalf("external transformer = %#v", transformer)
	}
	voice, ok := external.Transformer.(externalVoiceTransformer)
	if !ok || voice.TTSPattern != "voice/voice-a" {
		t.Fatalf("external voice = %#v", external.Transformer)
	}
	for _, notWant := range []string{"speaker_id=", "mode=s2s"} {
		if strings.Contains(voice.ASTPattern, notWant) {
			t.Fatalf("external AST pattern = %q, contains %q", voice.ASTPattern, notWant)
		}
	}
	for _, want := range []string{"mode=s2t", "source_language=zhen", "target_language=zhen", "enable_source_language_detect=true"} {
		if !strings.Contains(voice.ASTPattern, want) {
			t.Fatalf("external AST pattern = %q, missing %q", voice.ASTPattern, want)
		}
	}
}

func TestExternalVoiceTransformerForwardsASTTextAndTTSAudio(t *testing.T) {
	transformer := &scriptedTransformer{
		streams: []genx.Stream{
			streamFromChunks(
				&genx.MessageChunk{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{Label: "assistant"}, Part: genx.Text("bonjour")},
				&genx.MessageChunk{Ctrl: &genx.StreamCtrl{Label: "user"}, Part: genx.Text("ignored")},
			),
			streamFromChunks(
				&genx.MessageChunk{
					Role: genx.RoleModel,
					Ctrl: &genx.StreamCtrl{StreamID: "tts-1"},
					Part: &genx.Blob{MIMEType: "audio/mpeg; codec=mp3", Data: []byte{1, 2, 3}},
				},
			),
		},
	}
	agent := externalVoiceTransformer{
		Transformer: transformer,
		ASTPattern:  "model/ast?source_language=zh&target_language=en",
		TTSPattern:  "voice/voice-a",
	}
	out, err := agent.Transform(context.Background(), emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks, err := collectStream(out)
	if err != nil {
		t.Fatalf("collectStream() error = %v", err)
	}
	if len(transformer.patterns) != 2 || transformer.patterns[0] != agent.ASTPattern || transformer.patterns[1] != agent.TTSPattern {
		t.Fatalf("patterns = %#v", transformer.patterns)
	}
	if len(chunks) < 3 {
		t.Fatalf("output chunks = %d, want visible AST text and TTS audio routes", len(chunks))
	}
	var sawASTText bool
	var sawTTSAudio bool
	for _, chunk := range chunks {
		if text, ok := chunk.Part.(genx.Text); ok && string(text) == "bonjour" {
			sawASTText = true
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if ok && blob.MIMEType == "audio/mpeg; codec=mp3" && chunk.Ctrl != nil && chunk.Ctrl.Label == "assistant" && chunk.Ctrl.StreamID != "" {
			sawTTSAudio = true
		}
	}
	if !sawASTText {
		t.Fatalf("output chunks missing AST text: %#v", chunks)
	}
	if !sawTTSAudio {
		t.Fatalf("output chunks missing labeled TTS audio: %#v", chunks)
	}
}

func TestInterruptibleOutputDropsQueuedAssistantChunks(t *testing.T) {
	output := newInterruptibleOutput()
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("stale"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push stale text: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push stale audio: %v", err)
	}

	output.interrupt("turn-2")

	first, err := output.Next()
	if err != nil {
		t.Fatalf("Next first: %v", err)
	}
	second, err := output.Next()
	if err != nil {
		t.Fatalf("Next second: %v", err)
	}
	for _, chunk := range []*genx.MessageChunk{first, second} {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "turn-1" || chunk.Ctrl.Label != "assistant" || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != "interrupted" {
			t.Fatalf("interrupt chunk = %#v", chunk)
		}
	}
	if _, ok := first.Part.(genx.Text); !ok {
		t.Fatalf("first interrupt part = %T, want text", first.Part)
	}
	if blob, ok := second.Part.(*genx.Blob); !ok || blob.MIMEType != "audio/opus" {
		t.Fatalf("second interrupt part = %#v, want audio/opus", second.Part)
	}

	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("late-stale"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push late stale text: %v", err)
	}
	output.close()
	if chunk, err := output.Next(); err == nil || chunk != nil {
		t.Fatalf("Next after close = %#v, %v; want EOF without stale chunk", chunk, err)
	}
}

func TestInterruptibleOutputDropsASTSegmentFamily(t *testing.T) {
	output := newInterruptibleOutput()
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("stale-base"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push base text: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("stale-segment"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1:ast:2", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push segment text: %v", err)
	}

	output.interrupt("turn-2")

	for i := 0; i < 2; i++ {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("Next interrupt chunk %d: %v", i, err)
		}
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "turn-1" || chunk.Ctrl.Label != "assistant" || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != "interrupted" {
			t.Fatalf("interrupt chunk %d = %#v", i, chunk)
		}
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1:ast:2", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push late segment audio: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{2}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push late base audio: %v", err)
	}
	output.close()
	if chunk, err := output.Next(); err == nil || chunk != nil {
		t.Fatalf("Next after close = %#v, %v; want EOF without stale AST family chunk", chunk, err)
	}
}

func TestInterruptibleOutputKeepsExternalTTSPendingAfterTextEOS(t *testing.T) {
	output := newInterruptibleOutput(true)
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("translated"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push text: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", EndOfStream: true},
	}); err != nil {
		t.Fatalf("push text eos: %v", err)
	}

	output.interrupt("turn-2")

	first, err := output.Next()
	if err != nil {
		t.Fatalf("Next first interrupt: %v", err)
	}
	second, err := output.Next()
	if err != nil {
		t.Fatalf("Next second interrupt: %v", err)
	}
	for _, chunk := range []*genx.MessageChunk{first, second} {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "turn-1" || chunk.Ctrl.Label != "assistant" || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != "interrupted" {
			t.Fatalf("interrupt chunk = %#v", chunk)
		}
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push late audio: %v", err)
	}
	output.close()
	if chunk, err := output.Next(); err == nil || chunk != nil {
		t.Fatalf("Next after close = %#v, %v; want EOF without late audio", chunk, err)
	}

	output = newInterruptibleOutput(true)
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("push completed bos: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("translated"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push completed text: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", EndOfStream: true},
	}); err != nil {
		t.Fatalf("push completed text eos: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", EndOfStream: true},
	}); err != nil {
		t.Fatalf("push completed audio eos: %v", err)
	}
	output.interrupt("turn-2")
	for i := 0; i < 4; i++ {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("Next completed chunk %d: %v", i, err)
		}
		if chunk.Ctrl == nil || chunk.Ctrl.Error == "interrupted" {
			t.Fatalf("completed chunk %d = %#v, want original output without interrupt", i, chunk)
		}
	}
	output.close()
	if chunk, err := output.Next(); err == nil || chunk != nil {
		t.Fatalf("Next completed after close = %#v, %v; want EOF", chunk, err)
	}
}

func TestInterruptibleTransformerBranches(t *testing.T) {
	if _, err := (interruptibleTransformer{}).Transform(context.Background(), emptyStream{}); err == nil {
		t.Fatalf("Transform() without inner transformer succeeded, want error")
	}

	expected := errors.New("inner failed")
	failing := interruptibleTransformer{Transformer: transformFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
		return nil, expected
	})}
	if _, err := failing.Transform(context.Background(), emptyStream{}); !errors.Is(err, expected) {
		t.Fatalf("Transform() error = %v, want %v", err, expected)
	}

	forwarding := interruptibleTransformer{Transformer: transformFunc(func(_ context.Context, input genx.Stream) (genx.Stream, error) {
		return &inputEchoStream{input: input}, nil
	})}
	out, err := forwarding.Transform(context.Background(), streamFromChunks(genx.NewBeginOfStream("turn-1")))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunk, err := out.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if got := string(chunk.Part.(genx.Text)); got != "turn-1" {
		t.Fatalf("forwarded stream id = %q, want turn-1", got)
	}
	if _, err := out.Next(); !isStreamDone(err) {
		t.Fatalf("Next() after forwarded input = %v, want done", err)
	}
}

func TestInterruptibleTransformerObservesInputBeforeInnerReads(t *testing.T) {
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	innerOutput := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	transformer := interruptibleTransformer{Transformer: transformFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
		return innerOutput, nil
	})}
	out, err := transformer.Transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(context.Background(), genx.NewBeginOfStream("turn-1")); err != nil {
		t.Fatalf("Push turn-1 BOS: %v", err)
	}
	if err := innerOutput.Push(context.Background(), &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("stale"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("Push stale assistant: %v", err)
	}

	result := make(chan *genx.MessageChunk, 1)
	errs := make(chan error, 1)
	go func() {
		chunk, err := out.Next()
		if err != nil {
			errs <- err
			return
		}
		result <- chunk
	}()

	time.Sleep(50 * time.Millisecond)
	if err := input.Push(context.Background(), genx.NewBeginOfStream("turn-2")); err != nil {
		t.Fatalf("Push turn-2 BOS: %v", err)
	}

	select {
	case err := <-errs:
		t.Fatalf("Next() error = %v", err)
	case chunk := <-result:
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "turn-1" || chunk.Ctrl.Label != "assistant" || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != "interrupted" {
			t.Fatalf("first output = %#v, want interrupted EOS for stale assistant", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("Next() timed out")
	}
}

func TestInterruptibleOutputCloseBranches(t *testing.T) {
	output := newInterruptibleOutput()
	output.interrupt("unused")
	output.closeWithError(errors.New("boom"))
	if chunk, err := output.Next(); err == nil || err.Error() != "boom" || chunk != nil {
		t.Fatalf("Next() after closeWithError = %#v, %v; want boom", chunk, err)
	}
	if err := output.push(&genx.MessageChunk{Part: genx.Text("late")}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("push() after closeWithError = %v, want ErrClosedPipe", err)
	}

	output = newInterruptibleOutput()
	output.close()
	if err := output.push(&genx.MessageChunk{Part: genx.Text("late")}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("push() after close = %v, want ErrClosedPipe", err)
	}
	if chunk, err := output.Next(); !errors.Is(err, io.EOF) || chunk != nil {
		t.Fatalf("Next() after close = %#v, %v; want EOF", chunk, err)
	}
}

func TestForwardASTTranslateTTSDecodesOggOpus(t *testing.T) {
	raw := marshalOggPackets(t,
		[]byte("OpusHead\x01\x02"),
		[]byte("OpusTags\x01\x02"),
		[]byte{0xaa, 0xbb},
	)
	input := streamFromChunks(&genx.MessageChunk{
		Ctrl: &genx.StreamCtrl{StreamID: "ogg-1", EndOfStream: true},
		Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: raw},
	})
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	if err := forwardASTTranslateTTS(context.Background(), input, output); err != nil {
		t.Fatalf("forwardASTTranslateTTS() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("output.Done() error = %v", err)
	}
	chunks, err := collectStream(output.Stream())
	if err != nil {
		t.Fatalf("collectStream() error = %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("output chunks = %d, want opus frame and EOS", len(chunks))
	}
	frame, ok := chunks[0].Part.(*genx.Blob)
	if !ok || frame.MIMEType != "audio/opus" || string(frame.Data) != string([]byte{0xaa, 0xbb}) {
		t.Fatalf("frame chunk = %#v", chunks[0].Part)
	}
	eos, ok := chunks[1].Part.(*genx.Blob)
	if !ok || eos.MIMEType != "audio/opus" || len(eos.Data) != 0 || !chunks[1].IsEndOfStream() {
		t.Fatalf("EOS chunk = %#v ctrl=%#v", chunks[1].Part, chunks[1].Ctrl)
	}
}

func TestASTTranslateOggOpusFrameDecoder(t *testing.T) {
	raw := marshalOggPackets(t,
		[]byte("OpusHead\x01\x02"),
		[]byte("OpusTags\x01\x02"),
		[]byte{0x11, 0x22, 0x33},
	)
	decoder := newASTTranslateOggOpusFrameDecoder()
	frames, err := decoder.Write(raw[:10])
	if err != nil {
		t.Fatalf("Write(partial) error = %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("partial frames = %#v, want none", frames)
	}
	frames, err = decoder.Write(raw[10:])
	if err != nil {
		t.Fatalf("Write(rest) error = %v", err)
	}
	if len(frames) != 1 || string(frames[0]) != string([]byte{0x11, 0x22, 0x33}) {
		t.Fatalf("frames = %#v", frames)
	}
	if err := decoder.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := newASTTranslateOggOpusFrameDecoder().Write([]byte("bad")); err == nil {
		t.Fatalf("Write(bad prefix) succeeded, want error")
	}
	truncated := newASTTranslateOggOpusFrameDecoder()
	if _, err := truncated.Write([]byte("OggS")); err != nil {
		t.Fatalf("Write(truncated prefix) error = %v", err)
	}
	if err := truncated.Close(); err == nil {
		t.Fatalf("Close(truncated) succeeded, want error")
	}
}

func TestNewErrors(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatalf("New() without transformer succeeded, want error")
	}
	if _, err := New(Config{Transformer: &scriptedTransformer{}}); err == nil {
		t.Fatalf("New() without model succeeded, want error")
	}
	if _, err := (patternTransformer{}).Transform(context.Background(), emptyStream{}); err == nil {
		t.Fatalf("patternTransformer.Transform() without transformer succeeded, want error")
	}
}

func TestASTTranslateUtilityBranches(t *testing.T) {
	if got := voicePattern(" voice-a "); got != "voice/voice-a" {
		t.Fatalf("voicePattern(simple) = %q", got)
	}
	if got := voicePattern("volc-tenant:name:voice"); got != "voice/volc-tenant:name:voice" {
		t.Fatalf("voicePattern(colon) = %q", got)
	}
	if got := voicePattern("kind/name"); got != "kind/name" {
		t.Fatalf("voicePattern(path) = %q", got)
	}
	if got := baseMIME(" Audio/OGG ; codecs=opus "); got != "audio/ogg" {
		t.Fatalf("baseMIME() = %q", got)
	}
	textChunk := &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("translated"),
		Ctrl: &genx.StreamCtrl{Label: "assistant"},
	}
	if !shouldGraceASTAssistantChunk(textChunk) {
		t.Fatalf("assistant text chunk did not retain interrupt grace")
	}
	audioChunk := &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{Label: "assistant"},
	}
	if shouldGraceASTAssistantChunk(audioChunk) {
		t.Fatalf("assistant audio chunk retained per-frame interrupt grace")
	}
	if !isStreamDone(genx.ErrDone) || !isStreamDone(io.EOF) || isStreamDone(errors.New("boom")) {
		t.Fatalf("isStreamDone returned unexpected values")
	}
	if err := normalizeLanguagePair(nil, true); err == nil {
		t.Fatalf("normalizeLanguagePair(nil, required) succeeded, want error")
	}
	params := map[string]any{"lang_pair": " "}
	if err := normalizeLanguagePair(params, false); err != nil {
		t.Fatalf("normalizeLanguagePair(optional empty) error = %v", err)
	}
	if got := appendPatternParams("model/demo?existing=1", map[string]any{"bad": 1.2, "ok": true}); got != "model/demo?existing=1&ok=true" {
		t.Fatalf("appendPatternParams() = %q", got)
	}
}

type transformFunc func(context.Context, genx.Stream) (genx.Stream, error)

func (f transformFunc) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return f(ctx, input)
}

type inputEchoStream struct {
	input genx.Stream
}

func (s *inputEchoStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.input.Next()
	if err != nil || chunk == nil {
		return nil, err
	}
	streamID := ""
	if chunk.Ctrl != nil {
		streamID = chunk.Ctrl.StreamID
	}
	return &genx.MessageChunk{Part: genx.Text(streamID)}, nil
}

func (s *inputEchoStream) Close() error {
	return s.input.Close()
}

func (s *inputEchoStream) CloseWithError(err error) error {
	return s.input.CloseWithError(err)
}

type scriptedTransformer struct {
	patterns []string
	streams  []genx.Stream
}

func (t *scriptedTransformer) Transform(_ context.Context, pattern string, _ genx.Stream) (genx.Stream, error) {
	t.patterns = append(t.patterns, pattern)
	if len(t.streams) == 0 {
		return nil, io.EOF
	}
	stream := t.streams[0]
	t.streams = t.streams[1:]
	return stream, nil
}

type emptyStream struct{}

func (emptyStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }
func (emptyStream) Close() error                      { return nil }
func (emptyStream) CloseWithError(error) error        { return nil }

func streamFromChunks(chunks ...*genx.MessageChunk) genx.Stream {
	builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), len(chunks)+1)
	_ = builder.Add(chunks...)
	_ = builder.Done(genx.Usage{})
	return builder.Stream()
}

func collectStream(stream genx.Stream) ([]*genx.MessageChunk, error) {
	defer stream.Close()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				return chunks, nil
			}
			return nil, err
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
}

func marshalOggPackets(t *testing.T, packets ...[]byte) []byte {
	t.Helper()
	var pages []*ogg.Page
	for i, packet := range packets {
		packetPages, err := ogg.BuildPacketPages(1, uint32(i), packet, uint64(i), i == 0, i == len(packets)-1)
		if err != nil {
			t.Fatalf("BuildPacketPages(%d): %v", i, err)
		}
		pages = append(pages, packetPages...)
	}
	raw, err := ogg.MarshalPages(pages)
	if err != nil {
		t.Fatalf("MarshalPages(): %v", err)
	}
	return raw
}
