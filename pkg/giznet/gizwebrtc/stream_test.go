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
	mu     sync.Mutex
	writes []int
}

func (f *fakeStreamRaw) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (f *fakeStreamRaw) Write(p []byte) (int, error) {
	return f.WriteDataChannel(p, false)
}

func (f *fakeStreamRaw) ReadDataChannel([]byte) (int, bool, error) {
	return 0, false, io.EOF
}

func (f *fakeStreamRaw) WriteDataChannel(p []byte, _ bool) (int, error) {
	f.mu.Lock()
	f.writes = append(f.writes, len(p))
	f.mu.Unlock()
	return len(p), nil
}

func (f *fakeStreamRaw) Close() error {
	return nil
}

func (f *fakeStreamRaw) SetReadDeadline(time.Time) error {
	return nil
}

func (f *fakeStreamRaw) SetWriteDeadline(time.Time) error {
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
