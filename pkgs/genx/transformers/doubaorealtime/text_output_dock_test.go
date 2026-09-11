package doubaorealtime

import (
	"context"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
)

// A realtime barge-in while external TTS is still playing a text-only reply
// must reach Audio Dock and stop that synthesis.
func TestTransformerTextOutputRealtimeBargeInStopsExternalTTS(t *testing.T) {
	firstAudioSent := make(chan struct{})
	eventPaused := make(chan struct{})
	resumeEvents := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       firstAudioSent,
		firstAudioSent:   firstAudioSent,
		pauseBeforeEvent: 4,
		eventPaused:      eventPaused,
		resumeEvents:     resumeEvents,
		blockAfterEvents: make(chan struct{}),
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "question"},
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventChatResponse, Text: "answer"},
			{Type: doubaospeech.EventChatEnded},
			{Type: doubaospeech.EventASRInfo},
		},
	}
	replacement := &fakeTransformerSession{beforeRecv: make(chan struct{})}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(&fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}, {session: replacement}}}),
		withMode(ModeRealtime),
		withOutput(OutputText),
		withInputFormat("pcm"),
		withInputTranscode(false),
	)
	ttsCancelled := make(chan struct{})
	dock, err := audiodock.New(audiodock.Config{
		Agent: tfr,
		TTS:   playingTTS{cancelled: ttsCancelled},
		ResolveVoice: func(context.Context, audiodock.VoiceRequest) (string, error) {
			return "voice/narrator", nil
		},
	})
	if err != nil {
		t.Fatalf("audiodock.New() error = %v", err)
	}
	input := newBufferStream(8)
	for _, chunk := range []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "mic", BeginOfStream: true}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "mic"}},
	} {
		if err := input.Push(chunk); err != nil {
			t.Fatalf("Push(input) error = %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	output, err := dock.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()

	next := func() *genx.MessageChunk {
		t.Helper()
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("output Next() error = %v", err)
		}
		return chunk
	}
	replyStreamID := ""
	for replyStreamID == "" {
		chunk := next()
		if blob, ok := chunk.Part.(*genx.Blob); ok && chunk.Role == genx.RoleModel && len(blob.Data) > 0 {
			replyStreamID = chunk.Ctrl.StreamID
		}
	}
	select {
	case <-eventPaused:
	case <-time.After(time.Second):
		t.Fatal("provider did not reach the barge-in event")
	}
	close(resumeEvents)
	for {
		chunk := next()
		if chunk.Role != genx.RoleModel || chunk.Ctrl == nil || chunk.Ctrl.StreamID != replyStreamID || !chunk.IsEndOfStream() {
			continue
		}
		if _, ok := chunk.Part.(*genx.Blob); !ok {
			continue
		}
		if chunk.Ctrl.Error == "" {
			t.Fatalf("playing reply audio ended without an interruption error: %#v", chunk)
		}
		break
	}
	select {
	case <-ttsCancelled:
	case <-time.After(time.Second):
		t.Fatal("barge-in did not cancel the external TTS")
	}
}

// playingTTS answers the first text chunk with audio and keeps playing until
// its context is cancelled.
type playingTTS struct {
	cancelled chan struct{}
}

func (p playingTTS) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	output := newBufferStream(8)
	go func() {
		defer output.Close()
		first, err := input.Next()
		if err != nil || first == nil || first.Ctrl == nil {
			return
		}
		streamID := first.Ctrl.StreamID
		_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true}})
		_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}})
		<-ctx.Done()
		close(p.cancelled)
	}()
	return output, nil
}

// An empty push-to-talk turn has no reply text for external TTS; its empty
// audio route must still pass through Audio Dock so the client sees both
// assistant terminals.
func TestTransformerTextOutputEmptyPushToTalkTurnKeepsAudioTerminalThroughDock(t *testing.T) {
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(&fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: &fakeTransformerSession{beforeRecv: make(chan struct{})}}}}),
		withMode(ModePushToTalk),
		withOutput(OutputText),
		withInputFormat("pcm"),
		withInputTranscode(false),
	)
	dock, err := audiodock.New(audiodock.Config{
		Agent: tfr,
		TTS:   playingTTS{cancelled: make(chan struct{})},
		ResolveVoice: func(context.Context, audiodock.VoiceRequest) (string, error) {
			return "voice/narrator", nil
		},
	})
	if err != nil {
		t.Fatalf("audiodock.New() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	output, err := dock.Transform(ctx, &sliceRealtimeStream{chunks: []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
	}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	var textEOS, audioEOS bool
	for !textEOS || !audioEOS {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("output ended before both assistant terminals (text %v, audio %v): %v", textEOS, audioEOS, err)
		}
		if chunk.Role != genx.RoleModel || !chunk.IsEndOfStream() {
			continue
		}
		switch chunk.Part.(type) {
		case genx.Text:
			textEOS = true
		case *genx.Blob:
			audioEOS = true
		}
	}
}
