package doubaorealtime

import (
	"context"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTransformerASRContinuationBeforeAssistantBOSKeepsSession(t *testing.T) {
	ready := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv: ready, firstAudioSent: ready, blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "first part", QuestionID: "q-1", IsFinal: true},
			{Type: doubaospeech.EventASREnded, QuestionID: "q-1"},
			// Speech continues before any assistant route exists. Closing the
			// provider here loses audio it already consumed for this question.
			{Type: doubaospeech.EventASRInfo},
			{Type: doubaospeech.EventASRResponse, Text: "continued question", QuestionID: "q-2", IsFinal: true},
			{Type: doubaospeech.EventASREnded, QuestionID: "q-2"},
			{Type: doubaospeech.EventTTSStarted, Text: "answer", QuestionID: "q-2", ReplyID: "r-2"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{2, 0}},
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
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	pushPTTTestTurn(t, input, "turn", 1)
	text, audio, terminals := false, false, 0
	for terminals < 2 {
		chunk, err := output.Next()
		if err != nil {
			t.Fatal(err)
		}
		if chunk.Role != genx.RoleModel {
			continue
		}
		if chunk.Ctrl.Error != "" {
			t.Fatalf("pending ASR continuation interrupted its response: %s", chunk.Ctrl.Error)
		}
		switch part := chunk.Part.(type) {
		case genx.Text:
			text = text || string(part) == "answer"
		case *genx.Blob:
			audio = audio || len(part.Data) > 0
		}
		if chunk.IsEndOfStream() {
			terminals++
		}
	}
	if !text || !audio || opener.callCount() != 1 || session.interruptCount() != 0 {
		t.Fatalf("continuation text=%t audio=%t sessions=%d provider interrupts=%d", text, audio, opener.callCount(), session.interruptCount())
	}
}
