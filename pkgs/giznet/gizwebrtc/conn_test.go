package gizwebrtc

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/datachannel"
	"github.com/pion/webrtc/v4"
)

type fakeServiceDataChannel struct {
	mu        sync.Mutex
	onOpen    func()
	onClose   func()
	onError   func(error)
	raw       datachannel.ReadWriteCloserDeadliner
	detachErr error
	ready     chan struct{}
	readyOnce sync.Once
	closed    atomic.Int32
}

type fakeDetachedChannel struct {
	net.Conn
}

func (c *fakeDetachedChannel) ReadDataChannel(payload []byte) (int, bool, error) {
	n, err := c.Read(payload)
	return n, false, err
}

func (c *fakeDetachedChannel) WriteDataChannel(payload []byte, _ bool) (int, error) {
	return c.Write(payload)
}

func newFakeServiceDataChannel(raw datachannel.ReadWriteCloserDeadliner) *fakeServiceDataChannel {
	return &fakeServiceDataChannel{raw: raw, ready: make(chan struct{})}
}

func (d *fakeServiceDataChannel) OnOpen(fn func()) {
	d.mu.Lock()
	d.onOpen = fn
	d.mu.Unlock()
}

func (d *fakeServiceDataChannel) OnClose(fn func()) {
	d.mu.Lock()
	d.onClose = fn
	d.mu.Unlock()
}

func (d *fakeServiceDataChannel) OnError(fn func(error)) {
	d.mu.Lock()
	d.onError = fn
	d.mu.Unlock()
	d.readyOnce.Do(func() { close(d.ready) })
}

func (d *fakeServiceDataChannel) DetachWithDeadline() (datachannel.ReadWriteCloserDeadliner, error) {
	return d.raw, d.detachErr
}

func (d *fakeServiceDataChannel) Close() error {
	d.closed.Add(1)
	return nil
}

func (d *fakeServiceDataChannel) triggerOpen() {
	d.mu.Lock()
	fn := d.onOpen
	d.mu.Unlock()
	fn()
}

func (d *fakeServiceDataChannel) triggerClose() {
	d.mu.Lock()
	fn := d.onClose
	d.mu.Unlock()
	fn()
}

func (d *fakeServiceDataChannel) triggerError(err error) {
	d.mu.Lock()
	fn := d.onError
	d.mu.Unlock()
	fn(err)
}

func TestPeerConnectionStateIsTerminal(t *testing.T) {
	tests := []struct {
		name  string
		state webrtc.PeerConnectionState
		want  bool
	}{
		{name: "new", state: webrtc.PeerConnectionStateNew},
		{name: "connecting", state: webrtc.PeerConnectionStateConnecting},
		{name: "connected", state: webrtc.PeerConnectionStateConnected},
		{name: "disconnected can recover", state: webrtc.PeerConnectionStateDisconnected},
		{name: "failed", state: webrtc.PeerConnectionStateFailed, want: true},
		{name: "closed", state: webrtc.PeerConnectionStateClosed, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := peerConnectionStateIsTerminal(test.state); got != test.want {
				t.Fatalf("peerConnectionStateIsTerminal(%s) = %t, want %t", test.state, got, test.want)
			}
		})
	}
}

func TestDrainRTCPReusesBufferUntilReaderCloses(t *testing.T) {
	reads := 0
	var first *byte
	err := drainRTCP(func(buffer []byte) error {
		reads++
		if len(buffer) != 1500 {
			t.Fatalf("RTCP buffer length = %d, want 1500", len(buffer))
		}
		if first == nil {
			first = &buffer[0]
		} else if first != &buffer[0] {
			t.Fatal("drainRTCP replaced its read buffer")
		}
		if reads == 3 {
			return io.EOF
		}
		return nil
	})
	if !errors.Is(err, io.EOF) || reads != 3 {
		t.Fatalf("drainRTCP = %v after %d reads, want EOF after 3", err, reads)
	}
	if err := drainRTCP(nil); err == nil {
		t.Fatal("drainRTCP accepted a nil reader")
	}
}

func TestConnReadReportsUnexpectedCloseCause(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection error = %v", err)
	}
	conn := &Conn{
		pc:       pc,
		services: make(map[uint64]*ServiceListener),
		streams:  make(map[uint64]map[*dataChannelConn]struct{}),
		closeCh:  make(chan struct{}),
		readCh:   make(chan directPacket),
	}
	want := errors.New("transport failed")
	if err := conn.closeWithError(want); err != nil {
		t.Fatalf("closeWithError error = %v", err)
	}
	if _, _, err := conn.Read(make([]byte, 1)); !errors.Is(err, want) {
		t.Fatalf("Read error = %v, want %v", err, want)
	}
	if _, _, err := conn.Read(make([]byte, 1)); !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("Read error = %v, want connection-closed sentinel", err)
	}
}

func TestConnReadReportsNormalClose(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection error = %v", err)
	}
	conn := &Conn{
		pc:       pc,
		services: make(map[uint64]*ServiceListener),
		streams:  make(map[uint64]map[*dataChannelConn]struct{}),
		closeCh:  make(chan struct{}),
		readCh:   make(chan directPacket),
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if _, _, err := conn.Read(make([]byte, 1)); !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("Read error = %v, want %v", err, giznet.ErrConnClosed)
	}
}

func TestSelectedICECandidatePairPrefersNominatedTraffic(t *testing.T) {
	report := webrtc.StatsReport{
		"non-nominated": webrtc.ICECandidatePairStats{
			ID:            "non-nominated",
			BytesSent:     1000,
			BytesReceived: 1000,
		},
		"nominated-low": webrtc.ICECandidatePairStats{
			ID:            "nominated-low",
			Nominated:     true,
			BytesSent:     10,
			BytesReceived: 10,
		},
		"nominated-high": webrtc.ICECandidatePairStats{
			ID:            "nominated-high",
			Nominated:     true,
			BytesSent:     20,
			BytesReceived: 20,
		},
	}
	pair, ok := selectedICECandidatePair(report)
	if !ok {
		t.Fatal("selectedICECandidatePair found no pair")
	}
	if pair.ID != "nominated-high" {
		t.Fatalf("selected pair = %q, want nominated-high", pair.ID)
	}
}

func TestDetachWhenOpenResolvesPreOpenEventsOnce(t *testing.T) {
	for _, test := range []struct {
		name    string
		trigger func(*fakeServiceDataChannel, context.CancelFunc, chan struct{})
		want    error
	}{
		{
			name: "context cancellation",
			trigger: func(_ *fakeServiceDataChannel, cancel context.CancelFunc, _ chan struct{}) {
				cancel()
			},
			want: context.Canceled,
		},
		{
			name: "data channel close",
			trigger: func(dc *fakeServiceDataChannel, _ context.CancelFunc, _ chan struct{}) {
				dc.triggerClose()
			},
			want: ErrServiceOpen,
		},
		{
			name: "data channel error",
			trigger: func(dc *fakeServiceDataChannel, _ context.CancelFunc, _ chan struct{}) {
				dc.triggerError(errors.New("sctp reset"))
			},
			want: ErrServiceOpen,
		},
		{
			name: "parent close",
			trigger: func(_ *fakeServiceDataChannel, _ context.CancelFunc, parent chan struct{}) {
				close(parent)
			},
			want: giznet.ErrConnClosed,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			parent := make(chan struct{})
			dc := newFakeServiceDataChannel(nil)
			result := make(chan error, 1)
			go func() {
				_, err := detachWhenOpen(ctx, dc, parent, func() error { return giznet.ErrConnClosed })
				result <- err
			}()
			<-dc.ready
			test.trigger(dc, cancel, parent)
			select {
			case err := <-result:
				if !errors.Is(err, test.want) {
					t.Fatalf("detachWhenOpen error = %v, want %v", err, test.want)
				}
			case <-time.After(time.Second):
				t.Fatal("detachWhenOpen did not resolve")
			}
		})
	}
}

func TestDetachWhenOpenPreservesSanitizedFailureCause(t *testing.T) {
	for _, test := range []struct {
		name    string
		cause   error
		prepare func(*fakeServiceDataChannel, error)
		want    string
	}{
		{
			name:  "detach failure",
			cause: errors.New("private detach detail"),
			prepare: func(dc *fakeServiceDataChannel, cause error) {
				dc.detachErr = cause
				dc.triggerOpen()
			},
			want: "gizwebrtc: service open: data channel detach failed",
		},
		{
			name:  "data channel error",
			cause: errors.New("private SCTP detail"),
			prepare: func(dc *fakeServiceDataChannel, cause error) {
				dc.triggerError(cause)
			},
			want: "gizwebrtc: service open: data channel error",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dc := newFakeServiceDataChannel(nil)
			result := make(chan error, 1)
			go func() {
				_, err := detachWhenOpen(context.Background(), dc, make(chan struct{}), nil)
				result <- err
			}()
			<-dc.ready
			test.prepare(dc, test.cause)
			err := <-result
			if !errors.Is(err, ErrServiceOpen) {
				t.Fatalf("detachWhenOpen error = %v, want ErrServiceOpen", err)
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("detachWhenOpen error does not preserve cause %v", test.cause)
			}
			if err.Error() != test.want {
				t.Fatalf("detachWhenOpen error = %q, want sanitized %q", err, test.want)
			}
		})
	}
}

func TestDetachWhenOpenClosesLateDetachedChannel(t *testing.T) {
	raw, peer := net.Pipe()
	defer peer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	parent := make(chan struct{})
	dc := newFakeServiceDataChannel(&fakeDetachedChannel{Conn: raw})
	result := make(chan error, 1)
	go func() {
		_, err := detachWhenOpen(ctx, dc, parent, func() error { return giznet.ErrConnClosed })
		result <- err
	}()
	<-dc.ready
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("detachWhenOpen error = %v, want context canceled", err)
	}
	dc.triggerOpen()
	if _, err := peer.Read(make([]byte, 1)); err == nil ||
		!errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("late detached channel read error = %v, want closed channel", err)
	}
}

func TestDialContextReturnsCanceledContextBeforeOpeningDataChannel(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	conn := &Conn{
		pc:        pc,
		services:  make(map[uint64]*ServiceListener),
		streams:   make(map[uint64]map[*dataChannelConn]struct{}),
		closedSvc: make(map[uint64]bool),
		closeCh:   make(chan struct{}),
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := conn.DialContext(ctx, 100); !errors.Is(err, context.Canceled) {
		t.Fatalf("DialContext error = %v, want context canceled", err)
	}
}
