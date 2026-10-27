package doubaoast

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func buildASRAudioFrame(frameSize, channels int) []int16 {
	frame := make([]int16, frameSize*channels)
	for i := range frame {
		frame[i] = int16((i * 97) % 24000)
	}
	return frame
}

func TestTransformerStreamsTranslationAndAudio(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	input := newBufferStream(4)
	sourcePacket := buildASTTranslateRawOpusPacket(t, buildASRAudioFrame(doubaoASTTranslateSourceSampleRate/50, 1))
	tr := newTransformer(doubaospeech.NewClient("app-id"),
		withMode(doubaospeech.ASTTranslateModeS2S),
		withSourceLanguage("zh"),
		withTargetLanguage("ja"),
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
			{Type: doubaospeech.ASTEventUsageResponse, Usage: &doubaospeech.ASTTranslateUsage{
				DurationMS: 2560,
				Items: []doubaospeech.ASTTranslateBillingItem{
					{Unit: "output_text_tokens", Quantity: 34},
					{Unit: "output_audio_tokens", Quantity: 67},
					{Unit: "input_audio_tokens", Quantity: 16},
				},
			}},
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	tr.newSession = func(_ context.Context, cfg doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		if cfg.Mode != doubaospeech.ASTTranslateModeS2S || cfg.SourceLanguage != "zh" || cfg.TargetLanguage != "ja" {
			t.Fatalf("cfg = %+v, want s2s zh->ja", cfg)
		}
		return fake, nil
	}
	var records []genx.UsageRecord
	var recordsMu sync.Mutex
	ctx := genx.WithUsageRecorder(context.Background(), func(record genx.UsageRecord) {
		recordsMu.Lock()
		defer recordsMu.Unlock()
		records = append(records, record)
	})
	out, err := tr.transform(ctx, input)
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
	sentAudio := fake.sentAudioSnapshot()
	if len(sentAudio) < 2 || len(sentAudio[0]) == 0 || bytes.Equal(sentAudio[0], sourcePacket) {
		t.Fatalf("sentAudio = %d chunks, want decoded speech then trailing silence", len(sentAudio))
	}
	for _, chunk := range sentAudio[1:] {
		if len(bytes.Trim(chunk, "\x00")) != 0 {
			t.Fatalf("audio after the speech is not the trailing silence")
		}
	}
	if _, finished := fake.state(); !finished {
		t.Fatalf("session was not finished")
	}
	recordsMu.Lock()
	wantUsage := []genx.UsageRecord{
		{Provider: "volc", Model: doubaospeech.ResourceASTTranslate, Modality: genx.UsageModalityText, Unit: genx.UsageUnitToken, Output: 34},
		{Provider: "volc", Model: doubaospeech.ResourceASTTranslate, Modality: genx.UsageModalityAudio, Unit: genx.UsageUnitToken, Input: 16, Output: 67},
	}
	if !slices.Equal(records, wantUsage) {
		t.Fatalf("usage records = %+v, want %+v", records, wantUsage)
	}
	recordsMu.Unlock()
	assertASTTranslateTextChunk(t, chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-1", "你好")
	assertASTTranslateHistoryAudioChunk(t, chunks, "turn-1", sourcePacket)
	assertASTTranslateTextChunk(t, chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1", "こんにちは")
	assertASTTranslateAudioChunk(t, chunks, "turn-1", []byte{1, 2, 3})
	assertASTTranslateEOS(t, chunks, genx.RoleUser, doubaoASTTranslateTranscriptLabel, "turn-1")
	assertASTTranslateEOS(t, chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1")
	routes := make(map[string][]*genx.MessageChunk)
	for _, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil {
			continue
		}
		mimeType, ok := chunk.MIMEType()
		if !ok {
			continue
		}
		key := string(chunk.Role) + "\x00" + chunk.Ctrl.Label + "\x00" + chunk.Ctrl.StreamID + "\x00" + mimeType
		routes[key] = append(routes[key], chunk)
	}
	if len(routes) != 4 {
		t.Fatalf("generated routes = %d, want transcript, history audio, assistant text, and assistant audio: %#v", len(routes), chunks)
	}
	for key, route := range routes {
		if len(route) < 2 || !route[0].IsBeginOfStream() || !route[len(route)-1].IsEndOfStream() {
			t.Fatalf("route %q lifecycle = %#v, want BOS...EOS", key, route)
		}
		for index, chunk := range route {
			if index > 0 && chunk.IsBeginOfStream() {
				t.Fatalf("route %q has duplicate BOS: %#v", key, route)
			}
			if index < len(route)-1 && chunk.IsEndOfStream() {
				t.Fatalf("route %q ended before its final chunk: %#v", key, route)
			}
		}
	}
}

func TestTransformerConcurrentCallsOwnSessions(t *testing.T) {
	transformer := newTransformer(doubaospeech.NewClient("app-id"), withRealtimePacing(false))
	recvGate := make(chan struct{})
	var mu sync.Mutex
	var sessions []*fakeASTTranslateSession
	transformer.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		session := &fakeASTTranslateSession{
			beforeRecv: recvGate,
			events:     []*doubaospeech.ASTTranslateEvent{{Type: doubaospeech.ASTEventSessionFinished}},
		}
		mu.Lock()
		sessions = append(sessions, session)
		mu.Unlock()
		return session, nil
	}

	const calls = 8
	outputs := make(chan genx.Stream, calls)
	inputs := make(chan *bufferStream, calls)
	errs := make(chan error, calls)
	var wg sync.WaitGroup
	for i := range calls {
		wg.Go(func() {
			input := newBufferStream(2)
			streamID := fmt.Sprintf("turn-%d", i)
			output, err := transformer.Transform(context.Background(), input)
			if err != nil {
				errs <- err
				return
			}
			_ = input.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true}})
			_ = input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}})
			inputs <- input
			outputs <- output
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Transform() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		opened := len(sessions)
		mu.Unlock()
		if opened == calls {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("opened sessions = %d, want %d", opened, calls)
		}
		time.Sleep(time.Millisecond)
	}
	close(inputs)
	for input := range inputs {
		_ = input.Close()
	}
	close(recvGate)
	close(outputs)
	for output := range outputs {
		readAllASTTranslateChunks(t, output)
	}
}

func TestTransformerSplitsProviderSubtitleSegments(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	input := newBufferStream(8)
	firstAudio := buildASTTranslateRawOpusPacket(t, buildASRAudioFrame(doubaoASTTranslateSourceSampleRate/50, 1))
	secondAudio := buildASTTranslateRawOpusPacket(t, buildASRAudioFrame(doubaoASTTranslateSourceSampleRate/50, 1))
	tr := newTransformer(doubaospeech.NewClient("app-id"),
		withMode(doubaospeech.ASTTranslateModeS2S),
		withSourceLanguage("zh"),
		withTargetLanguage("ja"),
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
	out, err := tr.transform(context.Background(), input)
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

func TestTransformerPadsTrailingSilenceBeforeFinish(t *testing.T) {
	for _, mode := range []string{"push-to-talk", "realtime"} {
		t.Run(string(mode), func(t *testing.T) {
			testTransformerPadsTrailingSilenceBeforeFinish(t, mode)
		})
	}
}

func testTransformerPadsTrailingSilenceBeforeFinish(t *testing.T, mode string) {
	input := newBufferStream(4)
	tr := newTransformer(doubaospeech.NewClient("app-id"))
	fake := &fakeASTTranslateSession{
		beforeRecv: make(chan struct{}),
		events:     []*doubaospeech.ASTTranslateEvent{{Type: doubaospeech.ASTEventSessionFinished}},
	}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		return fake, nil
	}
	out, err := tr.transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	speech := []byte{1, 0, 2, 0}
	for _, chunk := range []*genx.MessageChunk{
		genx.NewBeginOfStream("turn"),
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: speech}, Ctrl: &genx.StreamCtrl{StreamID: "turn"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true}},
	} {
		if err := input.Push(chunk); err != nil {
			t.Fatalf("Push(): %v", err)
		}
	}
	close(fake.beforeRecv)
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}
	readAllASTTranslateChunks(t, out)

	fake.mu.Lock()
	sent := append([][]byte(nil), fake.sentAudio...)
	sentAtFinish, finished := fake.sentAtFinish, fake.finished
	fake.mu.Unlock()
	if !finished {
		t.Fatal("session was not finished")
	}
	if len(sent) < 2 || !bytes.Equal(sent[0], speech) {
		t.Fatalf("sent audio = %d chunks, want speech then silence", len(sent))
	}
	if sentAtFinish != len(sent) {
		t.Fatalf("sent %d chunks before Finish, %d in total; silence must precede Finish", sentAtFinish, len(sent))
	}
	silence := 0
	for _, chunk := range sent[1:] {
		for _, b := range chunk {
			if b != 0 {
				t.Fatalf("trailing chunk is not silence: %v", chunk)
			}
		}
		silence += len(chunk)
	}
	want := doubaoASTTranslateSourceSampleRate * (doubaoASTTranslateSourceBits / 8) * int(doubaoASTTranslateTrailingSilence/time.Millisecond) / 1000
	if silence != want {
		t.Fatalf("trailing silence = %d bytes, want %d", silence, want)
	}
}

// TestTransformerPushToTalkEmptyProviderResultClosesRoutes covers a turn the
// provider finishes without a subtitle. The assistant routes still close
// normally, so the turn ends instead of waiting for output that never comes.
func TestTransformerPushToTalkEmptyProviderResultClosesRoutes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mode      doubaospeech.ASTTranslateMode
		wantAudio bool
	}{
		{name: "s2t", mode: doubaospeech.ASTTranslateModeS2T},
		{name: "s2s", mode: doubaospeech.ASTTranslateModeS2S, wantAudio: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := newBufferStream(4)
			tr := newTransformer(doubaospeech.NewClient("app-id"), withMode(tc.mode))
			fake := &fakeASTTranslateSession{
				beforeRecv: make(chan struct{}),
				events:     []*doubaospeech.ASTTranslateEvent{{Type: doubaospeech.ASTEventUsageResponse}, {Type: doubaospeech.ASTEventSessionFinished}},
			}
			tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
				return fake, nil
			}
			out, err := tr.transform(context.Background(), input)
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			for _, chunk := range []*genx.MessageChunk{
				genx.NewBeginOfStream("turn"),
				{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn"}},
				{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true}},
			} {
				if err := input.Push(chunk); err != nil {
					t.Fatalf("Push(): %v", err)
				}
			}
			close(fake.beforeRecv)
			if err := input.Close(); err != nil {
				t.Fatalf("Close(input): %v", err)
			}
			chunks := readAllASTTranslateChunks(t, out)

			routes := map[string][2]int{}
			for _, chunk := range chunks {
				if chunk.Role != genx.RoleModel || chunk.Ctrl == nil || chunk.Ctrl.Label != doubaoASTTranslateAssistantLabel {
					continue
				}
				kind := "text"
				switch part := chunk.Part.(type) {
				case genx.Text:
					if part != "" {
						t.Fatalf("empty turn carried assistant text %q", part)
					}
				case *genx.Blob:
					kind = "audio"
					if len(part.Data) != 0 {
						t.Fatal("empty turn carried assistant audio")
					}
				}
				if chunk.Ctrl.Error != "" {
					t.Fatalf("%s route ended with error %q", kind, chunk.Ctrl.Error)
				}
				count := routes[kind]
				if chunk.IsBeginOfStream() {
					count[0]++
				}
				if chunk.IsEndOfStream() {
					count[1]++
				}
				routes[kind] = count
			}
			if routes["text"] != [2]int{1, 1} {
				t.Fatalf("assistant text BOS/EOS = %v, want one each", routes["text"])
			}
			wantAudio := [2]int{}
			if tc.wantAudio {
				wantAudio = [2]int{1, 1}
			}
			if routes["audio"] != wantAudio {
				t.Fatalf("assistant audio BOS/EOS = %v, want %v", routes["audio"], wantAudio)
			}
		})
	}
}

// TestTransformerKeepsProviderSubwordTokenSpacing replays translation subtitle
// tokens recorded from the provider. Tokens are subword pieces that carry their
// own spacing, so the assistant text must join them without adding or dropping
// spaces; otherwise TTS reads "H ola" as a spelled letter.
func TestTransformerKeepsProviderSubwordTokenSpacing(t *testing.T) {
	cases := []struct {
		name      string
		sentences [][]string
		want      string
	}{
		{
			name: "french",
			sentences: [][]string{
				{"Bon", "jour", ",", " "},
				{"n", "ous", " nous", " rencontr", "er", "ons", " dem", "ain", " à", " ", "9", " heures", " devant", " l", "'école", ".", " "},
			},
			want: "Bonjour, nous nous rencontrerons demain à 9 heures devant l'école.",
		},
		{
			name: "spanish",
			sentences: [][]string{
				{"H", "ola", ",", " nos", " v", "emos", " mañana", " en", " la", " pu", "erta", " de", " la", " escuela", ".", " "},
			},
			want: "Hola, nos vemos mañana en la puerta de la escuela.",
		},
		{
			name: "english",
			sentences: [][]string{
				{"Hello", ",", " "},
				{"let", "'s", " meet", " at", " ", "9", " a", ".m", ".", " tomorrow", ".", " "},
			},
			want: "Hello, let's meet at 9 a.m. tomorrow.",
		},
		{
			name: "word boundary with provider space",
			sentences: [][]string{
				{"Hello", " "},
				{"world", "."},
			},
			want: "Hello world.",
		},
		{
			name: "word boundary without provider space",
			sentences: [][]string{
				{"Okay"},
				{"let", "'s", " go", "."},
			},
			want: "Okay let's go.",
		},
		{
			name: "japanese",
			sentences: [][]string{
				{"こ", "ん", "に", "ち", "は", "、"},
				{"今日", "は", "天", "気", "が", "良", "い", "で", "す", "。"},
			},
			want: "こんにちは、今日は天気が良いです。",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := []*doubaospeech.ASTTranslateEvent{
				{Type: doubaospeech.ASTEventSourceSubtitleStart},
				{Type: doubaospeech.ASTEventSourceSubtitleResponse, Text: "你好"},
				{Type: doubaospeech.ASTEventSourceSubtitleEnd, Text: "你好"},
			}
			for _, tokens := range tc.sentences {
				events = append(events, &doubaospeech.ASTTranslateEvent{Type: doubaospeech.ASTEventTranslationSubtitleStart})
				final := ""
				for _, token := range tokens {
					final += token
					events = append(events, &doubaospeech.ASTTranslateEvent{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: token})
				}
				events = append(events, &doubaospeech.ASTTranslateEvent{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: final})
			}
			events = append(events, &doubaospeech.ASTTranslateEvent{Type: doubaospeech.ASTEventSessionFinished})

			providerDone := make(chan struct{})
			input := newBufferStream(4)
			tr := newTransformer(doubaospeech.NewClient("app-id"))
			fake := &fakeASTTranslateSession{doneCh: providerDone, events: events}
			tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
				return fake, nil
			}
			out, err := tr.transform(context.Background(), input)
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			if err := input.Push(genx.NewBeginOfStream("turn")); err != nil {
				t.Fatalf("Push(BOS): %v", err)
			}
			if err := input.Push(&genx.MessageChunk{
				Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}},
				Ctrl: &genx.StreamCtrl{StreamID: "turn"},
			}); err != nil {
				t.Fatalf("Push(audio): %v", err)
			}
			select {
			case <-providerDone:
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for provider events")
			}
			if err := input.Push(&genx.MessageChunk{
				Part: &genx.Blob{MIMEType: "audio/pcm"},
				Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true},
			}); err != nil {
				t.Fatalf("Push(EOS): %v", err)
			}
			if err := input.Close(); err != nil {
				t.Fatalf("Close(input): %v", err)
			}
			chunks := readAllASTTranslateChunks(t, out)
			got := collectASTTranslateText(chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn")
			if strings.TrimSpace(got) != tc.want {
				t.Fatalf("assistant text = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTransformerRealtimeStillPublishesBeforeEOS(t *testing.T) {
	providerDone := make(chan struct{})
	input := newBufferStream(4)
	tr := newTransformer(doubaospeech.NewClient("app-id"))
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
	out, err := tr.transform(context.Background(), input)
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

func TestTransformerRealtimeCompletionTimeoutClosesSilentSession(t *testing.T) {
	input := newBufferStream(4)
	tr := newTransformer(doubaospeech.NewClient("app-id"),
		withRealtimePacing(false),
		withRealtimeCompletionTimeout(20*time.Millisecond),
	)
	fake := &fakeASTTranslateSession{closeCh: make(chan struct{})}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		return fake, nil
	}
	out, err := tr.transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-silent")); err != nil {
		t.Fatalf("Push(BOS): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-silent"},
	}); err != nil {
		t.Fatalf("Push(audio): %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-silent", EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(EOS): %v", err)
	}

	chunk, err := nextASTTranslateChunk(t, out)
	if chunk != nil || !errors.Is(err, errDoubaoASTTranslateRealtimeCompletionTimeout) {
		t.Fatalf("Next() after silent session = chunk=%#v err=%v", chunk, err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		closed, finished := fake.state()
		if closed && finished {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("silent session closed/finished = %t/%t, want true/true", closed, finished)
		}
		time.Sleep(time.Millisecond)
	}
	if err := input.Push(genx.NewBeginOfStream("turn-after-timeout")); !errors.Is(err, errDoubaoASTTranslateRealtimeCompletionTimeout) {
		t.Fatalf("Push() after timeout error = %v", err)
	}
}

// TestTransformerRealtimeCompletionTimeoutClosesInputBeforeOutput pins the
// close order that keeps the timeout cause on the input. Closing output first
// wakes the cancellation watcher, which can close input with context.Canceled.
func TestTransformerRealtimeCompletionTimeoutClosesInputBeforeOutput(t *testing.T) {
	input := &firstCloseRecordingStream{bufferStream: newBufferStream(4), closed: make(chan struct{})}
	tr := newTransformer(doubaospeech.NewClient("app-id"),
		withRealtimePacing(false),
		withRealtimeCompletionTimeout(20*time.Millisecond),
	)
	fake := &fakeASTTranslateSession{closeCh: make(chan struct{})}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		return fake, nil
	}
	out, err := tr.transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	output, ok := out.(*bufferStream)
	if !ok {
		t.Fatalf("Transform() output = %T, want *bufferStream", out)
	}
	input.output = output
	for _, chunk := range []*genx.MessageChunk{
		genx.NewBeginOfStream("turn-silent"),
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-silent"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-silent", EndOfStream: true}},
	} {
		if err := input.Push(chunk); err != nil {
			t.Fatalf("Push(%s): %v", chunk.Ctrl.StreamID, err)
		}
	}
	select {
	case <-input.closed:
	case <-time.After(time.Second):
		t.Fatal("input was not closed after the completion timeout")
	}
	if !errors.Is(input.firstErr, errDoubaoASTTranslateRealtimeCompletionTimeout) {
		t.Fatalf("first input close error = %v, want completion timeout", input.firstErr)
	}
	if input.outputClosedFirst {
		t.Fatal("output was closed before input; the cancellation watcher can replace the input's timeout cause")
	}
}

// firstCloseRecordingStream records the first CloseWithError and whether the
// transformer output was already closed at that point.
type firstCloseRecordingStream struct {
	*bufferStream
	output            *bufferStream
	once              sync.Once
	closed            chan struct{}
	firstErr          error
	outputClosedFirst bool
}

func (s *firstCloseRecordingStream) CloseWithError(err error) error {
	s.once.Do(func() {
		s.firstErr = err
		select {
		case <-s.output.Done():
			s.outputClosedFirst = true
		default:
		}
		close(s.closed)
	})
	return s.bufferStream.CloseWithError(err)
}

func TestTransformerInterruptsActiveSessionOnNewInputStream(t *testing.T) {
	input := newBufferStream(8)
	tr := newTransformer(doubaospeech.NewClient("app-id"),
		withMode(doubaospeech.ASTTranslateModeS2S),
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
	out, err := tr.transform(context.Background(), input)
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
	if closed, finished := first.state(); !closed || !finished {
		t.Fatalf("first session closed/finished = %t/%t, want finished and closed interrupt", closed, finished)
	}
	if _, finished := second.state(); !finished {
		t.Fatal("second session was not finished")
	}
	assertASTTranslateInterruptedEOS(t, chunks, "turn-1", genx.Text(""))
	assertASTTranslateInterruptedEOS(t, chunks, "turn-1", &genx.Blob{MIMEType: "audio/opus"})
	assertASTTranslateInterruptedLifecycles(t, chunks, "turn-1")
	if got := collectASTTranslateText(chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1"); strings.Contains(got, "stale") {
		t.Fatalf("stale interrupted session output leaked: text=%q chunks=%#v", got, chunks)
	}
}

func TestTransformerClosesRepeatedInterruptedTurnsBeforeReplacementBOS(t *testing.T) {
	input := newBufferStream(16)
	tr := newTransformer(doubaospeech.NewClient("app-id"),
		withMode(doubaospeech.ASTTranslateModeS2S),
	)
	first := &fakeASTTranslateSession{
		closeCh:           make(chan struct{}),
		sentAudioNotify:   make(chan struct{}),
		notifySentAudioAt: 1,
	}
	second := &fakeASTTranslateSession{
		closeCh:           make(chan struct{}),
		sentAudioNotify:   make(chan struct{}),
		notifySentAudioAt: 1,
	}
	third := &fakeASTTranslateSession{
		sentAudioNotify:   make(chan struct{}),
		notifySentAudioAt: 1,
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "third"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "third"},
			{Type: doubaospeech.ASTEventTTSSentenceStart},
			{Type: doubaospeech.ASTEventTTSResponse, Audio: buildASTTranslateOggPackets(t, astTranslateOpusHeadPacket(48000, 1), astTranslateOpusTagsPacket("test"), []byte{1, 2, 3})},
			{Type: doubaospeech.ASTEventTTSSentenceEnd},
			{Type: doubaospeech.ASTEventSessionFinished},
		},
	}
	sessions := []*fakeASTTranslateSession{first, second, third}
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		if len(sessions) == 0 {
			t.Fatal("unexpected extra AST session")
		}
		next := sessions[0]
		sessions = sessions[1:]
		return next, nil
	}
	out, err := tr.transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	pushTurn := func(streamID string, sample byte) {
		t.Helper()
		for _, chunk := range []*genx.MessageChunk{
			genx.NewBeginOfStream(streamID),
			{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{sample, 0, sample + 1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}},
			{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
		} {
			if err := input.Push(chunk); err != nil {
				t.Fatalf("Push(%s): %v", streamID, err)
			}
		}
	}

	pushTurn("turn-1", 1)
	first.waitSentAudio(t)
	pushTurn("turn-2", 3)
	second.waitSentAudio(t)
	pushTurn("turn-3", 5)
	third.waitSentAudio(t)
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input): %v", err)
	}

	chunks := readAllASTTranslateChunks(t, out)
	for _, session := range []*fakeASTTranslateSession{first, second} {
		if closed, finished := session.state(); !closed || !finished {
			t.Fatalf("interrupted session closed/finished = %t/%t", closed, finished)
		}
	}
	if _, finished := third.state(); !finished {
		t.Fatal("third session was not finished")
	}
	assertASTTranslateHandoffOrder(t, chunks, "turn-1", "turn-2")
	assertASTTranslateHandoffOrder(t, chunks, "turn-2", "turn-3")
}

func TestTransformerRealtimeStartsNextSessionAfterPreviousFinished(t *testing.T) {
	input := newBufferStream(8)
	tr := newTransformer(doubaospeech.NewClient("app-id"),
		withMode(doubaospeech.ASTTranslateModeS2S),
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
	out, err := tr.transform(context.Background(), input)
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

func TestTransformerIgnoresLateInterruptedStreamChunks(t *testing.T) {
	input := newBufferStream(10)
	tr := newTransformer(doubaospeech.NewClient("app-id"),
		withMode(doubaospeech.ASTTranslateModeS2S),
	)
	first := &fakeASTTranslateSession{
		closeCh: make(chan struct{}),
		events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "stale"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "stale"},
		},
	}
	secondDone := make(chan struct{})
	second := &fakeASTTranslateSession{doneCh: secondDone,
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
	out, err := tr.transform(context.Background(), input)
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
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second provider did not finish")
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
	if got := collectASTTranslateText(chunks, genx.RoleModel, doubaoASTTranslateAssistantLabel, "turn-1"); !strings.HasSuffix(got, "reused fresh") {
		t.Fatalf("reused turn-1 text = %q, want reused fresh; chunks=%#v", got, chunks)
	}
	if len(sessions) != 0 {
		t.Fatalf("unused sessions = %d", len(sessions))
	}
	if _, finished := second.state(); !finished {
		t.Fatal("second session was not finished")
	}
	if _, finished := reused.state(); !finished {
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
	sentAtFinish        int
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
	s.sentAtFinish = len(s.sentAudio)
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

// state reads closed and finished under mu because provider cleanup may still
// call Close from a context.AfterFunc goroutine after the transform stream ends.
func (s *fakeASTTranslateSession) state() (closed, finished bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed, s.finished
}

func (s *fakeASTTranslateSession) sentAudioSnapshot() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte(nil), s.sentAudio...)
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

func TestHistoryUserAudioChunkDoesNotCopySourceBoundaries(t *testing.T) {
	source := &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("audio")},
		Ctrl: &genx.StreamCtrl{
			StreamID:      "source",
			Label:         "source",
			BeginOfStream: true,
			EndOfStream:   true,
			Error:         "source error",
		},
	}

	got := historyUserAudioChunk(source, "history")
	if got.Ctrl == nil || got.Ctrl.StreamID != "history" || got.Ctrl.Label != genx.HistoryUserAudioLabel {
		t.Fatalf("history route = %#v", got.Ctrl)
	}
	if got.IsBeginOfStream() || got.IsEndOfStream() || got.Ctrl.Error != "" {
		t.Fatalf("history data copied source boundaries = %#v", got.Ctrl)
	}
	if !source.IsBeginOfStream() || !source.IsEndOfStream() || source.Ctrl.Error != "source error" {
		t.Fatalf("source chunk was mutated = %#v", source.Ctrl)
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

func assertASTTranslateInterruptedLifecycles(t *testing.T, chunks []*genx.MessageChunk, streamID string) {
	t.Helper()
	routes := make(map[string][]*genx.MessageChunk)
	for _, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.StreamID != streamID ||
			chunk.Ctrl.Label != doubaoASTTranslateAssistantLabel {
			continue
		}
		mimeType, ok := chunk.MIMEType()
		if !ok {
			continue
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

func assertASTTranslateHandoffOrder(t *testing.T, chunks []*genx.MessageChunk, previousID, nextID string) {
	t.Helper()
	lastPreviousEOS := -1
	firstNextBOS := -1
	previousEOS := make(map[string]int)
	for index, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil || chunk.Role != genx.RoleModel ||
			chunk.Ctrl.Label != doubaoASTTranslateAssistantLabel {
			continue
		}
		mimeType, ok := chunk.MIMEType()
		if !ok {
			continue
		}
		if chunk.Ctrl.StreamID == previousID {
			if endedAt, ended := previousEOS[mimeType]; ended && index > endedAt {
				t.Fatalf("%s emitted %s after EOS at indexes %d and %d: %#v", previousID, mimeType, endedAt, index, chunks)
			}
			if chunk.IsEndOfStream() {
				if chunk.Ctrl.Error != "interrupted" {
					t.Fatalf("%s %s EOS error = %q, want interrupted", previousID, mimeType, chunk.Ctrl.Error)
				}
				previousEOS[mimeType] = index
				lastPreviousEOS = max(lastPreviousEOS, index)
			}
		}
		if chunk.Ctrl.StreamID == nextID && chunk.IsBeginOfStream() && firstNextBOS < 0 {
			firstNextBOS = index
		}
	}
	if len(previousEOS) != 2 || previousEOS["text/plain"] == 0 || previousEOS["audio/opus"] == 0 {
		t.Fatalf("%s interrupted EOS routes = %#v, want text/plain and audio/opus: %#v", previousID, previousEOS, chunks)
	}
	if firstNextBOS < 0 || lastPreviousEOS >= firstNextBOS {
		t.Fatalf("%s last EOS index = %d, %s first BOS index = %d: %#v", previousID, lastPreviousEOS, nextID, firstNextBOS, chunks)
	}
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
	var out strings.Builder
	for _, chunk := range chunks {
		if chunk.Role != role || chunk.Ctrl == nil || chunk.Ctrl.Label != label || chunk.Ctrl.StreamID != streamID {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			out.WriteString(string(text))
		}
	}
	return out.String()
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
