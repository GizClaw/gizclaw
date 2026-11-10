package gizwebrtc

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media"
)

func TestLocalAudioSharesSampleClockAndRetainsMissingTime(t *testing.T) {
	var writes []*rtp.Packet
	writer := &opusSampleWriter{seq: 100, ts: 1000, track: testRTPWriter(func(p *rtp.Packet) error { writes = append(writes, p.Clone()); return nil })}
	payload := []byte{0xf8, 0xff, 0xfe}
	if err := writer.WriteSample(media.Sample{Data: payload, Duration: 20 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		seq uint16
		ts  uint32
	}{{10, 0}, {12, 3840}} {
		timing := packet.NewOpusTiming(&packet.OpusInfo{SourceID: "speaker", Header: rtp.Header{Version: 2, SequenceNumber: item.seq, Timestamp: item.ts}, Duration48: 960})
		if _, err := writer.WriteLocalAudio(t.Context(), payload, timing, "floor"); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.WriteSample(media.Sample{Data: payload, Duration: 20 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if len(writes) != 4 {
		t.Fatalf("writes=%d", len(writes))
	}
	wantSeq := []uint16{100, 101, 103, 104}
	wantTS := []uint32{1000, 1960, 5800, 6760}
	for i, p := range writes {
		if p.SequenceNumber != wantSeq[i] || p.Timestamp != wantTS[i] {
			t.Fatalf("write %d = seq %d ts %d", i, p.SequenceNumber, p.Timestamp)
		}
	}
	if !writes[1].Marker || writes[2].Marker || !writes[3].Marker {
		t.Fatal("source boundaries did not survive shared clock")
	}
}
