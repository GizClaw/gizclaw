package transformers

import (
	"bytes"
	"context"
	"errors"
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
			EnableCustomVAD:   new(true),
			EnableASRTwopass:  new(true),
			Context: &doubaospeech.RealtimeASRContext{
				Hotwords:     []doubaospeech.RealtimeHotword{{Word: "GizClaw"}},
				CorrectWords: map[string]string{"吉斯克劳": "GizClaw"},
			},
		}),
		WithDoubaoRealtimeTTSExtra(doubaospeech.RealtimeTTSExtra{
			ExplicitDialect: "sichuan",
			TTS20Model:      "expressive",
			AIGCMetadata: &doubaospeech.RealtimeAIGCMetadata{
				Enable:          new(true),
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
			EnableVolcWebsearch:          new(true),
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

func TestDoubaoRealtimePushToTalkWaitsForAudioEOS(t *testing.T) {
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
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	}}
	output := newBufferStream(16)

	if err := runDoubaoRealtimeProcessLoop(t, tfr, input, output, session); err != nil {
		t.Fatalf("processLoop() error = %v", err)
	}
	if got := session.endASRCount(); got != 1 {
		t.Fatalf("EndASR calls = %d, want 1", got)
	}
	if sent := session.audioFrames(); len(sent) != 2 {
		t.Fatalf("SendAudio calls = %d, want 2", len(sent))
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
				endASR:           endASR,
				blockAfterEvents: make(chan struct{}),
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

func TestDoubaoPushToTalkStateLifecycleAndBargeIn(t *testing.T) {
	state := &doubaoPushToTalkState{}
	if got := state.current(); got != doubaoPushToTalkIdle {
		t.Fatalf("initial phase = %v, want idle", got)
	}
	bargeIn, interrupted, err := state.begin("turn-1")
	if err != nil || bargeIn {
		t.Fatalf("begin() = (%v, %q, %v), want (false, empty, nil)", bargeIn, interrupted, err)
	}
	if err := state.requireCapturing("audio"); err != nil {
		t.Fatalf("requireCapturing() error = %v", err)
	}
	if err := state.end(); err != nil {
		t.Fatalf("end() error = %v", err)
	}
	if got := state.current(); got != doubaoPushToTalkWaitingResponse {
		t.Fatalf("phase after end = %v, want waiting response", got)
	}
	bargeIn, interrupted, err = state.begin("turn-2")
	if err != nil || !bargeIn || interrupted != "turn-1" {
		t.Fatalf("begin() while waiting = (%v, %q, %v), want (true, turn-1, nil)", bargeIn, interrupted, err)
	}
	if err := state.end(); err != nil {
		t.Fatalf("second end() error = %v", err)
	}
	state.responseStarted(true)
	if got := state.current(); got != doubaoPushToTalkResponding {
		t.Fatalf("phase after response = %v, want responding", got)
	}
	bargeIn, interrupted, err = state.begin("turn-3")
	if err != nil || !bargeIn || interrupted != "turn-2" {
		t.Fatalf("begin() while responding = (%v, %q, %v), want (true, turn-2, nil)", bargeIn, interrupted, err)
	}
}

func TestDoubaoRealtimePTTTurnCommitsLatestHypothesisBeforeAssistantOutput(t *testing.T) {
	output := &recordingRealtimeOutput{}
	turn := &doubaoRealtimePTTTurn{}
	turn.begin(output, "turn-1", doubaoRealtimeAssistantLabel, doubaoRealtimePTTOutputLimit)
	turn.updateHypothesis("partial")
	turn.updateHypothesis("final")
	if err := turn.pushAssistant(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("answer"),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: doubaoRealtimeAssistantLabel},
	}); err != nil {
		t.Fatalf("pushAssistant() error = %v", err)
	}
	if err := turn.markASREnded(); err != nil {
		t.Fatalf("markASREnded() error = %v", err)
	}
	if got := output.chunks(); len(got) != 0 {
		t.Fatalf("output before input EOS = %#v, want none", got)
	}
	if err := turn.markInputEnded(); err != nil {
		t.Fatalf("markInputEnded() error = %v", err)
	}

	chunks := output.chunks()
	if len(chunks) != 3 {
		t.Fatalf("output chunks = %d, want transcript, transcript EOS, assistant", len(chunks))
	}
	if text, ok := chunks[0].Part.(genx.Text); !ok || text != "final" {
		t.Fatalf("committed transcript = %#v, want final snapshot", chunks[0])
	}
	if chunks[1].Ctrl == nil || !chunks[1].Ctrl.EndOfStream || chunks[1].Ctrl.Label != doubaoRealtimeTranscriptLabel {
		t.Fatalf("second chunk = %#v, want transcript EOS", chunks[1])
	}
	if text, ok := chunks[2].Part.(genx.Text); !ok || text != "answer" {
		t.Fatalf("assistant output = %#v, want retained answer", chunks[2])
	}
}

func TestDoubaoRealtimeProviderLossDoesNotRepeatCommittedPTTTranscriptEOS(t *testing.T) {
	tfr := NewDoubaoRealtime(nil)
	runtime := newDoubaoRealtimeRuntime(tfr)
	defer runtime.close()
	output := &recordingRealtimeOutput{}
	runtime.pttTurn.begin(output, "turn-1", doubaoRealtimeAssistantLabel, doubaoRealtimePTTOutputLimit)
	runtime.pttTurn.updateHypothesis("final")
	if err := runtime.pttTurn.markInputEnded(); err != nil {
		t.Fatalf("markInputEnded() error = %v", err)
	}
	if err := runtime.pttTurn.markASREnded(); err != nil {
		t.Fatalf("markASREnded() error = %v", err)
	}
	before := len(output.chunks())
	runtime.providerLost(tfr, output, errors.New("provider lost"))
	if got := len(output.chunks()); got != before {
		t.Fatalf("output chunks after provider loss = %d, want unchanged %d", got, before)
	}
}

func TestRealtimePTTOutputGateEnforcesOpusDurationLimit(t *testing.T) {
	packet := []byte{0x98}
	packetDuration := time.Duration(historyOpusPacketDurationMS(packet)) * time.Millisecond
	if packetDuration <= 0 {
		t.Fatalf("packet duration = %s, want positive", packetDuration)
	}
	chunk := func() *genx.MessageChunk {
		return &genx.MessageChunk{
			Role: genx.RoleModel,
			Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: doubaoRealtimeAssistantLabel},
		}
	}
	belowOutput := &recordingRealtimeOutput{}
	below := newRealtimePTTOutputGate(belowOutput, "turn-below", doubaoRealtimeAssistantLabel, 2*packetDuration)
	if err := below.Push(chunk()); err != nil {
		t.Fatalf("below-limit Push() error = %v", err)
	}
	if err := below.Commit(); err != nil {
		t.Fatalf("below-limit Commit() error = %v", err)
	}
	if got := len(belowOutput.chunks()); got != 1 {
		t.Fatalf("below-limit output chunks = %d, want 1", got)
	}

	output := &recordingRealtimeOutput{}
	gate := newRealtimePTTOutputGate(output, "turn-1", doubaoRealtimeAssistantLabel, 2*packetDuration)
	if err := gate.Push(chunk()); err != nil {
		t.Fatalf("first Push() error = %v", err)
	}
	if err := gate.Push(chunk()); err != nil {
		t.Fatalf("exact-limit Push() error = %v", err)
	}
	if err := gate.Push(chunk()); !errors.Is(err, errRealtimePTTOutputLimit) {
		t.Fatalf("over-limit Push() error = %v, want output limit", err)
	}
	chunks := output.chunks()
	if len(chunks) != 1 || chunks[0].Ctrl == nil || !chunks[0].Ctrl.EndOfStream || chunks[0].Ctrl.Error == "" {
		t.Fatalf("limit output = %#v, want one error EOS", chunks)
	}
	if err := gate.Commit(); !errors.Is(err, errRealtimePTTOutputLimit) {
		t.Fatalf("Commit() error = %v, want output limit", err)
	}
	if got := len(output.chunks()); got != 1 {
		t.Fatalf("output chunks after Commit = %d, want 1", got)
	}

	nextOutput := &recordingRealtimeOutput{}
	next := newRealtimePTTOutputGate(nextOutput, "turn-2", doubaoRealtimeAssistantLabel, 2*packetDuration)
	if err := next.Push(chunk()); err != nil {
		t.Fatalf("next-turn Push() error = %v", err)
	}
	if err := next.Commit(); err != nil {
		t.Fatalf("next-turn Commit() error = %v", err)
	}
	if got := len(nextOutput.chunks()); got != 1 {
		t.Fatalf("next-turn output chunks = %d, want 1", got)
	}
}

func TestDoubaoRealtimeEOSIsLocalInRealtimeMode(t *testing.T) {
	session := &fakeDoubaoRealtimeSession{blockAfterEvents: make(chan struct{})}
	tfr := NewDoubaoRealtime(nil,
		WithDoubaoRealtimeMode(DoubaoRealtimeModeRealtime),
		WithDoubaoRealtimeInputFormat("pcm"),
		WithDoubaoRealtimeInputTranscode(false),
	)
	input := &sliceRealtimeStream{chunks: []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	}}
	if err := runDoubaoRealtimeProcessLoop(t, tfr, input, newBufferStream(8), session); err != nil {
		t.Fatalf("processLoop() error = %v", err)
	}
	if got := session.endASRCount(); got != 0 {
		t.Fatalf("EndASR calls = %d, want 0", got)
	}
	if got := len(session.audioFrames()); got != 1 {
		t.Fatalf("SendAudio calls = %d, want only the client audio frame", got)
	}
}

func TestDoubaoRealtimeASRInfoInterruptsPendingAssistantOnce(t *testing.T) {
	eventsDrained := make(chan struct{})
	allowEOF := make(chan struct{})
	session := &fakeDoubaoRealtimeSession{
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventASRInfo},
			{Type: doubaospeech.EventASRInfo},
		},
		eventsDrained:    eventsDrained,
		blockAfterEvents: make(chan struct{}),
	}
	tfr := NewDoubaoRealtime(nil, WithDoubaoRealtimeMode(DoubaoRealtimeModeRealtime))
	input := &gatedRealtimeStream{gate: allowEOF}
	output := newBufferStream(8)
	errCh := make(chan error, 1)
	go func() { errCh <- runDoubaoRealtimeProcessLoop(t, tfr, input, output, session) }()
	select {
	case <-eventsDrained:
	case <-time.After(2 * time.Second):
		t.Fatal("realtime events were not drained")
	}
	close(allowEOF)
	if err := <-errCh; err != nil {
		t.Fatalf("processLoop() error = %v", err)
	}
	if got := session.interruptCount(); got != 1 {
		t.Fatalf("Interrupt calls = %d, want 1", got)
	}
}

func TestDoubaoRealtimeSessionLoopRetriesAndReusesDialogID(t *testing.T) {
	opener := &fakeDoubaoRealtimeOpener{results: []fakeDoubaoRealtimeOpenResult{
		{err: errors.New("connect-1")},
		{err: errors.New("connect-2")},
		{session: &fakeDoubaoRealtimeSession{blockAfterEvents: make(chan struct{})}},
	}}
	tfr := NewDoubaoRealtime(nil,
		withDoubaoRealtimeOpener(opener),
		WithDoubaoRealtimeDialogID("dialog-1"),
	)
	tfr.retryInitial = time.Millisecond
	tfr.retryMax = 2 * time.Millisecond
	output, err := tfr.Transform(context.Background(), "", &sliceRealtimeStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if chunks := drainRealtimeTestOutput(t, output); len(chunks) != 0 {
		t.Fatalf("output = %#v, want none", chunks)
	}
	if got := opener.callCount(); got != 3 {
		t.Fatalf("OpenSession calls = %d, want two retries then one session", got)
	}
	for i, dialogID := range opener.dialogIDs() {
		if dialogID != "dialog-1" {
			t.Fatalf("OpenSession call %d dialog ID = %q, want dialog-1", i+1, dialogID)
		}
	}
}

func TestDoubaoRealtimeSessionLoopReplacesFinishedSession(t *testing.T) {
	opener := &fakeDoubaoRealtimeOpener{results: []fakeDoubaoRealtimeOpenResult{
		{session: &fakeDoubaoRealtimeSession{events: []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventSessionFinished}}}},
		{session: &fakeDoubaoRealtimeSession{blockAfterEvents: make(chan struct{})}},
	}}
	tfr := NewDoubaoRealtime(nil, withDoubaoRealtimeOpener(opener))
	input := newBlockingRealtimeStream()
	output, err := tfr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if !opener.waitForCalls(2, 2*time.Second) {
		t.Fatalf("OpenSession calls = %d, want replacement session", opener.callCount())
	}
	if err := input.CloseWithError(io.EOF); err != nil {
		t.Fatalf("CloseWithError(input) error = %v", err)
	}
	if chunks := drainRealtimeTestOutput(t, output); len(chunks) != 0 {
		t.Fatalf("output = %#v, want none", chunks)
	}
	if got := opener.callCount(); got != 2 {
		t.Fatalf("OpenSession calls = %d, want replacement session", got)
	}
}

func TestDoubaoRealtimeSessionLoopStopsRetryOnContextCancellation(t *testing.T) {
	opener := &fakeDoubaoRealtimeOpener{results: []fakeDoubaoRealtimeOpenResult{
		{err: errors.New("connect-1")},
		{err: errors.New("connect-2")},
		{err: errors.New("connect-3")},
	}}
	tfr := NewDoubaoRealtime(nil, withDoubaoRealtimeOpener(opener))
	tfr.retryInitial = time.Millisecond
	tfr.retryMax = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	output, err := tfr.Transform(ctx, "", newBlockingRealtimeStream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if !opener.waitForCalls(3, 2*time.Second) {
		t.Fatalf("OpenSession calls = %d, want ongoing retries", opener.callCount())
	}
	cancel()
	if _, err := output.Next(); !errors.Is(err, context.Canceled) {
		t.Fatalf("output Next() error = %v, want context canceled", err)
	}
	calls := opener.callCount()
	time.Sleep(5 * time.Millisecond)
	if got := opener.callCount(); got != calls {
		t.Fatalf("OpenSession calls after cancellation = %d, want stable %d", got, calls)
	}
}

func TestDoubaoRealtimeDoesNotReplayAmbiguousAudioAfterReconnect(t *testing.T) {
	first := &fakeDoubaoRealtimeSession{
		blockAfterEvents: make(chan struct{}),
		sendAudioErr:     errors.New("write failed after handoff"),
		sendAudioErrAt:   1,
	}
	second := &fakeDoubaoRealtimeSession{blockAfterEvents: make(chan struct{})}
	opener := &fakeDoubaoRealtimeOpener{results: []fakeDoubaoRealtimeOpenResult{{session: first}, {session: second}}}
	tfr := NewDoubaoRealtime(nil,
		withDoubaoRealtimeOpener(opener),
		WithDoubaoRealtimeMode(DoubaoRealtimeModeRealtime),
		WithDoubaoRealtimeInputFormat("pcm"),
		WithDoubaoRealtimeInputTranscode(false),
	)
	input := &sliceRealtimeStream{chunks: []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
	}}
	output, err := tfr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	_ = drainRealtimeTestOutput(t, output)
	if got := first.audioFrames(); len(got) != 1 || !bytes.Equal(got[0], []byte{1, 0}) {
		t.Fatalf("first session audio = %v, want first frame attempt", got)
	}
	if got := second.audioFrames(); len(got) != 1 || !bytes.Equal(got[0], []byte{2, 0}) {
		t.Fatalf("replacement session audio = %v, want only unread second frame", got)
	}
}

func TestDoubaoRealtimePTTDiscardsFailedTurnRemainderAfterReconnect(t *testing.T) {
	first := &fakeDoubaoRealtimeSession{
		blockAfterEvents: make(chan struct{}),
		sendAudioErr:     errors.New("provider lost"),
		sendAudioErrAt:   1,
	}
	second := &fakeDoubaoRealtimeSession{blockAfterEvents: make(chan struct{})}
	opener := &fakeDoubaoRealtimeOpener{results: []fakeDoubaoRealtimeOpenResult{{session: first}, {session: second}}}
	tfr := NewDoubaoRealtime(nil,
		withDoubaoRealtimeOpener(opener),
		WithDoubaoRealtimeMode(DoubaoRealtimeModePushToTalk),
		WithDoubaoRealtimeInputFormat("pcm"),
		WithDoubaoRealtimeInputTranscode(false),
	)
	input := &sliceRealtimeStream{chunks: []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{3, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-2"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-2", EndOfStream: true}},
	}}
	output, err := tfr.Transform(context.Background(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	_ = drainRealtimeTestOutput(t, output)
	if got := second.audioFrames(); len(got) != 1 || !bytes.Equal(got[0], []byte{3, 0}) {
		t.Fatalf("replacement session audio = %v, want only next turn frame", got)
	}
	if got := second.endASRCount(); got != 1 {
		t.Fatalf("replacement EndASR calls = %d, want only next turn", got)
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
	tfr := NewDoubaoRealtime(nil,
		WithDoubaoRealtimeMode(DoubaoRealtimeModeRealtime),
		WithDoubaoRealtimeFormat("pcm"),
	)
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
	firstAudioSent := make(chan struct{})
	session := &fakeDoubaoRealtimeSession{
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "第一段"},
			{Type: doubaospeech.EventASREnded},
		},
		beforeRecv:       firstAudioSent,
		firstAudioSent:   firstAudioSent,
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
		gate: allowNextInput,
		rest: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output := newBufferStream(16)
	errCh := make(chan error, 1)
	go func() {
		_, err := tfr.processLoop(ctx, input, output, session)
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

func TestDoubaoRealtimePushToTalkBargeInWhileWaitingResponse(t *testing.T) {
	eventsDrained := make(chan struct{})
	releaseEvents := make(chan struct{})
	allowNextInput := make(chan struct{})
	endASR := make(chan struct{})
	session := &fakeDoubaoRealtimeSession{
		events:           []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventASRResponse, Text: "第一段"}, {Type: doubaospeech.EventASREnded}},
		beforeRecv:       endASR,
		endASR:           endASR,
		eventsDrained:    eventsDrained,
		blockAfterEvents: releaseEvents,
	}
	tfr := NewDoubaoRealtime(nil,
		WithDoubaoRealtimeMode(DoubaoRealtimeModePushToTalk),
		WithDoubaoRealtimeInputFormat("pcm"),
		WithDoubaoRealtimeInputTranscode(false),
		WithDoubaoRealtimeFormat("pcm"),
	)
	input := &gatedRealtimeStream{
		first: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
			{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		},
		gate: allowNextInput,
		rest: []*genx.MessageChunk{{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output := newBufferStream(16)
	errCh := make(chan error, 1)
	go func() {
		_, err := tfr.processLoop(ctx, input, output, session)
		output.Close()
		errCh <- err
	}()

	select {
	case <-eventsDrained:
	case <-ctx.Done():
		t.Fatalf("events did not reach waiting-response state: %v", ctx.Err())
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
	if got := session.endASRCount(); got != 1 {
		t.Fatalf("EndASR calls = %d, want 1", got)
	}
	if got := session.interruptCount(); got != 1 {
		t.Fatalf("Interrupt calls = %d, want 1", got)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if !hasRealtimeInterruptedEOS(chunks, "turn-1", genx.RoleModel, false) ||
		!hasRealtimeInterruptedEOS(chunks, "turn-1", genx.RoleModel, true) {
		t.Fatalf("missing interrupted response EOS: %#v", chunks)
	}
}

func TestDoubaoRealtimeBargeInPropagatesInterruptFailure(t *testing.T) {
	eventsDrained := make(chan struct{})
	releaseEvents := make(chan struct{})
	allowInput := make(chan struct{})
	session := &fakeDoubaoRealtimeSession{
		events:           []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventASREnded}},
		eventsDrained:    eventsDrained,
		blockAfterEvents: releaseEvents,
		interruptErr:     errors.New("interrupt failed"),
	}
	input := &gatedRealtimeStream{
		gate: allowInput,
		rest: []*genx.MessageChunk{{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}}},
	}
	tfr := NewDoubaoRealtime(nil, WithDoubaoRealtimeMode(DoubaoRealtimeModePushToTalk))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		reader := newDoubaoRealtimeInputReader(input)
		defer reader.Close()
		runtime := newDoubaoRealtimeRuntime(tfr)
		defer runtime.close()
		err := tfr.processSession(ctx, reader, newBufferStream(8), session, runtime)
		errCh <- err
	}()
	select {
	case <-eventsDrained:
	case <-ctx.Done():
		t.Fatalf("events did not make response interruptible: %v", ctx.Err())
	}
	close(allowInput)
	select {
	case err := <-errCh:
		close(releaseEvents)
		if err == nil || !strings.Contains(err.Error(), "interrupt failed") {
			t.Fatalf("processLoop() error = %v, want interrupt failure", err)
		}
	case <-ctx.Done():
		close(releaseEvents)
		t.Fatalf("processLoop() timed out: %v", ctx.Err())
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

type recordingRealtimeOutput struct {
	mu    sync.Mutex
	items []*genx.MessageChunk
}

func (o *recordingRealtimeOutput) Push(chunk *genx.MessageChunk) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.items = append(o.items, chunk.Clone())
	return nil
}

func (o *recordingRealtimeOutput) chunks() []*genx.MessageChunk {
	o.mu.Lock()
	defer o.mu.Unlock()
	items := make([]*genx.MessageChunk, 0, len(o.items))
	for _, chunk := range o.items {
		items = append(items, chunk.Clone())
	}
	return items
}

type fakeDoubaoRealtimeOpenResult struct {
	session doubaoRealtimeSession
	err     error
}

type fakeDoubaoRealtimeOpener struct {
	mu      sync.Mutex
	results []fakeDoubaoRealtimeOpenResult
	calls   int
	dialogs []string
}

func (o *fakeDoubaoRealtimeOpener) OpenSession(_ context.Context, cfg *doubaospeech.RealtimeConfig) (doubaoRealtimeSession, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls++
	if cfg == nil {
		o.dialogs = append(o.dialogs, "")
	} else {
		o.dialogs = append(o.dialogs, cfg.Dialog.DialogID)
	}
	if len(o.results) == 0 {
		return nil, errors.New("unexpected extra OpenSession call")
	}
	result := o.results[0]
	o.results = o.results[1:]
	return result.session, result.err
}

func (o *fakeDoubaoRealtimeOpener) callCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls
}

func (o *fakeDoubaoRealtimeOpener) dialogIDs() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.dialogs...)
}

func (o *fakeDoubaoRealtimeOpener) waitForCalls(want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if o.callCount() >= want {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return o.callCount() >= want
}

type fakeDoubaoRealtimeSession struct {
	events           []*doubaospeech.RealtimeEvent
	beforeRecv       <-chan struct{}
	endASR           chan struct{}
	eventsDrained    chan<- struct{}
	blockAfterEvents <-chan struct{}
	interruptErr     error
	sendAudioErr     error
	sendAudioErrAt   int
	firstAudioSent   chan struct{}

	mu                sync.Mutex
	audio             [][]byte
	texts             []string
	endCount          int
	interrupts        int
	closed            bool
	closedCh          chan struct{}
	endOnce           sync.Once
	firstAudioOnce    sync.Once
	closeOnce         sync.Once
	eventsDrainedOnce sync.Once
}

func (s *fakeDoubaoRealtimeSession) SendAudio(ctx context.Context, audio []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audio = append(s.audio, append([]byte(nil), audio...))
	if s.firstAudioSent != nil {
		s.firstAudioOnce.Do(func() { close(s.firstAudioSent) })
	}
	if s.sendAudioErr != nil && len(s.audio) == s.sendAudioErrAt {
		return s.sendAudioErr
	}
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
	return s.interruptErr
}

func (s *fakeDoubaoRealtimeSession) Recv() iter.Seq2[*doubaospeech.RealtimeEvent, error] {
	return func(yield func(*doubaospeech.RealtimeEvent, error) bool) {
		closed := s.closedSignal()
		if s.beforeRecv != nil {
			select {
			case <-s.beforeRecv:
			case <-closed:
				return
			}
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
			select {
			case <-s.blockAfterEvents:
			case <-closed:
			}
		}
	}
}

func (s *fakeDoubaoRealtimeSession) Close() error {
	closed := s.closedSignal()
	s.closeOnce.Do(func() { close(closed) })
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *fakeDoubaoRealtimeSession) closedSignal() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closedCh == nil {
		s.closedCh = make(chan struct{})
	}
	return s.closedCh
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
