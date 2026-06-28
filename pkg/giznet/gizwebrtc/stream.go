package gizwebrtc

import (
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/datachannel"
)

type dataChannelFlow interface {
	BufferedAmount() uint64
	SetBufferedAmountLowThreshold(uint64)
	OnBufferedAmountLow(func())
}

type dataChannelConn struct {
	raw    datachannel.ReadWriteCloserDeadliner
	flow   dataChannelFlow
	local  net.Addr
	remote net.Addr

	readMu  sync.Mutex
	pending []byte

	writeMu sync.Mutex

	deadlineMu    sync.Mutex
	writeDeadline time.Time
	deadlineWake  chan struct{}

	lowCh     chan struct{}
	closeCh   chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool
}

func newDataChannelConn(raw datachannel.ReadWriteCloserDeadliner, flow dataChannelFlow, local, remote net.Addr) *dataChannelConn {
	c := &dataChannelConn{
		raw:          raw,
		flow:         flow,
		local:        local,
		remote:       remote,
		deadlineWake: make(chan struct{}),
		lowCh:        make(chan struct{}, 1),
		closeCh:      make(chan struct{}),
	}
	if flow != nil {
		flow.SetBufferedAmountLowThreshold(streamWriteLowWater)
		flow.OnBufferedAmountLow(c.signalBufferedAmountLow)
	}
	return c
}

func (c *dataChannelConn) Read(p []byte) (int, error) {
	if c == nil || c.raw == nil {
		return 0, ErrConnClosed
	}
	c.readMu.Lock()
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		c.readMu.Unlock()
		return n, nil
	}
	c.readMu.Unlock()

	buf := make([]byte, maxPacketMessageSize)
	n, _, err := c.raw.ReadDataChannel(buf)
	if err != nil {
		if c.closed.Load() {
			return 0, io.EOF
		}
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	copied := copy(p, buf[:n])
	if copied < n {
		c.readMu.Lock()
		c.pending = append(c.pending[:0], buf[copied:n]...)
		c.readMu.Unlock()
	}
	return copied, nil
}

func (c *dataChannelConn) Write(p []byte) (int, error) {
	if c == nil || c.raw == nil {
		return 0, ErrConnClosed
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	written := 0
	for len(p) > 0 {
		if err := c.waitWriteBudget(); err != nil {
			return written, err
		}
		chunk := len(p)
		if chunk > streamChunkSize {
			chunk = streamChunkSize
		}
		n, err := c.raw.WriteDataChannel(p[:chunk], false)
		written += n
		if err != nil {
			return written, err
		}
		if n != chunk {
			return written, io.ErrShortWrite
		}
		p = p[chunk:]
	}
	return written, nil
}

func (c *dataChannelConn) Close() error {
	if c == nil || c.raw == nil {
		return nil
	}
	var err error
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		close(c.closeCh)
		c.signalBufferedAmountLow()
		err = c.raw.Close()
	})
	return err
}

func (c *dataChannelConn) LocalAddr() net.Addr {
	if c == nil {
		return nil
	}
	return c.local
}

func (c *dataChannelConn) RemoteAddr() net.Addr {
	if c == nil {
		return nil
	}
	return c.remote
}

func (c *dataChannelConn) SetDeadline(t time.Time) error {
	if c == nil || c.raw == nil {
		return ErrConnClosed
	}
	if err := c.raw.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

func (c *dataChannelConn) SetReadDeadline(t time.Time) error {
	if c == nil || c.raw == nil {
		return ErrConnClosed
	}
	return c.raw.SetReadDeadline(t)
}

func (c *dataChannelConn) SetWriteDeadline(t time.Time) error {
	if c == nil || c.raw == nil {
		return ErrConnClosed
	}
	c.deadlineMu.Lock()
	c.writeDeadline = t
	close(c.deadlineWake)
	c.deadlineWake = make(chan struct{})
	c.deadlineMu.Unlock()
	return c.raw.SetWriteDeadline(t)
}

func (c *dataChannelConn) waitWriteBudget() error {
	for {
		if c.closed.Load() {
			return ErrConnClosed
		}
		if c.flow == nil || c.flow.BufferedAmount() < streamWriteHighWater {
			return nil
		}
		deadline, deadlineWake := c.writeDeadlineSnapshot()
		var timer *time.Timer
		var timerCh <-chan time.Time
		if !deadline.IsZero() {
			delay := time.Until(deadline)
			if delay <= 0 {
				return os.ErrDeadlineExceeded
			}
			timer = time.NewTimer(delay)
			timerCh = timer.C
		}
		select {
		case <-c.lowCh:
		case <-c.closeCh:
			if timer != nil {
				timer.Stop()
			}
			return ErrConnClosed
		case <-deadlineWake:
		case <-timerCh:
			return os.ErrDeadlineExceeded
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

func (c *dataChannelConn) writeDeadlineSnapshot() (time.Time, <-chan struct{}) {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	return c.writeDeadline, c.deadlineWake
}

func (c *dataChannelConn) signalBufferedAmountLow() {
	select {
	case c.lowCh <- struct{}{}:
	default:
	}
}
