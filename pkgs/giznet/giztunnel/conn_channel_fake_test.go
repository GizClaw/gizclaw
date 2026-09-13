package giztunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// memChannelConn is an in-memory giznet.ChannelConn that lets Router tests run
// without WebRTC. Reliable channels are byte streams whose reads can be split
// into fixed-size chunks; unreliable channels keep message boundaries.
type memChannelConn struct {
	key       giznet.PublicKey
	peer      *memChannelConn
	packets   chan memPacket
	readChunk int

	mu       sync.Mutex
	prefix   string
	handler  func(giznet.Channel)
	gen      uint64
	channels map[*memChannel]struct{}
	closed   bool
	closeCh  chan struct{}
}

type memPacket struct {
	protocol byte
	payload  []byte
}

func newMemChannelConnPair() (*memChannelConn, *memChannelConn) {
	newConn := func() *memChannelConn {
		key, err := giznet.GenerateKeyPair()
		if err != nil {
			panic(err)
		}
		return &memChannelConn{
			key:      key.Public,
			packets:  make(chan memPacket, 64),
			channels: make(map[*memChannel]struct{}),
			closeCh:  make(chan struct{}),
		}
	}
	left, right := newConn(), newConn()
	left.peer, right.peer = right, left
	return left, right
}

func (c *memChannelConn) OpenChannel(
	ctx context.Context,
	label string,
	reliability giznet.ChannelReliability,
) (giznet.Channel, error) {
	if reliability != giznet.ChannelReliable && reliability != giznet.ChannelUnreliable {
		return nil, errors.New("mem: unsupported channel reliability")
	}
	return c.openChannel(ctx, label, reliability, reliability)
}

// openChannel lets tests declare a remote reliability that differs from the
// local one, which a real transport would report after negotiation.
func (c *memChannelConn) openChannel(
	ctx context.Context,
	label string,
	local, remote giznet.ChannelReliability,
) (*memChannel, error) {
	if ctx == nil {
		return nil, errors.New("mem: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil, giznet.ErrConnClosed
	}
	up, down := newMemQueue(), newMemQueue()
	localEnd := &memChannel{label: label, reliability: local, in: down, out: up, chunk: c.readChunk}
	remoteEnd := &memChannel{label: label, reliability: remote, in: up, out: down, chunk: c.peer.readChunk}
	go c.peer.deliver(remoteEnd)
	return localEnd, nil
}

func (c *memChannelConn) deliver(channel *memChannel) {
	c.mu.Lock()
	handler := c.handler
	if c.closed || handler == nil || !strings.HasPrefix(channel.label, c.prefix) {
		c.mu.Unlock()
		_ = channel.Close()
		return
	}
	c.channels[channel] = struct{}{}
	channel.onClose = func() {
		c.mu.Lock()
		delete(c.channels, channel)
		c.mu.Unlock()
	}
	c.mu.Unlock()
	handler(channel)
}

func (c *memChannelConn) HandleChannels(prefix string, handler func(giznet.Channel)) (func(), error) {
	if prefix == "" || handler == nil {
		return nil, errors.New("mem: invalid channel handler")
	}
	c.mu.Lock()
	if c.handler != nil {
		c.mu.Unlock()
		return nil, errors.New("mem: channel handler already registered")
	}
	c.prefix, c.handler = prefix, handler
	c.gen++
	gen := c.gen
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			if c.gen != gen {
				c.mu.Unlock()
				return
			}
			c.handler, c.prefix = nil, ""
			channels := make([]*memChannel, 0, len(c.channels))
			for channel := range c.channels {
				channels = append(channels, channel)
			}
			c.mu.Unlock()
			for _, channel := range channels {
				_ = channel.Close()
			}
		})
	}, nil
}

func (c *memChannelConn) handlerRegistered() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.handler != nil
}

func (c *memChannelConn) deliveredChannels() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.channels)
}

func (c *memChannelConn) Dial(uint64) (net.Conn, error)               { return nil, errors.ErrUnsupported }
func (c *memChannelConn) ListenService(uint64) giznet.ServiceListener { return nil }
func (c *memChannelConn) CloseService(uint64) error                   { return nil }
func (c *memChannelConn) PublicKey() giznet.PublicKey                 { return c.peer.key }

func (c *memChannelConn) PeerInfo() *giznet.PeerInfo {
	return &giznet.PeerInfo{PublicKey: c.peer.key, State: giznet.PeerStateEstablished}
}

func (c *memChannelConn) Read(buf []byte) (byte, int, error) {
	select {
	case packet := <-c.packets:
		if len(packet.payload) > len(buf) {
			return 0, 0, giznet.ErrPacketBuffer
		}
		return packet.protocol, copy(buf, packet.payload), nil
	case <-c.closeCh:
		return 0, 0, giznet.ErrConnClosed
	}
}

func (c *memChannelConn) Write(protocol byte, payload []byte) (int, error) {
	packet := memPacket{protocol: protocol, payload: append([]byte(nil), payload...)}
	select {
	case c.peer.packets <- packet:
		return len(payload), nil
	case <-c.closeCh:
		return 0, giznet.ErrConnClosed
	}
}

func (c *memChannelConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.closeCh)
	channels := make([]*memChannel, 0, len(c.channels))
	for channel := range c.channels {
		channels = append(channels, channel)
	}
	c.mu.Unlock()
	for _, channel := range channels {
		_ = channel.Close()
	}
	return nil
}

type memQueue struct {
	mu     sync.Mutex
	data   []byte
	msgs   [][]byte
	closed bool
	notify chan struct{}
}

func newMemQueue() *memQueue { return &memQueue{notify: make(chan struct{})} }

func (q *memQueue) signalLocked() {
	close(q.notify)
	q.notify = make(chan struct{})
}

func (q *memQueue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		q.signalLocked()
	}
}

type memChannel struct {
	label       string
	reliability giznet.ChannelReliability
	in, out     *memQueue
	chunk       int
	onClose     func()

	deadlineMu   sync.Mutex
	readDeadline time.Time
	deadlineWake chan struct{}
	closeOnce    sync.Once
}

func (c *memChannel) Label() string                                { return c.label }
func (c *memChannel) Reliability() giznet.ChannelReliability       { return c.reliability }
func (c *memChannel) SetWriteBudgets(...*giznet.WriteBudget) error { return nil }
func (c *memChannel) LocalAddr() net.Addr                          { return memAddr("local") }
func (c *memChannel) RemoteAddr() net.Addr                         { return memAddr("remote") }
func (c *memChannel) SetWriteDeadline(time.Time) error             { return nil }

func (c *memChannel) SetDeadline(deadline time.Time) error { return c.SetReadDeadline(deadline) }

func (c *memChannel) SetReadDeadline(deadline time.Time) error {
	c.deadlineMu.Lock()
	c.readDeadline = deadline
	if c.deadlineWake != nil {
		close(c.deadlineWake)
	}
	c.deadlineWake = make(chan struct{})
	c.deadlineMu.Unlock()
	return nil
}

// wait blocks until notify fires, the read deadline passes, or the deadline
// changes. It reports os.ErrDeadlineExceeded once the deadline has passed.
func (c *memChannel) wait(notify <-chan struct{}) error {
	c.deadlineMu.Lock()
	deadline := c.readDeadline
	if c.deadlineWake == nil {
		c.deadlineWake = make(chan struct{})
	}
	wake := c.deadlineWake
	c.deadlineMu.Unlock()
	var timer <-chan time.Time
	if !deadline.IsZero() {
		delay := time.Until(deadline)
		if delay <= 0 {
			return os.ErrDeadlineExceeded
		}
		t := time.NewTimer(delay)
		defer t.Stop()
		timer = t.C
	}
	select {
	case <-notify:
	case <-wake:
	case <-timer:
		return os.ErrDeadlineExceeded
	}
	return nil
}

func (c *memChannel) Read(buf []byte) (int, error) {
	for {
		c.in.mu.Lock()
		if len(c.in.data) > 0 {
			limit := len(buf)
			if c.chunk > 0 {
				limit = min(limit, c.chunk)
			}
			n := copy(buf[:limit], c.in.data)
			c.in.data = c.in.data[n:]
			c.in.mu.Unlock()
			return n, nil
		}
		if c.in.closed {
			c.in.mu.Unlock()
			return 0, io.EOF
		}
		notify := c.in.notify
		c.in.mu.Unlock()
		if err := c.wait(notify); err != nil {
			return 0, err
		}
	}
}

func (c *memChannel) Write(payload []byte) (int, error) {
	c.out.mu.Lock()
	defer c.out.mu.Unlock()
	if c.out.closed {
		return 0, io.ErrClosedPipe
	}
	c.out.data = append(c.out.data, payload...)
	c.out.signalLocked()
	return len(payload), nil
}

func (c *memChannel) ReadMessage(buf []byte) (int, error) {
	for {
		c.in.mu.Lock()
		if len(c.in.msgs) > 0 {
			message := c.in.msgs[0]
			if len(message) > len(buf) {
				c.in.mu.Unlock()
				return 0, io.ErrShortBuffer
			}
			c.in.msgs = c.in.msgs[1:]
			c.in.mu.Unlock()
			return copy(buf, message), nil
		}
		if c.in.closed {
			c.in.mu.Unlock()
			return 0, io.EOF
		}
		notify := c.in.notify
		c.in.mu.Unlock()
		if err := c.wait(notify); err != nil {
			return 0, err
		}
	}
}

func (c *memChannel) WriteMessage(payload []byte) (int, error) {
	c.out.mu.Lock()
	defer c.out.mu.Unlock()
	if c.out.closed {
		return 0, io.ErrClosedPipe
	}
	c.out.msgs = append(c.out.msgs, append([]byte(nil), payload...))
	c.out.signalLocked()
	return len(payload), nil
}

func (c *memChannel) Close() error {
	c.closeOnce.Do(func() {
		c.in.close()
		c.out.close()
		if c.onClose != nil {
			c.onClose()
		}
	})
	return nil
}

type memAddr string

func (a memAddr) Network() string { return "mem" }
func (a memAddr) String() string  { return string(a) }

var (
	_ giznet.ChannelConn = (*memChannelConn)(nil)
	_ giznet.Channel     = (*memChannel)(nil)
)
