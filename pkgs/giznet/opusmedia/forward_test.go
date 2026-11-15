package opusmedia

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/rtpstream"
	"github.com/pion/rtp"
)

type captureRTP func(*rtp.Packet) error

func (f captureRTP) WriteRTP(p *rtp.Packet) error { return f(p) }

func timingAt(source string, seq uint16, ts, duration uint32) *packet.OpusTiming {
	return packet.NewOpusTiming(&packet.OpusInfo{SourceID: source, Header: rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: ts, SSRC: 77}, Duration48: duration})
}

func TestForwardPreservesVariableDurationLossThroughTwoHops(t *testing.T) {
	// The missing packet lasts 60 ms; the next real packet lasts 20 ms. A
	// PrevDroppedPackets-based sample writer would incorrectly skip only 20 ms.
	original := []byte{0xf8, 0xff, 0xfe}
	receive, err := NewReceiver(Config{OutputMode: OutputEncoded})
	if err != nil {
		t.Fatal(err)
	}
	defer receive.Close()
	var frames []Frame
	now := time.Now()
	for _, timing := range []*packet.OpusTiming{timingAt("device", 10, 0, 960), timingAt("device", 12, 3840, 960)} {
		got, err := receive.Push(original, timing.MediaInfo(), now)
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, got...)
	}
	flushed, err := receive.Flush()
	if err != nil {
		t.Fatal(err)
	}
	frames = append(frames, flushed...)
	var uplink []*rtp.Packet
	publish := NewWriter(captureRTP(func(p *rtp.Packet) error { uplink = append(uplink, p.Clone()); return nil }))
	for _, f := range frames {
		if err := publish.Write(t.Context(), f.Original, packet.NewOpusTiming(f.Info), "utterance"); err != nil {
			t.Fatal(err)
		}
	}
	if len(uplink) != 2 || uplink[1].SequenceNumber-uplink[0].SequenceNumber != 2 || uplink[1].Timestamp-uplink[0].Timestamp != 3840 {
		t.Fatalf("uplink timing lost: %+v", uplink)
	}
	// Model SFU subscription, including a quiet-tail reorder flush.
	reorder := NewReorder(0, 0)
	var downlink []*rtp.Packet
	device := NewWriter(captureRTP(func(p *rtp.Packet) error { downlink = append(downlink, p.Clone()); return nil }))
	for _, p := range uplink {
		for _, f := range reorder.Push(EncodedRTP(p, "publisher")) {
			if err := device.Write(t.Context(), f.Data, f.Timing, "floor"); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, f := range reorder.Flush() {
		if err := device.Write(t.Context(), f.Data, f.Timing, "floor"); err != nil {
			t.Fatal(err)
		}
	}
	if len(downlink) != 2 {
		t.Fatalf("got %d real packets", len(downlink))
	}
	jitter := rtpstream.New(rtpstream.Config{})
	var ordered []rtpstream.Frame
	for _, p := range downlink {
		if !bytes.Equal(p.Payload, original) {
			t.Fatal("original was replaced")
		}
		ordered = append(ordered, jitter.Push(p.Payload, EncodedRTP(p, "listener").Timing.MediaInfo(), now)...)
	}
	ordered = append(ordered, jitter.Flush()...)
	var gaps int
	for _, f := range ordered {
		if f.Info.Missing {
			gaps++
			if f.Info.Duration48 != 2880 || len(f.Payload) != 0 {
				t.Fatalf("wrong known loss: %+v", f)
			}
		}
	}
	if gaps != 1 {
		t.Fatalf("listener saw %d gaps, want one exact 60ms interval", gaps)
	}
	if opus.IsRuntimeSupported() {
		pcm, err := NewReceiver(Config{})
		if err != nil {
			t.Fatal(err)
		}
		defer pcm.Close()
		var decoded []Frame
		for _, p := range downlink {
			f, err := pcm.Push(p.Payload, EncodedRTP(p, "listener").Timing.MediaInfo(), now)
			if err != nil {
				t.Fatal(err)
			}
			decoded = append(decoded, f...)
		}
		tail, err := pcm.Flush()
		if err != nil {
			t.Fatal(err)
		}
		decoded = append(decoded, tail...)
		concealed := 0
		for _, f := range decoded {
			if f.Concealed {
				concealed += len(f.PCM)
				if len(f.Original) != 0 {
					t.Fatal("PLC invented encoded originals")
				}
			}
		}
		if concealed != 1920 {
			t.Fatalf("listener concealed %d PCM bytes, want exact 60ms", concealed)
		}
	}
}

func TestForwardSourceAndRouteBoundariesUseSharedClock(t *testing.T) {
	c := Clock{Sequence: 65535, Timestamp: ^uint32(0) - 959}
	payload := []byte{0xf8, 0xff, 0xfe}
	first, err := c.Next(payload, timingAt("a", 65535, ^uint32(0)-959, 960), "one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Next(payload, timingAt("a", 1, 960, 960), "one")
	if err != nil {
		t.Fatal(err)
	}
	if second.Header.SequenceNumber != 1 || second.Header.Timestamp != 960 || second.Header.Marker {
		t.Fatal("wraparound lost")
	}
	source, err := c.Next(payload, timingAt("b", 3, 100, 960), "two")
	if err != nil {
		t.Fatal(err)
	}
	if source.Header.SequenceNumber != 2 || source.Header.Timestamp != 1920 || !source.Header.Marker {
		t.Fatalf("source switch invented gap: %+v", source)
	}
	ordinary, err := c.Next(payload, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.Header.SequenceNumber != 3 || ordinary.Header.Timestamp != 2880 || !ordinary.Header.Marker {
		t.Fatal("ordinary output resumed stale timeline")
	}
	buffer := rtpstream.New(rtpstream.Config{})
	var frames []rtpstream.Frame
	for _, info := range []*packet.OpusInfo{first, second, source, ordinary} {
		info.SourceID = "bound-track"
		frames = append(frames, buffer.Push(payload, info, time.Now())...)
	}
	frames = append(frames, buffer.Flush()...)
	if len(frames) != 5 {
		t.Fatalf("frames=%d, want four originals and one known gap", len(frames))
	}
	if !frames[3].Info.Discontinuity || !frames[4].Info.Discontinuity || frames[3].Info.Missing || frames[4].Info.Missing {
		t.Fatal("contiguous marked source switch did not reset media history")
	}
}

func TestForwardCancellationAndStateLockProgress(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	w := NewWriter(captureRTP(func(*rtp.Packet) error { calls++; return nil }))
	if err := w.Write(ctx, []byte{0xf8, 0xff, 0xfe}, nil, "one"); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled writer sent audio")
	}
	entered, release := make(chan struct{}), make(chan struct{})
	w = NewWriter(captureRTP(func(p *rtp.Packet) error {
		if p.SequenceNumber == 0 {
			close(entered)
			<-release
		}
		return nil
	}))
	done := make(chan error, 1)
	go func() { done <- w.Write(t.Context(), []byte{0xf8, 0xff, 0xfe}, nil, "one") }()
	<-entered
	second := make(chan error, 1)
	go func() { second <- w.Write(t.Context(), []byte{0xf8, 0xff, 0xfe}, nil, "one") }()
	select {
	case err := <-second:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("transport I/O retained clock mutex")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
