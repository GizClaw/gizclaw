package opusmedia

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

type blockedRemote struct {
	packets  chan *rtp.Packet
	wake     chan struct{}
	entered  chan struct{}
	mu       sync.Mutex
	deadline time.Time
}

func (r *blockedRemote) ReadRTP() (*rtp.Packet, interceptor.Attributes, error) {
	r.entered <- struct{}{}
	select {
	case p := <-r.packets:
		return p, nil, nil
	case <-r.wake:
		return nil, nil, os.ErrDeadlineExceeded
	}
}
func (r *blockedRemote) SetReadDeadline(deadline time.Time) error {
	r.mu.Lock()
	r.deadline = deadline
	r.mu.Unlock()
	if !deadline.IsZero() && !deadline.After(time.Now()) {
		select {
		case r.wake <- struct{}{}:
		default:
		}
	}
	return nil
}
func TestRemoteReaderCancellationStopsOnlyItsRead(t *testing.T) {
	raw := &blockedRemote{packets: make(chan *rtp.Packet, 1), wake: make(chan struct{}, 1), entered: make(chan struct{}, 1)}
	reader := NewRemoteReader(raw, "track")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := reader.ReadAudio(ctx); done <- err }()
	<-raw.entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled remote read leaked")
	}
	p := &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: 12, Timestamp: 1920, SSRC: 7}, Payload: []byte{0xf8, 0xff, 0xfe}}
	raw.packets <- p
	got, err := reader.ReadAudio(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	p.Payload[0] = 0
	p.SequenceNumber = 99
	info := got.Timing.MediaInfo()
	if got.Data[0] != 0xf8 || info.Header.SequenceNumber != 12 || info.SourceID != "track" {
		t.Fatal("remote handoff borrowed RTP storage")
	}
}

func TestReorderFlushRetainsSourceAndExactGap(t *testing.T) {
	reorder := NewReorder(0, 0)
	p := &EncodedPacket{Data: []byte{0xf8, 0xff, 0xfe}, Timing: timingAt("track", 10, 0, 960)}
	if len(reorder.Push(p)) != 1 {
		t.Fatal("first packet not delivered")
	}
	p = &EncodedPacket{Data: []byte{0xf8, 0xff, 0xfe}, Timing: timingAt("track", 12, 3840, 960)}
	if len(reorder.Push(p)) != 0 {
		t.Fatal("missing predecessor not awaited")
	}
	flushed := reorder.Flush()
	if len(flushed) != 1 {
		t.Fatal("quiet tail not flushed")
	}
	info := flushed[0].Timing.MediaInfo()
	if info.SourceID != "track" || info.Header.SequenceNumber != 12 || info.Header.Timestamp != 3840 {
		t.Fatal("flush erased loss timing")
	}
}
