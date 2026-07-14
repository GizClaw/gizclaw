package transformers

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestDoubaoRealtimeAdaptersShareMediaContract(t *testing.T) {
	factories := []struct {
		name string
		new  func() *doubaoRealtimeAudioInputs
	}{
		{name: "realtime", new: func() *doubaoRealtimeAudioInputs {
			return newDoubaoRealtimeAudioInputs("pcm", 16000, 1, false)
		}},
		{name: "duplex", new: func() *doubaoRealtimeAudioInputs {
			return newDoubaoRealtimeDuplexAudioInputs("pcm", 16000, 1, false)
		}},
	}

	for _, factory := range factories {
		t.Run(factory.name, func(t *testing.T) {
			inputs := factory.new()
			pcm := &genx.Blob{
				MIMEType: "audio/pcm;rate=16000",
				Data:     []byte{1, 0, 2, 0},
			}
			input, err := inputs.streamForBlob("turn-1", pcm)
			if err != nil {
				t.Fatalf("streamForBlob(PCM) error = %v", err)
			}
			frames, err := input.prepareFrames(pcm)
			if err != nil {
				t.Fatalf("prepareFrames(PCM) error = %v", err)
			}
			if len(frames) != 1 || string(frames[0]) != string([]byte{1, 0, 2, 0}) {
				t.Fatalf("prepareFrames(PCM) = %#v", frames)
			}

			if _, err := inputs.streamForBlob("turn-1", &genx.Blob{MIMEType: "audio/opus"}); err == nil || !strings.Contains(err.Error(), "changed MIME type") {
				t.Fatalf("streamForBlob(MIME change) error = %v", err)
			}
			inputs.close()
			if len(inputs.streams) != 0 || len(inputs.mimeTypes) != 0 {
				t.Fatalf("close() retained streams=%d mimeTypes=%d", len(inputs.streams), len(inputs.mimeTypes))
			}
		})
	}
}

func TestDoubaoRealtimeAdaptersShareStreamIDContract(t *testing.T) {
	factories := []struct {
		name string
		new  func() *doubaoRealtimeStreamIDs
	}{
		{name: "realtime", new: func() *doubaoRealtimeStreamIDs {
			return newDoubaoRealtimeStreamIDs(DoubaoRealtimeModeRealtime)
		}},
		{name: "duplex", new: func() *doubaoRealtimeStreamIDs {
			return newDoubaoRealtimeDuplexStreamIDs()
		}},
	}

	for _, factory := range factories {
		t.Run(factory.name, func(t *testing.T) {
			ids := factory.new()
			ids.beginInput("turn")
			if got := ids.input(); got != "turn:rt:1" {
				t.Fatalf("input() = %q, want turn:rt:1", got)
			}
			if got := ids.endInputSegment(); got != "turn:rt:1" {
				t.Fatalf("endInputSegment() = %q, want turn:rt:1", got)
			}
			if got := ids.response(); got != "turn:rt:1" {
				t.Fatalf("response() = %q, want turn:rt:1", got)
			}
			if got := ids.input(); got != "turn:rt:2" {
				t.Fatalf("next input() = %q, want turn:rt:2", got)
			}
		})
	}
}

func TestRealtimeAssistantLifecycleInterruptsCurrentEpoch(t *testing.T) {
	assistant := newRealtimeAssistantLifecycle()
	epoch := assistant.markStarted("turn-1")
	if !assistant.canPush(epoch) {
		t.Fatal("started response rejected current epoch")
	}
	streamID, interrupted := assistant.interrupt("fallback", false)
	if !interrupted || streamID != "turn-1" {
		t.Fatalf("interrupt() = (%q, %v), want (turn-1, true)", streamID, interrupted)
	}
	if assistant.canPush(epoch) {
		t.Fatal("interrupted response still accepts old epoch")
	}
	assistant.setAccept(true)
	next := assistant.nextEpoch()
	assistant.markPending("turn-2", next)
	if !assistant.canPush(next) {
		t.Fatal("next response rejected current epoch")
	}
	assistant.markDone(next)
	if _, interrupted := assistant.interrupt("turn-2", false); interrupted {
		t.Fatal("completed response remained active")
	}
}

func TestRealtimeAssistantLifecycleIgnoresStaleStreamCompletion(t *testing.T) {
	assistant := newRealtimeAssistantLifecycle()
	assistant.markStarted("turn-1")
	assistant.markStarted("turn-2")
	assistant.markDoneStream("turn-1")
	streamID, interrupted := assistant.interrupt("fallback", false)
	if !interrupted || streamID != "turn-2" {
		t.Fatalf("interrupt() after stale completion = (%q, %v), want (turn-2, true)", streamID, interrupted)
	}
}
