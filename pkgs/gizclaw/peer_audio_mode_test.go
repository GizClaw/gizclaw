package gizclaw

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/opusmedia"
	"github.com/pion/rtp"
)

type peerAudioModePacket struct {
	payload []byte
	info    *packet.OpusInfo
}

type peerAudioModeConn struct {
	testGiznetConn
	packets chan peerAudioModePacket
	entered chan struct{}
	closed  chan struct{}
	once    sync.Once
	client  net.Conn
	server  net.Conn
}

func newPeerAudioModeConn(t *testing.T) *peerAudioModeConn {
	t.Helper()
	client, server := net.Pipe()
	conn := &peerAudioModeConn{packets: make(chan peerAudioModePacket, 8), entered: make(chan struct{}, 8), closed: make(chan struct{}), client: client, server: server}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func (c *peerAudioModeConn) RTPAudio() bool { return true }
func (c *peerAudioModeConn) ReadPacket(ctx context.Context, buf []byte) (byte, int, *packet.OpusInfo, error) {
	select {
	case c.entered <- struct{}{}:
	default:
	}
	select {
	case p := <-c.packets:
		return giznet.ProtocolOpusPacket, copy(buf, p.payload), p.info.Clone(), nil
	case <-ctx.Done():
		return 0, 0, nil, ctx.Err()
	case <-c.closed:
		return 0, 0, nil, giznet.ErrConnClosed
	}
}
func (c *peerAudioModeConn) Dial(uint64) (net.Conn, error) {
	select {
	case <-c.closed:
		return nil, giznet.ErrConnClosed
	default:
		return c.client, nil
	}
}
func (c *peerAudioModeConn) Close() error {
	c.once.Do(func() {
		close(c.closed)
		_ = c.client.Close()
		_ = c.server.Close()
	})
	return nil
}

func peerAudioModePacketAt(seq uint16, timestamp uint32) peerAudioModePacket {
	return peerAudioModePacket{payload: []byte{0xf8, 0xff, 0xfe}, info: &packet.OpusInfo{SourceID: "s", Header: rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: timestamp, SSRC: 77}, Duration48: 960}}
}

func peerAudioModeInput(t *testing.T, input *countingPeerAgentInput) *genx.MessageChunk {
	t.Helper()
	select {
	case chunk := <-input.pushed:
		return chunk
	case <-time.After(3 * time.Second):
		t.Fatal("audio input did not advance")
		return nil
	}
}

func startPeerAudioModePump(t *testing.T, peer *PeerConn, conn *peerAudioModeConn) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		defer peer.close()
		done <- peer.serveDirectPackets()
	}()
	t.Cleanup(func() {
		_ = peer.close()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("audio packet pump terminated: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("audio packet pump leaked")
		}
	})
	select {
	case <-conn.entered:
	case <-time.After(time.Second):
		t.Fatal("audio packet pump did not start")
	}
}

func startPeerAudioModeEvents(t *testing.T, peer *PeerConn) net.Conn {
	t.Helper()
	server, client := net.Pipe()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	done := make(chan error, 1)
	go func() { done <- peer.readEventStream(server) }()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, net.ErrClosed) {
				t.Errorf("event reader terminated: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("event reader leaked")
		}
	})
	return client
}

func assertPeerAudioModeService(t *testing.T, conn *peerAudioModeConn) {
	t.Helper()
	stream, err := conn.Dial(ServicePeerRPC)
	if err != nil {
		t.Fatalf("reliable service closed: %v", err)
	}
	_ = stream.SetDeadline(time.Now().Add(time.Second))
	_ = conn.server.SetDeadline(time.Now().Add(time.Second))
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn.server, buf); err != nil {
			done <- err
			return
		}
		_, err := conn.server.Write(buf)
		done <- err
	}()
	if _, err := stream.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(stream, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("reliable service roundtrip=%q,%v", buf, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPeerConnSFUEncodedBOSBeforePacketPump(t *testing.T) {
	conn := newPeerAudioModeConn(t)
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 8)}
	broker := newPeerStreamEventBroker()
	var output peerStreamLockedBuffer
	unsubscribe, err := broker.Subscribe(&output)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	peer := &PeerConn{Conn: conn, Service: &PeerService{manager: &Manager{}}, agentInput: input, events: broker}
	peer.permissions.valid = true
	peer.permissions.value = peerInputPermission{checkedAt: time.Now(), isSFU: true}
	events := startPeerAudioModeEvents(t, peer)
	if err := writePeerStreamEvent(events, audioBOS("sfu")); err != nil {
		t.Fatal(err)
	}
	bos := peerAudioModeInput(t, input)
	if !bos.IsBeginOfStream() || bos.Part.(*genx.Blob).MIMEType != "audio/opus" || peer.mediaReader.Load() != nil {
		t.Fatalf("SFU BOS did not select encoded output before pump installation: %+v", bos)
	}
	startPeerAudioModePump(t, peer, conn)
	if peer.mediaReader.Load().OutputMode() != opusmedia.OutputEncoded {
		t.Fatal("packet reader installed an obsolete PCM mode")
	}
	conn.packets <- peerAudioModePacketAt(12, 1920)
	conn.packets <- peerAudioModePacketAt(10, 0)
	for _, seq := range []uint16{10, 12} {
		blob := peerAudioModeInput(t, input).Part.(*genx.Blob)
		info := blob.AudioTiming.MediaInfo()
		if info == nil || info.Header.SequenceNumber != seq || info.Header.Timestamp != uint32(seq-10)*960 {
			t.Fatalf("encoded SFU input lost timing: %+v", info)
		}
		if blob.MIMEType != "audio/opus" || !bytes.Equal(blob.Data, []byte{0xf8, 0xff, 0xfe}) || blob.ConcealedAudio || blob.OriginalAudio != nil {
			t.Fatalf("encoded SFU output changed format or synthesized a packet: %+v", blob)
		}
	}
	assertPeerAudioModeService(t, conn)
	if err := writePeerStreamEvent(events, audioEndEvent("sfu")); err != nil {
		t.Fatal(err)
	}
	eos := peerAudioModeInput(t, input)
	if !eos.IsEndOfStream() || eos.Part.(*genx.Blob).MIMEType != "audio/opus" || peer.mediaReader.Load().OutputMode() != opusmedia.OutputEncoded {
		t.Fatalf("SFU EOS changed the active route format: %+v", eos)
	}
	if opus.IsRuntimeSupported() {
		// Switching to a normal Workspace restores PCM before its BOS, not on
		// the first packet, and keeps that format after admission ends at EOS.
		peer.permissions.mu.Lock()
		peer.permissions.value.isSFU = false
		peer.permissions.mu.Unlock()
		if err := writePeerStreamEvent(events, audioBOS("pcm")); err != nil {
			t.Fatal(err)
		}
		bos := peerAudioModeInput(t, input)
		if bos.Part.(*genx.Blob).MIMEType != "audio/pcm;rate=16000;channels=1" || peer.mediaReader.Load().OutputMode() != opusmedia.OutputPCM {
			t.Fatal("normal route did not restore PCM before BOS")
		}
		conn.packets <- peerAudioModePacketAt(13, 2880)
		blob := peerAudioModeInput(t, input).Part.(*genx.Blob)
		if blob.MIMEType != "audio/pcm;rate=16000;channels=1" || len(blob.Data) != 640 || blob.OriginalAudio == nil || blob.ConcealedAudio {
			t.Fatalf("normal route lost native PCM/provenance: %+v", blob)
		}
		if err := writePeerStreamEvent(events, audioEndEvent("pcm")); err != nil {
			t.Fatal(err)
		}
		if eos := peerAudioModeInput(t, input); !eos.IsEndOfStream() || eos.Part.(*genx.Blob).MIMEType != "audio/pcm;rate=16000;channels=1" {
			t.Fatalf("normal route EOS changed MIME: %+v", eos)
		}
	}
}

func TestPeerConnUnsupportedPCMRouteKeepsReliableServices(t *testing.T) {
	if opus.IsRuntimeSupported() {
		t.Skip("requires unavailable native runtime")
	}
	conn := newPeerAudioModeConn(t)
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 8)}
	broker := newPeerStreamEventBroker()
	var output peerStreamLockedBuffer
	unsubscribe, err := broker.Subscribe(&output)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	peer := &PeerConn{Conn: conn, Service: &PeerService{manager: &Manager{}}, agentInput: input, events: broker}
	peer.permissions.valid = true
	peer.permissions.value = peerInputPermission{checkedAt: time.Now()}
	startPeerAudioModePump(t, peer, conn)
	events := startPeerAudioModeEvents(t, peer)
	if err := writePeerStreamEvent(events, audioBOS("pcm")); err != nil {
		t.Fatal(err)
	}
	waitForPeerStreamBytes(t, &output)
	terminal := readLockedPeerStreamEvent(t, &output)
	if terminal.Type != eventpb.PeerEventType_PEER_EVENT_TYPE_EOS || terminal.StreamID() != "pcm" || terminal.GetEos().GetError().GetCode() != "AUDIO_DECODER_UNAVAILABLE" {
		t.Fatalf("unsupported PCM did not report a route terminal: %+v", terminal)
	}
	conn.packets <- peerAudioModePacketAt(1, 0)
	if err := writePeerStreamEvent(events, audioEndEvent("pcm")); err != nil {
		t.Fatal(err)
	}
	text := peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA, "text", nil)
	text.GetTextDelta().Text = "after denied audio"
	if err := writePeerStreamEvent(events, text); err != nil {
		t.Fatal(err)
	}
	chunk := peerAudioModeInput(t, input)
	if _, ok := chunk.Part.(genx.Text); !ok {
		t.Fatalf("denied PCM reached the agent or blocked subsequent text: %+v", chunk)
	}
	assertPeerAudioModeService(t, conn)
	if output.Len() != 0 {
		t.Fatal("unsupported PCM route emitted duplicate terminal or readiness")
	}
	if peer.isClosed() {
		t.Fatal("unsupported PCM route closed the Peer")
	}
}
