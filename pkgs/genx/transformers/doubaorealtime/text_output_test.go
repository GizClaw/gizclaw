package doubaorealtime

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTransformerTextOutputRequestsTextModality(t *testing.T) {
	tfr := newTransformer(nil,
		withOutput(OutputText),
		withDialogExtra(doubaospeech.RealtimeDialogExtra{AuditResponse: "blocked"}),
	)
	cfg := tfr.realtimeConfig()
	if cfg.Dialog.Extra == nil || cfg.Dialog.Extra.AuditResponse != "blocked" {
		t.Fatalf("dialog extra = %#v, want configured fields preserved", cfg.Dialog.Extra)
	}
	want := []doubaospeech.RealtimeOutputModality{doubaospeech.RealtimeOutputModalityText}
	if !slices.Equal(cfg.Dialog.Extra.OutputModalities, want) {
		t.Fatalf("output modalities = %v, want %v", cfg.Dialog.Extra.OutputModalities, want)
	}
	if tfr.dialogExtra.OutputModalities != nil {
		t.Fatalf("session config mutated the configured dialog extra: %#v", tfr.dialogExtra)
	}
	if cfg.TTS.Speaker == "" {
		t.Fatal("text output must keep tts.speaker; the provider rejects sessions without it")
	}

	cfg = newTransformer(nil, withOutput(OutputText)).realtimeConfig()
	if cfg.Dialog.Extra == nil || !slices.Equal(cfg.Dialog.Extra.OutputModalities, want) {
		t.Fatalf("output modalities without dialog extra = %#v, want %v", cfg.Dialog.Extra, want)
	}

	for _, output := range []Output{"", OutputAudio} {
		cfg := newTransformer(nil, withOutput(output)).realtimeConfig()
		if cfg.Dialog.Extra != nil && cfg.Dialog.Extra.OutputModalities != nil {
			t.Fatalf("Output %q sent output modalities %v, want provider default", output, cfg.Dialog.Extra.OutputModalities)
		}
	}
}

func TestNewRejectsUnsupportedOutput(t *testing.T) {
	_, err := New(Config{Client: doubaospeech.NewClient("app"), Model: "1.2.1.1", Output: "video"})
	if err == nil || !strings.Contains(err.Error(), "unsupported Output") {
		t.Fatalf("New() error = %v, want unsupported Output", err)
	}
	if _, err := New(Config{Client: doubaospeech.NewClient("app"), Model: "1.2.1.1", Output: OutputText}); err != nil {
		t.Fatalf("New(OutputText) error = %v", err)
	}
}

func TestTransformerTextOutputStreamsChatTextAndCompletesAtChatEnded(t *testing.T) {
	textSent := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       textSent,
		firstTextSent:    textSent,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventChatResponse, Text: "你好，"},
			{Type: doubaospeech.EventUsageResponse},
			{Type: doubaospeech.EventChatResponse, Text: "世界"},
			// A provider that ignored output_modalities must not reopen audio.
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2}},
			{Type: doubaospeech.EventChatEnded},
		},
	}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(&fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}),
		withMode(ModeText),
		withOutput(OutputText),
		withFormat("pcm"),
	)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, &sliceRealtimeStream{chunks: []*genx.MessageChunk{{Part: genx.Text("question")}}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if got := realtimeTestAssistantTexts(chunks); !slices.Equal(got, []string{"你好，", "世界"}) {
		t.Fatalf("assistant texts = %q, want ChatResponse deltas", got)
	}
	requireRealtimeTestNoModelAudio(t, chunks)
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleModel, doubaoRealtimeAssistantLabel, 1)
}

func TestTransformerTextOutputPushToTalkCompletesWithoutTTS(t *testing.T) {
	endASR := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       endASR,
		endASR:           endASR,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "question", QuestionID: "q-1"},
			{Type: doubaospeech.EventASREnded, QuestionID: "q-1"},
			{Type: doubaospeech.EventChatResponse, Text: "answer", QuestionID: "q-1", ReplyID: "r-1"},
			{Type: doubaospeech.EventChatEnded, QuestionID: "q-1", ReplyID: "r-1"},
		},
	}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(&fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}),
		withMode(ModePushToTalk),
		withOutput(OutputText),
		withInputFormat("pcm"),
		withInputTranscode(false),
	)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, &sliceRealtimeStream{chunks: pttTestTurn("turn-1", 1)})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if !hasRealtimeTestText(chunks, genx.RoleUser, "question") {
		t.Fatalf("output missing transcript: %#v", chunks)
	}
	if got := realtimeTestAssistantTexts(chunks); !slices.Equal(got, []string{"answer"}) {
		t.Fatalf("assistant texts = %q, want answer", got)
	}
	requireRealtimeTestNoModelAudio(t, chunks)
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleModel, doubaoRealtimeAssistantLabel, 1)
}

func TestTransformerTextOutputRealtimeFinishesResponseDeadlineAtChatEnded(t *testing.T) {
	firstAudioSent := make(chan struct{})
	eventsDrained := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       firstAudioSent,
		firstAudioSent:   firstAudioSent,
		eventsDrained:    eventsDrained,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "question"},
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventChatResponse, Text: "answer"},
			{Type: doubaospeech.EventChatEnded},
		},
	}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(&fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}),
		withMode(ModeRealtime),
		withOutput(OutputText),
		withInputFormat("pcm"),
		withInputTranscode(false),
		// Long enough that scheduler delay between ASREnded and ChatEnded
		// cannot expire it, short enough that a leaked deadline fires below.
		withResponseDeadline(200*time.Millisecond),
	)
	input := newBufferStream(4)
	for _, chunk := range []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	} {
		if err := input.Push(chunk); err != nil {
			t.Fatalf("Push(input) error = %v", err)
		}
	}
	output, err := tfr.Transform(t.Context(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	select {
	case <-eventsDrained:
	case <-time.After(2 * time.Second):
		t.Fatal("provider response did not reach ChatEnded")
	}
	time.Sleep(300 * time.Millisecond)
	if session.isClosed() {
		t.Fatal("text-only response left its deadline armed after ChatEnded")
	}
	if err := input.Close(); err != nil {
		t.Fatalf("Close(input) error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if got := realtimeTestAssistantTexts(chunks); !slices.Equal(got, []string{"answer"}) {
		t.Fatalf("assistant texts = %q, want answer", got)
	}
	requireRealtimeTestNoModelAudio(t, chunks)
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleModel, doubaoRealtimeAssistantLabel, 1)
}

func TestRealtimeAssistantLifecycleTextOnlyCompletesWithTextRoute(t *testing.T) {
	assistant := newRealtimeAssistantLifecycle()
	assistant.textOnly = true
	epoch := assistant.nextEpoch()
	assistant.markPending("reply-1", epoch)
	assistant.markRouteDone(epoch, true)
	if interruption := assistant.interruptRoutes("reply-1", false); interruption.interrupted {
		t.Fatalf("completed text-only response was interrupted: %#v", interruption)
	}

	epoch = assistant.nextEpoch()
	assistant.markPending("reply-2", epoch)
	interruption := assistant.interruptRoutes("reply-2", false)
	if !interruption.interrupted || !interruption.textOpen || interruption.audioOpen {
		t.Fatalf("active text-only interruption = %#v, want only the text route open", interruption)
	}
	if forced := assistant.interruptRoutes("reply-3", true); !forced.textOpen || forced.audioOpen {
		t.Fatalf("forced text-only interruption = %#v, want only the text route open", forced)
	}
}

func TestDoubaoPushToTalkStateTextOnlyIdlesOnTextEOS(t *testing.T) {
	state := &doubaoPushToTalkState{textOnly: true}
	if _, _, err := state.begin("turn-1"); err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if err := state.end(); err != nil {
		t.Fatalf("end() error = %v", err)
	}
	state.responseStarted("turn-1", false)
	state.ttsFinished("turn-1")
	state.observeAssistantOutput(doubaoRealtimeAssistantLabel, &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: doubaoRealtimeAssistantLabel, EndOfStream: true},
	})
	if got := state.current(); got != doubaoPushToTalkIdle {
		t.Fatalf("phase after text EOS = %v, want idle", got)
	}
}

func TestTransformerTextOutputEmptyReplyKeepsAudioTerminal(t *testing.T) {
	textSent := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       textSent,
		firstTextSent:    textSent,
		blockAfterEvents: make(chan struct{}),
		events:           []*doubaospeech.RealtimeEvent{{Type: doubaospeech.EventChatEnded}},
	}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(&fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}),
		withMode(ModeText),
		withOutput(OutputText),
	)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, &sliceRealtimeStream{chunks: []*genx.MessageChunk{{Part: genx.Text("question")}}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if got := realtimeTestAssistantTexts(chunks); len(got) != 0 {
		t.Fatalf("assistant texts = %q, want none", got)
	}
	// External TTS has nothing to speak, so the empty reply closes an empty
	// audio route itself and clients still observe text and audio terminals.
	requireRealtimeTestNoModelAudio(t, chunks)
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleModel, doubaoRealtimeAssistantLabel, 2)
}

func realtimeTestAssistantTexts(chunks []*genx.MessageChunk) []string {
	var texts []string
	for _, chunk := range chunks {
		if chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil || chunk.Ctrl.Label != doubaoRealtimeAssistantLabel {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok && text != "" {
			texts = append(texts, string(text))
		}
	}
	return texts
}

func requireRealtimeTestNoModelAudio(t *testing.T, chunks []*genx.MessageChunk) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk == nil || chunk.Role != genx.RoleModel {
			continue
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
			t.Fatalf("text output emitted model audio data: %#v", chunk)
		}
	}
}
