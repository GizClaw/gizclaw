package sfu

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/opusmedia"
	"github.com/pion/rtp"
)

func TestOutputEvictionRetainsMissingMediaTime(t *testing.T) {
	queue := newOutputStream()
	payload := []byte{0xf8, 0xff, 0xfe}
	var wire []*rtp.Packet
	writer := opusmedia.NewWriter(testWireWriter(func(p *rtp.Packet) error { wire = append(wire, p.Clone()); return nil }))
	makeChunk := func(seq uint16) *genx.MessageChunk {
		timing := packet.NewOpusTiming(&packet.OpusInfo{SourceID: "speaker", Header: rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: uint32(seq) * 960}, Duration48: 960})
		return &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: payload, AudioTiming: timing}, Ctrl: &genx.StreamCtrl{StreamID: "floor"}}
	}
	first := makeChunk(0).Part.(*genx.Blob)
	if err := writer.Write(t.Context(), first.Data, first.AudioTiming, "floor"); err != nil {
		t.Fatal(err)
	}
	for seq := 1; seq <= maxQueuedOutputChunks+1; seq++ {
		queue.push(makeChunk(uint16(seq)))
	}
	chunk, err := queue.Next()
	if err != nil {
		t.Fatal(err)
	}
	next := chunk.Part.(*genx.Blob)
	if err := writer.Write(t.Context(), next.Data, next.AudioTiming, chunk.Ctrl.StreamID); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 2 || wire[1].SequenceNumber-wire[0].SequenceNumber != 2 || wire[1].Timestamp-wire[0].Timestamp != 1920 {
		t.Fatal("output eviction collapsed missing media time")
	}
	_ = queue.Close()
}
