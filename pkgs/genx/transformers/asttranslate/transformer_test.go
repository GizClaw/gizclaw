package asttranslate

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
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
				&genx.MessageChunk{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: "ast-1", Label: "assistant", BeginOfStream: true}},
				&genx.MessageChunk{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: "ast-1", Label: "assistant"}, Part: genx.Text("bonjour")},
				&genx.MessageChunk{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: "ast-1", Label: "assistant", EndOfStream: true}, Part: genx.Text("")},
				&genx.MessageChunk{Ctrl: &genx.StreamCtrl{Label: "user"}, Part: genx.Text("ignored")},
			),
			streamFromChunks(
				&genx.MessageChunk{
					Role: genx.RoleModel,
					Ctrl: &genx.StreamCtrl{StreamID: "tts-1", BeginOfStream: true},
					Part: &genx.Blob{MIMEType: "audio/mpeg; codec=mp3"},
				},
				&genx.MessageChunk{
					Role: genx.RoleModel,
					Ctrl: &genx.StreamCtrl{StreamID: "tts-1"},
					Part: &genx.Blob{MIMEType: "audio/mpeg; codec=mp3", Data: []byte{1, 2, 3}},
				},
				&genx.MessageChunk{
					Role: genx.RoleModel,
					Ctrl: &genx.StreamCtrl{StreamID: "tts-1", EndOfStream: true},
					Part: &genx.Blob{MIMEType: "audio/mpeg; codec=mp3"},
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
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("push stale text BOS: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("stale"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push stale text: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("push stale audio BOS: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push stale audio: %v", err)
	}

	output.interrupt("turn-2")

	var interrupted []*genx.MessageChunk
	for i := range 4 {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("Next interrupt chunk %d: %v", i, err)
		}
		interrupted = append(interrupted, chunk)
	}
	requireASTInterruptedRoutes(t, interrupted, "turn-1")

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

func TestInterruptibleOutputClosesRepeatedTurnsBeforeReplacementBOS(t *testing.T) {
	output := newInterruptibleOutput()
	var observed []*genx.MessageChunk
	startTurn := func(streamID string, sample byte) {
		t.Helper()
		chunks := []*genx.MessageChunk{
			{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", BeginOfStream: true}},
			{Role: genx.RoleModel, Part: genx.Text("partial"), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant"}},
			{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", BeginOfStream: true}},
			{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{sample}}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant"}},
		}
		for _, chunk := range chunks {
			if err := output.push(chunk); err != nil {
				t.Fatalf("push %s: %v", streamID, err)
			}
		}
		for range chunks {
			chunk, err := output.Next()
			if err != nil {
				t.Fatalf("Next %s: %v", streamID, err)
			}
			observed = append(observed, chunk)
		}
	}
	interruptTurn := func(nextID string) {
		t.Helper()
		output.interrupt(nextID)
		for range 2 {
			chunk, err := output.Next()
			if err != nil {
				t.Fatalf("Next interrupt before %s: %v", nextID, err)
			}
			observed = append(observed, chunk)
		}
	}

	startTurn("turn-1", 1)
	interruptTurn("turn-2")
	startTurn("turn-2", 2)
	interruptTurn("turn-3")
	startTurn("turn-3", 3)
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-3", Label: "assistant", EndOfStream: true}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-3", Label: "assistant", EndOfStream: true}},
	} {
		if err := output.push(chunk); err != nil {
			t.Fatalf("push final EOS: %v", err)
		}
	}
	output.close()
	for {
		chunk, err := output.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next final: %v", err)
		}
		observed = append(observed, chunk)
	}

	requireASTHandoffOrder(t, observed, "turn-1", "turn-2")
	requireASTHandoffOrder(t, observed, "turn-2", "turn-3")
}

func TestInterruptibleOutputBlocksEveryActiveResponse(t *testing.T) {
	output := newInterruptibleOutput()
	for _, streamID := range []string{"first", "second"} {
		for _, chunk := range []*genx.MessageChunk{
			{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", BeginOfStream: true}},
			{Role: genx.RoleModel, Part: genx.Text("partial"), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant"}},
		} {
			if err := output.push(chunk); err != nil {
				t.Fatal(err)
			}
			if _, err := output.Next(); err != nil {
				t.Fatal(err)
			}
		}
	}

	output.interrupt("replacement")
	for _, streamID := range []string{"first", "second"} {
		if !output.isBlockedStream(streamID) {
			t.Fatalf("active response %q was not blocked", streamID)
		}
	}
}

func TestInterruptibleOutputDropsASTSegmentFamily(t *testing.T) {
	output := newInterruptibleOutput()
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("push base text BOS: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("stale-base"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push base text: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1:ast:2", Label: "assistant", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("push segment text BOS: %v", err)
	}
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("stale-segment"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1:ast:2", Label: "assistant"},
	}); err != nil {
		t.Fatalf("push segment text: %v", err)
	}

	output.interrupt("turn-2")

	var interrupted []*genx.MessageChunk
	for i := range 4 {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("Next interrupt chunk %d: %v", i, err)
		}
		interrupted = append(interrupted, chunk)
	}
	requireASTInterruptedRoutes(t, interrupted, "turn-1")
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
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("push text BOS: %v", err)
	}
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

	var interrupted []*genx.MessageChunk
	for i := range 4 {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("Next interrupt chunk %d: %v", i, err)
		}
		interrupted = append(interrupted, chunk)
	}
	requireASTInterruptedRoutes(t, interrupted, "turn-1")
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
	var queuedInterrupted []*genx.MessageChunk
	for i := range 4 {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("Next queued interrupt chunk %d: %v", i, err)
		}
		queuedInterrupted = append(queuedInterrupted, chunk)
	}
	requireASTInterruptedRoutes(t, queuedInterrupted, "turn-1")
	output.close()
	if chunk, err := output.Next(); err == nil || chunk != nil {
		t.Fatalf("Next queued interrupt after close = %#v, %v; want EOF", chunk, err)
	}
}

func TestInterruptibleOutputDoesNotInterruptDeliveredResponse(t *testing.T) {
	output := newInterruptibleOutput()
	chunks := []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", BeginOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("translated"), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", EndOfStream: true}},
	}
	for _, chunk := range chunks {
		if err := output.push(chunk); err != nil {
			t.Fatalf("push completed response: %v", err)
		}
	}
	for range chunks {
		if _, err := output.Next(); err != nil {
			t.Fatalf("Next completed response: %v", err)
		}
	}
	output.interrupt("turn-2")
	output.close()
	if chunk, err := output.Next(); err == nil || chunk != nil {
		t.Fatalf("Next after delivered response = %#v, %v; want EOF", chunk, err)
	}
}

func TestInterruptibleOutputInterruptsPulledButUnobservedAudioEOS(t *testing.T) {
	output := newInterruptibleOutput()
	output.DeferOutputObservation()
	chunks := []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", BeginOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("translated"), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", EndOfStream: true}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", BeginOfStream: true}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", EndOfStream: true}},
	}
	for _, chunk := range chunks {
		if err := output.push(chunk); err != nil {
			t.Fatalf("push response: %v", err)
		}
	}
	for index := range chunks {
		pulled, err := output.Next()
		if err != nil {
			t.Fatalf("Next response chunk %d: %v", index, err)
		}
		if pulled != chunks[index] {
			t.Fatalf("Next response chunk %d = %p, want %p", index, pulled, chunks[index])
		}
		if index < len(chunks)-1 {
			output.ObserveOutput(pulled)
		}
	}

	output.interrupt("turn-2")
	interrupted, err := output.Next()
	if err != nil {
		t.Fatalf("Next audio interrupt: %v", err)
	}
	blob, audio := interrupted.Part.(*genx.Blob)
	if !audio || blob.MIMEType != "audio/opus" || !interrupted.IsEndOfStream() || interrupted.Ctrl.Error != "interrupted" {
		t.Fatalf("audio interrupt = %#v", interrupted)
	}
	output.AbandonOutputObservation(chunks[len(chunks)-1])
	output.ObserveOutput(interrupted)
	output.close()
	if chunk, err := output.Next(); err == nil || chunk != nil {
		t.Fatalf("Next after interrupt = %#v, %v; want EOF", chunk, err)
	}
}

func TestInterruptibleTransformerBranches(t *testing.T) {
	if _, err := (interruptibleTransformer{}).Transform(context.Background(), emptyStream{}); err == nil {
		t.Fatalf("Transform() without inner transformer succeeded, want error")
	}
	if _, err := (interruptibleTransformer{Transformer: transformFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
		t.Fatal("inner transformer was called for a nil input stream")
		return nil, nil
	})}).Transform(context.Background(), nil); err == nil {
		t.Fatalf("Transform() with nil input stream succeeded, want error")
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
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push stale assistant BOS: %v", err)
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
		interrupted := []*genx.MessageChunk{chunk}
		for i := 1; i < 4; i++ {
			next, err := out.Next()
			if err != nil {
				t.Fatalf("Next interrupt chunk %d: %v", i, err)
			}
			interrupted = append(interrupted, next)
		}
		requireASTInterruptedRoutes(t, interrupted, "turn-1")
	case <-time.After(time.Second):
		t.Fatal("Next() timed out")
	}
}

func TestInterruptibleTransformerClosesRepeatedTurnsAndDropsLateOutput(t *testing.T) {
	ctx := t.Context()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	innerOutput := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	inputBOS := make(chan string, 3)
	transformer := interruptibleTransformer{Transformer: transformFunc(func(_ context.Context, observedInput genx.Stream) (genx.Stream, error) {
		go func() {
			for {
				chunk, err := observedInput.Next()
				if err != nil {
					return
				}
				if chunk != nil && chunk.IsBeginOfStream() && chunk.Ctrl != nil {
					inputBOS <- chunk.Ctrl.StreamID
				}
			}
		}()
		return innerOutput, nil
	})}
	out, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	var observed []*genx.MessageChunk
	pushInputBOS := func(streamID string) {
		t.Helper()
		if err := input.Push(ctx, genx.NewBeginOfStream(streamID)); err != nil {
			t.Fatalf("Push input BOS %s: %v", streamID, err)
		}
		select {
		case observedID := <-inputBOS:
			if observedID != streamID {
				t.Fatalf("observed input BOS = %s, want %s", observedID, streamID)
			}
		case <-time.After(time.Second):
			t.Fatalf("input BOS %s was not observed", streamID)
		}
	}
	pushInner := func(chunk *genx.MessageChunk) {
		t.Helper()
		if err := innerOutput.Push(ctx, chunk); err != nil {
			t.Fatalf("Push inner chunk: %v", err)
		}
	}
	read := func(count int) {
		t.Helper()
		for range count {
			chunk, err := out.Next()
			if err != nil {
				t.Fatalf("Next output: %v", err)
			}
			observed = append(observed, chunk)
		}
	}
	startAssistant := func(streamID string, sample byte) {
		t.Helper()
		for _, chunk := range []*genx.MessageChunk{
			{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", BeginOfStream: true}},
			{Role: genx.RoleModel, Part: genx.Text("answer-" + streamID), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant"}},
			{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", BeginOfStream: true}},
			{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{sample}}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant"}},
		} {
			pushInner(chunk)
		}
		read(4)
	}
	interruptFor := func(nextInputID string) {
		t.Helper()
		pushInputBOS(nextInputID)
		read(2)
	}

	pushInputBOS("input-1")
	startAssistant("turn-1", 1)
	interruptFor("input-2")
	pushInner(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("late-turn-1"), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant"}})
	startAssistant("turn-2", 2)
	interruptFor("input-3")
	pushInner(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("late-turn-2")}, Ctrl: &genx.StreamCtrl{StreamID: "turn-2", Label: "assistant"}})
	startAssistant("turn-3", 3)
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-3", Label: "assistant", EndOfStream: true}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-3", Label: "assistant", EndOfStream: true}},
	} {
		pushInner(chunk)
	}
	read(2)

	requireASTHandoffOrder(t, observed, "turn-1", "turn-2")
	requireASTHandoffOrder(t, observed, "turn-2", "turn-3")
	for _, chunk := range observed {
		switch part := chunk.Part.(type) {
		case genx.Text:
			if strings.HasPrefix(string(part), "late-") {
				t.Fatalf("late text escaped: %#v", chunk)
			}
		case *genx.Blob:
			if part != nil && strings.HasPrefix(string(part.Data), "late-") {
				t.Fatalf("late audio escaped: %#v", chunk)
			}
		}
	}
	_ = innerOutput.Close()
	_ = input.Close()
}

func requireASTInterruptedRoutes(t *testing.T, chunks []*genx.MessageChunk, streamID string) {
	t.Helper()
	routes := make(map[string][]*genx.MessageChunk)
	for _, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.StreamID != streamID || chunk.Ctrl.Label != "assistant" {
			t.Fatalf("interrupt chunk = %#v, want assistant stream %q", chunk, streamID)
		}
		mimeType, ok := chunk.MIMEType()
		if !ok {
			t.Fatalf("interrupt chunk = %#v, want MIME-bearing boundary", chunk)
		}
		routes[mimeType] = append(routes[mimeType], chunk)
	}
	if len(routes) != 2 {
		t.Fatalf("interrupted routes = %#v, want text and audio", routes)
	}
	for mimeType, route := range routes {
		if len(route) != 2 || !route[0].IsBeginOfStream() || route[0].IsEndOfStream() ||
			route[1].IsBeginOfStream() || !route[1].IsEndOfStream() || route[1].Ctrl.Error != "interrupted" {
			t.Fatalf("interrupted route %q = %#v, want BOS/error EOS", mimeType, route)
		}
	}
}

func requireASTHandoffOrder(t *testing.T, chunks []*genx.MessageChunk, previousID, nextID string) {
	t.Helper()
	lastPreviousEOS := -1
	firstNextBOS := -1
	previousEOS := make(map[string]bool)
	ended := make(map[string]bool)
	for index, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil || chunk.Role != genx.RoleModel || chunk.Ctrl.Label != "assistant" {
			continue
		}
		mimeType, ok := chunk.MIMEType()
		if !ok {
			continue
		}
		if chunk.Ctrl.StreamID == previousID {
			if ended[mimeType] {
				t.Fatalf("%s emitted %s after EOS at index %d: %#v", previousID, mimeType, index, chunks)
			}
			if chunk.IsEndOfStream() {
				if chunk.Ctrl.Error != "interrupted" {
					t.Fatalf("%s %s EOS error = %q, want interrupted", previousID, mimeType, chunk.Ctrl.Error)
				}
				ended[mimeType] = true
				previousEOS[mimeType] = true
				lastPreviousEOS = max(lastPreviousEOS, index)
			}
		}
		if chunk.Ctrl.StreamID == nextID && chunk.IsBeginOfStream() && firstNextBOS < 0 {
			firstNextBOS = index
		}
	}
	if !previousEOS["text/plain"] || !previousEOS["audio/opus"] {
		t.Fatalf("%s interrupted EOS routes = %#v, want text/plain and audio/opus: %#v", previousID, previousEOS, chunks)
	}
	if firstNextBOS < 0 || lastPreviousEOS >= firstNextBOS {
		t.Fatalf("%s last EOS index = %d, %s first BOS index = %d: %#v", previousID, lastPreviousEOS, nextID, firstNextBOS, chunks)
	}
}

func TestObservedInputBoundsQueueAndCancellationUnblocksProducer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &repeatingInputStream{}
	observed := newObservedInputStream(ctx, source, nil)
	deadline := time.Now().Add(time.Second)
	for {
		observed.mu.Lock()
		queued := len(observed.queue)
		observed.mu.Unlock()
		if queued == observedInputQueueCapacity {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("observed input queue = %d, want %d", queued, observedInputQueueCapacity)
		}
		time.Sleep(time.Millisecond)
	}
	if got, want := source.nexts.Load(), int32(observedInputQueueCapacity+1); got != want {
		t.Fatalf("source.Next() calls = %d, want one in-flight chunk beyond capacity %d", got, want)
	}
	cancel()
	deadline = time.Now().Add(time.Second)
	for !source.closed.Load() {
		if time.Now().After(deadline) {
			t.Fatal("source was not closed after cancellation")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := observed.Next(); !errors.Is(err, context.Canceled) {
		t.Fatalf("observed.Next() after cancellation = %v, want context canceled", err)
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
		Ctrl: &genx.StreamCtrl{StreamID: "ogg-1", BeginOfStream: true, EndOfStream: true},
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
	if len(chunks) != 3 {
		t.Fatalf("output chunks = %d, want BOS, opus frame, and EOS", len(chunks))
	}
	bos, ok := chunks[0].Part.(*genx.Blob)
	if !ok || bos.MIMEType != "audio/opus" || len(bos.Data) != 0 || !chunks[0].IsBeginOfStream() {
		t.Fatalf("BOS chunk = %#v ctrl=%#v", chunks[0].Part, chunks[0].Ctrl)
	}
	frame, ok := chunks[1].Part.(*genx.Blob)
	if !ok || frame.MIMEType != "audio/opus" || string(frame.Data) != string([]byte{0xaa, 0xbb}) || chunks[1].IsBeginOfStream() || chunks[1].IsEndOfStream() {
		t.Fatalf("frame chunk = %#v ctrl=%#v", chunks[1].Part, chunks[1].Ctrl)
	}
	eos, ok := chunks[2].Part.(*genx.Blob)
	if !ok || eos.MIMEType != "audio/opus" || len(eos.Data) != 0 || !chunks[2].IsEndOfStream() {
		t.Fatalf("EOS chunk = %#v ctrl=%#v", chunks[2].Part, chunks[2].Ctrl)
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
	if isStreamDone(nil) || !isStreamDone(genx.ErrDone) || !isStreamDone(io.EOF) || isStreamDone(errors.New("boom")) {
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

type repeatingInputStream struct {
	nexts  atomic.Int32
	closed atomic.Bool
}

func (s *repeatingInputStream) Next() (*genx.MessageChunk, error) {
	s.nexts.Add(1)
	return &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}}, nil
}

func (s *repeatingInputStream) Close() error {
	s.closed.Store(true)
	return nil
}

func (s *repeatingInputStream) CloseWithError(err error) error {
	s.closed.Store(true)
	return nil
}

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
