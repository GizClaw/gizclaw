package gizwebrtc

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/datachannel"
	"github.com/pion/webrtc/v4"
)

type dataChannelFlow interface {
	BufferedAmount() uint64
	SetBufferedAmountLowThreshold(uint64)
	OnBufferedAmountLow(func())
}

type dataChannelDiagnosticFlow interface {
	ID() *uint16
	ReadyState() webrtc.DataChannelState
}

// StreamDiagnostics is a bounded, address-free snapshot of one detached
// DataChannel stream. It is captured before a failed RPC closes the stream so
// transport stalls can be distinguished from errors that occur while opening
// or writing the request.
type StreamDiagnostics struct {
	ID             *uint16
	ReadyState     string
	BufferedAmount uint64
	RXBytes        uint64
	TXBytes        uint64
	Closed         bool
}

func (d StreamDiagnostics) String() string {
	id := "unknown"
	if d.ID != nil {
		id = fmt.Sprint(*d.ID)
	}
	return fmt.Sprintf(
		"id=%s ready_state=%s buffered_amount=%d rx_bytes=%d tx_bytes=%d closed=%t",
		id,
		d.ReadyState,
		d.BufferedAmount,
		d.RXBytes,
		d.TXBytes,
		d.Closed,
	)
}

var streamWriteBufferPool = sync.Pool{New: func() any {
	buffer := make([]byte, 0, streamChunkSize)
	return &buffer
}}

type dataChannelConn struct {
	raw      datachannel.ReadWriteCloserDeadliner
	flow     dataChannelFlow
	local    net.Addr
	remote   net.Addr
	rx       *atomic.Uint64
	tx       *atomic.Uint64
	activity *atomic.Int64
	streamRX atomic.Uint64
	streamTX atomic.Uint64

	readMu sync.Mutex
	// Start at the normal service-message size and grow only when SCTP reports a
	// larger queued message. The short-buffer read does not consume that message,
	// so retrying preserves the supported DataChannel message boundary without
	// retaining a maximum-sized buffer for every small-message stream.
	readBuffer []byte
	pending    []byte

	writeMu           sync.Mutex
	budgetMu          sync.Mutex
	writeBudgets      []*giznet.WriteBudget
	budgetOutstanding uint64
	budgetMonitor     bool

	deadlineMu    sync.Mutex
	writeDeadline time.Time
	deadlineWake  chan struct{}

	lowCh     chan struct{}
	closeCh   chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool
	onClose   func()
}

// acquireWriteBudget waits until b reserves size bytes for stream. Reservations
// are released by each channel as its BufferedAmount drains.
func acquireWriteBudget(b *giznet.WriteBudget, size uint64, stream *dataChannelConn) error {
	for {
		acquired, wake, err := b.TryAcquire(size)
		if err != nil {
			return err
		}
		if acquired {
			return nil
		}
		deadline, deadlineWake := stream.writeDeadlineSnapshot()
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
		case <-wake:
		case <-stream.closeCh:
			if timer != nil {
				timer.Stop()
			}
			return giznet.ErrConnClosed
		case <-deadlineWake:
		case <-timerCh:
			return os.ErrDeadlineExceeded
		}
		if timer != nil {
			timer.Stop()
		}
	}
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

// touch advances the parent connection's last-activity clock. Streams share
// the connection counters, so stream transfers count as connection activity.
func (c *dataChannelConn) touch() {
	if c == nil || c.activity == nil {
		return
	}
	c.activity.Store(time.Now().UnixNano())
}

func (c *dataChannelConn) Read(p []byte) (int, error) {
	if c == nil || c.raw == nil {
		return 0, giznet.ErrConnClosed
	}
	c.readMu.Lock()
	defer c.readMu.Unlock()
	// Keep readMu held through the raw read, any adaptive resize and retry, and
	// the pending-tail update. net.Conn permits concurrent callers, but one SCTP
	// stream still has a single ordered receive sequence.
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		return n, nil
	}

	if c.readBuffer == nil {
		c.readBuffer = make([]byte, streamChunkSize)
	}
	buf := c.readBuffer
	n, _, err := c.raw.ReadDataChannel(buf)
	if errors.Is(err, io.ErrShortBuffer) && n > len(buf) && n <= maxPacketMessageSize {
		c.readBuffer = make([]byte, n)
		buf = c.readBuffer
		n, _, err = c.raw.ReadDataChannel(buf)
	}
	if err != nil {
		if c.closed.Load() {
			return 0, io.EOF
		}
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	if c.rx != nil {
		c.rx.Add(uint64(n))
		monitorRX.Add(uint64(n))
	}
	c.touch()
	c.streamRX.Add(uint64(n))
	copied := copy(p, buf[:n])
	if copied < n {
		c.pending = append(c.pending[:0], buf[copied:n]...)
	}
	return copied, nil
}

func (c *dataChannelConn) Write(p []byte) (int, error) {
	if c == nil || c.raw == nil {
		return 0, giznet.ErrConnClosed
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	written := 0
	for len(p) > 0 {
		if err := c.waitWriteBudget(); err != nil {
			return written, err
		}
		chunk := min(len(p), streamChunkSize)
		n, err := c.writeDataChannelBudgeted(p[:chunk])
		written += n
		if c.tx != nil && n > 0 {
			c.tx.Add(uint64(n))
			monitorTX.Add(uint64(n))
		}
		if n > 0 {
			c.touch()
			c.streamTX.Add(uint64(n))
		}
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

func (c *dataChannelConn) WriteBuffers(buffers net.Buffers) (int64, error) {
	if c == nil || c.raw == nil {
		return 0, giznet.ErrConnClosed
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	pooled := streamWriteBufferPool.Get().(*[]byte)
	chunk := (*pooled)[:0]
	defer func() {
		*pooled = chunk[:0]
		streamWriteBufferPool.Put(pooled)
	}()
	var written int64
	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		if err := c.waitWriteBudget(); err != nil {
			return err
		}
		n, err := c.writeDataChannelBudgeted(chunk)
		written += int64(n)
		if c.tx != nil && n > 0 {
			c.tx.Add(uint64(n))
			monitorTX.Add(uint64(n))
		}
		if n > 0 {
			c.touch()
			c.streamTX.Add(uint64(n))
		}
		if err != nil {
			return err
		}
		if n != len(chunk) {
			return io.ErrShortWrite
		}
		chunk = chunk[:0]
		return nil
	}
	for _, buffer := range buffers {
		for len(buffer) > 0 {
			count := min(len(buffer), streamChunkSize-len(chunk))
			chunk = append(chunk, buffer[:count]...)
			buffer = buffer[count:]
			if len(chunk) == streamChunkSize {
				if err := flush(); err != nil {
					return written, err
				}
			}
		}
	}
	if err := flush(); err != nil {
		return written, err
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
		c.releaseAllSharedWriteBudgets()
		if c.onClose != nil {
			c.onClose()
		}
	})
	return err
}

func (c *dataChannelConn) setWriteBudgets(budgets ...*giznet.WriteBudget) error {
	c.budgetMu.Lock()
	defer c.budgetMu.Unlock()
	if c.budgetOutstanding != 0 || c.streamTX.Load() != 0 {
		return errors.New("gizwebrtc: write budgets configured after writing")
	}
	c.writeBudgets = append([]*giznet.WriteBudget(nil), budgets...)
	return nil
}

func (c *dataChannelConn) reserveSharedWriteBudgets(size uint64) error {
	c.budgetMu.Lock()
	budgets := append([]*giznet.WriteBudget(nil), c.writeBudgets...)
	c.budgetMu.Unlock()
	acquired := 0
	for _, budget := range budgets {
		if err := acquireWriteBudget(budget, size, c); err != nil {
			for index := acquired - 1; index >= 0; index-- {
				budgets[index].Release(size)
			}
			return err
		}
		acquired++
	}
	return nil
}

func (c *dataChannelConn) writeDataChannelBudgeted(payload []byte) (int, error) {
	size := uint64(len(payload))
	if err := c.reserveSharedWriteBudgets(size); err != nil {
		return 0, err
	}
	c.budgetMu.Lock()
	n, err := c.raw.WriteDataChannel(payload, false)
	startMonitor := false
	if err == nil && n == len(payload) {
		c.budgetOutstanding += uint64(n)
		if !c.budgetMonitor && len(c.writeBudgets) > 0 {
			c.budgetMonitor = true
			startMonitor = true
		}
	}
	c.budgetMu.Unlock()
	if err != nil || n != len(payload) {
		c.releaseSharedWriteBudgets(size)
	} else if startMonitor {
		go c.monitorBufferedWrites()
	}
	return n, err
}

func (c *dataChannelConn) releaseSharedWriteBudgets(size uint64) {
	c.budgetMu.Lock()
	budgets := append([]*giznet.WriteBudget(nil), c.writeBudgets...)
	c.budgetMu.Unlock()
	for _, budget := range budgets {
		budget.Release(size)
	}
}

func (c *dataChannelConn) monitorBufferedWrites() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.budgetMu.Lock()
			buffered := uint64(0)
			if c.flow != nil {
				buffered = c.flow.BufferedAmount()
			}
			outstanding := c.budgetOutstanding
			if buffered > outstanding {
				buffered = outstanding
			}
			drained := outstanding - buffered
			c.budgetOutstanding = buffered
			if buffered == 0 {
				c.budgetMonitor = false
			}
			c.budgetMu.Unlock()
			c.releaseSharedWriteBudgets(drained)
			if buffered == 0 {
				return
			}
		case <-c.closeCh:
			return
		}
	}
}

func (c *dataChannelConn) releaseAllSharedWriteBudgets() {
	c.budgetMu.Lock()
	outstanding := c.budgetOutstanding
	c.budgetOutstanding = 0
	c.budgetMonitor = false
	c.budgetMu.Unlock()
	c.releaseSharedWriteBudgets(outstanding)
}

// Diagnostics returns the current request-scoped DataChannel state without
// addresses, payloads, credentials, or other unbounded values.
func (c *dataChannelConn) Diagnostics() StreamDiagnostics {
	if c == nil {
		return StreamDiagnostics{ReadyState: "unknown", Closed: true}
	}
	diagnostics := StreamDiagnostics{
		ReadyState: "unknown",
		RXBytes:    c.streamRX.Load(),
		TXBytes:    c.streamTX.Load(),
		Closed:     c.closed.Load(),
	}
	if c.flow != nil {
		diagnostics.BufferedAmount = c.flow.BufferedAmount()
	}
	if flow, ok := c.flow.(dataChannelDiagnosticFlow); ok {
		if id := flow.ID(); id != nil {
			value := *id
			diagnostics.ID = &value
		}
		state := flow.ReadyState()
		diagnostics.ReadyState = state.String()
		diagnostics.Closed = diagnostics.Closed || state == webrtc.DataChannelStateClosed
	}
	return diagnostics
}

// DiagnosticString exposes the bounded snapshot without requiring transport
// consumers to depend on the concrete WebRTC diagnostics type.
func (c *dataChannelConn) DiagnosticString() string {
	return c.Diagnostics().String()
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
		return giznet.ErrConnClosed
	}
	if err := c.raw.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

func (c *dataChannelConn) SetReadDeadline(t time.Time) error {
	if c == nil || c.raw == nil {
		return giznet.ErrConnClosed
	}
	return c.raw.SetReadDeadline(t)
}

func (c *dataChannelConn) SetWriteDeadline(t time.Time) error {
	if c == nil || c.raw == nil {
		return giznet.ErrConnClosed
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
			return giznet.ErrConnClosed
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
			return giznet.ErrConnClosed
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
