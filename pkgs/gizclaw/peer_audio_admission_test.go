package gizclaw

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/opusmedia"
	"github.com/pion/rtp"
)

type peerConnRTPPacketConn struct {
	peerConnPacketConn
	infos       []*packet.OpusInfo
	packetReads int
}

func (c *peerConnRTPPacketConn) RTPAudio() bool { return true }

func (c *peerConnRTPPacketConn) ReadPacket(ctx context.Context, buf []byte) (byte, int, *packet.OpusInfo, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, nil, err
	}
	c.packetReads++
	if len(c.packets) == 0 {
		return 0, 0, nil, giznet.ErrClosed
	}
	p, info := c.packets[0], c.infos[0]
	c.packets, c.infos = c.packets[1:], c.infos[1:]
	return p.protocol, copy(buf, p.payload), info.Clone(), nil
}

func newPeerConnRTPPackets() *peerConnRTPPacketConn {
	conn := &peerConnRTPPacketConn{}
	for index := range 2 {
		conn.packets = append(conn.packets, peerConnTestPacket{protocol: giznet.ProtocolOpusPacket, payload: []byte{0xf8, 0xff, 0xfe}})
		conn.infos = append(conn.infos, &packet.OpusInfo{SourceID: "source", Header: rtp.Header{Version: 2, SequenceNumber: 65535 + uint16(index), Timestamp: ^uint32(0) - 959 + uint32(index)*960, SSRC: 77}, Duration48: 960})
	}
	return conn
}

func TestPeerConnRawAudioAdmissionBeforeNativeDecode(t *testing.T) {
	for name, setup := range map[string]func(*PeerConn){
		"no-BOS": func(h *PeerConn) { h.acceptedAudioInput = false },
		"denied": func(h *PeerConn) {
			h.acceptedAudioSFU = true
			h.markDeniedInputStream("turn", eventpb.StreamKind_STREAM_KIND_AUDIO)
		},
		"expired": func(h *PeerConn) {
			h.permissions.valid = true
			h.permissions.value.checkedAt = time.Now().Add(-peerPermissionMaxAge - time.Second)
		},
		"revoked": func(h *PeerConn) {
			h.permissions.valid = true
			h.permissions.value = peerInputPermission{checkedAt: time.Now(), denial: &inputAccessError{}}
		},
		"retiring": func(h *PeerConn) { h.retiring.Store(true) },
	} {
		t.Run(name, func(t *testing.T) {
			peer := &PeerConn{acceptedAudioInput: true, acceptedAudioStream: "turn"}
			setup(peer)
			conn := newPeerConnRTPPackets()
			conn.packets = append(conn.packets, peerConnTestPacket{protocol: 0x42, payload: []byte{9}})
			conn.infos = append(conn.infos, nil)
			media, err := opusmedia.NewPacketReader(peer.admitAudioPackets(conn.ReadPacket), opusmedia.Config{})
			if err != nil {
				t.Fatal(err)
			}
			defer media.Close()
			buf := make([]byte, 64)
			protocol, n, frame, err := media.Read(t.Context(), buf)
			if err != nil || protocol != 0x42 || n != 1 || buf[0] != 9 || frame != nil {
				t.Fatalf("denied RTP reached the media receiver: protocol=%x n=%d frame=%+v error=%v", protocol, n, frame, err)
			}
			if _, _, _, err := media.Read(t.Context(), buf); !errors.Is(err, giznet.ErrClosed) {
				t.Fatalf("raw terminal error changed: %v", err)
			}
			if conn.packetReads != 4 || conn.reads != 0 {
				t.Fatalf("RTP/legacy reads=%d/%d", conn.packetReads, conn.reads)
			}
			if name == "denied" && peer.sfuDroppedPackets.Load() != 2 {
				t.Fatalf("SFU rejected packet count=%d", peer.sfuDroppedPackets.Load())
			}
		})
	}
}

func TestPeerConnRawAudioAdmissionPreservesAuthorizedRTP(t *testing.T) {
	peer := &PeerConn{acceptedAudioInput: true, acceptedAudioStream: "turn"}
	conn := newPeerConnRTPPackets()
	read := peer.admitAudioPackets(conn.ReadPacket)
	buf := make([]byte, 64)
	for index := range 2 {
		protocol, n, info, err := read(t.Context(), buf)
		if err != nil || protocol != giznet.ProtocolOpusPacket || !bytes.Equal(buf[:n], []byte{0xf8, 0xff, 0xfe}) || info == nil {
			t.Fatalf("authorized packet=%x %x %+v %v", protocol, buf[:n], info, err)
		}
		if info.Header.SequenceNumber != 65535+uint16(index) || info.Header.Timestamp != ^uint32(0)-959+uint32(index)*960 || info.Duration48 != 960 {
			t.Fatalf("RTP clock changed: %+v", info)
		}
	}
}

func TestPeerConnRawAudioAdmissionHonorsCancellation(t *testing.T) {
	peer := &PeerConn{}
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	read := peer.admitAudioPackets(func(context.Context, []byte) (byte, int, *packet.OpusInfo, error) {
		calls++
		if !peer.inputAccessMu.TryLock() {
			t.Fatal("raw I/O ran under the authorization mutex")
		}
		peer.inputAccessMu.Unlock()
		if !peer.permissions.mu.TryLock() {
			t.Fatal("raw I/O ran under the permission snapshot mutex")
		}
		peer.permissions.mu.Unlock()
		if calls == 32 {
			cancel()
		}
		return giznet.ProtocolOpusPacket, 0, nil, nil
	})
	if _, _, _, err := read(ctx, nil); !errors.Is(err, context.Canceled) || calls != 32 {
		t.Fatalf("denied flood ignored cancellation: calls=%d error=%v", calls, err)
	}
	if _, _, _, err := read(ctx, nil); !errors.Is(err, context.Canceled) || calls != 32 {
		t.Fatalf("canceled read reached transport: calls=%d error=%v", calls, err)
	}
}

func TestPeerConnRawAudioAdmissionAllowsControlInterrupt(t *testing.T) {
	peer := &PeerConn{}
	entered := make(chan struct{}, 1)
	first := true
	media, err := opusmedia.NewPacketReader(peer.admitAudioPackets(func(ctx context.Context, buf []byte) (byte, int, *packet.OpusInfo, error) {
		if first {
			first = false
			buf[0] = 0xf8
			return giznet.ProtocolOpusPacket, 1, &packet.OpusInfo{Header: rtp.Header{Version: 2}, Duration48: 960}, nil
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return 0, 0, nil, ctx.Err()
	}), opusmedia.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, _, err := media.Read(ctx, make([]byte, 64))
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("packet pump did not continue past denied media")
	}
	control, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()
	if err := media.Discard(control); err != nil {
		t.Fatalf("BOS discard deadlocked with raw admission: %v", err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("packet pump did not stop")
	}
	if err := media.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPeerConnRTPAudioDeniedDoesNotRequireDecoder(t *testing.T) {
	for _, denied := range []bool{false, true} {
		name := "no-BOS"
		if denied {
			name = "denied"
		}
		t.Run(name, func(t *testing.T) {
			conn := newPeerConnRTPPackets()
			conn.packets = append(conn.packets, peerConnTestPacket{protocol: 0x42, payload: []byte{9}})
			conn.infos = append(conn.infos, nil)
			input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 1)}
			peer := &PeerConn{Conn: conn, agentInput: input}
			if denied {
				peer.markDeniedInputStream("turn", eventpb.StreamKind_STREAM_KIND_AUDIO)
			}
			if err := peer.serveDirectPackets(); err != nil {
				t.Fatalf("denied media started a decoder or closed the packet pump: %v", err)
			}
			if len(input.pushed) != 0 || conn.reads != 0 || conn.packetReads != 4 || peer.mediaReader.Load() != nil {
				t.Fatalf("denied media dispatch=%d legacy reads=%d RTP reads=%d reader=%p", len(input.pushed), conn.reads, conn.packetReads, peer.mediaReader.Load())
			}
		})
	}
}

func TestPeerConnRTPAudioNativeInitFailureIsTerminal(t *testing.T) {
	if opus.IsRuntimeSupported() {
		t.Skip("unsupported runtime required to exercise native initialization failure")
	}
	peer := &PeerConn{Conn: newPeerConnRTPPackets()}
	peer.acceptInputEvent(audioBOS("turn"), "turn", "", false, 0)
	if err := peer.serveDirectPackets(); !errors.Is(err, codecconv.ErrOpusDecoderInit) {
		t.Fatalf("authorized native startup failure was swallowed: %v", err)
	}
}

func TestPeerConnRTPAudioDispatchesPCMWithOriginal(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native Opus required for accepted media")
	}
	conn := newPeerConnRTPPackets()
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 2)}
	peer := &PeerConn{Conn: conn, agentInput: input}
	peer.acceptInputEvent(audioBOS("turn"), "turn", "", false, 0)
	if err := peer.serveDirectPackets(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case chunk := <-input.pushed:
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok || blob.MIMEType != "audio/pcm;rate=16000;channels=1" || len(blob.Data) != 640 || blob.OriginalAudio == nil || !bytes.Equal(blob.OriginalAudio.Data, []byte{0xf8, 0xff, 0xfe}) || blob.ConcealedAudio {
				t.Fatalf("decoded dispatch=%+v", chunk)
			}
		default:
			t.Fatal("authorized RTP did not reach PCM input")
		}
	}
	if conn.reads != 0 || conn.packetReads != 3 {
		t.Fatalf("RTP/legacy reads=%d/%d", conn.packetReads, conn.reads)
	}
}
