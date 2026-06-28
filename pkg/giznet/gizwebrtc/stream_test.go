package gizwebrtc

import (
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

func TestDataChannelConnWriteWaitsForBufferedAmountLow(t *testing.T) {
	flow := newFakeDataChannelFlow()
	flow.setBufferedAmount(streamWriteHighWater)
	raw := &fakeStreamRaw{}
	conn := newDataChannelConn(raw, flow, addr("local"), addr("remote"))
	defer conn.Close()

	writeDone := make(chan error, 1)
	go func() {
		_, err := conn.Write([]byte("hello"))
		writeDone <- err
	}()

	select {
	case err := <-writeDone:
		t.Fatalf("Write returned before low-watermark signal: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if got := raw.writeCount(); got != 0 {
		t.Fatalf("write count before low-watermark = %d, want 0", got)
	}

	flow.setBufferedAmount(streamWriteLowWater)
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("Write error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Write did not resume after low-watermark signal")
	}
	if got := raw.writeCount(); got != 1 {
		t.Fatalf("write count after low-watermark = %d, want 1", got)
	}
}

func TestDataChannelConnWriteDeadlineExpiresWhileWaitingForBackpressure(t *testing.T) {
	flow := newFakeDataChannelFlow()
	flow.setBufferedAmount(streamWriteHighWater)
	raw := &fakeStreamRaw{}
	conn := newDataChannelConn(raw, flow, addr("local"), addr("remote"))
	defer conn.Close()

	if err := conn.SetWriteDeadline(time.Now().Add(25 * time.Millisecond)); err != nil {
		t.Fatalf("SetWriteDeadline error = %v", err)
	}
	_, err := conn.Write([]byte("hello"))
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("Write error = %v, want %v", err, os.ErrDeadlineExceeded)
	}
	if got := raw.writeCount(); got != 0 {
		t.Fatalf("write count after deadline = %d, want 0", got)
	}
}

func TestDataChannelConnWriteChunksLargePayload(t *testing.T) {
	raw := &fakeStreamRaw{}
	conn := newDataChannelConn(raw, nil, addr("local"), addr("remote"))
	defer conn.Close()

	payload := make([]byte, streamChunkSize*2+17)
	n, err := conn.Write(payload)
	if err != nil {
		t.Fatalf("Write error = %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Write n = %d, want %d", n, len(payload))
	}
	want := []int{streamChunkSize, streamChunkSize, 17}
	if got := raw.writeSizes(); !equalInts(got, want) {
		t.Fatalf("write sizes = %v, want %v", got, want)
	}
}

func TestDataChannelConnReadReassemblesMessageAsByteStream(t *testing.T) {
	raw := &fakeStreamRaw{reads: [][]byte{[]byte("abcdef")}}
	conn := newDataChannelConn(raw, nil, addr("local"), addr("remote"))
	defer conn.Close()

	buf := make([]byte, 3)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("first Read error = %v", err)
	}
	if string(buf[:n]) != "abc" {
		t.Fatalf("first Read = %q, want abc", string(buf[:n]))
	}
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("second Read error = %v", err)
	}
	if string(buf[:n]) != "def" {
		t.Fatalf("second Read = %q, want def", string(buf[:n]))
	}
	if got := raw.readCount(); got != 1 {
		t.Fatalf("raw read count = %d, want 1", got)
	}
}

func TestDataChannelConnCloseWakesBlockedWriter(t *testing.T) {
	flow := newFakeDataChannelFlow()
	flow.setBufferedAmount(streamWriteHighWater)
	raw := &fakeStreamRaw{}
	conn := newDataChannelConn(raw, flow, addr("local"), addr("remote"))

	writeDone := make(chan error, 1)
	go func() {
		_, err := conn.Write([]byte("hello"))
		writeDone <- err
	}()

	select {
	case err := <-writeDone:
		t.Fatalf("Write returned before close: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	select {
	case err := <-writeDone:
		if !errors.Is(err, ErrConnClosed) {
			t.Fatalf("Write error = %v, want %v", err, ErrConnClosed)
		}
	case <-time.After(time.Second):
		t.Fatal("Write did not wake after close")
	}
}

func TestDataChannelConnDeadlinesForwardToRawChannel(t *testing.T) {
	raw := &fakeStreamRaw{}
	conn := newDataChannelConn(raw, nil, addr("local"), addr("remote"))
	defer conn.Close()

	if conn.LocalAddr().String() != "local" {
		t.Fatalf("LocalAddr = %v, want local", conn.LocalAddr())
	}
	if conn.RemoteAddr().String() != "remote" {
		t.Fatalf("RemoteAddr = %v, want remote", conn.RemoteAddr())
	}
	deadline := time.Now().Add(time.Second)
	if err := conn.SetDeadline(deadline); err != nil {
		t.Fatalf("SetDeadline error = %v", err)
	}
	if !raw.readDeadline.Equal(deadline) {
		t.Fatalf("read deadline = %v, want %v", raw.readDeadline, deadline)
	}
	if !raw.writeDeadline.Equal(deadline) {
		t.Fatalf("write deadline = %v, want %v", raw.writeDeadline, deadline)
	}
	readDeadline := deadline.Add(time.Second)
	if err := conn.SetReadDeadline(readDeadline); err != nil {
		t.Fatalf("SetReadDeadline error = %v", err)
	}
	if !raw.readDeadline.Equal(readDeadline) {
		t.Fatalf("read deadline = %v, want %v", raw.readDeadline, readDeadline)
	}
	if (*dataChannelConn)(nil).LocalAddr() != nil {
		t.Fatal("nil LocalAddr returned non-nil addr")
	}
	if (*dataChannelConn)(nil).RemoteAddr() != nil {
		t.Fatal("nil RemoteAddr returned non-nil addr")
	}
}

func TestCloseServiceClosesQueuedAndActiveStreams(t *testing.T) {
	conn := &Conn{
		localAddr:  addr("local"),
		remoteAddr: addr("remote"),
		services:   make(map[uint64]*ServiceListener),
		streams:    make(map[uint64]map[*dataChannelConn]struct{}),
		closedSvc:  make(map[uint64]bool),
		closeCh:    make(chan struct{}),
	}
	listener := conn.ListenService(42)
	serviceListener, ok := listener.(*ServiceListener)
	if !ok {
		t.Fatalf("listener type = %T", listener)
	}
	queuedRaw := &fakeStreamRaw{}
	queued := newDataChannelConn(queuedRaw, nil, addr("local"), addr("remote"))
	conn.trackStream(42, queued)
	if err := serviceListener.enqueue(queued); err != nil {
		t.Fatalf("enqueue queued stream: %v", err)
	}
	activeRaw := &fakeStreamRaw{}
	active := newDataChannelConn(activeRaw, nil, addr("local"), addr("remote"))
	conn.trackStream(42, active)

	if err := conn.CloseService(42); err != nil {
		t.Fatalf("CloseService error = %v", err)
	}
	if _, err := listener.Accept(); !errors.Is(err, ErrServiceClosed) {
		t.Fatalf("Accept after CloseService error = %v, want %v", err, ErrServiceClosed)
	}
	if !queuedRaw.closed || !activeRaw.closed {
		t.Fatalf("queued/active raw closed = %t/%t, want both true", queuedRaw.closed, activeRaw.closed)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type fakeDataChannelFlow struct {
	mu        sync.Mutex
	buffered  uint64
	threshold uint64
	onLow     func()
}

func newFakeDataChannelFlow() *fakeDataChannelFlow {
	return &fakeDataChannelFlow{}
}

func (f *fakeDataChannelFlow) BufferedAmount() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buffered
}

func (f *fakeDataChannelFlow) SetBufferedAmountLowThreshold(th uint64) {
	f.mu.Lock()
	f.threshold = th
	f.mu.Unlock()
}

func (f *fakeDataChannelFlow) OnBufferedAmountLow(fn func()) {
	f.mu.Lock()
	f.onLow = fn
	f.mu.Unlock()
}

func (f *fakeDataChannelFlow) setBufferedAmount(n uint64) {
	f.mu.Lock()
	wasAbove := f.buffered > f.threshold
	f.buffered = n
	nowLow := f.buffered <= f.threshold
	fn := f.onLow
	f.mu.Unlock()
	if wasAbove && nowLow && fn != nil {
		fn()
	}
}

type fakeStreamRaw struct {
	mu            sync.Mutex
	writes        []int
	reads         [][]byte
	readCalls     int
	closed        bool
	readDeadline  time.Time
	writeDeadline time.Time
}

func (f *fakeStreamRaw) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (f *fakeStreamRaw) Write(p []byte) (int, error) {
	return f.WriteDataChannel(p, false)
}

func (f *fakeStreamRaw) ReadDataChannel(p []byte) (int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readCalls++
	if len(f.reads) == 0 {
		return 0, false, io.EOF
	}
	msg := f.reads[0]
	f.reads = f.reads[1:]
	return copy(p, msg), false, nil
}

func (f *fakeStreamRaw) WriteDataChannel(p []byte, _ bool) (int, error) {
	f.mu.Lock()
	f.writes = append(f.writes, len(p))
	f.mu.Unlock()
	return len(p), nil
}

func (f *fakeStreamRaw) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

func (f *fakeStreamRaw) SetReadDeadline(t time.Time) error {
	f.mu.Lock()
	f.readDeadline = t
	f.mu.Unlock()
	return nil
}

func (f *fakeStreamRaw) SetWriteDeadline(t time.Time) error {
	f.mu.Lock()
	f.writeDeadline = t
	f.mu.Unlock()
	return nil
}

func (f *fakeStreamRaw) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

func (f *fakeStreamRaw) writeSizes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.writes...)
}

func (f *fakeStreamRaw) readCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.readCalls
}
