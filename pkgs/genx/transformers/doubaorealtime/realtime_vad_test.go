package doubaorealtime

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTransformerRealtimeVADKeepsAcceptedAudioAndRejectsInterruptedOutput(t *testing.T) {
	ready, drained := make(chan struct{}), make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv: ready, firstAudioSent: ready, eventsDrained: drained, blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRInfo, QuestionID: "q-1"},
			{Type: doubaospeech.EventASRResponse, Text: "first segment", QuestionID: "q-1"},
			{Type: doubaospeech.EventASREnded, QuestionID: "q-1"},
			{Type: doubaospeech.EventTTSStarted, QuestionID: "q-1", ReplyID: "r-1"},
			{Type: doubaospeech.EventChatResponse, Text: "first answer", QuestionID: "q-1", ReplyID: "r-1"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2}},
			{Type: doubaospeech.EventASRInfo, QuestionID: "q-2"},
			{Type: doubaospeech.EventASRResponse, Text: "continued speech", QuestionID: "q-2"},
			{Type: doubaospeech.EventASREnded, QuestionID: "q-2"},
			{Type: doubaospeech.EventTTSStarted, QuestionID: "q-1", ReplyID: "r-1"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{9, 9}},
			{Type: doubaospeech.EventChatResponse, Text: "stale answer", QuestionID: "q-1", ReplyID: "r-1"},
			{Type: doubaospeech.EventChatEnded, QuestionID: "q-1", ReplyID: "r-1"},
			{Type: doubaospeech.EventTTSFinished, QuestionID: "q-1", ReplyID: "r-1"},
			{Type: doubaospeech.EventTTSStarted, QuestionID: "q-2", ReplyID: "r-2"},
			{Type: doubaospeech.EventChatResponse, Text: "second answer", QuestionID: "q-2", ReplyID: "r-2"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{2, 3}},
			{Type: doubaospeech.EventASRInfo, QuestionID: "q-2"},
			{Type: doubaospeech.EventChatEnded, QuestionID: "q-1", ReplyID: "r-1"},
			{Type: doubaospeech.EventTTSFinished, QuestionID: "q-1", ReplyID: "r-1"},
			{Type: doubaospeech.EventChatEnded, QuestionID: "q-2", ReplyID: "r-2"},
			{Type: doubaospeech.EventTTSFinished, QuestionID: "q-2", ReplyID: "r-2"},
		},
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}
	transformer := newTransformer(nil, withDoubaoRealtimeOpener(opener), withMode(ModeRealtime),
		withInputFormat("pcm"), withInputTranscode(false), withFormat("pcm"))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	input := newBufferStream(8)
	defer input.Close()
	output, err := transformer.transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	for _, chunk := range []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "mic", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "mic"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "mic"}},
	} {
		if err := input.Push(chunk); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-drained:
	case <-ctx.Done():
		t.Fatalf("VAD continuation was lost; provider opens=%d", opener.callCount())
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if got := opener.callCount(); got != 1 {
		t.Fatalf("provider opens=%d, want the same session for VAD continuation", got)
	}
	if got := session.interruptCount(); got != 0 {
		t.Fatalf("ClientInterrupt calls=%d, want none in realtime mode", got)
	}
	frames := session.audioFrames()
	if len(frames) != 2 || !bytes.Equal(frames[0], []byte{1, 0}) || !bytes.Equal(frames[1], []byte{2, 0}) {
		t.Fatalf("audio frames=%v, want both input frames exactly once", frames)
	}
	if !hasRealtimeTestText(chunks, genx.RoleModel, "second answer") || hasRealtimeTestText(chunks, genx.RoleModel, "stale answer") {
		t.Fatalf("continuation reply or stale output filtering failed: %#v", chunks)
	}
	if !hasRealtimeInterruptedEOS(chunks, "mic:rt:1", genx.RoleModel, false) || !hasRealtimeInterruptedEOS(chunks, "mic:rt:1", genx.RoleModel, true) {
		t.Fatal("the interrupted reply did not close both modalities")
	}
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleModel, doubaoRealtimeAssistantLabel, 4)
}

func TestRealtimeVADQuestionHistoryIsBounded(t *testing.T) {
	var responses realtimeVADResponses
	for i := range 1000 {
		if accepted, changed := responses.questionStarted("q-" + strconv.Itoa(i)); !accepted || !changed {
			t.Fatalf("question %d: accepted=%t changed=%t", i, accepted, changed)
		}
	}
	if len(responses.retired) != 64 {
		t.Fatalf("retired questions=%d, want bounded history", len(responses.retired))
	}
	if accepted, changed := responses.questionStarted("q-998"); accepted || changed {
		t.Fatal("a late retired ASR event replaced the current question")
	}
	if accepted, changed := responses.questionStarted("q-999"); !accepted || changed {
		t.Fatal("a duplicate ASR event changed the current question")
	}
}

func TestTransformerRealtimeUnscopedTextAfterVoiceReply(t *testing.T) {
	ready, paused, textSent, drained := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv: ready, firstAudioSent: ready, firstTextSent: textSent, pauseBeforeEvent: 8,
		eventPaused: paused, resumeEvents: textSent, eventsDrained: drained,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRInfo, QuestionID: "voice-q"},
			{Type: doubaospeech.EventASRResponse, Text: "voice question", QuestionID: "voice-q"},
			{Type: doubaospeech.EventASREnded, QuestionID: "voice-q"},
			{Type: doubaospeech.EventTTSStarted, QuestionID: "voice-q", ReplyID: "voice-r"},
			{Type: doubaospeech.EventChatResponse, Text: "voice answer", QuestionID: "voice-q", ReplyID: "voice-r"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2}},
			{Type: doubaospeech.EventChatEnded, QuestionID: "voice-q", ReplyID: "voice-r"},
			{Type: doubaospeech.EventTTSFinished, QuestionID: "voice-q", ReplyID: "voice-r"},
			{Type: doubaospeech.EventTTSStarted, QuestionID: "text-q", ReplyID: "text-r"},
			{Type: doubaospeech.EventChatResponse, Text: "text answer", QuestionID: "text-q", ReplyID: "text-r"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{2, 3}},
			{Type: doubaospeech.EventChatEnded, QuestionID: "text-q", ReplyID: "text-r"},
			{Type: doubaospeech.EventTTSFinished, QuestionID: "text-q", ReplyID: "text-r"},
		},
	}
	transformer := newTransformer(nil,
		withDoubaoRealtimeOpener(&fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}),
		withMode(ModeRealtime), withInputFormat("pcm"), withInputTranscode(false), withFormat("pcm"))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	input := newBufferStream(8)
	defer input.Close()
	output, err := transformer.transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	pushPTTTestTurn(t, input, "mic", 1)
	select {
	case <-paused:
	case <-ctx.Done():
		t.Fatal("voice reply did not finish")
	}
	if err := input.Push(&genx.MessageChunk{Part: genx.Text("typed question")}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-drained:
	case <-ctx.Done():
		t.Fatal("text reply did not finish")
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if chunks := drainRealtimeTestOutput(t, output); !hasRealtimeTestText(chunks, genx.RoleModel, "text answer") {
		t.Fatal("the voice question identity filtered the later unscoped text reply")
	}
}
