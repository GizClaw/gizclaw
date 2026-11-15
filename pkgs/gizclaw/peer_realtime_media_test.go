package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestPeerRealtimeSourcePCMReplacementPreservesWireMIME(t *testing.T) {
	var replaced peerAudioInputRoute
	source := newPeerRealtimeSourceWithRouteReplacement(nil, func(_ context.Context, route peerAudioInputRoute) error {
		replaced = route
		return nil
	}, genx.WithRealtimeStreamDelay(0))
	defer source.Close()
	input, err := source.OpenAgentInput(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	blob := &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", OriginalAudio: &genx.Blob{MIMEType: "audio/opus"}}
	if err := source.Push(t.Context(), &genx.MessageChunk{Part: blob, Ctrl: &genx.StreamCtrl{StreamID: "microphone", BeginOfStream: true}}); err != nil {
		t.Fatal(err)
	}
	chunk, err := input.Next()
	if err != nil || chunk.Part.(*genx.Blob).MIMEType != blob.MIMEType {
		t.Fatalf("Agent input stopped being PCM: %+v, %v", chunk, err)
	}
	if _, err := source.OpenAgentInput(t.Context()); err != nil {
		t.Fatal(err)
	}
	eos := peerAudioInputRouteReloadedEvent(replaced).GetEos()
	if replaced.streamID != "microphone" || eos.GetMimeType() != "audio/opus" || eos.GetError().GetCode() != inputRouteReloadedCode {
		t.Fatalf("internal PCM format leaked into peer route replacement: %+v", eos)
	}
}

func TestPeerRealtimeSourceBindsRecoveredPCMAndConcealment(t *testing.T) {
	source := newPeerRealtimeSource()
	source.bindAudioStreamIDLocked(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}})
	for _, concealed := range []bool{false, true} {
		blob := &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 640), ConcealedAudio: concealed}
		if !concealed {
			blob.OriginalAudio = &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}
		}
		chunk := source.bindAudioStreamIDLocked(&genx.MessageChunk{Part: blob, Ctrl: &genx.StreamCtrl{StreamID: "audio"}})
		if chunk == nil || chunk.Ctrl.StreamID != "turn" {
			t.Fatal("recovered media lost the authorized logical route")
		}
	}
	source.bindAudioStreamIDLocked(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true}})
	late := &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 640), ConcealedAudio: true}
	if source.bindAudioStreamIDLocked(&genx.MessageChunk{Part: late}) != nil {
		t.Fatal("concealment crossed the ended logical route")
	}
}
