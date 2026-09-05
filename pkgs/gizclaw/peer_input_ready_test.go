package gizclaw

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// Hold the authorization lookup rather than sleeping: the packet channel can
// make progress while the independent event channel is still accepting BOS.
type inputReadyRunStore struct {
	kv.Store
	entered, release chan struct{}
	once             sync.Once
}

func (s *inputReadyRunStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	s.once.Do(func() { close(s.entered); <-s.release })
	return s.Store.Get(ctx, key)
}

func TestAudioInputReadyPreservesAudioAcrossAuthorization(t *testing.T) {
	for _, waitForReady := range []bool{false, true} {
		name := "without_acknowledgement"
		if waitForReady {
			name = "with_acknowledgement"
		}
		t.Run(name, func(t *testing.T) {
			store := &inputReadyRunStore{Store: kv.NewMemory(nil), entered: make(chan struct{}), release: make(chan struct{})}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(store.release) }) }
			defer release()
			input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 256)}
			conn := &peerConnPacketConn{testGiznetConn: testGiznetConn{publicKey: giznet.PublicKey{9}}}
			for range 200 {
				conn.packets = append(conn.packets, peerConnTestPacket{protocol: giznet.ProtocolOpusPacket, payload: []byte{0xf8, 0xff, 0xfe}})
			}
			peer := &PeerConn{Conn: conn, agentInput: input, events: newPeerStreamEventBroker(), Service: &PeerService{manager: &Manager{PeerRun: &peerrun.Server{Store: store}}}}
			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()
			done := make(chan error, 1)
			go func() { done <- peer.handleEventStream(server) }()
			if err := writePeerStreamEvent(client, audioBOS("delayed-turn")); err != nil {
				t.Fatal(err)
			}
			<-store.entered
			acknowledged := make(chan *eventpb.PeerEvent, 1)
			go func() { event, _ := readPeerStreamEvent(client); acknowledged <- event }()
			select {
			case <-acknowledged:
				t.Fatal("acknowledged before authorization")
			default:
			}
			if !waitForReady {
				if err := peer.serveDirectPackets(); err != nil {
					t.Fatal(err)
				}
			}
			release()
			select {
			case event := <-acknowledged:
				if event.GetAudioInputReady().GetStreamId() != "delayed-turn" {
					t.Fatalf("unexpected acknowledgement: %v", event)
				}
				if len(input.pushed) != 1 {
					t.Fatal("acknowledged before input accepted BOS")
				}
			case <-time.After(time.Second):
				t.Fatal("missing input acknowledgement")
			}
			if waitForReady {
				if err := peer.serveDirectPackets(); err != nil {
					t.Fatal(err)
				}
			}
			eos := &eventpb.PeerEvent{Version: eventpb.Version, Type: eventpb.PeerEventType_PEER_EVENT_TYPE_EOS, Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{StreamId: "delayed-turn", Kind: eventpb.StreamKind_STREAM_KIND_AUDIO}}}
			if err := writePeerStreamEvent(client, eos); err != nil {
				t.Fatal(err)
			}
			_ = client.Close()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("event loop did not finish")
			}
			audio, boundaries := 0, 0
			for len(input.pushed) > 0 {
				chunk := <-input.pushed
				if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
					audio++
				}
				if chunk.IsBeginOfStream() || chunk.IsEndOfStream() {
					boundaries++
				}
			}
			want := 0
			if waitForReady {
				want = 200
			}
			if audio != want || boundaries != 2 {
				t.Fatalf("audio=%d boundaries=%d, want %d and 2", audio, boundaries, want)
			}
			t.Logf("sent=200 received=%d boundaries=%d", audio, boundaries)
		})
	}
}
