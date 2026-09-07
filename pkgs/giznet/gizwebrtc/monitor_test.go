package gizwebrtc

import (
	"sync"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestMonitorInboundServiceChannelLifecycle(t *testing.T) {
	baseline := ReadMonitorSnapshot().InboundServiceChannels
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	conn := &Conn{pc: pc, closeCh: make(chan struct{}), streams: make(map[uint64]map[*dataChannelConn]struct{})}
	t.Cleanup(func() { _ = conn.Close() })
	channels := make([]webrtc.DataChannel, 3)
	releases := make([]func(), len(channels))
	for i := range channels {
		var ok bool
		releases[i], ok = conn.reserveInboundServiceStream(&channels[i])
		if !ok {
			t.Fatal("admission rejected")
		}
	}
	if _, ok := conn.reserveInboundServiceStream(&channels[0]); ok {
		t.Fatal("duplicate admission accepted")
	}
	if got := ReadMonitorSnapshot().InboundServiceChannels; got != baseline+3 {
		t.Fatalf("pending count = %d, want %d", got, baseline+3)
	}
	stream := newDataChannelConn(&fakeStreamRaw{}, nil, nil, nil)
	if err := conn.trackStream(48, stream, releases[0]); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	releases[0]()
	if got := ReadMonitorSnapshot().InboundServiceChannels; got != baseline+2 {
		t.Fatalf("after stream close = %d, want %d", got, baseline+2)
	}
	var wg sync.WaitGroup
	wg.Go(func() { _ = conn.Close() })
	wg.Go(releases[1])
	wg.Wait()
	// Pending opens are released by parent close even without an OnClose callback.
	releases[2]()
	if got := ReadMonitorSnapshot().InboundServiceChannels; got != baseline {
		t.Fatalf("after parent close = %d, want %d", got, baseline)
	}
	if _, ok := conn.reserveInboundServiceStream(&channels[0]); ok {
		t.Fatal("closed connection accepted admission")
	}
}
