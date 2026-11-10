package sfu

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/opusmedia"
	"github.com/pion/rtp"
)

func TestSessionPreservesTimingAcrossUplinkAndDownlink(t *testing.T) {
	h := newHarness(t, Config{FloorIdle: time.Second, TalkHangover: time.Second})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, input := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	input.push(inputBOS("in"))
	for _, item := range []struct {
		seq uint16
		ts  uint32
	}{{10, 0}, {12, 3840}} {
		frame := opusmedia.EncodedRTP(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: item.seq, Timestamp: item.ts, SSRC: 77}, Payload: []byte{0x78, 0x01}}, "device")
		input.push((&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: frame.Data, AudioTiming: frame.Timing}}).Clone())
	}
	waitFor(t, func() bool { client.mu.Lock(); defer client.mu.Unlock(); return len(client.wire) == 2 }, "timed uplink")
	client.mu.Lock()
	first, second := client.wire[0].Clone(), client.wire[1].Clone()
	client.mu.Unlock()
	if second.SequenceNumber-first.SequenceNumber != 2 || second.Timestamp-first.Timestamp != 3840 {
		t.Fatal("SFU input stripped known missing time")
	}
	input.push(inputEOS("in"))
	waitFor(t, func() bool { return !h.talking(testPeer) }, "uplink EOS")
	reader := newFakeReader()
	client.events.onTrackSubscribed(testRemote, "track", reader)
	client.remoteTalk(testRemote, talkTypeBOS, "remote", 1)
	reader.packets <- first
	reader.packets <- second
	waitFor(t, func() bool { return h.queued(testPeer) >= 3 }, "timed downlink idle flush")
	cancel()
	chunks, err := collect(t, output)
	if !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	var wire []*rtp.Packet
	writer := opusmedia.NewWriter(testWireWriter(func(p *rtp.Packet) error { wire = append(wire, p.Clone()); return nil }))
	for _, chunk := range chunks {
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || len(blob.Data) == 0 {
			continue
		}
		if blob.AudioTiming == nil {
			t.Fatal("SFU output stripped timing")
		}
		if err := writer.Write(t.Context(), blob.Data, blob.AudioTiming, chunk.Ctrl.StreamID); err != nil {
			t.Fatal(err)
		}
	}
	if len(wire) != 2 || wire[1].SequenceNumber-wire[0].SequenceNumber != 2 || wire[1].Timestamp-wire[0].Timestamp != 3840 {
		t.Fatalf("device received shortened audio: %+v", wire)
	}
}
