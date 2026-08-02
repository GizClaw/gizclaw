package giztunnel

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

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
