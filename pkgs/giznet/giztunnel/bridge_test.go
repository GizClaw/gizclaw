package giztunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestBridgeWithObservationPreservesServiceTerminal(t *testing.T) {
	tests := []struct {
		name          string
		leftTerminal  error
		rightTerminal error
		wantDirection string
		wantClass     string
	}{
		{name: "left eof", leftTerminal: io.EOF, wantDirection: "left_to_right", wantClass: "eof"},
		{name: "right connection closed", rightTerminal: giznet.ErrConnClosed, wantDirection: "right_to_left", wantClass: "connection_closed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left := newBridgeAggregateTestConn()
			right := newBridgeAggregateTestConn()
			if test.leftTerminal != nil {
				left.accepts <- bridgeAcceptResult{err: test.leftTerminal}
			} else {
				right.accepts <- bridgeAcceptResult{err: test.rightTerminal}
			}
			observation, err := BridgeWithObservation(left, right)
			if err != nil {
				t.Fatalf("BridgeWithObservation() error = %v", err)
			}
			if observation.Path != "service" || observation.Direction != test.wantDirection ||
				observation.Phase != "accept_source" || observation.ErrorClass != test.wantClass {
				t.Fatalf("BridgeWithObservation() = %+v", observation)
			}
		})
	}
}

func TestBridgeWithObservationKeepsOneConcurrentFirstTerminal(t *testing.T) {
	left := newBridgeAggregateTestConn()
	right := newBridgeAggregateTestConn()
	leftErr := errors.New("left terminal")
	rightErr := errors.New("right terminal")
	left.accepts <- bridgeAcceptResult{err: leftErr}
	right.accepts <- bridgeAcceptResult{err: rightErr}

	observation, err := BridgeWithObservation(left, right)
	switch {
	case errors.Is(err, leftErr):
		if observation.Direction != "left_to_right" {
			t.Fatalf("left winner observation = %+v", observation)
		}
	case errors.Is(err, rightErr):
		if observation.Direction != "right_to_left" {
			t.Fatalf("right winner observation = %+v", observation)
		}
	default:
		t.Fatalf("BridgeWithObservation() error = %v", err)
	}
	if observation.Path != "service" || observation.Phase != "accept_source" || observation.ErrorClass != "other" {
		t.Fatalf("concurrent winner observation = %+v", observation)
	}
}

func TestBridgeObservationFreezesRejectionsAtSelectedTerminal(t *testing.T) {
	state := &bridgeObservationState{}
	state.recordOpenRejection(bridgeDirectionLeftToRight, errors.New("before terminal"))

	observation, selected := state.selectTerminal(bridgeLoopResult{
		path:      bridgePathPacket,
		direction: bridgeDirectionRightToLeft,
		phase:     bridgePhaseReadSource,
		err:       io.EOF,
	})
	if !selected {
		t.Fatal("first terminal was not selected")
	}
	state.recordOpenRejection(
		bridgeDirectionRightToLeft,
		&channelCapacityError{scope: "session", active: 32, limit: 32},
	)
	if _, selected := state.selectTerminal(bridgeLoopResult{err: errors.New("later terminal")}); selected {
		t.Fatal("later terminal replaced the selected terminal")
	}

	if observation.OpenRejectionCount != 1 ||
		observation.FirstOpenRejectionDirection != bridgeDirectionLeftToRight ||
		observation.LastOpenRejectionDirection != bridgeDirectionLeftToRight {
		t.Fatalf("frozen rejection summary = %+v", observation)
	}
	if observation.CapacityScope != "" || observation.ActiveChannels != 0 || observation.ChannelLimit != 0 {
		t.Fatalf("post-terminal capacity leaked into observation = %+v", observation)
	}
	if observation.Path != bridgePathPacket || observation.Direction != bridgeDirectionRightToLeft ||
		observation.Phase != bridgePhaseReadSource || observation.ErrorClass != bridgeErrorClassEOF {
		t.Fatalf("selected terminal = %+v", observation)
	}
}

func TestBridgeErrorClassIsBounded(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{want: "clean"},
		{err: io.EOF, want: "eof"},
		{err: io.ErrUnexpectedEOF, want: "eof"},
		{err: net.ErrClosed, want: "closed"},
		{err: giznet.ErrConnClosed, want: "connection_closed"},
		{err: giznet.ErrServiceMuxClosed, want: "service_mux_closed"},
		{err: ErrBufferLimit, want: "buffer_limit"},
		{err: context.Canceled, want: "context_canceled"},
		{err: context.DeadlineExceeded, want: "deadline_exceeded"},
		{err: errors.New("private raw error"), want: "other"},
	} {
		if got := bridgeErrorClass(test.err); got != test.want {
			t.Errorf("bridgeErrorClass(%v) = %q, want %q", test.err, got, test.want)
		}
	}
}

func TestBridgeWithObservationPreservesPacketTerminalPhase(t *testing.T) {
	t.Run("read source", func(t *testing.T) {
		left := newBridgeAggregateTestConn()
		right := newBridgeAggregateTestConn()
		terminal := errors.New("read payload secret")
		left.packets <- bridgePacketResult{err: terminal}
		observation, err := BridgeWithObservation(left, right)
		if !errors.Is(err, terminal) {
			t.Fatalf("BridgeWithObservation() error = %v, want %v", err, terminal)
		}
		if observation.Path != "packet" || observation.Direction != "left_to_right" ||
			observation.Phase != "read_source" || observation.ErrorClass != "other" {
			t.Fatalf("BridgeWithObservation() = %+v", observation)
		}
	})

	t.Run("write destination", func(t *testing.T) {
		left := newBridgeAggregateTestConn()
		right := newBridgeAggregateTestConn()
		terminal := errors.New("write payload secret")
		right.write = func([]byte) (int, error) { return 0, terminal }
		left.packets <- bridgePacketResult{protocol: 0x55, data: []byte("private packet")}
		observation, err := BridgeWithObservation(left, right)
		if !errors.Is(err, terminal) {
			t.Fatalf("BridgeWithObservation() error = %v, want %v", err, terminal)
		}
		if observation.Path != "packet" || observation.Direction != "left_to_right" ||
			observation.Phase != "write_destination" || observation.ErrorClass != "other" {
			t.Fatalf("BridgeWithObservation() = %+v", observation)
		}
	})
}

func TestBridgeWithObservationAggregatesOpenRejections(t *testing.T) {
	left := newBridgeAggregateTestConn()
	right := newBridgeAggregateTestConn()
	firstStream := newBridgeTestConn()
	secondStream := newBridgeTestConn()
	left.accepts <- bridgeAcceptResult{service: 41, stream: firstStream}
	left.accepts <- bridgeAcceptResult{service: 42, stream: secondStream}
	left.accepts <- bridgeAcceptResult{err: io.EOF}
	dialCount := 0
	right.dial = func(uint64) (net.Conn, error) {
		dialCount++
		if dialCount == 1 {
			return nil, errors.New("destination name and credential")
		}
		return nil, &channelCapacityError{scope: "session", active: 32, limit: 32}
	}

	observation, err := BridgeWithObservation(left, right)
	if err != nil {
		t.Fatalf("BridgeWithObservation() error = %v", err)
	}
	if observation.OpenRejectionCount != 2 ||
		observation.FirstOpenRejectionDirection != "left_to_right" ||
		observation.FirstOpenRejectionClass != "other" ||
		observation.LastOpenRejectionDirection != "left_to_right" ||
		observation.LastOpenRejectionClass != "buffer_limit" {
		t.Fatalf("open rejection summary = %+v", observation)
	}
	if observation.CapacityScope != "session" || observation.ActiveChannels != 32 || observation.ChannelLimit != 32 {
		t.Fatalf("capacity summary = %+v", observation)
	}
	for name, stream := range map[string]*bridgeTestConn{"first": firstStream, "second": secondStream} {
		select {
		case <-stream.closed:
		default:
			t.Fatalf("%s rejected source stream was not closed", name)
		}
	}
	if observation.FirstOpenRejectionClass == "destination name and credential" {
		t.Fatal("observation copied a raw error")
	}
}

func TestBridgeCompatibilityMatchesObservedError(t *testing.T) {
	terminal := errors.New("terminal")
	leftObserved := newBridgeAggregateTestConn()
	rightObserved := newBridgeAggregateTestConn()
	leftObserved.accepts <- bridgeAcceptResult{err: terminal}
	_, observedErr := BridgeWithObservation(leftObserved, rightObserved)
	if !errors.Is(observedErr, terminal) {
		t.Fatalf("BridgeWithObservation() error = %v, want %v", observedErr, terminal)
	}

	leftLegacy := newBridgeAggregateTestConn()
	rightLegacy := newBridgeAggregateTestConn()
	leftLegacy.accepts <- bridgeAcceptResult{err: terminal}
	if err := Bridge(leftLegacy, rightLegacy); !errors.Is(err, terminal) {
		t.Fatalf("Bridge() error = %v, want %v", err, terminal)
	}
}

func TestBridgeStreamDrainsOppositeDirectionBeforeClosing(t *testing.T) {
	left := newBridgeTestConn()
	right := newBridgeTestConn()
	done := make(chan struct{})
	go func() {
		bridgeStream(left, right)
		close(done)
	}()

	right.reads <- bridgeTestRead{data: []byte("response")}
	right.reads <- bridgeTestRead{err: io.EOF}
	select {
	case got := <-left.writes:
		if string(got) != "response" {
			t.Fatalf("bridged response = %q, want response", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bridged response")
	}

	select {
	case <-left.closed:
		t.Fatal("left stream closed before the request direction drained")
	case <-time.After(50 * time.Millisecond):
	}

	left.reads <- bridgeTestRead{err: io.EOF}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bridge shutdown")
	}
	for name, conn := range map[string]*bridgeTestConn{"left": left, "right": right} {
		select {
		case <-conn.closed:
		default:
			t.Fatalf("%s stream was not closed", name)
		}
	}
}

type bridgeTestRead struct {
	data []byte
	err  error
}

type bridgeTestConn struct {
	reads     chan bridgeTestRead
	writes    chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func newBridgeTestConn() *bridgeTestConn {
	return &bridgeTestConn{
		reads:  make(chan bridgeTestRead, 2),
		writes: make(chan []byte, 2),
		closed: make(chan struct{}),
	}
}

func (c *bridgeTestConn) Read(p []byte) (int, error) {
	select {
	case read := <-c.reads:
		return copy(p, read.data), read.err
	case <-c.closed:
		return 0, net.ErrClosed
	}
}

func (c *bridgeTestConn) Write(p []byte) (int, error) {
	data := append([]byte(nil), p...)
	select {
	case c.writes <- data:
		return len(p), nil
	case <-c.closed:
		return 0, net.ErrClosed
	}
}

func (c *bridgeTestConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (*bridgeTestConn) LocalAddr() net.Addr              { return bridgeTestAddr("local") }
func (*bridgeTestConn) RemoteAddr() net.Addr             { return bridgeTestAddr("remote") }
func (*bridgeTestConn) SetDeadline(time.Time) error      { return nil }
func (*bridgeTestConn) SetReadDeadline(time.Time) error  { return nil }
func (*bridgeTestConn) SetWriteDeadline(time.Time) error { return nil }

type bridgeTestAddr string

func (a bridgeTestAddr) Network() string { return string(a) }
func (a bridgeTestAddr) String() string  { return string(a) }

type bridgeAcceptResult struct {
	service uint64
	stream  net.Conn
	err     error
}

type bridgePacketResult struct {
	protocol byte
	data     []byte
	err      error
}

type bridgeAggregateTestConn struct {
	accepts   chan bridgeAcceptResult
	packets   chan bridgePacketResult
	closed    chan struct{}
	closeOnce sync.Once
	dial      func(uint64) (net.Conn, error)
	write     func([]byte) (int, error)
}

func newBridgeAggregateTestConn() *bridgeAggregateTestConn {
	return &bridgeAggregateTestConn{
		accepts: make(chan bridgeAcceptResult, 4),
		packets: make(chan bridgePacketResult, 4),
		closed:  make(chan struct{}),
	}
}

func (c *bridgeAggregateTestConn) Dial(service uint64) (net.Conn, error) {
	if c.dial != nil {
		return c.dial(service)
	}
	return newBridgeTestConn(), nil
}

func (*bridgeAggregateTestConn) ListenService(uint64) giznet.ServiceListener { return nil }
func (*bridgeAggregateTestConn) CloseService(uint64) error                   { return nil }

func (c *bridgeAggregateTestConn) AcceptService() (uint64, net.Conn, error) {
	select {
	case result := <-c.accepts:
		return result.service, result.stream, result.err
	case <-c.closed:
		return 0, nil, net.ErrClosed
	}
}

func (*bridgeAggregateTestConn) EnableServiceAccept() {}

func (c *bridgeAggregateTestConn) Read(p []byte) (byte, int, error) {
	select {
	case result := <-c.packets:
		return result.protocol, copy(p, result.data), result.err
	case <-c.closed:
		return 0, 0, net.ErrClosed
	}
}

func (c *bridgeAggregateTestConn) Write(_ byte, payload []byte) (int, error) {
	if c.write != nil {
		return c.write(payload)
	}
	return len(payload), nil
}

func (*bridgeAggregateTestConn) PublicKey() giznet.PublicKey { return giznet.PublicKey{} }
func (*bridgeAggregateTestConn) PeerInfo() *giznet.PeerInfo  { return nil }

func (c *bridgeAggregateTestConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

var (
	_ giznet.Conn                 = (*bridgeAggregateTestConn)(nil)
	_ giznet.ServiceAcceptor      = (*bridgeAggregateTestConn)(nil)
	_ giznet.ServiceAcceptEnabler = (*bridgeAggregateTestConn)(nil)
)
