package transformers

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestDoubaoASTTranslateStreamsTranslationAndAudio(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	input := newBufferStream(4)
	sourcePacket := buildASTTranslateRawOpusPacket(t, buildASRAudioFrame(doubaoASTTranslateSourceSampleRate/50, 1))
	tr := NewDoubaoASTTranslate(doubaospeech.NewClient("app-id"),
		WithDoubaoASTTranslateMode(doubaospeech.ASTTranslateModeS2S),
		WithDoubaoASTTranslateSourceLanguage("zh"),
		WithDoubaoASTTranslateTargetLanguage("ja"),
	)
	fake := &fakeASTTranslateSession{
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventSourceSubtitleStart},
			{Type: doubaospeech.ASTEventSourceSubtitleResponse, Text: "你好"},
			{Type: doubaospeech.ASTEventSourceSubtitleEnd, Text: "你好"},
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "こんにちは"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "こんにちは"},
			{Type: doubaospeech.ASTEventTTSSentenceStart},
			{Type: doubaospeech.ASTEventTTSResponse, Audio: buildASTTranslateOggPackets(t, astTranslateOpusHeadPacket(48000, 1), astTranslateOpusTagsPacket("test"), []byte{1, 2, 3})},
			{Type: doubaospeech.ASTEventTTSSentenceEnd},
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	tr.newSession = func(_ context.Context, cfg doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		if cfg.Mode != doubaospeech.ASTTranslateModeS2S || cfg.SourceLanguage != "zh" || cfg.TargetLanguage != "ja" {
			t.Fatalf("cfg = %+v, want s2s zh->ja", cfg)
		}
		return fake, nil
	}
	out, err := tr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}}); err != nil {
		t.Fatalf("Push(BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: sourcePacket},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Timestamp: 1_000},
	}); err != nil {
		t.Fatalf("Push(audio): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}}); err != nil {
		t.Fatalf("Push(EOS): %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}

	chunks := readAllASTTranslateChunks(t, out)
	if len(fake.sentAudio) != 1 || len(fake.sentAudio[0]) == 0 || bytes.Equal(fake.sentAudio[0], sourcePacket) {
		t.Fatalf("sentAudio = %v", fake.sentAudio)
	}
	if !fake.finished {
		t.Fatalf("session was not finished")
	}
	assertASTTranslateTextChunk(t, chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-1", "你好")
	assertASTTranslateHistoryAudioChunk(t, chunks, "turn-1", sourcePacket)
	assertASTTranslateTextChunk(t, chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1", "こんにちは")
	assertASTTranslateAudioChunk(t, chunks, "turn-1", []byte{1, 2, 3})
	assertASTTranslateEOS(t, chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-1")
	assertASTTranslateEOS(t, chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1")
}

func TestDoubaoASTTranslateSplitsProviderSubtitleSegments(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	input := newBufferStream(8)
	firstAudio := buildASTTranslateRawOpusPacket(t, buildASRAudioFrame(doubaoASTTranslateSourceSampleRate/50, 1))
	secondAudio := buildASTTranslateRawOpusPacket(t, buildASRAudioFrame(doubaoASTTranslateSourceSampleRate/50, 1))
	tr := NewDoubaoASTTranslate(doubaospeech.NewClient("app-id"),
		WithDoubaoASTTranslateMode(doubaospeech.ASTTranslateModeS2S),
		WithDoubaoASTTranslateSourceLanguage("zh"),
		WithDoubaoASTTranslateTargetLanguage("ja"),
	)
	fake := &fakeASTTranslateSession{
		beforeRecv:        make(chan struct{}),
		notifySentAudioAt: 2,
		sentAudioNotify:   make(chan struct{}),
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventSourceSubtitleStart},
			{Type: doubaospeech.ASTEventSourceSubtitleResponse, Text: "第一段"},
			{Type: doubaospeech.ASTEventSourceSubtitleEnd, Text: "第一段", StartTimeMS: 0, EndTimeMS: 20},
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "一つ目"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "一つ目"},
			{Type: doubaospeech.ASTEventTTSSentenceStart},
			{Type: doubaospeech.ASTEventTTSResponse, Audio: buildASTTranslateOggPackets(t, astTranslateOpusHeadPacket(48000, 1), astTranslateOpusTagsPacket("test"), []byte{1, 2, 3})},
			{Type: doubaospeech.ASTEventTTSSentenceEnd},
			{Type: doubaospeech.ASTEventSourceSubtitleStart},
			{Type: doubaospeech.ASTEventSourceSubtitleResponse, Text: "第二段"},
			{Type: doubaospeech.ASTEventSourceSubtitleEnd, Text: "第二段", StartTimeMS: 20, EndTimeMS: 40},
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "二つ目"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "二つ目"},
			{Type: doubaospeech.ASTEventTTSSentenceStart},
			{Type: doubaospeech.ASTEventTTSResponse, Audio: buildASTTranslateOggPackets(t, astTranslateOpusHeadPacket(48000, 1), astTranslateOpusTagsPacket("test"), []byte{4, 5, 6})},
			{Type: doubaospeech.ASTEventTTSSentenceEnd},
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		return fake, nil
	}
	out, err := tr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}}); err != nil {
		t.Fatalf("Push(BOS): %v", err)
	}
	for i, data := range [][]byte{firstAudio, secondAudio} {
		if err := input.Push(&genx.MessageChunk{
			Part: &genx.Blob{MIMEType: "audio/opus", Data: data},
			Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Timestamp: 1_000 + int64(i*20)},
		}); err != nil {
			t.Fatalf("Push(audio): %v", err)
		}
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}}); err != nil {
		t.Fatalf("Push(EOS): %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}
	fake.waitSentAudio(t)
	close(fake.beforeRecv)

	chunks := readAllASTTranslateChunks(t, out)
	assertASTTranslateTextChunk(t, chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-1", "第一段")
	assertASTTranslateHistoryAudioChunk(t, chunks, "turn-1", firstAudio)
	assertASTTranslateTextChunk(t, chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1", "一つ目")
	assertASTTranslateAudioChunk(t, chunks, "turn-1", []byte{1, 2, 3})
	assertASTTranslateTextChunk(t, chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-1:ast:2", "第二段")
	assertASTTranslateHistoryAudioChunk(t, chunks, "turn-1:ast:2", secondAudio)
	assertASTTranslateTextChunk(t, chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1:ast:2", "二つ目")
	assertASTTranslateAudioChunk(t, chunks, "turn-1:ast:2", []byte{4, 5, 6})
	assertASTTranslateEOS(t, chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-1")
	assertASTTranslateEOS(t, chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-1:ast:2")
	assertASTTranslateEOS(t, chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1")
	assertASTTranslateEOS(t, chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1:ast:2")
}

func TestDoubaoASTTranslatePushToTalkKeepsProviderSegmentsInOneTurn(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	input := newBufferStream(8)
	audioPacket := buildASTTranslateRawOpusPacket(t, buildASRAudioFrame(doubaoASTTranslateSourceSampleRate/50, 1))
	tr := NewDoubaoASTTranslate(doubaospeech.NewClient("app-id"),
		WithDoubaoASTTranslateMode(doubaospeech.ASTTranslateModeS2S),
		WithDoubaoASTTranslateInputMode(DoubaoASTTranslateInputModePushToTalk),
	)
	providerEventsDone := make(chan struct{})
	fake := &fakeASTTranslateSession{
		doneCh: providerEventsDone,
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventSourceSubtitleStart},
			{Type: doubaospeech.ASTEventSourceSubtitleResponse, Text: "好"},
			{Type: doubaospeech.ASTEventSourceSubtitleEnd, Text: "好的", StartTimeMS: 0, EndTimeMS: 20},
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "Okay"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "Okay"},
			{Type: doubaospeech.ASTEventTTSSentenceStart},
			{Type: doubaospeech.ASTEventTTSResponse, Audio: buildASTTranslateOggPackets(t, astTranslateOpusHeadPacket(48000, 1), astTranslateOpusTagsPacket("test"), []byte{1, 2, 3})},
			{Type: doubaospeech.ASTEventTTSSentenceEnd},
			{Type: doubaospeech.ASTEventSourceSubtitleStart},
			{Type: doubaospeech.ASTEventSourceSubtitleResponse, Text: "我们继续下一轮测试"},
			{Type: doubaospeech.ASTEventSourceSubtitleEnd, Text: "好的我们继续下一轮测试", StartTimeMS: 20, EndTimeMS: 40},
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "let's continue"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "Okay let's continue to the next round of testing."},
			{Type: doubaospeech.ASTEventTTSSentenceStart},
			{Type: doubaospeech.ASTEventTTSResponse, Audio: buildASTTranslateOggPackets(t, astTranslateOpusHeadPacket(48000, 1), astTranslateOpusTagsPacket("test"), []byte{4, 5, 6})},
			{Type: doubaospeech.ASTEventTTSSentenceEnd},
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		return fake, nil
	}
	out, err := tr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}}); err != nil {
		t.Fatalf("Push(BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: audioPacket},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Timestamp: 1_000},
	}); err != nil {
		t.Fatalf("Push(audio): %v", err)
	}
	select {
	case <-providerEventsDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pre-EOS provider events")
	}
	type nextResult struct {
		chunk *genx.MessageChunk
		err   error
	}
	firstResult := make(chan nextResult, 1)
	go func() {
		chunk, err := out.Next()
		firstResult <- nextResult{chunk: chunk, err: err}
	}()
	select {
	case result := <-firstResult:
		t.Fatalf("pre-EOS output = chunk=%#v err=%v, want blocked", result.chunk, result.err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}}); err != nil {
		t.Fatalf("Push(EOS): %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}

	var first nextResult
	select {
	case first = <-firstResult:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for committed output")
	}
	if first.err != nil || first.chunk == nil {
		t.Fatalf("first committed output = chunk=%#v err=%v", first.chunk, first.err)
	}
	chunks := append([]*genx.MessageChunk{first.chunk}, readAllASTTranslateChunks(t, out)...)
	if got := collectASTTranslateText(chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-1"); got != "好的我们继续下一轮测试" {
		t.Fatalf("transcript text = %q, want full push-to-talk transcript", got)
	}
	if got := collectASTTranslateText(chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1"); got != "Okay let's continue to the next round of testing." {
		t.Fatalf("assistant text = %q, want full push-to-talk translation", got)
	}
	assertASTTranslateAudioChunk(t, chunks, "turn-1", []byte{1, 2, 3})
	assertASTTranslateAudioChunk(t, chunks, "turn-1", []byte{4, 5, 6})
	assertASTTranslateHistoryAudioChunk(t, chunks, "turn-1", audioPacket)
	if got := countASTTranslateTextEOS(chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1"); got != 1 {
		t.Fatalf("assistant text EOS count = %d, want 1; chunks=%#v", got, chunks)
	}
	if got := countASTTranslateAudioEOS(chunks, "turn-1"); got != 1 {
		t.Fatalf("assistant audio EOS count = %d, want 1; chunks=%#v", got, chunks)
	}
	if got := countASTTranslateTextEOS(chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-1"); got != 1 {
		t.Fatalf("transcript EOS count = %d, want 1; chunks=%#v", got, chunks)
	}
	if got := countASTTranslateHistoryAudioEOS(chunks, "turn-1"); got != 1 {
		t.Fatalf("history audio EOS count = %d, want 1; chunks=%#v", got, chunks)
	}
	if got := countASTTranslateStreamChunks(chunks, "turn-1:ast:2"); got != 0 {
		t.Fatalf("split stream chunks = %d, want 0; chunks=%#v", got, chunks)
	}
}

func TestDoubaoASTTranslatePTTOutputGateOpusDurationLimit(t *testing.T) {
	packet := []byte{0x98}
	packetDuration := time.Duration(historyOpusPacketDurationMS(packet)) * time.Millisecond
	if packetDuration <= 0 || doubaoASTTranslatePTTOutputLimit%packetDuration != 0 {
		t.Fatalf("output limit %s is not divisible by packet duration %s", doubaoASTTranslatePTTOutputLimit, packetDuration)
	}
	framesAtLimit := int(doubaoASTTranslatePTTOutputLimit / packetDuration)

	for _, tc := range []struct {
		name      string
		frames    int
		wantLimit bool
	}{
		{name: "below", frames: framesAtLimit - 1},
		{name: "exact", frames: framesAtLimit},
		{name: "above", frames: framesAtLimit + 1, wantLimit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := &recordingASTTranslateOutput{}
			gate := newASTTranslatePTTOutputGate(output, func() bool { return true }, "turn-limit")
			if err := gate.Push(&genx.MessageChunk{
				Role: genx.RoleModel,
				Part: genx.Text("buffered"),
				Ctrl: &genx.StreamCtrl{StreamID: "turn-limit", Label: doubaoASTTranslateAssistantLabel},
			}); err != nil {
				t.Fatalf("Push(text): %v", err)
			}

			var pushErr error
			for i := 0; i < tc.frames; i++ {
				pushErr = gate.Push(&genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
					Ctrl: &genx.StreamCtrl{
						StreamID:  "turn-limit",
						Label:     doubaoASTTranslateAssistantLabel,
						Timestamp: int64(i + 1),
					},
				})
				if pushErr != nil {
					break
				}
			}

			if tc.wantLimit {
				if !errors.Is(pushErr, errDoubaoASTTranslatePTTOutputLimit) {
					t.Fatalf("Push(over limit) error = %v", pushErr)
				}
				if !gate.LimitExceeded() {
					t.Fatal("LimitExceeded() = false, want true")
				}
				if err := gate.Commit(); !errors.Is(err, errDoubaoASTTranslatePTTOutputLimit) {
					t.Fatalf("Commit(over limit) error = %v", err)
				}
				chunks := output.snapshot()
				if len(chunks) != 1 {
					t.Fatalf("published chunks = %d, want one error EOS; chunks=%#v", len(chunks), chunks)
				}
				chunk := chunks[0]
				if chunk.Part != nil || chunk.Ctrl == nil ||
					chunk.Ctrl.StreamID != "turn-limit" ||
					chunk.Ctrl.Label != doubaoASTTranslateAssistantLabel ||
					!chunk.Ctrl.EndOfStream ||
					!strings.Contains(chunk.Ctrl.Error, "turn-limit") ||
					!strings.Contains(chunk.Ctrl.Error, doubaoASTTranslatePTTOutputLimit.String()) {
					t.Fatalf("limit chunk = %#v", chunk)
				}
				return
			}

			if pushErr != nil {
				t.Fatalf("Push() error = %v", pushErr)
			}
			if chunks := output.snapshot(); len(chunks) != 0 {
				t.Fatalf("pre-commit chunks = %d, want 0", len(chunks))
			}
			if err := gate.Commit(); err != nil {
				t.Fatalf("Commit(): %v", err)
			}
			chunks := output.snapshot()
			if len(chunks) != tc.frames+1 {
				t.Fatalf("committed chunks = %d, want %d", len(chunks), tc.frames+1)
			}
			if text, ok := chunks[0].Part.(genx.Text); !ok || string(text) != "buffered" {
				t.Fatalf("first committed chunk = %#v", chunks[0])
			}
			if got := chunks[len(chunks)-1].Ctrl.Timestamp; got != int64(tc.frames) {
				t.Fatalf("last timestamp = %d, want %d", got, tc.frames)
			}
		})
	}
}

func TestDoubaoASTTranslatePTTOutputLimitKeepsTransformerUsable(t *testing.T) {
	packet := []byte{0x98}
	packetDuration := time.Duration(historyOpusPacketDurationMS(packet)) * time.Millisecond
	framesAtLimit := int(doubaoASTTranslatePTTOutputLimit / packetDuration)
	packets := make([][]byte, 0, framesAtLimit+3)
	packets = append(packets, astTranslateOpusHeadPacket(48000, 1), astTranslateOpusTagsPacket("test"))
	for i := 0; i <= framesAtLimit; i++ {
		packets = append(packets, packet)
	}

	input := newBufferStream(12)
	tr := NewDoubaoASTTranslate(doubaospeech.NewClient("app-id"),
		WithDoubaoASTTranslateMode(doubaospeech.ASTTranslateModeS2S),
		WithDoubaoASTTranslateInputMode(DoubaoASTTranslateInputModePushToTalk),
	)
	limited := &fakeASTTranslateSession{
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventSourceSubtitleStart},
			{Type: doubaospeech.ASTEventSourceSubtitleResponse, Text: "stale source"},
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "stale assistant"},
			{Type: doubaospeech.ASTEventTTSSentenceStart},
			{Type: doubaospeech.ASTEventTTSResponse, Audio: buildASTTranslateOggPackets(t, packets...)},
		},
	}
	fresh := &fakeASTTranslateSession{
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "fresh assistant"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "fresh assistant"},
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	sessions := []*fakeASTTranslateSession{limited, fresh}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		if len(sessions) == 0 {
			t.Fatal("unexpected extra AST session")
		}
		next := sessions[0]
		sessions = sessions[1:]
		return next, nil
	}
	out, err := tr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-reused")); err != nil {
		t.Fatalf("Push(first BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-reused"},
	}); err != nil {
		t.Fatalf("Push(first audio): %v", err)
	}

	limitChunk, err := nextASTTranslateChunk(t, out)
	if err != nil {
		t.Fatalf("Next(limit) error = %v", err)
	}
	if limitChunk == nil || limitChunk.Part != nil || limitChunk.Ctrl == nil ||
		limitChunk.Ctrl.StreamID != "turn-reused" ||
		!limitChunk.Ctrl.EndOfStream ||
		!strings.Contains(limitChunk.Ctrl.Error, doubaoASTTranslatePTTOutputLimit.String()) {
		t.Fatalf("limit chunk = %#v", limitChunk)
	}

	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-reused", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(first EOS): %v", err)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-reused")); err != nil {
		t.Fatalf("Push(reused BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{3, 0, 4, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-reused"},
	}); err != nil {
		t.Fatalf("Push(reused audio): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-reused", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(reused EOS): %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}

	chunks := append([]*genx.MessageChunk{limitChunk}, readAllASTTranslateChunks(t, out)...)
	if !limited.closed || limited.finished {
		t.Fatalf("limited session closed/finished = %t/%t, want true/false", limited.closed, limited.finished)
	}
	if !fresh.finished {
		t.Fatal("reused StreamID session was not finished")
	}
	if len(sessions) != 0 {
		t.Fatalf("unused sessions = %d", len(sessions))
	}
	if got := collectASTTranslateText(chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-reused"); strings.Contains(got, "stale") {
		t.Fatalf("stale transcript leaked: %q", got)
	}
	if got := collectASTTranslateText(chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-reused"); got != "fresh assistant" {
		t.Fatalf("assistant text = %q, want fresh assistant; chunks=%#v", got, chunks)
	}
	errorCount := 0
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			errorCount++
		}
	}
	if errorCount != 1 {
		t.Fatalf("error chunk count = %d, want 1; chunks=%#v", errorCount, chunks)
	}
}

func TestDoubaoASTTranslatePTTOutputGateCommitWaitsForProviderFailure(t *testing.T) {
	providerErr := &doubaospeech.Error{Code: 500, Message: "provider failed concurrently with input EOS"}
	output := &recordingASTTranslateOutput{}
	gate := newASTTranslatePTTOutputGate(output, func() bool { return true }, "turn-provider-error")
	if err := gate.Push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("must stay buffered"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-provider-error", Label: doubaoASTTranslateAssistantLabel},
	}); err != nil {
		t.Fatalf("Push(buffered): %v", err)
	}

	eventDelivered := make(chan struct{})
	allowFailure := make(chan struct{})
	receiverDone := make(chan struct{})
	events := gate.providerEventSequence(func(yield func(*doubaospeech.ASTTranslateEvent, error) bool) {
		yield(&doubaospeech.ASTTranslateEvent{Type: doubaospeech.ASTEventSessionFailed, Error: providerErr}, nil)
	})
	go func() {
		defer close(receiverDone)
		for event, err := range events {
			close(eventDelivered)
			<-allowFailure
			if err != nil {
				gate.Fail(err)
				return
			}
			gate.Fail(event.Error)
			return
		}
	}()

	select {
	case <-eventDelivered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for provider event delivery")
	}
	commitDone := make(chan error, 1)
	go func() {
		commitDone <- gate.Commit()
	}()
	select {
	case err := <-commitDone:
		t.Fatalf("Commit() returned before provider failure arbitration: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(allowFailure)
	select {
	case err := <-commitDone:
		if !errors.Is(err, providerErr) {
			t.Fatalf("Commit() error = %v, want provider error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Commit()")
	}
	select {
	case <-receiverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for provider receiver")
	}
	if chunks := output.snapshot(); len(chunks) != 0 {
		t.Fatalf("published chunks = %#v, want none", chunks)
	}
}

func TestDoubaoASTTranslatePTTProviderErrorBeforeEOSDoesNotLeak(t *testing.T) {
	providerErr := &doubaospeech.Error{Code: 500, Message: "provider failed before input EOS"}
	terminalHandled := make(chan struct{})
	allowRecvReturn := make(chan struct{})
	providerDone := make(chan struct{})
	var releaseRecv sync.Once
	release := func() {
		releaseRecv.Do(func() {
			close(allowRecvReturn)
		})
	}
	defer release()
	input := newBufferStream(4)
	tr := NewDoubaoASTTranslate(doubaospeech.NewClient("app-id"),
		WithDoubaoASTTranslateMode(doubaospeech.ASTTranslateModeS2S),
		WithDoubaoASTTranslateInputMode(DoubaoASTTranslateInputModePushToTalk),
	)
	fake := &fakeASTTranslateSession{
		yieldReturnedCh: terminalHandled,
		allowRecvReturn: allowRecvReturn,
		doneCh:          providerDone,
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "must stay buffered"},
			{Type: doubaospeech.ASTEventSessionFailed, Error: providerErr},
		},
	}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		return fake, nil
	}
	out, err := tr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-provider-error")); err != nil {
		t.Fatalf("Push(BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-provider-error"},
	}); err != nil {
		t.Fatalf("Push(audio): %v", err)
	}
	select {
	case <-terminalHandled:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for provider terminal event handling")
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-provider-error", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(EOS): %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}
	chunk, err := nextASTTranslateChunk(t, out)
	if chunk != nil || !errors.Is(err, providerErr) {
		t.Fatalf("Next() = chunk=%#v err=%v, want provider error without buffered output", chunk, err)
	}
	release()
	select {
	case <-providerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for provider receive cleanup")
	}
	fake.mu.Lock()
	closed := fake.closed
	fake.mu.Unlock()
	if !closed {
		t.Fatal("provider session was not closed after terminal failure")
	}
}

func TestDoubaoASTTranslatePushToTalkS2TCommitsAtEOS(t *testing.T) {
	providerDone := make(chan struct{})
	input := newBufferStream(4)
	tr := NewDoubaoASTTranslate(doubaospeech.NewClient("app-id"),
		WithDoubaoASTTranslateInputMode(DoubaoASTTranslateInputModePushToTalk),
	)
	fake := &fakeASTTranslateSession{
		doneCh: providerDone,
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventSourceSubtitleStart},
			{Type: doubaospeech.ASTEventSourceSubtitleResponse, Text: "source"},
			{Type: doubaospeech.ASTEventSourceSubtitleEnd, Text: "source"},
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "translated"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "translated"},
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	tr.newSession = func(_ context.Context, cfg doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		if cfg.Mode != doubaospeech.ASTTranslateModeS2T {
			t.Fatalf("mode = %q, want S2T", cfg.Mode)
		}
		return fake, nil
	}
	out, err := tr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-s2t")); err != nil {
		t.Fatalf("Push(BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-s2t"},
	}); err != nil {
		t.Fatalf("Push(audio): %v", err)
	}
	select {
	case <-providerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for S2T provider events")
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-s2t", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(EOS): %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}
	chunks := readAllASTTranslateChunks(t, out)
	if got := collectASTTranslateText(chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-s2t"); got != "source" {
		t.Fatalf("source text = %q", got)
	}
	if got := collectASTTranslateText(chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-s2t"); got != "translated" {
		t.Fatalf("translated text = %q", got)
	}
	for _, chunk := range chunks {
		if blob, ok := chunk.Part.(*genx.Blob); ok && baseAudioMIME(blob.MIMEType) == "audio/opus" && chunk.Role == genx.RoleModel {
			t.Fatalf("unexpected S2T assistant audio: %#v", chunk)
		}
	}
}

func TestDoubaoASTTranslateRealtimeStillPublishesBeforeEOS(t *testing.T) {
	providerDone := make(chan struct{})
	input := newBufferStream(4)
	tr := NewDoubaoASTTranslate(doubaospeech.NewClient("app-id"),
		WithDoubaoASTTranslateInputMode(DoubaoASTTranslateInputModeRealtime),
	)
	fake := &fakeASTTranslateSession{
		doneCh: providerDone,
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "incremental"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "incremental"},
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		return fake, nil
	}
	out, err := tr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-realtime")); err != nil {
		t.Fatalf("Push(BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-realtime"},
	}); err != nil {
		t.Fatalf("Push(audio): %v", err)
	}
	select {
	case <-providerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for realtime provider events")
	}
	chunk, err := nextASTTranslateChunk(t, out)
	if err != nil || chunk == nil {
		t.Fatalf("pre-EOS realtime output = chunk=%#v err=%v", chunk, err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-realtime", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(EOS): %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}
	_ = readAllASTTranslateChunks(t, out)
}

func TestDoubaoASTTranslatePushToTalkCancelBeforeEOSDoesNotLeak(t *testing.T) {
	input := newBufferStream(4)
	ctx, cancel := context.WithCancel(context.Background())
	tr := NewDoubaoASTTranslate(doubaospeech.NewClient("app-id"),
		WithDoubaoASTTranslateInputMode(DoubaoASTTranslateInputModePushToTalk),
	)
	fake := &fakeASTTranslateSession{
		sentAudioNotify:   make(chan struct{}),
		notifySentAudioAt: 1,
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "discard on cancel"},
		},
	}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		return fake, nil
	}
	out, err := tr.Transform(ctx, "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-cancel")); err != nil {
		t.Fatalf("Push(BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-cancel"},
	}); err != nil {
		t.Fatalf("Push(audio): %v", err)
	}
	fake.waitSentAudio(t)
	cancel()
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}
	chunk, err := nextASTTranslateChunk(t, out)
	if chunk != nil || (err == nil || (!errors.Is(err, context.Canceled) && !errors.Is(err, genx.ErrDone) && !errors.Is(err, io.EOF))) {
		t.Fatalf("Next() after cancel = chunk=%#v err=%v", chunk, err)
	}
}

func TestDoubaoASTTranslateInterruptsActiveSessionOnNewInputStream(t *testing.T) {
	input := newBufferStream(8)
	tr := NewDoubaoASTTranslate(doubaospeech.NewClient("app-id"),
		WithDoubaoASTTranslateMode(doubaospeech.ASTTranslateModeS2S),
	)
	first := &fakeASTTranslateSession{
		closeCh: make(chan struct{}),
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "stale"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "stale"},
		},
	}
	second := &fakeASTTranslateSession{
		events: []*doubaospeech.ASTTranslateEvent{{Type: doubaospeech.ASTEventSessionFinished}},
	}
	sessions := []*fakeASTTranslateSession{first, second}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		if len(sessions) == 0 {
			t.Fatal("unexpected extra AST session")
		}
		next := sessions[0]
		sessions = sessions[1:]
		return next, nil
	}
	out, err := tr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}}); err != nil {
		t.Fatalf("Push(first BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1"},
	}); err != nil {
		t.Fatalf("Push(first audio): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(first EOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}}); err != nil {
		t.Fatalf("Push(second BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{3, 0, 4, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-2"},
	}); err != nil {
		t.Fatalf("Push(second audio): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-2", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(second EOS): %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}

	chunks := readAllASTTranslateChunks(t, out)
	if !first.closed || !first.finished {
		t.Fatalf("first session closed/finished = %t/%t, want finished and closed interrupt", first.closed, first.finished)
	}
	if !second.finished {
		t.Fatal("second session was not finished")
	}
	assertASTTranslateInterruptedEOS(t, chunks, "turn-1", genx.Text(""))
	assertASTTranslateInterruptedEOS(t, chunks, "turn-1", &genx.Blob{MIMEType: "audio/opus"})
	if got := collectASTTranslateText(chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1"); strings.Contains(got, "stale") {
		t.Fatalf("stale interrupted session output leaked: text=%q chunks=%#v", got, chunks)
	}
}

func TestDoubaoASTTranslateRealtimeStartsNextSessionAfterPreviousFinished(t *testing.T) {
	input := newBufferStream(8)
	tr := NewDoubaoASTTranslate(doubaospeech.NewClient("app-id"),
		WithDoubaoASTTranslateMode(doubaospeech.ASTTranslateModeS2S),
		WithDoubaoASTTranslateInputMode(DoubaoASTTranslateInputModeRealtime),
	)
	firstDone := make(chan struct{})
	first := &fakeASTTranslateSession{
		doneCh: firstDone,
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	second := &fakeASTTranslateSession{
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "second"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "second"},
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	sessions := []*fakeASTTranslateSession{first, second}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		if len(sessions) == 0 {
			t.Fatal("unexpected extra AST session")
		}
		next := sessions[0]
		sessions = sessions[1:]
		return next, nil
	}
	out, err := tr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-1")); err != nil {
		t.Fatalf("Push(first BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1"},
	}); err != nil {
		t.Fatalf("Push(first audio): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(first EOS): %v", err)
	}
	<-firstDone
	if err := input.Push(genx.NewBeginOfStream("turn-2")); err != nil {
		t.Fatalf("Push(second BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{3, 0, 4, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-2"},
	}); err != nil {
		t.Fatalf("Push(second audio): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-2", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(second EOS): %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}

	chunks := readAllASTTranslateChunks(t, out)
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.Error == "interrupted" {
			t.Fatalf("unexpected interrupted chunk after normal session completion: %#v", chunk)
		}
	}
	if got := collectASTTranslateText(chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-2"); !strings.Contains(got, "second") {
		t.Fatalf("turn-2 text = %q, want second", got)
	}
}

func TestDoubaoASTTranslateIgnoresLateInterruptedStreamChunks(t *testing.T) {
	input := newBufferStream(10)
	tr := NewDoubaoASTTranslate(doubaospeech.NewClient("app-id"),
		WithDoubaoASTTranslateMode(doubaospeech.ASTTranslateModeS2S),
		WithDoubaoASTTranslateInputMode(DoubaoASTTranslateInputModePushToTalk),
	)
	first := &fakeASTTranslateSession{
		closeCh: make(chan struct{}),
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "stale"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "stale"},
		},
	}
	second := &fakeASTTranslateSession{
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "second"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "second"},
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	reused := &fakeASTTranslateSession{
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "reused fresh"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "reused fresh"},
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	sessions := []*fakeASTTranslateSession{first, second, reused}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		if len(sessions) == 0 {
			t.Fatal("unexpected extra AST session")
		}
		next := sessions[0]
		sessions = sessions[1:]
		return next, nil
	}
	out, err := tr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-1")); err != nil {
		t.Fatalf("Push first BOS: %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1"},
	}); err != nil {
		t.Fatalf("Push first audio: %v", err)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-2")); err != nil {
		t.Fatalf("Push second BOS: %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push stale first EOS: %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{3, 0, 4, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-2"},
	}); err != nil {
		t.Fatalf("Push second audio: %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-2", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push second EOS: %v", err)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-1")); err != nil {
		t.Fatalf("Push reused BOS: %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{5, 0, 6, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1"},
	}); err != nil {
		t.Fatalf("Push reused audio: %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push reused EOS: %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}

	chunks := readAllASTTranslateChunks(t, out)
	assertASTTranslateInterruptedEOS(t, chunks, "turn-1", genx.Text(""))
	if got := collectASTTranslateText(chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-2"); !strings.Contains(got, "second") {
		t.Fatalf("turn-2 text = %q, want second; chunks=%#v", got, chunks)
	}
	if got := collectASTTranslateText(chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1"); got != "reused fresh" {
		t.Fatalf("reused turn-1 text = %q, want reused fresh; chunks=%#v", got, chunks)
	}
	if len(sessions) != 0 {
		t.Fatalf("unused sessions = %d", len(sessions))
	}
	if !second.finished {
		t.Fatal("second session was not finished")
	}
	if !reused.finished {
		t.Fatal("reused StreamID session was not finished")
	}
}

type recordingASTTranslateOutput struct {
	mu     sync.Mutex
	chunks []*genx.MessageChunk
}

func (o *recordingASTTranslateOutput) Push(chunk *genx.MessageChunk) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.chunks = append(o.chunks, chunk.Clone())
	return nil
}

func (o *recordingASTTranslateOutput) snapshot() []*genx.MessageChunk {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]*genx.MessageChunk, 0, len(o.chunks))
	for _, chunk := range o.chunks {
		out = append(out, chunk.Clone())
	}
	return out
}

type fakeASTTranslateSession struct {
	events          []*doubaospeech.ASTTranslateEvent
	beforeRecv      chan struct{}
	yieldReturnedCh chan struct{}
	allowRecvReturn chan struct{}
	sentAudioNotify chan struct{}

	notifySentAudioAt int
	closeCh           chan struct{}
	doneCh            chan struct{}

	mu                  sync.Mutex
	sentAudio           [][]byte
	finished            bool
	closed              bool
	sentAudioNotifyOnce sync.Once
	closeOnce           sync.Once
}

func (s *fakeASTTranslateSession) SendAudio(_ context.Context, audio []byte) error {
	cp := append([]byte(nil), audio...)
	s.mu.Lock()
	s.sentAudio = append(s.sentAudio, cp)
	if s.sentAudioNotify != nil && s.notifySentAudioAt > 0 && len(s.sentAudio) >= s.notifySentAudioAt {
		s.sentAudioNotifyOnce.Do(func() {
			close(s.sentAudioNotify)
		})
	}
	s.mu.Unlock()
	return nil
}

func (s *fakeASTTranslateSession) Finish(context.Context) error {
	s.mu.Lock()
	s.finished = true
	s.mu.Unlock()
	return nil
}

func (s *fakeASTTranslateSession) Recv() iter.Seq2[*doubaospeech.ASTTranslateEvent, error] {
	return func(yield func(*doubaospeech.ASTTranslateEvent, error) bool) {
		s.mu.Lock()
		doneCh := s.doneCh
		closeCh := s.closeCh
		s.mu.Unlock()
		if doneCh != nil {
			defer func() {
				close(doneCh)
			}()
		}
		if s.beforeRecv != nil {
			<-s.beforeRecv
		}
		if closeCh != nil {
			<-closeCh
			for _, event := range s.events {
				if !yield(event, nil) {
					return
				}
			}
			_ = yield(nil, io.ErrClosedPipe)
			return
		}
		for _, event := range s.events {
			if !yield(event, nil) {
				if s.yieldReturnedCh != nil {
					close(s.yieldReturnedCh)
				}
				if s.allowRecvReturn != nil {
					<-s.allowRecvReturn
				}
				return
			}
		}
	}
}

func (s *fakeASTTranslateSession) Close() error {
	s.mu.Lock()
	s.closed = true
	closeCh := s.closeCh
	s.mu.Unlock()
	if closeCh != nil {
		s.closeOnce.Do(func() {
			close(closeCh)
		})
	}
	return nil
}

func (s *fakeASTTranslateSession) waitSentAudio(t *testing.T) {
	t.Helper()
	if s.sentAudioNotify == nil || s.notifySentAudioAt <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	select {
	case <-s.sentAudioNotify:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %d sent audio chunks", s.notifySentAudioAt)
	}
}

func readAllASTTranslateChunks(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
				return chunks
			}
			t.Fatalf("Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
}

func nextASTTranslateChunk(t *testing.T, stream genx.Stream) (*genx.MessageChunk, error) {
	t.Helper()
	type result struct {
		chunk *genx.MessageChunk
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		chunk, err := stream.Next()
		resultCh <- result{chunk: chunk, err: err}
	}()
	select {
	case result := <-resultCh:
		return result.chunk, result.err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for AST translate output")
		return nil, nil
	}
}

func assertASTTranslateTextChunk(t *testing.T, chunks []*genx.MessageChunk, role genx.Role, label, streamID, text string) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk.Role != role || chunk.Ctrl == nil || chunk.Ctrl.Label != label || chunk.Ctrl.StreamID != streamID {
			continue
		}
		if got, ok := chunk.Part.(genx.Text); ok && string(got) == text {
			return
		}
	}
	t.Fatalf("missing text chunk role=%s label=%s stream=%s text=%q in %#v", role, label, streamID, text, chunks)
}

func assertASTTranslateAudioChunk(t *testing.T, chunks []*genx.MessageChunk, streamID string, audio []byte) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk.Role != genx.RoleModel || chunk.Ctrl == nil || chunk.Ctrl.Label != doubaoASTTranslateAssistantLabel || chunk.Ctrl.StreamID != streamID {
			continue
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if ok && string(blob.Data) == string(audio) {
			return
		}
	}
	t.Fatalf("missing audio chunk stream=%s bytes=%v in %#v", streamID, audio, chunks)
}

func assertASTTranslateHistoryAudioChunk(t *testing.T, chunks []*genx.MessageChunk, streamID string, audio []byte) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk.Role != genx.RoleUser || chunk.Ctrl == nil || chunk.Ctrl.Label != genx.HistoryUserAudioLabel || chunk.Ctrl.StreamID != streamID {
			continue
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && bytes.Equal(blob.Data, audio) {
			return
		}
	}
	t.Fatalf("missing history audio chunk stream=%s bytes=%v in %#v", streamID, audio, chunks)
}

func assertASTTranslateEOS(t *testing.T, chunks []*genx.MessageChunk, role genx.Role, label, streamID string) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk.Role == role && chunk.Ctrl != nil && chunk.Ctrl.Label == label && chunk.Ctrl.StreamID == streamID && chunk.Ctrl.EndOfStream {
			return
		}
	}
	t.Fatalf("missing EOS role=%s label=%s stream=%s in %#v", role, label, streamID, chunks)
}

func assertASTTranslateInterruptedEOS(t *testing.T, chunks []*genx.MessageChunk, streamID string, part any) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk.Role != genx.RoleModel || chunk.Ctrl == nil ||
			chunk.Ctrl.Label != doubaoASTTranslateAssistantLabel ||
			chunk.Ctrl.StreamID != streamID ||
			!chunk.Ctrl.EndOfStream ||
			chunk.Ctrl.Error != "interrupted" {
			continue
		}
		switch part.(type) {
		case genx.Text:
			if _, ok := chunk.Part.(genx.Text); ok {
				return
			}
		case *genx.Blob:
			if _, ok := chunk.Part.(*genx.Blob); ok {
				return
			}
		}
	}
	t.Fatalf("missing interrupted EOS stream=%s part=%T in %#v", streamID, part, chunks)
}

func countASTTranslateTextEOS(chunks []*genx.MessageChunk, role genx.Role, label, streamID string) int {
	count := 0
	for _, chunk := range chunks {
		if chunk.Role == role && chunk.Ctrl != nil && chunk.Ctrl.Label == label && chunk.Ctrl.StreamID == streamID && chunk.Ctrl.EndOfStream {
			if _, ok := chunk.Part.(genx.Text); ok {
				count++
			}
		}
	}
	return count
}

func countASTTranslateAudioEOS(chunks []*genx.MessageChunk, streamID string) int {
	count := 0
	for _, chunk := range chunks {
		if chunk.Role == genx.RoleModel && chunk.Ctrl != nil && chunk.Ctrl.Label == doubaoASTTranslateAssistantLabel && chunk.Ctrl.StreamID == streamID && chunk.Ctrl.EndOfStream {
			if _, ok := chunk.Part.(*genx.Blob); ok {
				count++
			}
		}
	}
	return count
}

func countASTTranslateHistoryAudioEOS(chunks []*genx.MessageChunk, streamID string) int {
	count := 0
	for _, chunk := range chunks {
		if chunk.Role == genx.RoleUser && chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel && chunk.Ctrl.StreamID == streamID && chunk.Ctrl.EndOfStream {
			count++
		}
	}
	return count
}

func collectASTTranslateText(chunks []*genx.MessageChunk, role genx.Role, label, streamID string) string {
	var out string
	for _, chunk := range chunks {
		if chunk.Role != role || chunk.Ctrl == nil || chunk.Ctrl.Label != label || chunk.Ctrl.StreamID != streamID {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			out += string(text)
		}
	}
	return out
}

func countASTTranslateStreamChunks(chunks []*genx.MessageChunk, streamID string) int {
	count := 0
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID == streamID {
			count++
		}
	}
	return count
}

func buildASTTranslateRawOpusPacket(t *testing.T, frame []int16) []byte {
	t.Helper()
	enc, err := opus.NewEncoder(doubaoASTTranslateSourceSampleRate, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer func() {
		_ = enc.Close()
	}()
	packet, err := enc.Encode(frame, len(frame))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(packet) == 0 {
		t.Fatal("encoded opus packet is empty")
	}
	return packet
}

func astTranslateOpusHeadPacket(sampleRate, channels int) []byte {
	packet := make([]byte, 19)
	copy(packet[:8], "OpusHead")
	packet[8] = 1
	packet[9] = byte(channels)
	binary.LittleEndian.PutUint32(packet[12:16], uint32(sampleRate))
	return packet
}

func astTranslateOpusTagsPacket(vendor string) []byte {
	vendorBytes := []byte(vendor)
	packet := make([]byte, 8+4+len(vendorBytes)+4)
	copy(packet[:8], "OpusTags")
	binary.LittleEndian.PutUint32(packet[8:12], uint32(len(vendorBytes)))
	copy(packet[12:12+len(vendorBytes)], vendorBytes)
	return packet
}

func buildASTTranslateOggPackets(t *testing.T, packets ...[]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	sw, err := ogg.NewStreamWriter(&out, 77)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}
	for i, packet := range packets {
		if _, err := sw.WritePacket(packet, uint64(i), i == len(packets)-1); err != nil {
			t.Fatalf("WritePacket %d: %v", i, err)
		}
	}
	return out.Bytes()
}
