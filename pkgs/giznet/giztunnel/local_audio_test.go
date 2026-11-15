package giztunnel

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/rtp"
)

func TestLocalAudioSharesTunnelClockAndPreservesGap(t *testing.T) {
	pair := newMemRouters(t, Config{}, Config{})
	declaration := newMemDeclaration(t)
	declaration.RTPAudio = true
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	accepted := make(chan error, 1)
	go func() { _, _, err := pair.serverRouter.Accept(ctx); accepted <- err }()
	edge, err := pair.edgeRouter.Dial(ctx, declaration)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-accepted; err != nil {
		t.Fatal(err)
	}
	payload := []byte{0xf8, 0xff, 0xfe}
	if _, err := edge.Write(giznet.ProtocolOpusPacket, payload); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		seq uint16
		ts  uint32
	}{{10, 0}, {12, 3840}} {
		timing := packet.NewOpusTiming(&packet.OpusInfo{SourceID: "speaker", Header: rtp.Header{Version: 2, SequenceNumber: item.seq, Timestamp: item.ts}, Duration48: 960})
		if _, err := edge.WriteLocalAudio(ctx, payload, timing, "floor"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := edge.Write(giznet.ProtocolOpusPacket, payload); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 65536)
	for i, want := range []struct {
		seq    uint16
		ts     uint32
		marker bool
	}{{0, 0, true}, {1, 960, true}, {3, 4800, false}, {4, 5760, true}} {
		_, n, err := pair.serverPhysical.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		_, _, info, err := decodeRTPPacket(buf[:n])
		if err != nil {
			t.Fatal(err)
		}
		if info.Header.SequenceNumber != want.seq || info.Header.Timestamp != want.ts || info.Header.Marker != want.marker {
			t.Fatalf("packet %d = %+v", i, info)
		}
	}
}
