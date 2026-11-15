package giztunnel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/rtp"
)

func TestRTPAudioDeclarationAndMediaRoundTrip(t *testing.T) {
	pair := newMemRouters(t, Config{}, Config{})
	declaration := newMemDeclaration(t)
	declaration.RTPAudio = true
	label, err := controlLabel(declaration)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseLabel(label)
	if err != nil || !parsed.rtpAudio {
		t.Fatalf("RTP control declaration = %+v, %v", parsed, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	type accepted struct {
		conn *Conn
		err  error
	}
	ready := make(chan accepted, 1)
	go func() {
		conn, actual, err := pair.serverRouter.Accept(ctx)
		if err == nil && actual != declaration {
			err = ErrInvalidFrame
		}
		ready <- accepted{conn, err}
	}()
	edge, err := pair.edgeRouter.Dial(ctx, declaration)
	if err != nil {
		t.Fatal(err)
	}
	server := <-ready
	if server.err != nil {
		t.Fatal(server.err)
	}
	payload := []byte{0xf8, 0xff, 0xfe}
	info := &packet.OpusInfo{Header: rtp.Header{Version: 2, SSRC: 77, SequenceNumber: 65535, Timestamp: ^uint32(0) - 959}, Duration48: 960}
	if err := info.Header.SetExtension(3, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := edge.WritePacket(giznet.ProtocolOpusPacket, payload, info); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64*1024)
	protocol, n, err := pair.serverPhysical.Read(buf)
	if err != nil || protocol != giznet.ProtocolTunnelPacket || buf[0] != rtpPacketVersion {
		t.Fatalf("physical RTP frame: %x %d %v", protocol, n, err)
	}
	if err := pair.serverRouter.HandlePacket(buf[:n]); err != nil {
		t.Fatal(err)
	}
	protocol, n, actual, err := server.conn.ReadPacket(ctx, buf)
	if err != nil || protocol != giznet.ProtocolOpusPacket || !bytes.Equal(buf[:n], payload) || actual == nil {
		t.Fatalf("media read: %x %d %+v %v", protocol, n, actual, err)
	}
	if actual.Header.SequenceNumber != info.Header.SequenceNumber || actual.Header.Timestamp != info.Header.Timestamp || actual.Header.SSRC != info.Header.SSRC || !bytes.Equal(actual.Header.GetExtension(3), []byte{1, 2, 3}) {
		t.Fatal("RTP timing or extensions were lost")
	}
	legacy, err := encodeOpusPacket(declaration.SessionID, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := pair.serverRouter.HandlePacket(legacy); !errors.Is(err, ErrInvalidFrame) {
		t.Fatal("RTP session silently accepted unversioned raw audio")
	}
	for index := range uint16(2) {
		if _, err := server.conn.Write(giznet.ProtocolOpusPacket, payload); err != nil {
			t.Fatal(err)
		}
		protocol, n, err = pair.edgePhysical.Read(buf)
		if err != nil || protocol != giznet.ProtocolTunnelPacket {
			t.Fatal(err)
		}
		if err := pair.edgeRouter.HandlePacket(buf[:n]); err != nil {
			t.Fatal(err)
		}
		_, _, timing, err := edge.ReadPacket(ctx, buf)
		if err != nil || timing == nil || timing.Header.SequenceNumber != index || timing.Header.Timestamp != uint32(index)*960 {
			t.Fatalf("generated timeline %+v %v", timing, err)
		}
	}
}

func TestRTPMediaReadOwnsQueuedMetadata(t *testing.T) {
	pair := newMemTunnelPair(t, Config{}, Config{})
	payload := []byte{0xf8, 0xff, 0xfe}
	info := &packet.OpusInfo{SourceID: "source", Header: rtp.Header{Version: 2, SequenceNumber: 1, CSRC: []uint32{7}}, Duration48: 960}
	if err := info.Header.SetExtension(3, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := pair.server.enqueueOpusPacket(payload, info); err != nil {
			t.Fatal(err)
		}
	}
	clear(payload)
	info.Header.CSRC[0] = 99
	info.Header.GetExtension(3)[0] = 99
	buf := make([]byte, 64)
	protocol, n, first, err := pair.server.ReadPacket(t.Context(), buf)
	if err != nil || protocol != giznet.ProtocolOpusPacket || !bytes.Equal(buf[:n], []byte{0xf8, 0xff, 0xfe}) || first == nil {
		t.Fatalf("first media read = %x %d %+v %v", protocol, n, first, err)
	}
	if first.Header.CSRC[0] != 7 || !bytes.Equal(first.Header.GetExtension(3), []byte{1, 2, 3}) {
		t.Fatal("queued media retained borrowed metadata")
	}
	first.Header.CSRC[0] = 55
	first.Header.GetExtension(3)[0] = 55
	_, _, second, err := pair.server.ReadPacket(t.Context(), buf)
	if err != nil || second == nil || second.Header.CSRC[0] != 7 || !bytes.Equal(second.Header.GetExtension(3), []byte{1, 2, 3}) {
		t.Fatalf("returned media metadata shared storage: second=%+v error=%v", second, err)
	}
	if first.Header.CSRC[0] != 55 || first.Header.GetExtension(3)[0] != 55 {
		t.Fatal("later read changed retained metadata")
	}
}

func TestRTPMediaRejectsMalformedAndUndeclaredPackets(t *testing.T) {
	declaration := newMemDeclaration(t)
	info := &packet.OpusInfo{Header: rtp.Header{Version: 2, SSRC: 1}, Duration48: 960}
	encoded, err := encodeRTPPacket(declaration.SessionID, []byte{0xf8, 0xff, 0xfe}, info)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{0, 1, 16, 17, 20, len(encoded) - 3} {
		if _, _, _, err := decodeRTPPacket(encoded[:size]); err == nil {
			t.Fatalf("accepted truncated RTP media length %d", size)
		}
	}
	pair := newMemTunnelPair(t, Config{}, Config{})
	encoded, err = encodeRTPPacket(pair.declaration.SessionID, []byte{0xf8, 0xff, 0xfe}, info)
	if err != nil {
		t.Fatal(err)
	}
	if err := pair.serverRouter.HandlePacket(encoded); !errors.Is(err, ErrInvalidFrame) {
		t.Fatal("legacy session accepted an undeclared RTP format")
	}
}

func TestRTPMediaRoundTripStripsPadding(t *testing.T) {
	for _, padding := range []byte{1, 4, 255} {
		t.Run(fmt.Sprint(padding), func(t *testing.T) {
			original := []byte{0xf8, 0xff, 0xfe}
			incoming := &rtp.Packet{Header: rtp.Header{Version: 2, Padding: true, PaddingSize: padding, SequenceNumber: 65535, Timestamp: ^uint32(0) - 959, SSRC: 77, CSRC: []uint32{9}}, Payload: original}
			if err := incoming.Header.SetExtension(3, []byte{1, 2, 3}); err != nil {
				t.Fatal(err)
			}
			wire, err := incoming.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			var received rtp.Packet
			if err := received.Unmarshal(wire); err != nil {
				t.Fatal(err)
			}
			info := &packet.OpusInfo{Header: received.Header.Clone(), Duration48: 960}
			encoded, err := encodeRTPPacket(SessionID{1}, received.Payload, info)
			if err != nil {
				t.Fatal(err)
			}
			_, payload, actual, err := decodeRTPPacket(encoded)
			if err != nil || !bytes.Equal(payload, original) {
				t.Fatalf("payload=%x error=%v", payload, err)
			}
			if actual.Header.Padding || actual.Header.PaddingSize != 0 || actual.Header.SequenceNumber != 65535 || actual.Header.Timestamp != ^uint32(0)-959 || actual.Header.CSRC[0] != 9 || !bytes.Equal(actual.Header.GetExtension(3), []byte{1, 2, 3}) {
				t.Fatalf("forwarded header=%+v", actual.Header)
			}
			if !info.Header.Padding || info.Header.PaddingSize != padding || !bytes.Equal(received.Payload, original) {
				t.Fatal("encoding mutated borrowed metadata or payload")
			}
		})
	}
}

func TestRTPMediaEnvelopeBounds(t *testing.T) {
	headers := map[string]rtp.Header{
		"plain":  {Version: 2},
		"csrc":   {Version: 2, CSRC: make([]uint32, 15)},
		"opaque": {Version: 2, Extension: true, ExtensionProfile: 0x1234},
	}
	opaque := headers["opaque"]
	if err := opaque.SetExtension(0, []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	headers["opaque"] = opaque
	for name, header := range headers {
		t.Run(name, func(t *testing.T) {
			payload := make([]byte, 64*1024-1-opusHeaderSize-header.MarshalSize()+1)
			payload[0] = 0xf8
			info := &packet.OpusInfo{Header: header, Duration48: 960}
			frame, err := encodeRTPPacket(SessionID{1}, payload[:len(payload)-1], info)
			if err != nil || len(frame) != 64*1024-1 {
				t.Fatalf("maximum envelope length=%d error=%v", len(frame), err)
			}
			_, actual, _, err := decodeRTPPacket(frame)
			if err != nil || !bytes.Equal(actual, payload[:len(payload)-1]) {
				t.Fatalf("maximum envelope round trip: payload=%d error=%v", len(actual), err)
			}
			if _, err := encodeRTPPacket(SessionID{1}, payload, info); !errors.Is(err, giznet.ErrPacketTooLarge) {
				t.Fatalf("oversized envelope error=%v", err)
			}
		})
	}
	payload := make([]byte, 1<<20)
	payload[0] = 0xf8
	info := &packet.OpusInfo{Header: rtp.Header{Version: 2}}
	allocations := testing.AllocsPerRun(10, func() {
		if frame, err := encodeRTPPacket(SessionID{1}, payload, info); frame != nil || !errors.Is(err, giznet.ErrPacketTooLarge) {
			t.Fatalf("oversized payload frame=%d error=%v", len(frame), err)
		}
	})
	if allocations != 0 {
		t.Fatalf("oversized payload allocated %g times before rejection", allocations)
	}
}

func TestRTPMediaHeaderValidationPreservesErrorPrecedence(t *testing.T) {
	for name, header := range map[string]rtp.Header{
		"version":      {Version: 1},
		"payload-type": {Version: 2, PayloadType: 128},
		"csrc-count":   {Version: 2, CSRC: make([]uint32, 16)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := encodeRTPPacket(SessionID{1}, []byte{0xf8, 0xff, 0xfe}, &packet.OpusInfo{Header: header}); !errors.Is(err, ErrInvalidFrame) {
				t.Fatalf("invalid RTP header error=%v", err)
			}
		})
	}
	for _, extensionSize := range []int{3, 65520, 65521} {
		t.Run(fmt.Sprint(extensionSize), func(t *testing.T) {
			header := rtp.Header{Version: 2, Extension: true, ExtensionProfile: 0x1234}
			if err := header.SetExtension(0, make([]byte, extensionSize)); err != nil {
				t.Fatal(err)
			}
			payload := make([]byte, 64*1024)
			payload[0] = 0xf8
			want := giznet.ErrPacketTooLarge
			if extensionSize%4 != 0 {
				want = ErrInvalidFrame
			}
			if _, err := encodeRTPPacket(SessionID{1}, payload, &packet.OpusInfo{Header: header}); !errors.Is(err, want) {
				t.Fatalf("opaque extension size=%d error=%v want=%v", extensionSize, err, want)
			}
		})
	}
	// A parsed RFC8285 header may contain repeated IDs. Preserve them as Pion does.
	wire := []byte{0x90, 111, 0, 1, 0, 0, 3, 0xc0, 0, 0, 0, 77, 0xbe, 0xde, 0, 1, 0x30, 1, 0x30, 2, 0xf8, 0xff, 0xfe}
	var incoming rtp.Packet
	if err := incoming.Unmarshal(wire); err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeRTPPacket(SessionID{1}, incoming.Payload, &packet.OpusInfo{Header: incoming.Header})
	if err != nil || !bytes.Equal(encoded[opusHeaderSize:], wire) {
		t.Fatalf("duplicate extension IDs changed: %x %v", encoded, err)
	}
}
