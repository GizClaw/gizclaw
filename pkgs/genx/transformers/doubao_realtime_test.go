package transformers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestDoubaoRealtimeAudioInputPassesPCMThrough(t *testing.T) {
	input := newDoubaoRealtimeAudioInput("pcm", 16000, 1, false)
	got, err := input.prepare(&genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}})
	if err != nil {
		t.Fatalf("prepare() error = %v", err)
	}
	if !bytes.Equal(got, []byte{1, 0, 2, 0}) {
		t.Fatalf("prepare() = %v", got)
	}
}

func TestDoubaoRealtimeAudioInputEncodesSpeechOpusSilence(t *testing.T) {
	input := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, false)
	defer input.close()
	frames, err := input.silenceFrames(2)
	if err != nil {
		t.Fatalf("silenceFrames() error = %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("silence frame count = %d, want 2", len(frames))
	}
	for i, frame := range frames {
		if len(frame) == 0 {
			t.Fatalf("silence frame %d is empty", i)
		}
	}
}

func TestDoubaoRealtimeAudioInputsRejectMIMEChange(t *testing.T) {
	inputs := newDoubaoRealtimeAudioInputs("speech_opus", 16000, 1, true)
	defer inputs.close()
	if _, err := inputs.streamForBlob("turn", &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}); err != nil {
		t.Fatalf("first streamForBlob() error = %v", err)
	}
	_, err := inputs.streamForBlob("turn", &genx.Blob{MIMEType: "audio/mpeg", Data: []byte{1, 2}})
	if err == nil {
		t.Fatal("streamForBlob() error = nil, want MIME change error")
	}
	if _, ok := err.(*doubaoRealtimeStreamMIMEChangeError); !ok {
		t.Fatalf("streamForBlob() error = %T, want *doubaoRealtimeStreamMIMEChangeError", err)
	}
}

func TestDoubaoRealtimeStreamIDsSplitRealtimeTranscript(t *testing.T) {
	ids := newDoubaoRealtimeStreamIDs(DoubaoRealtimeModeRealtime)
	ids.beginInput("audio")
	if got := ids.input(); got != "audio:rt:1" {
		t.Fatalf("first input = %q", got)
	}
	if ended := ids.endInputSegment(); ended != "audio:rt:1" {
		t.Fatalf("ended input = %q", ended)
	}
	if got := ids.input(); got != "audio:rt:2" {
		t.Fatalf("second input = %q", got)
	}
	if response := ids.response(); response != "audio:rt:1" {
		t.Fatalf("response = %q", response)
	}
}

func TestDoubaoRealtimeTextDeltaNormalizesPrefix(t *testing.T) {
	if got := realtimeTextDelta("你好，", "你好，世界"); got != "世界" {
		t.Fatalf("delta = %q, want 世界", got)
	}
	if got := realtimeTextDelta("Hello!", "hello world"); got != " world" {
		t.Fatalf("normalized delta = %q, want space-world suffix", got)
	}
}

func TestDoubaoRealtimeOutputAudioBlobsPassesPCM(t *testing.T) {
	tfr := NewDoubaoRealtime(nil, WithDoubaoRealtimeFormat("pcm"))
	blobs, err := tfr.outputAudioBlobs([]byte{1, 2, 3})
	if err != nil {
		t.Fatalf("outputAudioBlobs() error = %v", err)
	}
	if len(blobs) != 1 || blobs[0].MIMEType != "audio/pcm" || !bytes.Equal(blobs[0].Data, []byte{1, 2, 3}) {
		t.Fatalf("outputAudioBlobs() = %#v", blobs)
	}
}

func TestDoubaoRealtimeConfigSetsRealtimeSession(t *testing.T) {
	tfr := NewDoubaoRealtime(nil,
		WithDoubaoRealtimeMode(DoubaoRealtimeModeText),
		WithDoubaoRealtimeModel("O"),
		WithDoubaoRealtimeSpeaker("voice-a"),
		WithDoubaoRealtimeFormat("pcm"),
		WithDoubaoRealtimeSampleRate(16000),
		WithDoubaoRealtimeChannels(1),
		WithDoubaoRealtimeSpeechRate(12),
		WithDoubaoRealtimeLoudnessRate(6),
		WithDoubaoRealtimeASRExtra(doubaospeech.RealtimeASRExtra{
			EndSmoothWindowMS: 800,
			EnableCustomVAD:   boolPtr(true),
			EnableASRTwopass:  boolPtr(true),
			Context: &doubaospeech.RealtimeASRContext{
				Hotwords:     []doubaospeech.RealtimeHotword{{Word: "GizClaw"}},
				CorrectWords: map[string]string{"吉斯克劳": "GizClaw"},
			},
		}),
		WithDoubaoRealtimeTTSExtra(doubaospeech.RealtimeTTSExtra{
			ExplicitDialect: "sichuan",
			TTS20Model:      "expressive",
			AIGCMetadata: &doubaospeech.RealtimeAIGCMetadata{
				Enable:          boolPtr(true),
				ContentProducer: "gizclaw",
				ProduceID:       "produce-1",
			},
		}),
		WithDoubaoRealtimeBotName("bot"),
		WithDoubaoRealtimeSystemRole("brief"),
		WithDoubaoRealtimeSpeakingStyle("warm"),
		WithDoubaoRealtimeCharacterManifest("manifest"),
		WithDoubaoRealtimeDialogID("dialog-1"),
		WithDoubaoRealtimeDialogExtra(doubaospeech.RealtimeDialogExtra{
			EnableVolcWebsearch:          boolPtr(true),
			VolcWebsearchType:            "web",
			VolcWebsearchResultCount:     3,
			VolcWebsearchNoResultMessage: "没有找到相关搜索结果。",
		}),
		WithDoubaoRealtimeSearchAPIKey("search-key"),
	)
	if tfr.dialogID != "dialog-1" {
		t.Fatalf("dialogID = %q, want dialog-1", tfr.dialogID)
	}
	cfg := tfr.realtimeConfig()
	if cfg.InputMode != doubaospeech.RealtimeInputModeText || cfg.Model != doubaospeech.RealtimeModelVersion("O") {
		t.Fatalf("mode/model = %q/%q", cfg.InputMode, cfg.Model)
	}
	if cfg.ASR.AudioInfo == nil ||
		cfg.ASR.AudioInfo.Format != doubaospeech.FormatSpeechOpus ||
		cfg.ASR.AudioInfo.SampleRate != doubaospeech.SampleRate16000 ||
		cfg.ASR.AudioInfo.Channel != 1 {
		t.Fatalf("asr audio info = %#v", cfg.ASR.AudioInfo)
	}
	if cfg.TTS.Speaker != "voice-a" || cfg.TTS.AudioConfig.Format != "pcm" || cfg.TTS.AudioConfig.SampleRate != 16000 || cfg.TTS.AudioConfig.Channel != 1 {
		t.Fatalf("tts config = %#v", cfg.TTS)
	}
	if cfg.TTS.AudioConfig.SpeechRate != 12 || cfg.TTS.AudioConfig.LoudnessRate != 6 {
		t.Fatalf("tts audio rates = %#v", cfg.TTS.AudioConfig)
	}
	if cfg.ASR.Extra == nil || cfg.ASR.Extra.EndSmoothWindowMS != 800 ||
		cfg.ASR.Extra.EnableCustomVAD == nil || !*cfg.ASR.Extra.EnableCustomVAD ||
		cfg.ASR.Extra.EnableASRTwopass == nil || !*cfg.ASR.Extra.EnableASRTwopass ||
		cfg.ASR.Extra.Context == nil || len(cfg.ASR.Extra.Context.Hotwords) != 1 ||
		cfg.ASR.Extra.Context.Hotwords[0].Word != "GizClaw" ||
		cfg.ASR.Extra.Context.CorrectWords["吉斯克劳"] != "GizClaw" {
		t.Fatalf("asr extra = %#v", cfg.ASR.Extra)
	}
	if cfg.TTS.Extra == nil || cfg.TTS.Extra.ExplicitDialect != "sichuan" ||
		cfg.TTS.Extra.TTS20Model != "expressive" ||
		cfg.TTS.Extra.AIGCMetadata == nil ||
		cfg.TTS.Extra.AIGCMetadata.Enable == nil || !*cfg.TTS.Extra.AIGCMetadata.Enable ||
		cfg.TTS.Extra.AIGCMetadata.ContentProducer != "gizclaw" ||
		cfg.TTS.Extra.AIGCMetadata.ProduceID != "produce-1" {
		t.Fatalf("tts extra = %#v", cfg.TTS.Extra)
	}
	if cfg.Dialog.BotName != "bot" || cfg.Dialog.SystemRole != "brief" ||
		cfg.Dialog.SpeakingStyle != "warm" || cfg.Dialog.CharacterManifest != "manifest" {
		t.Fatalf("dialog config = %#v", cfg.Dialog)
	}
	if cfg.Dialog.DialogID != "dialog-1" {
		t.Fatalf("dialog_id = %q, want dialog-1", cfg.Dialog.DialogID)
	}
	if cfg.Dialog.Extra == nil || cfg.Dialog.Extra.EnableVolcWebsearch == nil || !*cfg.Dialog.Extra.EnableVolcWebsearch {
		t.Fatalf("dialog extra search enabled = %#v, want true", cfg.Dialog.Extra)
	}
	if cfg.Dialog.Extra.VolcWebsearchAPIKey != "search-key" ||
		cfg.Dialog.Extra.VolcWebsearchType != "web" ||
		cfg.Dialog.Extra.VolcWebsearchResultCount != 3 ||
		cfg.Dialog.Extra.VolcWebsearchNoResultMessage != "没有找到相关搜索结果。" {
		t.Fatalf("dialog extra = %#v", cfg.Dialog.Extra)
	}
}

func TestDoubaoRealtimePushToTalkEndsASR(t *testing.T) {
	endASR := make(chan struct{})
	session := &fakeDoubaoRealtimeSession{
		beforeRecv: endASR,
		endASR:     endASR,
		events:     []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventSessionFinished}},
	}
	tfr := NewDoubaoRealtime(nil,
		WithDoubaoRealtimeInputFormat("pcm"),
		WithDoubaoRealtimeInputTranscode(false),
	)
	input := &sliceRealtimeStream{chunks: []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	}}
	output := newBufferStream(16)

	err := runDoubaoRealtimeProcessLoop(t, tfr, input, output, session)
	if err != nil {
		t.Fatalf("processLoop() error = %v", err)
	}
	if session.endASRCount() != 1 {
		t.Fatalf("EndASR calls = %d, want 1", session.endASRCount())
	}
	if sent := session.audioFrames(); len(sent) != 1 {
		t.Fatalf("SendAudio calls = %d, want 1", len(sent))
	}
}

func TestDoubaoRealtimePushToTalkRejectsInvalidInputTransitions(t *testing.T) {
	tests := []struct {
		name       string
		chunks     []*genx.MessageChunk
		wantErr    string
		wantEndASR int
	}{
		{
			name: "audio before BOS",
			chunks: []*genx.MessageChunk{
				{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
			},
			wantErr: "received audio outside an active BOS/EOS turn",
		},
		{
			name: "EOS before BOS",
			chunks: []*genx.MessageChunk{
				{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
			},
			wantErr: "received EOS before active BOS",
		},
		{
			name: "duplicate EOS",
			chunks: []*genx.MessageChunk{
				{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
				{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
				{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
				{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
			},
			wantErr:    "received EOS before active BOS",
			wantEndASR: 1,
		},
		{
			name: "nested BOS",
			chunks: []*genx.MessageChunk{
				{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
				{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
			},
			wantErr: "received BOS while already capturing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endASR := make(chan struct{})
			session := &fakeDoubaoRealtimeSession{
				beforeRecv: endASR,
				endASR:     endASR,
				events:     []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventSessionFinished}},
			}
			tfr := NewDoubaoRealtime(nil,
				WithDoubaoRealtimeInputFormat("pcm"),
				WithDoubaoRealtimeInputTranscode(false),
			)
			output := newBufferStream(16)
			err := runDoubaoRealtimeProcessLoop(t, tfr, &sliceRealtimeStream{chunks: tt.chunks}, output, session)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("processLoop() error = %v, want containing %q", err, tt.wantErr)
			}
			if got := session.endASRCount(); got != tt.wantEndASR {
				t.Fatalf("EndASR calls = %d, want %d", got, tt.wantEndASR)
			}
		})
	}
}

func TestDoubaoRealtimeMapsRealtimeEventsToStreamChunks(t *testing.T) {
	session := &fakeDoubaoRealtimeSession{
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "你好"},
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventTTSStarted},
			{Type: doubaospeech.EventChatResponse, Text: "收到"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2, 3}},
			{Type: doubaospeech.EventTTSFinished},
			{Type: doubaospeech.EventChatEnded},
			{Type: doubaospeech.EventSessionFinished},
		},
	}
	tfr := NewDoubaoRealtime(nil, WithDoubaoRealtimeFormat("pcm"))
	output := newBufferStream(16)

	err := runDoubaoRealtimeProcessLoop(t, tfr, &sliceRealtimeStream{}, output, session)
	if err != nil {
		t.Fatalf("processLoop() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if !hasRealtimeTestText(chunks, genx.RoleUser, "你好") {
		t.Fatalf("output missing user transcript: %#v", chunks)
	}
	if !hasRealtimeTestText(chunks, genx.RoleModel, "收到") {
		t.Fatalf("output missing model text: %#v", chunks)
	}
	if !hasRealtimeTestBlob(chunks, genx.RoleModel, "audio/pcm") {
		t.Fatalf("output missing model audio: %#v", chunks)
	}
}

func TestDoubaoRealtimeInterruptsPendingResponseBeforeTTS(t *testing.T) {
	eventsDrained := make(chan struct{})
	releaseEvents := make(chan struct{})
	allowNextInput := make(chan struct{})
	firstDrained := make(chan struct{})
	session := &fakeDoubaoRealtimeSession{
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "第一段"},
			{Type: doubaospeech.EventASREnded},
		},
		beforeRecv:       firstDrained,
		eventsDrained:    eventsDrained,
		blockAfterEvents: releaseEvents,
	}
	tfr := NewDoubaoRealtime(nil,
		WithDoubaoRealtimeMode(DoubaoRealtimeModeRealtime),
		WithDoubaoRealtimeInputFormat("pcm"),
		WithDoubaoRealtimeInputTranscode(false),
		WithDoubaoRealtimeFormat("pcm"),
	)
	input := &gatedRealtimeStream{
		first: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
			{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		},
		gate:         allowNextInput,
		firstDrained: firstDrained,
		rest: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output := newBufferStream(16)
	errCh := make(chan error, 1)
	go func() {
		next, err := tfr.processLoop(ctx, input, output, session)
		if err == nil {
			if next == nil || next.Ctrl == nil || next.Ctrl.StreamID != "turn-2" || !next.Ctrl.BeginOfStream {
				err = fmt.Errorf("processLoop() next chunk = %#v, want turn-2 BOS", next)
			}
		}
		output.Close()
		errCh <- err
	}()

	select {
	case <-eventsDrained:
	case <-ctx.Done():
		t.Fatalf("events did not reach pending response state: %v", ctx.Err())
	}
	close(allowNextInput)
	select {
	case err := <-errCh:
		close(releaseEvents)
		if err != nil {
			t.Fatalf("processLoop() error = %v", err)
		}
	case <-ctx.Done():
		close(releaseEvents)
		t.Fatalf("processLoop() timed out: %v", ctx.Err())
	}
	if got := session.interruptCount(); got != 1 {
		t.Fatalf("Interrupt calls = %d, want 1", got)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if !hasRealtimeInterruptedEOS(chunks, "turn-1:rt:1", genx.RoleModel, false) {
		t.Fatalf("missing interrupted text EOS for pending response: %#v", chunks)
	}
	if !hasRealtimeInterruptedEOS(chunks, "turn-1:rt:1", genx.RoleModel, true) {
		t.Fatalf("missing interrupted audio EOS for pending response: %#v", chunks)
	}
}

func runDoubaoRealtimeProcessLoop(t *testing.T, tfr *DoubaoRealtime, input genx.Stream, output *bufferStream, session *fakeDoubaoRealtimeSession) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, err := tfr.processLoop(ctx, input, output, session)
		output.Close()
		errCh <- err
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func drainRealtimeTestOutput(t *testing.T, output genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := output.Next()
		if err != nil {
			if err == io.EOF || err == genx.ErrDone {
				return chunks
			}
			t.Fatalf("output Next() error = %v", err)
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
}

func hasRealtimeTestText(chunks []*genx.MessageChunk, role genx.Role, text string) bool {
	for _, chunk := range chunks {
		got, ok := chunk.Part.(genx.Text)
		if chunk.Role == role && ok && string(got) == text {
			return true
		}
	}
	return false
}

func hasRealtimeTestBlob(chunks []*genx.MessageChunk, role genx.Role, mimeType string) bool {
	for _, chunk := range chunks {
		got, ok := chunk.Part.(*genx.Blob)
		if chunk.Role == role && ok && got.MIMEType == mimeType && len(got.Data) > 0 {
			return true
		}
	}
	return false
}

func hasRealtimeInterruptedEOS(chunks []*genx.MessageChunk, streamID string, role genx.Role, audio bool) bool {
	for _, chunk := range chunks {
		if chunk == nil || chunk.Role != role || chunk.Ctrl == nil ||
			chunk.Ctrl.StreamID != streamID || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != doubaoRealtimeInterrupted {
			continue
		}
		_, isAudio := chunk.Part.(*genx.Blob)
		if isAudio == audio {
			return true
		}
	}
	return false
}

type fakeDoubaoRealtimeSession struct {
	events           []*doubaospeech.RealtimeEvent
	beforeRecv       <-chan struct{}
	endASR           chan struct{}
	eventsDrained    chan<- struct{}
	blockAfterEvents <-chan struct{}

	mu                sync.Mutex
	audio             [][]byte
	texts             []string
	endCount          int
	interrupts        int
	closed            bool
	endOnce           sync.Once
	eventsDrainedOnce sync.Once
}

func (s *fakeDoubaoRealtimeSession) SendAudio(ctx context.Context, audio []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audio = append(s.audio, append([]byte(nil), audio...))
	return nil
}

func (s *fakeDoubaoRealtimeSession) SendText(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.texts = append(s.texts, text)
	return nil
}

func (s *fakeDoubaoRealtimeSession) EndASR(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.endCount++
	s.mu.Unlock()
	if s.endASR != nil {
		s.endOnce.Do(func() { close(s.endASR) })
	}
	return nil
}

func (s *fakeDoubaoRealtimeSession) Interrupt(context.Context) error {
	s.mu.Lock()
	s.interrupts++
	s.mu.Unlock()
	return nil
}

func (s *fakeDoubaoRealtimeSession) Recv() iter.Seq2[*doubaospeech.RealtimeEvent, error] {
	return func(yield func(*doubaospeech.RealtimeEvent, error) bool) {
		if s.beforeRecv != nil {
			<-s.beforeRecv
		}
		for _, event := range s.events {
			if !yield(event, nil) {
				return
			}
		}
		if s.eventsDrained != nil {
			s.eventsDrainedOnce.Do(func() {
				close(s.eventsDrained)
			})
		}
		if s.blockAfterEvents != nil {
			<-s.blockAfterEvents
		}
	}
}

func (s *fakeDoubaoRealtimeSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *fakeDoubaoRealtimeSession) endASRCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.endCount
}

func (s *fakeDoubaoRealtimeSession) interruptCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interrupts
}

func (s *fakeDoubaoRealtimeSession) audioFrames() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.audio))
	for i := range s.audio {
		out[i] = append([]byte(nil), s.audio[i]...)
	}
	return out
}

func boolPtr(value bool) *bool {
	return &value
}
