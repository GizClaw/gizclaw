package transformers

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"iter"
	"testing"

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
	fake := &fakeASTTranslateSession{
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
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}}); err != nil {
		t.Fatalf("Push(EOS): %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}

	chunks := readAllASTTranslateChunks(t, out)
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

type fakeASTTranslateSession struct {
	events    []*doubaospeech.ASTTranslateEvent
	sentAudio [][]byte
	finished  bool
	closed    bool
}

func (s *fakeASTTranslateSession) SendAudio(_ context.Context, audio []byte) error {
	cp := append([]byte(nil), audio...)
	s.sentAudio = append(s.sentAudio, cp)
	return nil
}

func (s *fakeASTTranslateSession) Finish(context.Context) error {
	s.finished = true
	return nil
}

func (s *fakeASTTranslateSession) Recv() iter.Seq2[*doubaospeech.ASTTranslateEvent, error] {
	return func(yield func(*doubaospeech.ASTTranslateEvent, error) bool) {
		for _, event := range s.events {
			if !yield(event, nil) {
				return
			}
		}
	}
}

func (s *fakeASTTranslateSession) Close() error {
	s.closed = true
	return nil
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
