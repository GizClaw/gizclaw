package gizwebrtc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
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

func TestDataChannelConnBackpressureIsScopedToOneChannel(t *testing.T) {
	blockedFlow := newFakeDataChannelFlow()
	blockedFlow.setBufferedAmount(streamWriteHighWater)
	blockedRaw := &fakeStreamRaw{}
	blocked := newDataChannelConn(blockedRaw, blockedFlow, addr("local"), addr("remote"))
	defer blocked.Close()

	readyRaw := &fakeStreamRaw{}
	ready := newDataChannelConn(readyRaw, nil, addr("local"), addr("remote"))
	defer ready.Close()

	blockedDone := make(chan error, 1)
	go func() {
		_, err := blocked.Write([]byte("blocked"))
		blockedDone <- err
	}()
	select {
	case err := <-blockedDone:
		t.Fatalf("blocked Write returned before low-water signal: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	readyDone := make(chan error, 1)
	go func() {
		_, err := ready.Write([]byte("ready"))
		readyDone <- err
	}()
	select {
	case err := <-readyDone:
		if err != nil {
			t.Fatalf("independent Write error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("independent channel was blocked by unrelated backpressure")
	}
	if got := readyRaw.writeCount(); got != 1 {
		t.Fatalf("independent channel write count = %d, want 1", got)
	}

	blockedFlow.setBufferedAmount(streamWriteLowWater)
	select {
	case err := <-blockedDone:
		if err != nil {
			t.Fatalf("blocked Write after low-water signal = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked channel did not resume")
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

func TestDataChannelConnWriteBuffersCoalescesAdjacentProtocolParts(t *testing.T) {
	raw := &fakeStreamRaw{}
	conn := newDataChannelConn(raw, nil, addr("local"), addr("remote"))
	defer conn.Close()

	buffers := net.Buffers{
		make([]byte, 9),
		make([]byte, streamChunkSize+8),
	}
	n, err := conn.WriteBuffers(buffers)
	if err != nil {
		t.Fatalf("WriteBuffers error = %v", err)
	}
	if n != streamChunkSize+17 {
		t.Fatalf("WriteBuffers n = %d, want %d", n, streamChunkSize+17)
	}
	if got, want := raw.writeSizes(), []int{streamChunkSize, 17}; !equalInts(got, want) {
		t.Fatalf("write sizes = %v, want %v", got, want)
	}
}

func TestDataChannelConnWriteKeepsTunnelFramesIntact(t *testing.T) {
	raw := &fakeStreamRaw{}
	conn := newDataChannelConn(raw, nil, addr("local"), addr("remote"))
	defer conn.Close()

	const tunnelFrameSize = 16 * 1024
	payload := make([]byte, 4*tunnelFrameSize)
	n, err := conn.Write(payload)
	if err != nil {
		t.Fatalf("Write error = %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Write n = %d, want %d", n, len(payload))
	}
	want := []int{2 * tunnelFrameSize, 2 * tunnelFrameSize}
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

func TestDataChannelConnReadPreservesMaximumMessageSize(t *testing.T) {
	message := make([]byte, maxPacketMessageSize)
	for index := range message {
		message[index] = byte(index)
	}
	raw := &fakeStreamRaw{reads: [][]byte{message}}
	conn := newDataChannelConn(raw, nil, addr("local"), addr("remote"))
	defer conn.Close()

	got := make([]byte, len(message))
	n, err := io.ReadFull(conn, got)
	if err != nil {
		t.Fatalf("ReadFull error = %v", err)
	}
	if n != len(message) || !bytes.Equal(got, message) {
		t.Fatalf("ReadFull read %d bytes, want the complete %d-byte message", n, len(message))
	}
	if got := raw.readCount(); got != 2 {
		t.Fatalf("raw read count = %d, want one sizing read and one retry", got)
	}
}

func TestDataChannelConnReadReusesMessageBuffer(t *testing.T) {
	raw := &repeatingStreamRaw{message: []byte("ping")}
	conn := newDataChannelConn(raw, nil, addr("local"), addr("remote"))
	defer conn.Close()

	buf := make([]byte, len(raw.message))
	var readErr error
	allocations := testing.AllocsPerRun(1000, func() {
		var n int
		n, readErr = conn.Read(buf)
		if n != len(raw.message) {
			readErr = io.ErrShortBuffer
		}
	})
	if readErr != nil {
		t.Fatalf("Read error = %v", readErr)
	}
	if allocations != 0 {
		t.Fatalf("Read allocations = %f, want 0", allocations)
	}
	if got := len(conn.readBuffer); got != streamChunkSize {
		t.Fatalf("read buffer size = %d, want %d", got, streamChunkSize)
	}
}

func TestDataChannelConnConcurrentReadsSerializeAdaptiveBuffer(t *testing.T) {
	const readers = 32
	reads := make([][]byte, readers)
	for index := range reads {
		reads[index] = bytes.Repeat([]byte{byte(index + 1)}, maxPacketMessageSize)
	}
	raw := &fakeStreamRaw{reads: reads}
	conn := newDataChannelConn(raw, nil, addr("local"), addr("remote"))
	defer conn.Close()

	type readResult struct {
		marker byte
		err    error
	}
	start := make(chan struct{})
	results := make(chan readResult, readers)
	var workers sync.WaitGroup
	for range readers {
		workers.Go(func() {
			<-start
			buffer := make([]byte, maxPacketMessageSize)
			n, err := io.ReadFull(conn, buffer)
			if err == nil && n != len(buffer) {
				err = fmt.Errorf("read %d bytes, want %d", n, len(buffer))
			}
			if err == nil && !bytes.Equal(buffer, bytes.Repeat(buffer[:1], len(buffer))) {
				err = errors.New("read combined bytes from different DataChannel messages")
			}
			results <- readResult{marker: buffer[0], err: err}
		})
	}
	close(start)
	workers.Wait()
	close(results)

	seen := make(map[byte]bool, readers)
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.marker == 0 || seen[result.marker] {
			t.Fatalf("duplicate or empty message marker %d", result.marker)
		}
		seen[result.marker] = true
	}
	if len(seen) != readers {
		t.Fatalf("read %d distinct messages, want %d", len(seen), readers)
	}
	if got := raw.readCount(); got != readers+1 {
		t.Fatalf("raw read count = %d, want one sizing read plus %d messages", got, readers)
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
		if !errors.Is(err, giznet.ErrConnClosed) {
			t.Fatalf("Write error = %v, want %v", err, giznet.ErrConnClosed)
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
	if err := conn.trackStream(42, queued, nil); err != nil {
		t.Fatalf("track queued stream: %v", err)
	}
	if err := serviceListener.enqueue(queued); err != nil {
		t.Fatalf("enqueue queued stream: %v", err)
	}
	activeRaw := &fakeStreamRaw{}
	active := newDataChannelConn(activeRaw, nil, addr("local"), addr("remote"))
	if err := conn.trackStream(42, active, nil); err != nil {
		t.Fatalf("track active stream: %v", err)
	}

	if err := conn.CloseService(42); err != nil {
		t.Fatalf("CloseService error = %v", err)
	}
	if _, err := listener.Accept(); !errors.Is(err, giznet.ErrServiceMuxClosed) {
		t.Fatalf("Accept after CloseService error = %v, want %v", err, giznet.ErrServiceMuxClosed)
	}
	if !queuedRaw.closed || !activeRaw.closed {
		t.Fatalf("queued/active raw closed = %t/%t, want both true", queuedRaw.closed, activeRaw.closed)
	}
	if queued := len(serviceListener.ch); queued != 0 {
		t.Fatalf("queued stream references after CloseService = %d, want 0", queued)
	}
}

func TestServiceListenerCloseDrainsConcurrentEnqueues(t *testing.T) {
	conn := &Conn{closeCh: make(chan struct{})}
	listener := newServiceListener(conn, 42)
	const streams = serviceQueueSize * 2
	raws := make([]*fakeStreamRaw, streams)
	var enqueues sync.WaitGroup
	for index := range streams {
		raw := &fakeStreamRaw{}
		raws[index] = raw
		stream := newDataChannelConn(raw, nil, addr("local"), addr("remote"))
		enqueues.Go(func() {
			_ = listener.enqueue(stream)
		})
	}
	deadline := time.Now().Add(time.Second)
	for len(listener.ch) != serviceQueueSize && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if queued := len(listener.ch); queued != serviceQueueSize {
		t.Fatalf("queued streams before Close = %d, want %d", queued, serviceQueueSize)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	enqueues.Wait()
	if queued := len(listener.ch); queued != 0 {
		t.Fatalf("queued stream references after Close = %d, want 0", queued)
	}
	for index, raw := range raws {
		if !raw.closed {
			t.Fatalf("stream %d was not closed", index)
		}
	}
}

func TestClosedStreamsDoNotAccumulate(t *testing.T) {
	conn := &Conn{
		streams:   make(map[uint64]map[*dataChannelConn]struct{}),
		closedSvc: make(map[uint64]bool),
		closeCh:   make(chan struct{}),
	}
	for index := range 10_000 {
		stream := newDataChannelConn(&fakeStreamRaw{}, nil, addr("local"), addr("remote"))
		if err := conn.trackStream(42, stream, nil); err != nil {
			t.Fatalf("track stream %d: %v", index, err)
		}
		if len(conn.streams[42]) != 1 {
			t.Fatalf("tracked streams after %d opens = %d, want 1", index+1, len(conn.streams[42]))
		}
		if err := stream.Close(); err != nil {
			t.Fatalf("close stream %d: %v", index, err)
		}
		if _, ok := conn.streams[42]; ok {
			t.Fatalf("closed stream service registry was retained after %d closes", index+1)
		}
	}
}

func TestTrackStreamRejectsClosedOwner(t *testing.T) {
	conn := &Conn{
		streams:   make(map[uint64]map[*dataChannelConn]struct{}),
		closedSvc: make(map[uint64]bool),
		closeCh:   make(chan struct{}),
	}
	conn.closed.Store(true)
	stream := newDataChannelConn(&fakeStreamRaw{}, nil, addr("local"), addr("remote"))
	if err := conn.trackStream(42, stream, nil); !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("track stream error = %v, want %v", err, giznet.ErrConnClosed)
	}
	if len(conn.streams) != 0 {
		t.Fatalf("tracked services = %d, want 0", len(conn.streams))
	}
}

func TestTrackStreamRejectsClosedService(t *testing.T) {
	conn := &Conn{
		streams:   make(map[uint64]map[*dataChannelConn]struct{}),
		closedSvc: map[uint64]bool{42: true},
		closeCh:   make(chan struct{}),
	}
	stream := newDataChannelConn(&fakeStreamRaw{}, nil, addr("local"), addr("remote"))
	if err := conn.trackStream(42, stream, nil); !errors.Is(err, giznet.ErrServiceMuxClosed) {
		t.Fatalf("track stream error = %v, want %v", err, giznet.ErrServiceMuxClosed)
	}
	if len(conn.streams) != 0 {
		t.Fatalf("tracked services = %d, want 0", len(conn.streams))
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

type repeatingStreamRaw struct {
	fakeStreamRaw
	message []byte
}

func (r *repeatingStreamRaw) ReadDataChannel(p []byte) (int, bool, error) {
	return copy(p, r.message), false, nil
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
	if len(p) < len(msg) {
		return len(msg), false, io.ErrShortBuffer
	}
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
