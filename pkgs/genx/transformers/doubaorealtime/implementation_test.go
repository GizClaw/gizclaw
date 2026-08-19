package doubaorealtime

import (
	"bytes"
	"context"
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
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTransformerAudioInputPassesPCMThrough(t *testing.T) {
	input := newDoubaoRealtimeAudioInput("pcm", 16000, 1, false)
	got, err := input.prepare(&genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}})
	if err != nil {
		t.Fatalf("prepare() error = %v", err)
	}
	if !bytes.Equal(got, []byte{1, 0, 2, 0}) {
		t.Fatalf("prepare() = %v", got)
	}
}

func TestTransformerAudioInputEncodesSpeechOpusSilence(t *testing.T) {
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

func TestTransformerAudioInputsRejectMIMEChange(t *testing.T) {
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

func TestTransformerStreamIDsSplitRealtimeTranscript(t *testing.T) {
	ids := newDoubaoRealtimeStreamIDs(ModeRealtime)
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

func TestTransformerConcurrentCallsOwnSessions(t *testing.T) {
	const calls = 8
	sessions := make([]*fakeTransformerSession, calls)
	results := make([]fakeTransformerOpenResult, calls)
	for i := range calls {
		sessions[i] = &fakeTransformerSession{blockAfterEvents: make(chan struct{})}
		results[i] = fakeTransformerOpenResult{session: sessions[i]}
	}
	opener := &fakeTransformerOpener{results: results}
	transformer := newTransformer(nil, withDoubaoRealtimeOpener(opener), withMode(ModeRealtime))

	cancels := make(chan context.CancelFunc, calls)
	inputs := make(chan *bufferStream, calls)
	errs := make(chan error, calls)
	var wg sync.WaitGroup
	for range calls {
		wg.Go(func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancels <- cancel
			input := newBufferStream(1)
			if _, err := transformer.Transform(ctx, input); err != nil {
				errs <- err
			}
			inputs <- input
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Transform() error = %v", err)
	}
	if !opener.waitForCalls(calls, 2*time.Second) {
		t.Fatalf("OpenSession calls = %d, want %d", opener.callCount(), calls)
	}
	close(inputs)
	for input := range inputs {
		_ = input.Close()
	}
	close(cancels)
	for cancel := range cancels {
		cancel()
	}
	for i, session := range sessions {
		if session == nil {
			t.Fatalf("session %d is nil", i)
		}
	}
}

func TestTransformerRealtimeUsesProviderKeepAliveMode(t *testing.T) {
	transformer := newTransformer(nil, withMode(ModeRealtime))

	if got := transformer.realtimeConfig().InputMode; got != doubaospeech.RealtimeInputModeKeepAlive {
		t.Fatalf("realtime input mode = %q, want %q", got, doubaospeech.RealtimeInputModeKeepAlive)
	}
}

func TestTransformerTextDeltaNormalizesPrefix(t *testing.T) {
	if got := realtimeTextDelta("你好，", "你好，世界"); got != "世界" {
		t.Fatalf("delta = %q, want 世界", got)
	}
	if got := realtimeTextDelta("Hello!", "hello world"); got != " world" {
		t.Fatalf("normalized delta = %q, want space-world suffix", got)
	}
}

func TestTransformerOutputAudioBlobsPassesPCM(t *testing.T) {
	tfr := newTransformer(nil, withFormat("pcm"))
	blobs, err := tfr.outputAudioBlobs([]byte{1, 2, 3})
	if err != nil {
		t.Fatalf("outputAudioBlobs() error = %v", err)
	}
	if len(blobs) != 1 || blobs[0].MIMEType != "audio/pcm" || !bytes.Equal(blobs[0].Data, []byte{1, 2, 3}) {
		t.Fatalf("outputAudioBlobs() = %#v", blobs)
	}
}

func TestTransformerConfigSetsRealtimeSession(t *testing.T) {
	tfr := newTransformer(nil,
		withMode(ModeText),
		withModel("O"),
		withSpeaker("voice-a"),
		withFormat("pcm"),
		withSampleRate(16000),
		withChannels(1),
		withSpeechRate(12),
		withLoudnessRate(6),
		withASRExtra(doubaospeech.RealtimeASRExtra{
			EndSmoothWindowMS: 800,
			EnableCustomVAD:   new(true),
			EnableASRTwopass:  new(true),
			Context: &doubaospeech.RealtimeASRContext{
				Hotwords:     []doubaospeech.RealtimeHotword{{Word: "GizClaw"}},
				CorrectWords: map[string]string{"吉斯克劳": "GizClaw"},
			},
		}),
		withTTSExtra(doubaospeech.RealtimeTTSExtra{
			ExplicitDialect: "sichuan",
			TTS20Model:      "expressive",
			AIGCMetadata: &doubaospeech.RealtimeAIGCMetadata{
				Enable:          new(true),
				ContentProducer: "gizclaw",
				ProduceID:       "produce-1",
			},
		}),
		withBotName("bot"),
		withInstructions("keep it brief"),
		withSystemRole("brief"),
		withSpeakingStyle("warm"),
		withCharacterManifest("manifest"),
		withDialogID("dialog-1"),
		withDialogExtra(doubaospeech.RealtimeDialogExtra{
			EnableVolcWebsearch:          new(true),
			VolcWebsearchType:            "web",
			VolcWebsearchResultCount:     3,
			VolcWebsearchNoResultMessage: "没有找到相关搜索结果。",
		}),
		withSearchAPIKey("search-key"),
	)
	if tfr.dialogID != "dialog-1" {
		t.Fatalf("dialogID = %q, want dialog-1", tfr.dialogID)
	}
	cfg := tfr.realtimeConfig()
	if cfg.InputMode != doubaospeech.RealtimeInputModeText || cfg.Model != doubaospeech.RealtimeModelVersion("O") {
		t.Fatalf("mode/model = %q/%q", cfg.InputMode, cfg.Model)
	}
	if cfg.Instructions != "keep it brief" {
		t.Fatalf("instructions = %q, want keep it brief", cfg.Instructions)
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

func TestTransformerPushToTalkEndsASR(t *testing.T) {
	endASR := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv: endASR,
		endASR:     endASR,
		events:     []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventSessionFinished}},
	}
	tfr := newTransformer(nil,
		withInputFormat("pcm"),
		withInputTranscode(false),
	)
	input := &sliceRealtimeStream{chunks: []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	}}
	output := newBufferStream(16)

	err := runTransformerProcessLoop(t, tfr, input, output, session)
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

func TestTransformerPushToTalkWaitsForAudioEOS(t *testing.T) {
	endASR := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv: endASR,
		endASR:     endASR,
		events:     []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventSessionFinished}},
	}
	tfr := newTransformer(nil,
		withInputFormat("pcm"),
		withInputTranscode(false),
	)
	input := &sliceRealtimeStream{chunks: []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	}}
	output := newBufferStream(16)

	if err := runTransformerProcessLoop(t, tfr, input, output, session); err != nil {
		t.Fatalf("processLoop() error = %v", err)
	}
	if got := session.endASRCount(); got != 1 {
		t.Fatalf("EndASR calls = %d, want 1", got)
	}
	if sent := session.audioFrames(); len(sent) != 2 {
		t.Fatalf("SendAudio calls = %d, want 2", len(sent))
	}
}

func TestTransformerPushToTalkRejectsInvalidInputTransitions(t *testing.T) {
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
			session := &fakeTransformerSession{
				endASR:           endASR,
				blockAfterEvents: make(chan struct{}),
			}
			tfr := newTransformer(nil,
				withInputFormat("pcm"),
				withInputTranscode(false),
			)
			output := newBufferStream(16)
			err := runTransformerProcessLoop(t, tfr, &sliceRealtimeStream{chunks: tt.chunks}, output, session)
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
	state.responseStarted("turn-2", true)
	if got := state.current(); got != doubaoPushToTalkResponding {
		t.Fatalf("phase after response = %v, want responding", got)
	}
	bargeIn, interrupted, err = state.begin("turn-3")
	if err != nil || !bargeIn || interrupted != "turn-2" {
		t.Fatalf("begin() while responding = (%v, %q, %v), want (true, turn-2, nil)", bargeIn, interrupted, err)
	}
}

func TestDoubaoPushToTalkStateWaitsForDeliveredAudio(t *testing.T) {
	state := &doubaoPushToTalkState{}
	if _, _, err := state.begin("turn-1"); err != nil {
		t.Fatal(err)
	}
	if err := state.end(); err != nil {
		t.Fatal(err)
	}
	state.responseStarted("turn-1", true)
	state.ttsFinished("turn-1")
	if got := state.current(); got != doubaoPushToTalkResponding {
		t.Fatalf("phase after provider TTS finish = %v, want responding until delivery", got)
	}
	bargeIn, interrupted, err := state.begin("turn-2")
	if err != nil || !bargeIn || interrupted != "turn-1" {
		t.Fatalf("begin() before delivery = (%v, %q, %v), want (true, turn-1, nil)", bargeIn, interrupted, err)
	}

	if err := state.end(); err != nil {
		t.Fatal(err)
	}
	state.responseStarted("turn-2", true)
	state.ttsFinished("turn-2")
	state.observeAssistantOutput(doubaoRealtimeAssistantLabel, &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-2", Label: doubaoRealtimeAssistantLabel, EndOfStream: true},
	})
	if got := state.current(); got != doubaoPushToTalkIdle {
		t.Fatalf("phase after delivered audio = %v, want idle", got)
	}
	bargeIn, interrupted, err = state.begin("turn-3")
	if err != nil || bargeIn || interrupted != "" {
		t.Fatalf("begin() after delivery = (%v, %q, %v), want (false, empty, nil)", bargeIn, interrupted, err)
	}
}

func TestDoubaoPushToTalkStateChatEndBeforeTTSRemainsInterruptible(t *testing.T) {
	state := &doubaoPushToTalkState{}
	if _, _, err := state.begin("turn-1"); err != nil {
		t.Fatal(err)
	}
	if err := state.end(); err != nil {
		t.Fatal(err)
	}
	state.responseStarted("turn-1", false)
	state.chatEnded("turn-1")
	state.responseStarted("turn-1", true)
	state.ttsFinished("turn-1")
	if got := state.current(); got != doubaoPushToTalkResponding {
		t.Fatalf("phase after ChatEnded-before-TTS = %v, want responding until delivery", got)
	}
	bargeIn, interrupted, err := state.begin("turn-2")
	if err != nil || !bargeIn || interrupted != "turn-1" {
		t.Fatalf("begin() before delivered TTS = (%v, %q, %v), want (true, turn-1, nil)", bargeIn, interrupted, err)
	}
}

func TestTransformerPTTTurnCommitsLatestHypothesisBeforeAssistantOutput(t *testing.T) {
	output := &recordingRealtimeOutput{}
	turn := &doubaoRealtimePTTTurn{}
	turn.begin(output, "turn-1", doubaoRealtimeAssistantLabel, doubaoRealtimePTTOutputLimit, 0)
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
	if len(chunks) != 4 {
		t.Fatalf("output chunks = %d, want transcript BOS, transcript, transcript EOS, assistant", len(chunks))
	}
	if !chunks[0].IsBeginOfStream() || chunks[0].Ctrl.Label != doubaoRealtimeTranscriptLabel {
		t.Fatalf("first chunk = %#v, want transcript BOS", chunks[0])
	}
	if text, ok := chunks[1].Part.(genx.Text); !ok || text != "final" {
		t.Fatalf("committed transcript = %#v, want final snapshot", chunks[1])
	}
	if chunks[2].Ctrl == nil || !chunks[2].Ctrl.EndOfStream || chunks[2].Ctrl.Label != doubaoRealtimeTranscriptLabel {
		t.Fatalf("third chunk = %#v, want transcript EOS", chunks[2])
	}
	if text, ok := chunks[3].Part.(genx.Text); !ok || text != "answer" {
		t.Fatalf("assistant output = %#v, want retained answer", chunks[3])
	}
}

func TestTransformerProviderLossDoesNotRepeatCommittedPTTTranscriptEOS(t *testing.T) {
	tfr := newTransformer(nil)
	runtime := newDoubaoRealtimeRuntime(tfr)
	defer runtime.close()
	output := &recordingRealtimeOutput{}
	runtime.pttTurn.begin(output, "turn-1", doubaoRealtimeAssistantLabel, doubaoRealtimePTTOutputLimit, 0)
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

func TestTransformerProviderLossClosesOnlyOpenAssistantRoutes(t *testing.T) {
	tfr := newTransformer(nil, withFormat("pcm"))
	runtime := newDoubaoRealtimeRuntime(tfr)
	defer runtime.close()
	output := &recordingRealtimeOutput{}
	epoch := runtime.assistant.currentEpoch()
	runtime.assistant.markPending("turn-1", epoch)
	runtime.assistant.markAudioDone(epoch)

	runtime.providerLost(tfr, output, errors.New("provider lost"))

	chunks := output.chunks()
	if len(chunks) != 2 {
		t.Fatalf("output chunks after provider loss = %#v, want text BOS and error EOS", chunks)
	}
	if !chunks[0].IsBeginOfStream() || chunks[0].IsEndOfStream() {
		t.Fatalf("provider-loss first chunk = %#v, want text BOS", chunks[0])
	}
	chunk := chunks[1]
	if chunk.Ctrl == nil || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error == "" {
		t.Fatalf("provider-loss chunk = %#v, want error EOS", chunk)
	}
	if _, ok := chunk.Part.(genx.Text); !ok {
		t.Fatalf("provider-loss chunk part = %T, want text route", chunk.Part)
	}
}

func TestTransformerRealtimeAudioIdleProviderErrorClosesEveryOpenedRoute(t *testing.T) {
	firstAudioSent := make(chan struct{})
	providerErr := errors.New("sami error: DialogAudioIdleTimeoutError (code=55000001)")
	session := &fakeTransformerSession{
		beforeRecv:     firstAudioSent,
		firstAudioSent: firstAudioSent,
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "question"},
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventChatResponse, Text: "answer"},
			{Type: doubaospeech.EventChatEnded},
			{Type: doubaospeech.EventTTSStarted, Text: "answer"},
		},
		recvErr: providerErr,
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModeRealtime),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withFormat("pcm"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	input := newBufferStream(4)
	defer input.Close()
	for _, chunk := range []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	} {
		if err := input.Push(chunk); err != nil {
			t.Fatalf("Push(input) error = %v", err)
		}
	}

	output, err := tfr.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	type routeKey struct {
		role     genx.Role
		mimeType string
	}
	routes := make(map[routeKey][]*genx.MessageChunk)
	for completed := 0; completed < 3; {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("Next() before all routes closed = %v; routes=%#v", nextErr, routes)
		}
		if chunk == nil || chunk.Ctrl == nil || chunk.Part == nil {
			continue
		}
		mimeType, ok := chunk.MIMEType()
		if !ok {
			continue
		}
		key := routeKey{role: chunk.Role, mimeType: mimeType}
		routes[key] = append(routes[key], chunk)
		if chunk.IsEndOfStream() {
			completed++
		}
	}

	transcript := routes[routeKey{role: genx.RoleUser, mimeType: "text/plain"}]
	assistantText := routes[routeKey{role: genx.RoleModel, mimeType: "text/plain"}]
	assistantAudio := routes[routeKey{role: genx.RoleModel, mimeType: "audio/pcm"}]
	for name, route := range map[string][]*genx.MessageChunk{
		"transcript":      transcript,
		"assistant text":  assistantText,
		"assistant audio": assistantAudio,
	} {
		if len(route) < 2 || !route[0].IsBeginOfStream() || !route[len(route)-1].IsEndOfStream() {
			t.Fatalf("%s route = %#v, want BOS...EOS", name, route)
		}
	}
	if got := transcript[len(transcript)-1].Ctrl.Error; got != "" {
		t.Fatalf("transcript EOS error = %q, want success", got)
	}
	for name, route := range map[string][]*genx.MessageChunk{
		"assistant text":  assistantText,
		"assistant audio": assistantAudio,
	} {
		if got := route[len(route)-1].Ctrl.Error; !strings.Contains(got, "DialogAudioIdleTimeoutError") {
			t.Fatalf("%s EOS error = %q, want DialogAudioIdleTimeoutError", name, got)
		}
	}
	if !hasRealtimeTestText(assistantText, genx.RoleModel, "answer") {
		t.Fatalf("assistant text route = %#v, want answer", assistantText)
	}
	if hasRealtimeTestBlob(assistantAudio, genx.RoleModel, "audio/pcm") {
		t.Fatalf("assistant audio route = %#v, want zero audio data before error EOS", assistantAudio)
	}
}

func TestTransformerRealtimeResponseIdleClosesEveryOpenedRoute(t *testing.T) {
	firstAudioSent := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       firstAudioSent,
		firstAudioSent:   firstAudioSent,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "question"},
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventChatResponse, Text: "answer"},
			{Type: doubaospeech.EventChatEnded},
			{Type: doubaospeech.EventTTSStarted, Text: "answer"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2}},
		},
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModeRealtime),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withFormat("pcm"),
		withResponseIdleTimeout(20*time.Millisecond),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	input := newBufferStream(4)
	defer input.Close()
	for _, chunk := range []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	} {
		if err := input.Push(chunk); err != nil {
			t.Fatalf("Push(input) error = %v", err)
		}
	}

	output, err := tfr.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	type routeKey struct {
		role     genx.Role
		mimeType string
	}
	routes := make(map[routeKey][]*genx.MessageChunk)
	for completed := 0; completed < 3; {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("Next() before all routes closed = %v; routes=%#v", nextErr, routes)
		}
		if chunk == nil || chunk.Ctrl == nil || chunk.Part == nil {
			continue
		}
		mimeType, ok := chunk.MIMEType()
		if !ok {
			continue
		}
		key := routeKey{role: chunk.Role, mimeType: mimeType}
		routes[key] = append(routes[key], chunk)
		if chunk.IsEndOfStream() {
			completed++
		}
	}

	for name, key := range map[string]routeKey{
		"assistant text":  {role: genx.RoleModel, mimeType: "text/plain"},
		"assistant audio": {role: genx.RoleModel, mimeType: "audio/pcm"},
	} {
		route := routes[key]
		if len(route) < 2 || !route[0].IsBeginOfStream() || !route[len(route)-1].IsEndOfStream() {
			t.Fatalf("%s route = %#v, want BOS...EOS", name, route)
		}
		if got := route[len(route)-1].Ctrl.Error; !strings.Contains(got, "response idle timeout") {
			t.Fatalf("%s EOS error = %q, want response idle timeout", name, got)
		}
	}
}

func TestTransformerPTTResponsesMatchQuestionAndReplyIDs(t *testing.T) {
	response := &doubaoRealtimePTTResponse{
		streamID: "turn-1",
		identity: doubaoRealtimePTTResponseIdentity{questionID: "q-1"},
	}
	responses := &doubaoRealtimePTTResponses{items: []*doubaoRealtimePTTResponse{response}}
	if got := responses.match(doubaoRealtimePTTResponseIdentity{questionID: "q-1", replyID: "r-1"}); got != response {
		t.Fatalf("match(question + reply) = %p, want %p", got, response)
	}
	if response.identity.replyID != "r-1" {
		t.Fatalf("bound reply ID = %q, want r-1", response.identity.replyID)
	}
	if got := responses.match(doubaoRealtimePTTResponseIdentity{questionID: "q-2", replyID: "r-2"}); got != nil {
		t.Fatalf("match(conflicting IDs) = %#v, want nil", got)
	}
}

func TestTransformerPTTLateTerminalEventKeepsOriginalTurnBinding(t *testing.T) {
	endASR := make(chan struct{})
	eventPaused := make(chan struct{})
	resumeEvents := make(chan struct{})
	eventsDrained := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       endASR,
		endASR:           endASR,
		eventsDrained:    eventsDrained,
		blockAfterEvents: make(chan struct{}),
		pauseBeforeEvent: 5,
		eventPaused:      eventPaused,
		resumeEvents:     resumeEvents,
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "first", QuestionID: "q-1"},
			{Type: doubaospeech.EventASREnded, QuestionID: "q-1"},
			{Type: doubaospeech.EventTTSStarted, QuestionID: "q-1", ReplyID: "r-1"},
			{Type: doubaospeech.EventChatResponse, Text: "first answer", ReplyID: "r-1"},
			{Type: doubaospeech.EventTTSFinished, ReplyID: "r-1"},
			{Type: doubaospeech.EventChatEnded, ReplyID: "r-1"},
			{Type: doubaospeech.EventASRResponse, Text: "second", QuestionID: "q-2"},
			{Type: doubaospeech.EventASREnded, QuestionID: "q-2"},
			{Type: doubaospeech.EventChatResponse, Text: "second answer", QuestionID: "q-2", ReplyID: "r-2"},
			{Type: doubaospeech.EventChatEnded, ReplyID: "r-2"},
			{Type: doubaospeech.EventTTSStarted, ReplyID: "r-2"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{2, 3}, ReplyID: "r-2"},
			{Type: doubaospeech.EventTTSFinished, ReplyID: "r-2"},
		},
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModePushToTalk),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withFormat("pcm"),
	)
	input := newBufferStream(16)
	output, err := tfr.transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for _, chunk := range []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	} {
		if err := input.Push(chunk); err != nil {
			t.Fatalf("Push(first turn) error = %v", err)
		}
	}
	select {
	case <-eventPaused:
	case <-time.After(2 * time.Second):
		t.Fatal("provider events did not pause before the first turn ChatEnded")
	}
	for _, chunk := range []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-2"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-2", EndOfStream: true}},
	} {
		if err := input.Push(chunk); err != nil {
			t.Fatalf("Push(second turn) error = %v", err)
		}
	}
	if !session.waitForEndASRCount(2, 2*time.Second) {
		t.Fatalf("EndASR calls = %d, want 2", session.endASRCount())
	}
	close(resumeEvents)
	select {
	case <-eventsDrained:
	case <-time.After(2 * time.Second):
		t.Fatal("provider events did not drain")
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input) error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	for _, streamID := range []string{"turn-1", "turn-2"} {
		count := 0
		for _, chunk := range chunks {
			if chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil ||
				chunk.Ctrl.StreamID != streamID || chunk.Ctrl.Label != doubaoRealtimeAssistantLabel || !chunk.Ctrl.EndOfStream {
				continue
			}
			if _, ok := chunk.Part.(genx.Text); ok {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("assistant text EOS count for %s = %d, want 1; chunks = %#v", streamID, count, chunks)
		}
	}
}

func TestTransformerPTTBargeInIgnoresDelayedASRTerminal(t *testing.T) {
	endASR := make(chan struct{})
	eventPaused := make(chan struct{})
	resumeEvents := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       endASR,
		endASR:           endASR,
		blockAfterEvents: make(chan struct{}),
		pauseBeforeEvent: 0,
		eventPaused:      eventPaused,
		resumeEvents:     resumeEvents,
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "old transcript", QuestionID: "q-1"},
			{Type: doubaospeech.EventASREnded, QuestionID: "q-1"},
			{Type: doubaospeech.EventASRResponse, Text: "new transcript", QuestionID: "q-2"},
			{Type: doubaospeech.EventASREnded, QuestionID: "q-2"},
			{Type: doubaospeech.EventChatResponse, Text: "new answer", QuestionID: "q-2", ReplyID: "r-2"},
			{Type: doubaospeech.EventChatEnded, ReplyID: "r-2"},
			{Type: doubaospeech.EventTTSStarted, ReplyID: "r-2"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{2, 3}, ReplyID: "r-2"},
			{Type: doubaospeech.EventTTSFinished, ReplyID: "r-2"},
		},
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModePushToTalk),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withFormat("pcm"),
	)
	input := newBufferStream(16)
	output, err := tfr.transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	pushTurn := func(streamID string, sample byte) {
		t.Helper()
		for _, chunk := range []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true}},
			{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{sample, 0}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}},
			{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
		} {
			if err := input.Push(chunk); err != nil {
				t.Fatalf("Push(%s) error = %v", streamID, err)
			}
		}
	}

	pushTurn("turn-1", 1)
	select {
	case <-eventPaused:
	case <-time.After(2 * time.Second):
		t.Fatal("provider events did not pause before the delayed first-turn ASR events")
	}
	pushTurn("turn-2", 2)
	if !session.waitForEndASRCount(2, 2*time.Second) {
		t.Fatalf("EndASR calls = %d, want 2", session.endASRCount())
	}
	close(resumeEvents)
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input) error = %v", err)
	}

	chunks := drainRealtimeTestOutput(t, output)
	for _, chunk := range chunks {
		text, ok := chunk.Part.(genx.Text)
		if !ok || chunk.Ctrl == nil {
			continue
		}
		if string(text) == "old transcript" {
			t.Fatalf("delayed first-turn transcript leaked into output: %#v", chunks)
		}
		if (string(text) == "new transcript" || string(text) == "new answer") && chunk.Ctrl.StreamID != "turn-2" {
			t.Fatalf("new-turn text %q used StreamID %q, want turn-2; chunks = %#v", text, chunk.Ctrl.StreamID, chunks)
		}
	}
	if !hasRealtimeTestText(chunks, genx.RoleUser, "new transcript") ||
		!hasRealtimeTestText(chunks, genx.RoleModel, "new answer") {
		t.Fatalf("new turn did not complete normally: %#v", chunks)
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
	below := newRealtimePTTOutputGate(belowOutput, "turn-below", doubaoRealtimeAssistantLabel, 2*packetDuration, 0)
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
	gate := newRealtimePTTOutputGate(output, "turn-1", doubaoRealtimeAssistantLabel, 2*packetDuration, 0)
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
	next := newRealtimePTTOutputGate(nextOutput, "turn-2", doubaoRealtimeAssistantLabel, 2*packetDuration, 0)
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

func TestRealtimePTTOutputGateEnforcesByteLimitForNonOpus(t *testing.T) {
	if got, want := realtimePTTOutputByteLimit(2*time.Minute, 24000, 1), int64(5_760_000); got != want {
		t.Fatalf("default PCM byte limit = %d, want %d", got, want)
	}
	maxInt := int(^uint(0) >> 1)
	if got := realtimePTTOutputByteLimit(2*time.Minute, maxInt, maxInt); got != doubaoRealtimePTTOutputMaxBytes {
		t.Fatalf("oversized format byte limit = %d, want hard cap %d", got, doubaoRealtimePTTOutputMaxBytes)
	}
	for _, mimeType := range []string{"audio/pcm", "audio/mpeg"} {
		t.Run(mimeType, func(t *testing.T) {
			output := &recordingRealtimeOutput{}
			gate := newRealtimePTTOutputGate(output, "turn-1", doubaoRealtimeAssistantLabel, time.Hour, 4)
			chunk := func(data []byte) *genx.MessageChunk {
				return &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: mimeType, Data: data},
					Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: doubaoRealtimeAssistantLabel},
				}
			}
			if err := gate.Push(chunk([]byte{1, 2, 3, 4})); err != nil {
				t.Fatalf("exact-limit Push() error = %v", err)
			}
			if err := gate.Push(chunk([]byte{5})); !errors.Is(err, errRealtimePTTOutputLimit) {
				t.Fatalf("over-limit Push() error = %v, want output limit", err)
			}
			chunks := output.chunks()
			if len(chunks) != 1 || chunks[0].Ctrl == nil || !chunks[0].Ctrl.EndOfStream || chunks[0].Ctrl.Error == "" {
				t.Fatalf("limit output = %#v, want one error EOS", chunks)
			}
		})
	}
}

func TestTransformerEOSIsLocalInRealtimeMode(t *testing.T) {
	session := &fakeTransformerSession{blockAfterEvents: make(chan struct{})}
	tfr := newTransformer(nil,
		withMode(ModeRealtime),
		withInputFormat("pcm"),
		withInputTranscode(false),
	)
	input := &sliceRealtimeStream{chunks: []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	}}
	if err := runTransformerProcessLoop(t, tfr, input, newBufferStream(8), session); err != nil {
		t.Fatalf("processLoop() error = %v", err)
	}
	if got := session.endASRCount(); got != 0 {
		t.Fatalf("EndASR calls = %d, want 0", got)
	}
	if got := len(session.audioFrames()); got != 2 {
		t.Fatalf("SendAudio calls = %d, want data from MIME BOS and EOS", got)
	}
}

func TestTransformerInputBoundariesRejectMIMEChange(t *testing.T) {
	for _, test := range []struct {
		name   string
		chunks []*genx.MessageChunk
	}{
		{
			name: "data differs from empty BOS declaration",
			chunks: []*genx.MessageChunk{
				{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
				{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
				{Part: &genx.Blob{MIMEType: "audio/mpeg", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
			},
		},
		{
			name: "empty EOS differs from data declaration",
			chunks: []*genx.MessageChunk{
				{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
				{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
				{Part: &genx.Blob{MIMEType: "audio/mpeg"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := &fakeTransformerSession{blockAfterEvents: make(chan struct{})}
			tfr := newTransformer(nil,
				withMode(ModeRealtime),
				withInputFormat("pcm"),
				withInputTranscode(false),
			)
			err := runTransformerProcessLoop(t, tfr, &sliceRealtimeStream{chunks: test.chunks}, newBufferStream(8), session)
			var mimeErr *doubaoRealtimeStreamMIMEChangeError
			if !errors.As(err, &mimeErr) {
				t.Fatalf("processLoop() error = %T %v, want MIME change", err, err)
			}
		})
	}
}

func TestTransformerASRInfoHandsRealtimeInterruptionToReplacementWithoutProviderInterrupt(t *testing.T) {
	allowEOF := make(chan struct{})
	session := &fakeTransformerSession{
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventASRInfo},
		},
		blockAfterEvents: make(chan struct{}),
	}
	tfr := newTransformer(nil, withMode(ModeRealtime))
	input := &gatedRealtimeStream{gate: allowEOF}
	output := newBufferStream(8)
	errCh := make(chan error, 1)
	go func() { errCh <- runTransformerProcessLoop(t, tfr, input, output, session) }()
	select {
	case err := <-errCh:
		close(allowEOF)
		if err != nil {
			t.Fatalf("processLoop() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		close(allowEOF)
		t.Fatal("realtime interruption was not handed off")
	}
	if got := session.interruptCount(); got != 0 {
		t.Fatalf("Interrupt calls = %d, want 0 outside push-to-talk", got)
	}
}

func TestTransformerRealtimeBoundsStalledPartialTranscript(t *testing.T) {
	firstAudioSent := make(chan struct{})
	eventsDrained := make(chan struct{})
	events := []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventASRResponse, Text: "partial transcript"}}
	for range 50 {
		events = append(events, &doubaospeech.RealtimeEvent{Type: doubaospeech.EventUsageResponse})
	}
	session := &fakeTransformerSession{
		beforeRecv:       firstAudioSent,
		firstAudioSent:   firstAudioSent,
		events:           events,
		eventInterval:    5 * time.Millisecond,
		eventsDrained:    eventsDrained,
		blockAfterEvents: make(chan struct{}),
	}
	tfr := newTransformer(nil,
		withMode(ModeRealtime),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withResponseIdleTimeout(20*time.Millisecond),
	)
	input := newBufferStream(4)
	for _, chunk := range []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	} {
		if err := input.Push(chunk); err != nil {
			t.Fatalf("Push(input) error = %v", err)
		}
	}
	output := newBufferStream(8)
	runtime := newDoubaoRealtimeRuntime(tfr)
	defer runtime.close()
	reader := newDoubaoRealtimeInputReader(input)
	defer reader.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := tfr.processSession(ctx, reader, output, session, runtime)
	if !isDoubaoRealtimeRecoverable(err) || !strings.Contains(err.Error(), "response idle timeout") {
		t.Fatalf("processSession() error = %v, want recoverable response idle timeout", err)
	}
	if !session.isClosed() {
		t.Fatal("stalled provider session was not closed")
	}
	select {
	case <-eventsDrained:
		t.Fatal("non-progress usage events kept the response idle deadline alive")
	default:
	}
	if err := output.Close(); err != nil {
		t.Fatalf("Close(output) error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if !hasRealtimeTestText(chunks, genx.RoleUser, "partial transcript") {
		t.Fatalf("output missing partial transcript: %#v", chunks)
	}
	var transcriptEOS *genx.MessageChunk
	for _, chunk := range chunks {
		if chunk != nil && chunk.Role == genx.RoleUser && chunk.Ctrl != nil &&
			chunk.Ctrl.Label == doubaoRealtimeTranscriptLabel && chunk.Ctrl.EndOfStream {
			transcriptEOS = chunk
			break
		}
	}
	if transcriptEOS == nil || !strings.Contains(transcriptEOS.Ctrl.Error, "response idle timeout") {
		t.Fatalf("transcript EOS = %#v, want response idle timeout", transcriptEOS)
	}
}

func TestTransformerSessionLoopRetriesAndReusesDialogID(t *testing.T) {
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{
		{err: errors.New("connect-1")},
		{err: errors.New("connect-2")},
		{session: &fakeTransformerSession{blockAfterEvents: make(chan struct{})}},
	}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withDialogID("dialog-1"),
	)
	tfr.retryInitial = time.Millisecond
	tfr.retryMax = 2 * time.Millisecond
	input := newBlockingRealtimeStream()
	output, err := tfr.transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if !opener.waitForCalls(3, 2*time.Second) {
		t.Fatalf("OpenSession calls = %d, want two retries then one session", opener.callCount())
	}
	if err := input.CloseWithError(io.EOF); err != nil {
		t.Fatalf("CloseWithError(input) error = %v", err)
	}
	if chunks := drainRealtimeTestOutput(t, output); len(chunks) != 0 {
		t.Fatalf("output = %#v, want none", chunks)
	}
	for i, dialogID := range opener.dialogIDs() {
		if dialogID != "dialog-1" {
			t.Fatalf("OpenSession call %d dialog ID = %q, want dialog-1", i+1, dialogID)
		}
	}
}

func TestTransformerSessionLoopStopsRetryWhenInputEnds(t *testing.T) {
	allowEOF := make(chan struct{})
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{
		{err: errors.New("connect failed")},
	}}
	tfr := newTransformer(nil, withDoubaoRealtimeOpener(opener))
	tfr.retryInitial = time.Hour
	tfr.retryMax = time.Hour
	output, err := tfr.transform(context.Background(), &gatedRealtimeStream{gate: allowEOF})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if !opener.waitForCalls(1, 2*time.Second) {
		t.Fatalf("OpenSession calls = %d, want initial attempt", opener.callCount())
	}
	close(allowEOF)
	if chunks := drainRealtimeTestOutput(t, output); len(chunks) != 0 {
		t.Fatalf("output = %#v, want none", chunks)
	}
	if got := opener.callCount(); got != 1 {
		t.Fatalf("OpenSession calls = %d, want no retry after input EOF", got)
	}
}

func TestTransformerSessionLoopReplacesFinishedSession(t *testing.T) {
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{
		{session: &fakeTransformerSession{events: []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventSessionFinished}}}},
		{session: &fakeTransformerSession{blockAfterEvents: make(chan struct{})}},
	}}
	tfr := newTransformer(nil, withDoubaoRealtimeOpener(opener))
	input := newBlockingRealtimeStream()
	output, err := tfr.transform(context.Background(), input)
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

func TestTransformerSessionLoopResetsBackoffAfterSuccessfulSession(t *testing.T) {
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{
		{session: &fakeTransformerSession{events: []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventSessionFinished}}}},
		{session: &fakeTransformerSession{events: []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventSessionFinished}}}},
		{session: &fakeTransformerSession{blockAfterEvents: make(chan struct{})}},
	}}
	tfr := newTransformer(nil, withDoubaoRealtimeOpener(opener))
	tfr.retryInitial = 10 * time.Millisecond
	tfr.retryMax = 20 * time.Millisecond
	var mu sync.Mutex
	var delays []time.Duration
	tfr.retryWait = func(_ context.Context, _ <-chan struct{}, delay time.Duration) bool {
		mu.Lock()
		delays = append(delays, delay)
		mu.Unlock()
		return true
	}
	input := newBlockingRealtimeStream()
	output, err := tfr.transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if !opener.waitForCalls(3, 2*time.Second) {
		t.Fatalf("OpenSession calls = %d, want 3", opener.callCount())
	}
	mu.Lock()
	gotDelays := append([]time.Duration(nil), delays...)
	mu.Unlock()
	if !slices.Equal(gotDelays, []time.Duration{10 * time.Millisecond, 10 * time.Millisecond}) {
		t.Fatalf("retry delays = %v, want [10ms 10ms]", gotDelays)
	}
	if err := input.CloseWithError(io.EOF); err != nil {
		t.Fatalf("CloseWithError(input) error = %v", err)
	}
	if chunks := drainRealtimeTestOutput(t, output); len(chunks) != 0 {
		t.Fatalf("output = %#v, want none", chunks)
	}
}

func TestTransformerTextDrainsFinalResponseAfterInputEOF(t *testing.T) {
	textSent := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       textSent,
		firstTextSent:    textSent,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventChatResponse, Text: "answer"},
			{Type: doubaospeech.EventChatEnded},
			{Type: doubaospeech.EventTTSStarted},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2}},
			{Type: doubaospeech.EventTTSFinished},
		},
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModeText),
		withFormat("pcm"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, &sliceRealtimeStream{chunks: []*genx.MessageChunk{{Part: genx.Text("question")}}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if !hasRealtimeTestText(chunks, genx.RoleModel, "answer") {
		t.Fatalf("output missing final assistant text: %#v", chunks)
	}
	if !hasRealtimeTestBlob(chunks, genx.RoleModel, "audio/pcm") {
		t.Fatalf("output missing final assistant audio: %#v", chunks)
	}
}

func TestTransformerTextPublishesTTSCanonicalTextWithSingleAudioRoute(t *testing.T) {
	textSent := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       textSent,
		firstTextSent:    textSent,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventChatResponse, Text: "chat duplicate"},
			{Type: doubaospeech.EventTTSStarted, Text: "first sentence"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2}},
			{Type: doubaospeech.EventTTSStarted, Text: "second sentence"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{3, 4}},
			{Type: doubaospeech.EventChatEnded},
			{Type: doubaospeech.EventTTSFinished},
		},
	}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(&fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}),
		withMode(ModeText),
		withFormat("pcm"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, &sliceRealtimeStream{chunks: []*genx.MessageChunk{{Part: genx.Text("question")}}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	var texts []string
	var audioBOS, audioEOS, textEOS int
	var terminalOrder []string
	streamID := ""
	for _, chunk := range chunks {
		if chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil || chunk.Ctrl.Label != doubaoRealtimeAssistantLabel {
			continue
		}
		if streamID == "" {
			streamID = chunk.Ctrl.StreamID
		} else if chunk.Ctrl.StreamID != streamID {
			t.Fatalf("assistant StreamID = %q, want %q", chunk.Ctrl.StreamID, streamID)
		}
		switch part := chunk.Part.(type) {
		case genx.Text:
			if part != "" {
				texts = append(texts, string(part))
			}
			if chunk.Ctrl.EndOfStream {
				textEOS++
				terminalOrder = append(terminalOrder, "text")
			}
		case *genx.Blob:
			if chunk.Ctrl.BeginOfStream {
				audioBOS++
			}
			if chunk.Ctrl.EndOfStream {
				audioEOS++
				terminalOrder = append(terminalOrder, "audio")
			}
		}
	}
	if !slices.Equal(texts, []string{"first sentence", "second sentence"}) {
		t.Fatalf("assistant texts = %v, want only TTS source", texts)
	}
	if audioBOS != 1 || audioEOS != 1 || textEOS != 1 {
		t.Fatalf("route terminals: audio BOS/EOS=%d/%d text EOS=%d, want 1/1/1", audioBOS, audioEOS, textEOS)
	}
	if !slices.Equal(terminalOrder, []string{"text", "audio"}) {
		t.Fatalf("route terminal order = %v, want text before audio", terminalOrder)
	}
}

func TestTransformerTextProviderLossClosesPartialResponseRoutes(t *testing.T) {
	textSent := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:    textSent,
		firstTextSent: textSent,
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventChatResponse, Text: "partial answer"},
			{Type: doubaospeech.EventTTSStarted},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2}},
		},
	}
	tfr := newTransformer(nil,
		withMode(ModeText),
		withFormat("pcm"),
	)
	runtime := newDoubaoRealtimeRuntime(tfr)
	defer runtime.close()
	output := newBufferStream(16)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := tfr.processSession(
		ctx,
		&sliceRealtimeStream{chunks: []*genx.MessageChunk{{Part: genx.Text("question")}}},
		output,
		session,
		runtime,
	)
	if !isDoubaoRealtimeRecoverable(err) {
		t.Fatalf("processSession() error = %v, want recoverable provider loss", err)
	}
	runtime.providerLost(tfr, output, errors.New("provider lost"))
	if err := output.Close(); err != nil {
		t.Fatalf("Close(output) error = %v", err)
	}

	chunks := drainRealtimeTestOutput(t, output)
	if hasRealtimeTestText(chunks, genx.RoleModel, "partial answer") {
		t.Fatalf("provider loss flushed buffered unspoken chat text: %#v", chunks)
	}
	if !hasRealtimeTestBlob(chunks, genx.RoleModel, "audio/pcm") {
		t.Fatalf("output missing partial assistant audio: %#v", chunks)
	}
	var textClosed, audioClosed bool
	for _, chunk := range chunks {
		if chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil ||
			chunk.Ctrl.StreamID != "audio" || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != "provider lost" {
			continue
		}
		switch chunk.Part.(type) {
		case genx.Text:
			textClosed = true
		case *genx.Blob:
			audioClosed = true
		}
	}
	if !textClosed || !audioClosed {
		t.Fatalf("provider loss did not close text and audio routes: %#v", chunks)
	}
}

func TestTransformerTextReplacementSessionRestoresOutputAcceptance(t *testing.T) {
	firstTextSent := make(chan struct{})
	first := &fakeTransformerSession{
		beforeRecv:    firstTextSent,
		firstTextSent: firstTextSent,
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventTTSStarted},
		},
	}
	secondTextSent := make(chan struct{})
	second := &fakeTransformerSession{
		beforeRecv:       secondTextSent,
		firstTextSent:    secondTextSent,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventChatResponse, Text: "replacement answer"},
			{Type: doubaospeech.EventChatEnded},
			{Type: doubaospeech.EventTTSStarted},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2}},
			{Type: doubaospeech.EventTTSFinished},
		},
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: first}, {session: second}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModeText),
		withFormat("pcm"),
	)
	tfr.retryWait = func(context.Context, <-chan struct{}, time.Duration) bool { return true }
	input := newBufferStream(4)
	if err := input.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}}); err != nil {
		t.Fatalf("Push(BOS) error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: genx.Text("first")}); err != nil {
		t.Fatalf("Push(first text) error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if !opener.waitForCalls(2, 2*time.Second) {
		t.Fatalf("OpenSession calls = %d, want replacement session", opener.callCount())
	}
	if err := input.Push(&genx.MessageChunk{Part: genx.Text("second")}); err != nil {
		t.Fatalf("Push(second text) error = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input) error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if !hasRealtimeTestText(chunks, genx.RoleModel, "replacement answer") {
		t.Fatalf("output missing replacement assistant text: %#v", chunks)
	}
	if !hasRealtimeTestBlob(chunks, genx.RoleModel, "audio/pcm") {
		t.Fatalf("output missing replacement assistant audio: %#v", chunks)
	}
}

func TestTransformerPTTDrainsFinalResponseAfterInputEOF(t *testing.T) {
	endASR := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       endASR,
		endASR:           endASR,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "question", QuestionID: "q-1"},
			{Type: doubaospeech.EventASREnded, QuestionID: "q-1"},
			{Type: doubaospeech.EventTTSStarted, QuestionID: "q-1", ReplyID: "r-1"},
			{Type: doubaospeech.EventChatResponse, Text: "answer", ReplyID: "r-1"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2}, ReplyID: "r-1"},
			{Type: doubaospeech.EventTTSFinished, ReplyID: "r-1"},
			{Type: doubaospeech.EventChatEnded, ReplyID: "r-1"},
		},
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModePushToTalk),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withFormat("pcm"),
	)
	input := &sliceRealtimeStream{chunks: []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if !hasRealtimeTestText(chunks, genx.RoleUser, "question") {
		t.Fatalf("output missing final transcript: %#v", chunks)
	}
	if !hasRealtimeTestText(chunks, genx.RoleModel, "answer") {
		t.Fatalf("output missing final assistant text: %#v", chunks)
	}
	if !hasRealtimeTestBlob(chunks, genx.RoleModel, "audio/pcm") {
		t.Fatalf("output missing final assistant audio: %#v", chunks)
	}
	if got := session.audioCount(); got != 2 {
		t.Fatalf("SendAudio calls = %d, want data from MIME BOS and EOS; audio=%v", got, session.audioChunks())
	}
	if got := session.endASRCount(); got != 1 {
		t.Fatalf("EndASR calls = %d, want one despite following route EOS", got)
	}
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleUser, genx.HistoryUserAudioLabel, 1)
}

func TestTransformerDoesNotReplayAmbiguousTextAfterReconnect(t *testing.T) {
	first := &fakeTransformerSession{
		blockAfterEvents: make(chan struct{}),
		sendTextErr:      errors.New("write failed after handoff"),
		sendTextErrAt:    1,
	}
	secondTextSent := make(chan struct{})
	second := &fakeTransformerSession{
		beforeRecv:       secondTextSent,
		firstTextSent:    secondTextSent,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventChatResponse, Text: "second answer"},
			{Type: doubaospeech.EventChatEnded},
			{Type: doubaospeech.EventTTSStarted},
			{Type: doubaospeech.EventTTSFinished},
		},
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: first}, {session: second}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModeText),
	)
	tfr.retryWait = func(context.Context, <-chan struct{}, time.Duration) bool { return true }
	input := newBufferStream(4)
	if err := input.Push(&genx.MessageChunk{Part: genx.Text("first")}); err != nil {
		t.Fatalf("Push(first text) error = %v", err)
	}
	output, err := tfr.transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if !opener.waitForCalls(2, 2*time.Second) {
		t.Fatalf("OpenSession calls = %d, want replacement session", opener.callCount())
	}
	bufferedOutput, ok := output.(*bufferStream)
	if !ok {
		t.Fatalf("output type = %T, want *bufferStream", output)
	}
	select {
	case <-bufferedOutput.Done():
		t.Fatal("output closed before unread text was supplied")
	case <-time.After(20 * time.Millisecond):
	}
	if err := input.Push(&genx.MessageChunk{Part: genx.Text("second")}); err != nil {
		t.Fatalf("Push(second text) error = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input) error = %v", err)
	}
	if chunks := drainRealtimeTestOutput(t, output); !hasRealtimeTestText(chunks, genx.RoleModel, "second answer") {
		t.Fatalf("output missing replacement response: %#v", chunks)
	}
	if got := first.textMessages(); !slices.Equal(got, []string{"first"}) {
		t.Fatalf("first session text = %v, want [first]", got)
	}
	if got := second.textMessages(); !slices.Equal(got, []string{"second"}) {
		t.Fatalf("replacement session text = %v, want [second]", got)
	}
}

func TestTransformerInterruptDiscardKeepsUserTranscript(t *testing.T) {
	streamID := "turn"
	transcript := &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "transcript"}}
	assistant := &genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel}}
	if isDoubaoRealtimeAssistantChunk(transcript, streamID) {
		t.Fatal("user transcript matched assistant discard")
	}
	if !isDoubaoRealtimeAssistantChunk(assistant, streamID) {
		t.Fatal("assistant audio did not match assistant discard")
	}
}

func TestTransformerSessionLoopStopsRetryOnContextCancellation(t *testing.T) {
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{
		{err: errors.New("connect-1")},
		{err: errors.New("connect-2")},
		{err: errors.New("connect-3")},
	}}
	tfr := newTransformer(nil, withDoubaoRealtimeOpener(opener))
	tfr.retryInitial = time.Millisecond
	tfr.retryMax = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	output, err := tfr.transform(ctx, newBlockingRealtimeStream())
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

func TestTransformerDoesNotReplayAmbiguousAudioAfterReconnect(t *testing.T) {
	first := &fakeTransformerSession{
		blockAfterEvents: make(chan struct{}),
		sendAudioErr:     errors.New("write failed after handoff"),
		sendAudioErrAt:   1,
	}
	second := &fakeTransformerSession{blockAfterEvents: make(chan struct{})}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: first}, {session: second}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModeRealtime),
		withInputFormat("pcm"),
		withInputTranscode(false),
	)
	input := &sliceRealtimeStream{chunks: []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
	}}
	output, err := tfr.transform(context.Background(), input)
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

func TestTransformerRealtimeInterruptHandsUnreadAudioToReplacementSession(t *testing.T) {
	firstAudioSent := make(chan struct{})
	firstEventsDrained := make(chan struct{})
	first := &fakeTransformerSession{
		beforeRecv:       firstAudioSent,
		firstAudioSent:   firstAudioSent,
		eventsDrained:    firstEventsDrained,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "first transcript"},
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventChatResponse, Text: "first answer"},
		},
	}
	secondAudioSent := make(chan struct{})
	secondEventsDrained := make(chan struct{})
	second := &fakeTransformerSession{
		beforeRecv:       secondAudioSent,
		firstAudioSent:   secondAudioSent,
		eventsDrained:    secondEventsDrained,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "second transcript"},
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventChatResponse, Text: "second answer"},
			{Type: doubaospeech.EventChatEnded},
			{Type: doubaospeech.EventTTSStarted},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{2, 3}},
			{Type: doubaospeech.EventTTSFinished},
		},
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: first}, {session: second}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModeRealtime),
		withDialogID("dialog-1"),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withFormat("pcm"),
	)
	input := newBufferStream(8)
	output, err := tfr.transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	push := func(chunk *genx.MessageChunk) {
		t.Helper()
		if err := input.Push(chunk); err != nil {
			t.Fatalf("Push() error = %v", err)
		}
	}
	push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}})
	push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}})
	push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}})
	select {
	case <-firstEventsDrained:
	case <-time.After(2 * time.Second):
		t.Fatal("first provider response did not start")
	}

	push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}})
	push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-2"}})
	push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-2", EndOfStream: true}})
	select {
	case <-secondEventsDrained:
	case <-time.After(2 * time.Second):
		t.Fatalf("replacement provider response did not complete; opens = %d", opener.callCount())
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input) error = %v", err)
	}

	chunks := drainRealtimeTestOutput(t, output)
	if got := first.interruptCount(); got != 0 {
		t.Fatalf("first Interrupt calls = %d, want 0 outside push-to-talk", got)
	}
	if got := opener.dialogIDs(); !slices.Equal(got, []string{"dialog-1", "dialog-1"}) {
		t.Fatalf("provider dialog IDs = %v, want stable dialog-1", got)
	}
	if got := first.audioFrames(); len(got) != 1 || !bytes.Equal(got[0], []byte{1, 0}) {
		t.Fatalf("first session audio = %v, want only first turn", got)
	}
	if got := second.audioFrames(); len(got) != 1 || !bytes.Equal(got[0], []byte{2, 0}) {
		t.Fatalf("replacement session audio = %v, want only unread second turn", got)
	}
	if !hasRealtimeInterruptedEOS(chunks, "turn-1:rt:1", genx.RoleModel, false) ||
		!hasRealtimeInterruptedEOS(chunks, "turn-1:rt:1", genx.RoleModel, true) {
		t.Fatalf("missing interrupted first response EOS: %#v", chunks)
	}
	if !hasRealtimeTestText(chunks, genx.RoleUser, "second transcript") ||
		!hasRealtimeTestText(chunks, genx.RoleModel, "second answer") ||
		!hasRealtimeTestBlob(chunks, genx.RoleModel, "audio/pcm") {
		t.Fatalf("replacement response did not complete: %#v", chunks)
	}
}

func TestTransformerRealtimeClosesInterruptedTurnsBeforeReplacementBOS(t *testing.T) {
	newSession := func(answer string, final bool) (*fakeTransformerSession, <-chan struct{}) {
		audioSent := make(chan struct{})
		eventsDrained := make(chan struct{})
		events := []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: answer + " transcript"},
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventChatResponse, Text: answer},
		}
		if final {
			events = append(events,
				&doubaospeech.RealtimeEvent{Type: doubaospeech.EventChatEnded},
				&doubaospeech.RealtimeEvent{Type: doubaospeech.EventTTSStarted},
				&doubaospeech.RealtimeEvent{Type: doubaospeech.EventTTSAudioData, Audio: []byte(answer)},
				&doubaospeech.RealtimeEvent{Type: doubaospeech.EventTTSFinished},
			)
		}
		return &fakeTransformerSession{
			beforeRecv:       audioSent,
			firstAudioSent:   audioSent,
			eventsDrained:    eventsDrained,
			blockAfterEvents: make(chan struct{}),
			events:           events,
		}, eventsDrained
	}
	first, firstDrained := newSession("first answer", false)
	second, secondDrained := newSession("second answer", false)
	third, thirdDrained := newSession("third answer", true)
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: first}, {session: second}, {session: third}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModeRealtime),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withFormat("pcm"),
	)
	input := newBufferStream(12)
	output, err := tfr.transform(t.Context(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	pushTurn := func(turn int) {
		t.Helper()
		streamID := fmt.Sprintf("turn-%d", turn)
		for _, chunk := range []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true}},
			{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{byte(turn), 0}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}},
			{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
		} {
			if err := input.Push(chunk); err != nil {
				t.Fatalf("Push(turn %d) error = %v", turn, err)
			}
		}
	}
	waitDrained := func(turn int, done <-chan struct{}) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("turn %d provider response did not start", turn)
		}
	}

	pushTurn(1)
	waitDrained(1, firstDrained)
	pushTurn(2)
	waitDrained(2, secondDrained)
	pushTurn(3)
	waitDrained(3, thirdDrained)
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input) error = %v", err)
	}

	chunks := drainRealtimeTestOutput(t, output)
	requireRealtimeAssistantHandoffOrder(t, chunks, "turn-1:rt:1", "turn-2:rt:1")
	requireRealtimeAssistantHandoffOrder(t, chunks, "turn-2:rt:1", "turn-3:rt:1")
}

func requireRealtimeAssistantHandoffOrder(t *testing.T, chunks []*genx.MessageChunk, previousID, nextID string) {
	t.Helper()
	previousTextEOS, previousAudioEOS, nextBOS := -1, -1, -1
	for index, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil || chunk.Role != genx.RoleModel || chunk.Ctrl.Label != doubaoRealtimeAssistantLabel {
			continue
		}
		if chunk.Ctrl.StreamID == previousID && chunk.IsEndOfStream() && chunk.Ctrl.Error == doubaoRealtimeInterrupted {
			switch chunk.Part.(type) {
			case genx.Text:
				previousTextEOS = index
			case *genx.Blob:
				previousAudioEOS = index
			}
		}
		if chunk.Ctrl.StreamID == nextID && chunk.IsBeginOfStream() && (nextBOS < 0 || index < nextBOS) {
			nextBOS = index
		}
	}
	if previousTextEOS < 0 || previousAudioEOS < 0 || nextBOS <= previousTextEOS || nextBOS <= previousAudioEOS {
		t.Fatalf("assistant handoff %q -> %q indices: text EOS=%d audio EOS=%d next BOS=%d; chunks=%#v", previousID, nextID, previousTextEOS, previousAudioEOS, nextBOS, chunks)
	}
}

func TestTransformerPTTDiscardsFailedTurnRemainderAfterReconnect(t *testing.T) {
	first := &fakeTransformerSession{
		blockAfterEvents: make(chan struct{}),
		sendAudioErr:     errors.New("provider lost"),
		sendAudioErrAt:   1,
	}
	secondEndASR := make(chan struct{})
	second := &fakeTransformerSession{
		beforeRecv:       secondEndASR,
		endASR:           secondEndASR,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventChatResponse, Text: "second answer"},
			{Type: doubaospeech.EventChatEnded},
			{Type: doubaospeech.EventTTSStarted},
			{Type: doubaospeech.EventTTSFinished},
		},
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: first}, {session: second}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModePushToTalk),
		withInputFormat("pcm"),
		withInputTranscode(false),
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
	output, err := tfr.transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleUser, genx.HistoryUserAudioLabel, 2)
	if got := second.audioFrames(); len(got) != 1 || !bytes.Equal(got[0], []byte{3, 0}) {
		t.Fatalf("replacement session audio = %v, want only next turn frame", got)
	}
	if got := second.endASRCount(); got != 1 {
		t.Fatalf("replacement EndASR calls = %d, want only next turn", got)
	}
}

func TestTransformerMapsRealtimeEventsToStreamChunks(t *testing.T) {
	session := &fakeTransformerSession{
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
	tfr := newTransformer(nil,
		withMode(ModeRealtime),
		withFormat("pcm"),
	)
	output := newBufferStream(16)

	err := runTransformerProcessLoop(t, tfr, &sliceRealtimeStream{}, output, session)
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
	if len(routes) != 3 {
		t.Fatalf("generated routes = %d, want transcript text, assistant text, and assistant audio: %#v", len(routes), chunks)
	}
	for key, route := range routes {
		if len(route) < 2 || !route[0].IsBeginOfStream() || !route[len(route)-1].IsEndOfStream() {
			t.Fatalf("route %q lifecycle = %#v, want BOS...EOS", key, route)
		}
		for _, chunk := range route[:len(route)-1] {
			if chunk.IsEndOfStream() {
				t.Fatalf("route %q ended before its final chunk: %#v", key, route)
			}
		}
	}
}

func TestTransformerInterruptsPendingResponseBeforeTTS(t *testing.T) {
	eventsDrained := make(chan struct{})
	releaseEvents := make(chan struct{})
	allowNextInput := make(chan struct{})
	firstAudioSent := make(chan struct{})
	session := &fakeTransformerSession{
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "第一段"},
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventTTSStarted},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{9, 8, 7}},
		},
		beforeRecv:       firstAudioSent,
		firstAudioSent:   firstAudioSent,
		eventsDrained:    eventsDrained,
		blockAfterEvents: releaseEvents,
	}
	tfr := newTransformer(nil,
		withMode(ModeRealtime),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withFormat("pcm"),
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
	if got := session.interruptCount(); got != 0 {
		t.Fatalf("Interrupt calls = %d, want 0 outside push-to-talk", got)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if !hasRealtimeInterruptedEOS(chunks, "turn-1:rt:1", genx.RoleModel, false) {
		t.Fatalf("missing interrupted text EOS for pending response: %#v", chunks)
	}
	if !hasRealtimeInterruptedEOS(chunks, "turn-1:rt:1", genx.RoleModel, true) {
		t.Fatalf("missing interrupted audio EOS for pending response: %#v", chunks)
	}
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleModel, doubaoRealtimeAssistantLabel, 2)
	if hasRealtimeTestBlob(chunks, genx.RoleModel, "audio/pcm") {
		t.Fatalf("interrupted audio backlog leaked before Error EOS: %#v", chunks)
	}
}

func TestTransformerPushToTalkBargeInWhileWaitingResponse(t *testing.T) {
	eventsDrained := make(chan struct{})
	releaseEvents := make(chan struct{})
	allowNextInput := make(chan struct{})
	endASR := make(chan struct{})
	session := &fakeTransformerSession{
		events:           []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventASRResponse, Text: "第一段"}, {Type: doubaospeech.EventASREnded}},
		beforeRecv:       endASR,
		endASR:           endASR,
		eventsDrained:    eventsDrained,
		blockAfterEvents: releaseEvents,
	}
	tfr := newTransformer(nil,
		withMode(ModePushToTalk),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withFormat("pcm"),
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
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleModel, doubaoRealtimeAssistantLabel, 2)
}

func TestTransformerInputErrorCreatesCompleteTranscriptLifecycle(t *testing.T) {
	tfr := newTransformer(nil)
	output := newBufferStream(2)
	tfr.pushInputEOSError(output, "turn-1", errors.New("invalid input"))
	if err := output.Close(); err != nil {
		t.Fatalf("Close(output) error = %v", err)
	}

	chunks := drainRealtimeTestOutput(t, output)
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleUser, doubaoRealtimeTranscriptLabel, 1)
	if got := chunks[len(chunks)-1].Ctrl.Error; got != "invalid input" {
		t.Fatalf("transcript EOS error = %q, want invalid input", got)
	}
}

func TestTransformerBargeInPropagatesInterruptFailure(t *testing.T) {
	eventsDrained := make(chan struct{})
	releaseEvents := make(chan struct{})
	allowInput := make(chan struct{})
	endASR := make(chan struct{})
	session := &fakeTransformerSession{
		events:           []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventASREnded}},
		beforeRecv:       endASR,
		endASR:           endASR,
		eventsDrained:    eventsDrained,
		blockAfterEvents: releaseEvents,
		interruptErr:     errors.New("interrupt failed"),
	}
	input := &gatedRealtimeStream{
		first: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
			{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
			{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		},
		gate: allowInput,
		rest: []*genx.MessageChunk{{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}}},
	}
	tfr := newTransformer(nil,
		withMode(ModePushToTalk),
		withInputFormat("pcm"),
		withInputTranscode(false),
	)
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

func runTransformerProcessLoop(t *testing.T, tfr *Transformer, input genx.Stream, output *bufferStream, session *fakeTransformerSession) error {
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

func requireRealtimeOwnedRouteLifecycles(t *testing.T, chunks []*genx.MessageChunk, role genx.Role, label string, want int) {
	t.Helper()
	routes := make(map[string][]*genx.MessageChunk)
	for _, chunk := range chunks {
		if chunk == nil || chunk.Role != role || chunk.Ctrl == nil || chunk.Ctrl.Label != label {
			continue
		}
		mimeType, ok := chunk.MIMEType()
		if !ok {
			continue
		}
		key := chunk.Ctrl.StreamID + "\x00" + mimeType
		routes[key] = append(routes[key], chunk)
	}
	if len(routes) != want {
		t.Fatalf("owned %q routes = %d, want %d: %#v", label, len(routes), want, chunks)
	}
	for key, route := range routes {
		if len(route) < 2 || !route[0].IsBeginOfStream() || !route[len(route)-1].IsEndOfStream() {
			t.Fatalf("owned route %q lifecycle = %#v, want BOS...EOS", key, route)
		}
		for i, chunk := range route {
			if i > 0 && chunk.IsBeginOfStream() {
				t.Fatalf("owned route %q has duplicate BOS: %#v", key, route)
			}
			if i < len(route)-1 && chunk.IsEndOfStream() {
				t.Fatalf("owned route %q ended before its final chunk: %#v", key, route)
			}
		}
	}
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

type fakeTransformerOpenResult struct {
	session doubaoRealtimeSession
	err     error
}

type fakeTransformerOpener struct {
	mu      sync.Mutex
	results []fakeTransformerOpenResult
	calls   int
	dialogs []string
}

func (o *fakeTransformerOpener) OpenSession(_ context.Context, cfg *doubaospeech.RealtimeConfig) (doubaoRealtimeSession, error) {
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

func (o *fakeTransformerOpener) callCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls
}

func (o *fakeTransformerOpener) dialogIDs() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.dialogs...)
}

func (o *fakeTransformerOpener) waitForCalls(want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if o.callCount() >= want {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return o.callCount() >= want
}

type fakeTransformerSession struct {
	events           []*doubaospeech.RealtimeEvent
	recvErr          error
	eventInterval    time.Duration
	beforeRecv       <-chan struct{}
	endASR           chan struct{}
	eventsDrained    chan<- struct{}
	blockAfterEvents <-chan struct{}
	interruptErr     error
	sendAudioErr     error
	sendAudioErrAt   int
	firstAudioSent   chan struct{}
	sendTextErr      error
	sendTextErrAt    int
	firstTextSent    chan struct{}
	pauseBeforeEvent int
	eventPaused      chan struct{}
	resumeEvents     <-chan struct{}

	mu                sync.Mutex
	audio             [][]byte
	texts             []string
	endCount          int
	interrupts        int
	closed            bool
	closedCh          chan struct{}
	endOnce           sync.Once
	firstAudioOnce    sync.Once
	firstTextOnce     sync.Once
	closeOnce         sync.Once
	eventsDrainedOnce sync.Once
	eventPauseOnce    sync.Once
}

func (s *fakeTransformerSession) SendAudio(ctx context.Context, audio []byte) error {
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

func (s *fakeTransformerSession) SendText(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.texts = append(s.texts, text)
	if s.firstTextSent != nil {
		s.firstTextOnce.Do(func() { close(s.firstTextSent) })
	}
	if s.sendTextErr != nil && len(s.texts) == s.sendTextErrAt {
		return s.sendTextErr
	}
	return nil
}

func (s *fakeTransformerSession) audioCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.audio)
}

func (s *fakeTransformerSession) audioChunks() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	chunks := make([][]byte, 0, len(s.audio))
	for _, chunk := range s.audio {
		chunks = append(chunks, append([]byte(nil), chunk...))
	}
	return chunks
}

func (s *fakeTransformerSession) EndASR(ctx context.Context) error {
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

func (s *fakeTransformerSession) Interrupt(context.Context) error {
	s.mu.Lock()
	s.interrupts++
	s.mu.Unlock()
	return s.interruptErr
}

func (s *fakeTransformerSession) Recv() iter.Seq2[*doubaospeech.RealtimeEvent, error] {
	return func(yield func(*doubaospeech.RealtimeEvent, error) bool) {
		closed := s.closedSignal()
		if s.beforeRecv != nil {
			select {
			case <-s.beforeRecv:
			case <-closed:
				return
			}
		}
		for i, event := range s.events {
			if i > 0 && s.eventInterval > 0 {
				timer := time.NewTimer(s.eventInterval)
				select {
				case <-timer.C:
				case <-closed:
					timer.Stop()
					return
				}
			}
			if s.eventPaused != nil && i == s.pauseBeforeEvent {
				s.eventPauseOnce.Do(func() { close(s.eventPaused) })
				select {
				case <-s.resumeEvents:
				case <-closed:
					return
				}
			}
			if !yield(event, nil) {
				return
			}
		}
		if s.recvErr != nil && !yield(nil, s.recvErr) {
			return
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

func (s *fakeTransformerSession) Close() error {
	closed := s.closedSignal()
	s.closeOnce.Do(func() { close(closed) })
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *fakeTransformerSession) closedSignal() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closedCh == nil {
		s.closedCh = make(chan struct{})
	}
	return s.closedCh
}

func (s *fakeTransformerSession) endASRCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.endCount
}

func (s *fakeTransformerSession) waitForEndASRCount(want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.endASRCount() >= want {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return s.endASRCount() >= want
}

func (s *fakeTransformerSession) interruptCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interrupts
}

func (s *fakeTransformerSession) audioFrames() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.audio))
	for i := range s.audio {
		out[i] = append([]byte(nil), s.audio[i]...)
	}
	return out
}

func (s *fakeTransformerSession) textMessages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.texts...)
}

func (s *fakeTransformerSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
