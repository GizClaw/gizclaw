package gizclaw

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
)

func lateTerminalAudio(streamID string, begin, end bool, errorText string) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", BeginOfStream: begin, EndOfStream: end, Error: errorText},
	}
}

func TestPeerAgentOutputLateInterruptionPreservesNextResponse(t *testing.T) {
	var events bytes.Buffer
	broker := newPeerStreamEventBroker()
	unsubscribe, err := broker.Subscribe(&events)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	output := &peerStreamSliceStream{chunks: []*genx.MessageChunk{
		lateTerminalAudio("finished", true, false, ""),
		lateTerminalAudio("finished", false, true, ""),
		lateTerminalAudio("next", true, false, ""),
		lateTerminalAudio("finished", false, true, "interrupted"),
		lateTerminalAudio("next", false, true, ""),
	}}
	if err := (peerAgentOutput{Events: broker}).ConsumeAgentOutput(t.Context(), output); err != nil {
		t.Fatalf("late interruption of a delivered terminal failed the next response: %v", err)
	}
	for _, want := range []struct {
		stream string
		kind   eventpb.PeerEventType
	}{
		{"finished", eventpb.PeerEventType_PEER_EVENT_TYPE_BOS},
		{"finished", eventpb.PeerEventType_PEER_EVENT_TYPE_EOS},
		{"next", eventpb.PeerEventType_PEER_EVENT_TYPE_BOS},
		{"next", eventpb.PeerEventType_PEER_EVENT_TYPE_EOS},
	} {
		event, err := readPeerStreamEvent(&events)
		if err != nil || event.StreamID() != want.stream || event.Type != want.kind {
			t.Fatalf("Peer boundary = %+v %v, want %s/%s", event, err, want.stream, want.kind)
		}
	}
	if _, err := readPeerStreamEvent(&events); err != io.EOF {
		t.Fatalf("duplicate public terminal: %v", err)
	}
}

func TestPeerAudioCompletedRouteStillRejectsMalformedTerminals(t *testing.T) {
	audio := newPeerAudioRouteAggregator()
	for _, kind := range []eventpb.PeerEventType{eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, eventpb.PeerEventType_PEER_EVENT_TYPE_EOS} {
		if _, err := audio.consume(peerOutputAudioBoundary(kind, "finished", "assistant", "audio/opus")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := audio.consume(peerOutputAudioBoundary(eventpb.PeerEventType_PEER_EVENT_TYPE_EOS, "finished", "assistant", "audio/opus")); err == nil {
		t.Fatal("ordinary duplicate EOS was accepted")
	}
	unknown := lateTerminalAudio("unknown", false, true, "interrupted")
	if _, err := audio.consumeChunk(unknown, peerStreamEventsFromChunk(unknown)[0]); err == nil {
		t.Fatal("interruption of an unknown route was accepted")
	}
	mismatch := lateTerminalAudio("finished", false, true, "interrupted")
	mismatch.Part = &genx.Blob{MIMEType: "audio/ogg"}
	if _, err := audio.consumeChunk(mismatch, peerStreamEventsFromChunk(mismatch)[0]); err == nil {
		t.Fatal("interruption with a different MIME was accepted")
	}
}

func TestPeerAudioCompletedRouteHistoryIsBounded(t *testing.T) {
	audio := newPeerAudioRouteAggregator()
	for i := range maxCompletedPeerAudioRoutes + 1 {
		for _, kind := range []eventpb.PeerEventType{eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, eventpb.PeerEventType_PEER_EVENT_TYPE_EOS} {
			if _, err := audio.consume(peerOutputAudioBoundary(kind, fmt.Sprintf("route-%d", i), "assistant", "audio/opus")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(audio.completed) > maxCompletedPeerAudioRoutes || len(audio.completedOrder) > maxCompletedPeerAudioRoutes {
		t.Fatal("completed route identities grew beyond the connection bound")
	}
	for _, test := range []struct {
		stream string
		known  bool
	}{{"route-0", false}, {fmt.Sprintf("route-%d", maxCompletedPeerAudioRoutes), true}} {
		chunk := lateTerminalAudio(test.stream, false, true, "interrupted")
		emit, err := audio.consumeChunk(chunk, peerStreamEventsFromChunk(chunk)[0])
		if (err == nil) != test.known || emit {
			t.Fatalf("bounded terminal recognition for %s: emit=%v err=%v", test.stream, emit, err)
		}
	}
}
