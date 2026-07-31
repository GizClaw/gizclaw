package gizcli

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"
)

const peerEventSubscriberQueueSize = 256

type peerEventSession struct {
	stream    net.Conn
	onFailure func()

	writeMu sync.Mutex
	mu      sync.Mutex
	subs    map[*peerEventConn]struct{}
	closed  bool
}

func newPeerEventSession(stream net.Conn, onFailure func()) *peerEventSession {
	return &peerEventSession{
		stream:    stream,
		onFailure: onFailure,
		subs:      make(map[*peerEventConn]struct{}),
	}
}

func (s *peerEventSession) start() {
	go s.readLoop()
}

func (s *peerEventSession) subscribe() (net.Conn, error) {
	if s == nil {
		return nil, io.ErrClosedPipe
	}
	conn := &peerEventConn{
		session: s,
		queue:   make(chan []byte, peerEventSubscriberQueueSize),
		done:    make(chan struct{}),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, io.ErrClosedPipe
	}
	s.subs[conn] = struct{}{}
	return conn, nil
}

func (s *peerEventSession) readLoop() {
	for {
		event, err := ReadPeerStreamEvent(s.stream)
		if err != nil {
			notify := s.shutdown()
			if notify && s.onFailure != nil {
				s.onFailure()
			}
			return
		}
		var frame bytes.Buffer
		if err := WritePeerStreamEvent(&frame, event); err != nil {
			notify := s.shutdown()
			if notify && s.onFailure != nil {
				s.onFailure()
			}
			return
		}
		s.publish(frame.Bytes())
	}
}

func (s *peerEventSession) publish(frame []byte) {
	s.mu.Lock()
	subs := make([]*peerEventConn, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub)
	}
	s.mu.Unlock()
	for _, sub := range subs {
		sub.enqueue(frame)
	}
}

func (s *peerEventSession) write(buffers net.Buffers) (int64, error) {
	if s == nil {
		return 0, io.ErrClosedPipe
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	n, err := buffers.WriteTo(s.stream)
	if err != nil {
		notify := s.shutdown()
		if notify && s.onFailure != nil {
			go s.onFailure()
		}
	}
	return n, err
}

func (s *peerEventSession) remove(conn *peerEventConn) {
	if s == nil || conn == nil {
		return
	}
	s.mu.Lock()
	delete(s.subs, conn)
	s.mu.Unlock()
}

func (s *peerEventSession) shutdown() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	s.closed = true
	subs := make([]*peerEventConn, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub)
	}
	s.subs = make(map[*peerEventConn]struct{})
	s.mu.Unlock()
	for _, sub := range subs {
		sub.closeLocal()
	}
	return true
}

func (s *peerEventSession) close() error {
	if s == nil {
		return nil
	}
	s.shutdown()
	if s.stream == nil {
		return nil
	}
	return s.stream.Close()
}

type peerEventConn struct {
	session *peerEventSession
	queue   chan []byte
	done    chan struct{}
	once    sync.Once

	readMu sync.Mutex
	read   []byte
}

func (c *peerEventConn) Read(p []byte) (int, error) {
	if c == nil {
		return 0, io.ErrClosedPipe
	}
	c.readMu.Lock()
	defer c.readMu.Unlock()
	select {
	case <-c.done:
		return 0, io.EOF
	default:
	}
	for len(c.read) == 0 {
		select {
		case <-c.done:
			return 0, io.EOF
		case frame := <-c.queue:
			select {
			case <-c.done:
				return 0, io.EOF
			default:
				c.read = frame
			}
		}
	}
	n := copy(p, c.read)
	c.read = c.read[n:]
	return n, nil
}

func (c *peerEventConn) Write(p []byte) (int, error) {
	if c == nil || c.session == nil {
		return 0, io.ErrClosedPipe
	}
	n, err := c.WriteBuffers(net.Buffers{p})
	return int(n), err
}

func (c *peerEventConn) WriteBuffers(buffers net.Buffers) (int64, error) {
	if c == nil || c.session == nil {
		return 0, io.ErrClosedPipe
	}
	select {
	case <-c.done:
		return 0, io.ErrClosedPipe
	default:
	}
	return c.session.write(buffers)
}

func (c *peerEventConn) Close() error {
	if c == nil {
		return nil
	}
	c.once.Do(func() {
		if c.session != nil {
			c.session.remove(c)
		}
		close(c.done)
	})
	return nil
}

func (c *peerEventConn) closeLocal() {
	if c == nil {
		return
	}
	c.once.Do(func() { close(c.done) })
}

func (c *peerEventConn) enqueue(frame []byte) {
	if c == nil {
		return
	}
	copied := append([]byte(nil), frame...)
	select {
	case <-c.done:
	case c.queue <- copied:
	default:
		_ = c.Close()
	}
}

func (c *peerEventConn) LocalAddr() net.Addr  { return peerEventAddr("local") }
func (c *peerEventConn) RemoteAddr() net.Addr { return peerEventAddr("remote") }

func (c *peerEventConn) SetDeadline(time.Time) error      { return nil }
func (c *peerEventConn) SetReadDeadline(time.Time) error  { return nil }
func (c *peerEventConn) SetWriteDeadline(time.Time) error { return nil }

type peerEventAddr string

func (a peerEventAddr) Network() string { return "gizclaw-event" }
func (a peerEventAddr) String() string  { return string(a) }

var _ net.Conn = (*peerEventConn)(nil)
var _ interface {
	WriteBuffers(net.Buffers) (int64, error)
} = (*peerEventConn)(nil)
