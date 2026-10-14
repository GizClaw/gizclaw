package gizclaw

import (
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
)

func TestRetiringPeerDrainsEventsUntilDeleteAcknowledgementClosesTransport(t *testing.T) {
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 2)}
	peer := &PeerConn{agentInput: input}
	peer.retiring.Store(true)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	done := make(chan error, 1)
	go func() { done <- peer.readEventStream(server) }()
	if err := client.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []eventpb.PeerEventType{eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, eventpb.PeerEventType_PEER_EVENT_TYPE_EOS} {
		if err := writePeerStreamEvent(client, peerInputEvent(kind, "late-audio", nil)); err != nil {
			t.Fatalf("retiring Peer did not drain an in-flight event: %v", err)
		}
	}
	select {
	case err := <-done:
		t.Fatalf("Event reader ended before the delete RPC's terminal close: %v", err)
	case <-input.pushed:
		t.Fatal("retiring Peer accepted new Agent input")
	case <-time.After(25 * time.Millisecond):
	}
	// The RPC owner closes the association after its response has been consumed.
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Event reader did not finish after terminal transport close")
	}
}
