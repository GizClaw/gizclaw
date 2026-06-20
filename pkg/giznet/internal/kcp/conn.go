package kcp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	gokcp "github.com/xtaci/kcp-go/v5"
)

var (
	ErrConnClosed       = errors.New("kcp: conn closed")
	ErrConnClosedLocal  = fmt.Errorf("%w: local", ErrConnClosed)
	ErrConnClosedByPeer = fmt.Errorf("%w: by peer", ErrConnClosed)
	ErrConnTimeout      = errors.New("kcp: timeout")
)

type kcpConnAddr string

func (a kcpConnAddr) Network() string { return "kcp" }
func (a kcpConnAddr) String() string  { return string(a) }

type timeoutError struct{}

func (timeoutError) Error() string   { return ErrConnTimeout.Error() }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type virtualPacketConn struct {
	output func([]byte)

	inputCh chan []byte
	closeCh chan struct{}
	closed  atomic.Bool
	once    sync.Once

	mu            sync.Mutex
	readDeadline  time.Time
	writeDeadline time.Time
	wakeDeadline  chan struct{}

	localAddr  net.Addr
	remoteAddr net.Addr
}

func newVirtualPacketConn(output func([]byte)) *virtualPacketConn {
	return &virtualPacketConn{
		output:       output,
		inputCh:      make(chan []byte, 4096),
		closeCh:      make(chan struct{}),
		wakeDeadline: make(chan struct{}),
		localAddr:    kcpConnAddr("kcp-local"),
		remoteAddr:   kcpConnAddr("kcp-remote"),
	}
}

func (c *virtualPacketConn) Input(data []byte) error {
	if c.closed.Load() {
		return io.ErrClosedPipe
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	select {
	case c.inputCh <- cp:
		return nil
	case <-c.closeCh:
		return io.ErrClosedPipe
	}
}

func (c *virtualPacketConn) ReadFrom(buf []byte) (int, net.Addr, error) {
	for {
		if c.closed.Load() {
			return 0, nil, io.ErrClosedPipe
		}

		deadline, wake := c.readDeadlineState()
		timer, timeout := deadlineTimer(deadline)
		select {
		case data := <-c.inputCh:
			stopTimer(timer)
			return copy(buf, data), c.remoteAddr, nil
		case <-c.closeCh:
			stopTimer(timer)
			return 0, nil, io.ErrClosedPipe
		case <-wake:
			stopTimer(timer)
			continue
		case <-timeout:
			return 0, nil, timeoutError{}
		}
	}
}

func (c *virtualPacketConn) WriteTo(buf []byte, _ net.Addr) (int, error) {
	if c.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	if c.output != nil {
		c.output(buf)
	}
	return len(buf), nil
}

func (c *virtualPacketConn) Close() error {
	c.once.Do(func() {
		c.closed.Store(true)
		close(c.closeCh)
		c.wakeDeadlines()
	})
	return nil
}

func (c *virtualPacketConn) LocalAddr() net.Addr { return c.localAddr }

func (c *virtualPacketConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDeadline = t
	c.writeDeadline = t
	c.resetWakeDeadlineLocked()
	c.mu.Unlock()
	return nil
}

func (c *virtualPacketConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDeadline = t
	c.resetWakeDeadlineLocked()
	c.mu.Unlock()
	return nil
}

func (c *virtualPacketConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.writeDeadline = t
	c.resetWakeDeadlineLocked()
	c.mu.Unlock()
	return nil
}

func (c *virtualPacketConn) readDeadlineState() (time.Time, <-chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readDeadline, c.wakeDeadline
}

func (c *virtualPacketConn) writeDeadlineExpired() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.writeDeadline.IsZero() && time.Now().After(c.writeDeadline)
}

func (c *virtualPacketConn) resetWakeDeadlineLocked() {
	close(c.wakeDeadline)
	c.wakeDeadline = make(chan struct{})
}

func (c *virtualPacketConn) wakeDeadlines() {
	c.mu.Lock()
	c.resetWakeDeadlineLocked()
	c.mu.Unlock()
}

func deadlineTimer(deadline time.Time) (*time.Timer, <-chan time.Time) {
	if deadline.IsZero() {
		return nil, nil
	}
	d := time.Until(deadline)
	if d <= 0 {
		timer := time.NewTimer(0)
		return timer, timer.C
	}
	timer := time.NewTimer(d)
	return timer, timer.C
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

// KCPConn wraps kcp-go UDPSession as io.ReadWriteCloser + net.Conn while using
// giznet packets as the underlying packet transport.
type KCPConn struct {
	pc      *virtualPacketConn
	session *gokcp.UDPSession

	closeOnce sync.Once
	closed    atomic.Bool

	idleMu    sync.Mutex
	idleTimer *time.Timer

	deadlineMu    sync.Mutex
	writeDeadline time.Time

	closeErrMu sync.Mutex
	closeErr   error
}

// NewKCPConn creates a KCPConn with the given conversation ID and output
// function. output is called when KCP wants to send a packet over the wire.
func NewKCPConn(conv uint32, output func([]byte)) *KCPConn {
	pc := newVirtualPacketConn(output)
	session, err := gokcp.NewConn4(conv, pc.remoteAddr, nil, 0, 0, false, pc)
	if err != nil {
		panic(fmt.Sprintf("kcp: create session: %v", err))
	}
	session.SetNoDelay(kcpNoDelay, kcpUpdateIntervalMs, kcpFastResend, kcpNoCongestionControl)
	session.SetWindowSize(kcpSendWindow, kcpRecvWindow)
	session.SetMtu(kcpMTU)
	session.SetACKNoDelay(false)
	session.SetWriteDelay(false)

	c := &KCPConn{
		pc:      pc,
		session: session,
	}
	c.idleTimer = time.AfterFunc(idleTimeoutPure, func() {
		c.closeSignal(ErrConnTimeout)
	})
	return c
}

// Input feeds an incoming KCP packet from the network layer.
func (c *KCPConn) Input(data []byte) error {
	if c.closed.Load() {
		return c.getCloseErr()
	}
	if err := c.pc.Input(data); err != nil {
		return c.mapErr(err)
	}
	c.touch()
	return nil
}

func (c *KCPConn) Read(b []byte) (int, error) {
	if c.closed.Load() {
		return 0, c.getCloseErr()
	}
	n, err := c.session.Read(b)
	if err != nil {
		return n, c.mapErr(err)
	}
	if n > 0 {
		c.touch()
	}
	return n, nil
}

func (c *KCPConn) Write(b []byte) (int, error) {
	if c.closed.Load() {
		return 0, c.getCloseErr()
	}
	if c.writeDeadlineExpired() {
		return 0, ErrConnTimeout
	}
	n, err := c.session.Write(b)
	if err != nil {
		return n, c.mapErr(err)
	}
	if n > 0 {
		c.touch()
	}
	return n, nil
}

func (c *KCPConn) WriteBuffers(buffers net.Buffers) (int64, error) {
	if c.closed.Load() {
		return 0, c.getCloseErr()
	}
	if c.writeDeadlineExpired() {
		return 0, ErrConnTimeout
	}
	n, err := c.session.WriteBuffers(buffers)
	if err != nil {
		return int64(n), c.mapErr(err)
	}
	if n > 0 {
		c.touch()
	}
	return int64(n), nil
}

// Drain is kept for KcpMux close sequencing. UDPSession owns KCP flushing
// internally, so this only observes the caller's deadline and closed state.
func (c *KCPConn) Drain(deadline time.Time) error {
	if c.closed.Load() {
		return c.getCloseErr()
	}
	if !deadline.IsZero() && time.Now().After(deadline) {
		return ErrConnTimeout
	}
	return nil
}

func (c *KCPConn) Close() error {
	c.closeSignal(ErrConnClosedLocal)
	c.finalizeClose()
	return nil
}

func (c *KCPConn) closeWithReason(reason error) error {
	c.closeSignal(reason)
	c.finalizeClose()
	return nil
}

func (c *KCPConn) closeSignal(reason error) {
	c.closeOnce.Do(func() {
		if reason == nil {
			reason = ErrConnClosed
		}
		c.setCloseErr(reason)
		c.closed.Store(true)
		c.stopIdleTimer()
		_ = c.session.Close()
		_ = c.pc.Close()
	})
}

func (c *KCPConn) finalizeClose() {}

func (c *KCPConn) touch() {
	c.idleMu.Lock()
	if c.idleTimer != nil {
		c.idleTimer.Reset(idleTimeoutPure)
	}
	c.idleMu.Unlock()
}

func (c *KCPConn) stopIdleTimer() {
	c.idleMu.Lock()
	if c.idleTimer != nil {
		c.idleTimer.Stop()
		c.idleTimer = nil
	}
	c.idleMu.Unlock()
}

func (c *KCPConn) writeDeadlineExpired() bool {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	return !c.writeDeadline.IsZero() && time.Now().After(c.writeDeadline)
}

func (c *KCPConn) setCloseErr(err error) {
	if err == nil {
		err = ErrConnClosed
	}
	c.closeErrMu.Lock()
	if c.closeErr == nil {
		c.closeErr = err
	}
	c.closeErrMu.Unlock()
}

func (c *KCPConn) getCloseErr() error {
	c.closeErrMu.Lock()
	defer c.closeErrMu.Unlock()
	if c.closeErr != nil {
		return c.closeErr
	}
	return ErrConnClosed
}

func (c *KCPConn) mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, io.ErrClosedPipe) || c.closed.Load() {
		return c.getCloseErr()
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrConnTimeout
	}
	return err
}

func (c *KCPConn) SetReadDeadline(t time.Time) error {
	return c.session.SetReadDeadline(t)
}

func (c *KCPConn) SetWriteDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	c.writeDeadline = t
	c.deadlineMu.Unlock()
	if err := c.session.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

func (c *KCPConn) SetDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	c.writeDeadline = t
	c.deadlineMu.Unlock()
	if err := c.session.SetDeadline(t); err != nil {
		return err
	}
	return nil
}

func (c *KCPConn) LocalAddr() net.Addr  { return c.pc.localAddr }
func (c *KCPConn) RemoteAddr() net.Addr { return c.pc.remoteAddr }
func (c *KCPConn) IsClosed() bool       { return c.closed.Load() }

const (
	kcpMTU                 = 1400
	kcpNoDelay             = 1
	kcpUpdateIntervalMs    = 10
	kcpFastResend          = 2
	kcpNoCongestionControl = 0
	kcpSendWindow          = 512
	kcpRecvWindow          = 512
)

const kcpDefaultIdleTimeout = 5 * time.Minute

var idleTimeoutPure = kcpDefaultIdleTimeout

var _ io.ReadWriteCloser = (*KCPConn)(nil)
var _ net.Conn = (*KCPConn)(nil)
var _ net.PacketConn = (*virtualPacketConn)(nil)
