package gizclaw

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/opusmedia"
	"github.com/pion/rtp"
)

type localTimingConn struct {
	testGiznetConn
	writer *opusmedia.Writer
}

func (c *localTimingConn) WriteLocalAudio(ctx context.Context, data []byte, timing *packet.OpusTiming, segment string) (int, error) {
	if err := c.writer.Write(ctx, data, timing, segment); err != nil {
		return 0, err
	}
	return len(data), nil
}

type localTimingSink func(*rtp.Packet) error

func (f localTimingSink) WriteRTP(p *rtp.Packet) error { return f(p) }

func TestPeerPassthroughHandsTimingAndRouteToMediaWriter(t *testing.T) {
	var wire []*rtp.Packet
	conn := &localTimingConn{writer: opusmedia.NewWriter(localTimingSink(func(p *rtp.Packet) error { wire = append(wire, p.Clone()); return nil }))}
	peer := &PeerConn{Conn: conn}
	output := peerAgentOutput{Packets: peer.writeLocalOpusPacket}
	for _, item := range []struct {
		seq uint16
		ts  uint32
	}{{10, 0}, {12, 3840}} {
		timing := packet.NewOpusTiming(&packet.OpusInfo{SourceID: "speaker", Header: rtp.Header{Version: 2, SequenceNumber: item.seq, Timestamp: item.ts}, Duration48: 960})
		chunk := &genx.MessageChunk{Part: &genx.Blob{MIMEType: agenthost.OpusPassthroughMIME, Data: []byte{0xf8, 0xff, 0xfe}, AudioTiming: timing}, Ctrl: &genx.StreamCtrl{StreamID: "floor-1"}}
		if err := output.writePassthroughContext(t.Context(), chunk); err != nil {
			t.Fatal(err)
		}
	}
	if len(wire) != 2 || wire[1].SequenceNumber-wire[0].SequenceNumber != 2 || wire[1].Timestamp-wire[0].Timestamp != 3840 {
		t.Fatal("Peer output stripped timing")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := peer.writeLocalOpusPacket(ctx, []byte{0xf8, 0xff, 0xfe}, nil, "floor-2"); !errors.Is(err, context.Canceled) || len(wire) != 2 {
		t.Fatal("canceled output forwarded audio")
	}
}
