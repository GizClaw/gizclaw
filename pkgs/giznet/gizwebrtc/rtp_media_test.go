package gizwebrtc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/rtp"
	"github.com/pion/srtp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

type testRTPWriter func(*rtp.Packet) error

func (f testRTPWriter) WriteRTP(p *rtp.Packet) error { return f(p) }

func TestRTPMediaReadPreservesTimingAndOwnsPayload(t *testing.T) {
	conn := &Conn{pc: &webrtc.PeerConnection{}, readCh: make(chan directPacket, 2), closeCh: make(chan struct{})}
	original := &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: 65535, Timestamp: ^uint32(0) - 959, SSRC: 77, CSRC: []uint32{9}}, Payload: []byte{0xf8, 0xff, 0xfe}}
	conn.enqueueRemoteOpusPacket(original, "session/source")
	original.Payload[0] = 0
	original.CSRC[0] = 0
	buf := make([]byte, 64)
	protocol, n, info, err := conn.ReadPacket(t.Context(), buf)
	if err != nil || protocol != giznet.ProtocolOpusPacket || !bytes.Equal(buf[:n], []byte{0xf8, 0xff, 0xfe}) || info == nil {
		t.Fatalf("read=%x,%d,%+v,%v", protocol, n, info, err)
	}
	if info.Header.SequenceNumber != 65535 || info.Header.Timestamp != ^uint32(0)-959 || info.Header.CSRC[0] != 9 || info.Duration48 != 960 {
		t.Fatal("media clock or header ownership lost")
	}
	// A later enqueue of the same caller-owned header must be independent of
	// both the caller's subsequent mutation and the previously returned header.
	original.Payload = []byte{0xf8, 0xff, 0xfe}
	original.SequenceNumber = 0
	original.CSRC[0] = 11
	conn.enqueueRemoteOpusPacket(original, "session/source")
	original.CSRC[0] = 0
	info.Header.CSRC[0] = 55
	_, _, next, err := conn.ReadPacket(t.Context(), buf)
	if err != nil || next == nil || next.Header.SequenceNumber != 0 || next.Header.CSRC[0] != 11 || info.Header.CSRC[0] != 55 {
		t.Fatalf("dequeued media metadata shared storage: next=%+v error=%v", next, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, _, err := conn.ReadPacket(ctx, buf); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	conn.enqueueRemoteOpusPacket(&rtp.Packet{Header: rtp.Header{Version: 2}, Payload: []byte{0xf8, 0xff, 0xfe}}, "session/source")
	if _, _, _, err := conn.ReadPacket(t.Context(), buf); err != nil {
		t.Fatalf("cancel closed the connection: %v", err)
	}
}

func TestRTPMediaForwardingKeepsGapsAndStripsHopExtensions(t *testing.T) {
	var writes []*rtp.Packet
	conn := &Conn{pc: &webrtc.PeerConnection{}, closeCh: make(chan struct{}), audioRTPTrack: testRTPWriter(func(p *rtp.Packet) error { writes = append(writes, p); return nil })}
	info := &packet.OpusInfo{Header: rtp.Header{Version: 2, SequenceNumber: 10, Timestamp: 960, SSRC: 77, CSRC: []uint32{9}}, Duration48: 960}
	if err := info.Header.SetExtension(3, []byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	for _, seq := range []uint16{10, 12} {
		info.Header.SequenceNumber = seq
		info.Header.Timestamp = uint32(seq-9) * 960
		if _, err := conn.WritePacket(giznet.ProtocolOpusPacket, []byte{0xf8, 0xff, 0xfe}, info); err != nil {
			t.Fatal(err)
		}
	}
	if len(writes) != 2 || writes[0].SequenceNumber != 10 || writes[1].SequenceNumber != 12 || writes[1].Timestamp-writes[0].Timestamp != 1920 {
		t.Fatal("relay regenerated the original RTP timeline")
	}
	if writes[0].Extension || len(writes[0].CSRC) != 0 || !info.Header.Extension || len(info.Header.CSRC) != 1 {
		t.Fatal("hop headers leaked or caller header was mutated")
	}
	info.Missing = true
	if _, err := conn.WritePacket(giznet.ProtocolOpusPacket, nil, info); err != nil || len(writes) != 2 {
		t.Fatal("PLC generated a fake encoded RTP packet")
	}
}

func TestOpusSampleTimelineDoesNotHoldStateDuringTransportIO(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	packets := make(chan *rtp.Packet, 2)
	writer := &opusSampleWriter{seq: 65535, ts: ^uint32(0) - 959}
	writer.track = testRTPWriter(func(p *rtp.Packet) error {
		if p.SequenceNumber == 0 {
			close(entered)
			<-release
		}
		packets <- p
		return nil
	})
	first := make(chan error, 1)
	go func() {
		first <- writer.WriteSample(media.Sample{Data: []byte{1}, Duration: 20 * time.Millisecond, PrevDroppedPackets: 1})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first write did not start")
	}
	second := make(chan error, 1)
	go func() { second <- writer.WriteSample(media.Sample{Data: []byte{2}, Duration: 20 * time.Millisecond}) }()
	select {
	case err := <-second:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("transport I/O held the sequence-state mutex")
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	later, earlier := <-packets, <-packets
	if earlier.SequenceNumber != 0 || earlier.Timestamp != 0 || later.SequenceNumber != 1 || later.Timestamp != 960 {
		t.Fatal("sequence/timestamp wrap or dropped-packet clock is incorrect")
	}
}

func TestRTPMediaForwardingStripsPaddingAcrossSRTP(t *testing.T) {
	for _, padding := range []byte{1, 4, 255} {
		t.Run(fmt.Sprint(padding), func(t *testing.T) {
			key, salt := make([]byte, 16), make([]byte, 12)
			encryptor, err := srtp.CreateContext(key, salt, srtp.ProtectionProfileAeadAes128Gcm)
			if err != nil {
				t.Fatal(err)
			}
			decryptor, err := srtp.CreateContext(key, salt, srtp.ProtectionProfileAeadAes128Gcm)
			if err != nil {
				t.Fatal(err)
			}
			original := []byte{0xf8, 0xff, 0xfe}
			conn := &Conn{pc: &webrtc.PeerConnection{}, closeCh: make(chan struct{})}
			conn.audioRTPTrack = testRTPWriter(func(p *rtp.Packet) error {
				// Match WriteStreamSRTP's header/payload serialization, including
				// a dirty reusable buffer so stale bytes cannot hide in zeroes.
				_, size := rtp.HeaderAndPacketMarshalSize(&p.Header, p.Payload)
				wire := bytes.Repeat([]byte{0xa5}, size)
				if _, err := rtp.MarshalPacketTo(wire, &p.Header, p.Payload); err != nil {
					return err
				}
				encrypted, err := encryptor.EncryptRTP(nil, wire, nil)
				if err != nil {
					return err
				}
				plain, err := decryptor.DecryptRTP(nil, encrypted, nil)
				if err != nil {
					return err
				}
				var received rtp.Packet
				if err := received.Unmarshal(plain); err != nil {
					return err
				}
				if p.Header.Padding || p.Header.PaddingSize != 0 || received.Padding || !bytes.Equal(received.Payload, original) || received.SequenceNumber != p.SequenceNumber || received.Timestamp != p.Timestamp {
					t.Fatalf("padding or media changed: header=%+v received=%+v payload=%x", p.Header, received.Header, received.Payload)
				}
				return nil
			})
			for index := range 2 {
				incoming := &rtp.Packet{Header: rtp.Header{Version: 2, Padding: true, PaddingSize: padding, PayloadType: 111, SSRC: 77, SequenceNumber: 65535 + uint16(index), Timestamp: ^uint32(0) - 959 + uint32(index)*960}, Payload: original}
				wire, err := incoming.Marshal()
				if err != nil {
					t.Fatal(err)
				}
				var received rtp.Packet
				if err := received.Unmarshal(wire); err != nil {
					t.Fatal(err)
				}
				info := &packet.OpusInfo{Header: received.Header.Clone(), Duration48: 960}
				if _, err := conn.WritePacket(giznet.ProtocolOpusPacket, received.Payload, info); err != nil {
					t.Fatal(err)
				}
				if !info.Header.Padding || info.Header.PaddingSize != padding || !bytes.Equal(received.Payload, original) {
					t.Fatal("forwarding mutated borrowed metadata or payload")
				}
			}
		})
	}
}

func TestRTPMediaInvalidDurationKeepsReliableService(t *testing.T) {
	conn := &Conn{pc: &webrtc.PeerConnection{}, readCh: make(chan directPacket, 1), closeCh: make(chan struct{})}
	service := newServiceListener(conn, 3)
	defer service.Close()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	if err := service.enqueue(server); err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{nil, {0xfb}, {0xfb, 0}, {0xfb, 7}, {0x1b, 3}} {
		conn.enqueueRemoteOpusPacket(&rtp.Packet{Header: rtp.Header{Version: 2}, Payload: payload}, "source")
		if len(conn.readCh) != 0 {
			t.Fatalf("impossible Opus duration entered relay: %x", payload)
		}
	}
	accepted, err := service.Accept()
	if err != nil {
		t.Fatalf("invalid media closed reliable service: %v", err)
	}
	if err := accepted.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	write := make(chan error, 1)
	go func() {
		_, err := client.Write([]byte("RPC still open"))
		write <- err
	}()
	buf := make([]byte, len("RPC still open"))
	if _, err := io.ReadFull(accepted, buf); err != nil || string(buf) != "RPC still open" {
		t.Fatalf("reliable service payload=%q error=%v", buf, err)
	}
	if err := <-write; err != nil {
		t.Fatal(err)
	}
	conn.enqueueRemoteOpusPacket(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: 12, Timestamp: 960}, Payload: []byte{0xf8, 0xff, 0xfe}}, "source")
	protocol, n, info, err := conn.ReadPacket(t.Context(), make([]byte, 64))
	if err != nil || protocol != giznet.ProtocolOpusPacket || n != 3 || info.Duration48 != 960 || info.Header.SequenceNumber != 12 || info.Header.Timestamp != 960 {
		t.Fatalf("valid media did not recover: protocol=%x n=%d info=%+v error=%v", protocol, n, info, err)
	}
}
