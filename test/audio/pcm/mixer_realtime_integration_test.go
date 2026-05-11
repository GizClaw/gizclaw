package pcm_integration_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	audiopcm "github.com/GizClaw/gizclaw-go/pkg/audio/pcm"
)

func TestMixerNoTracksReadBlocksUntilClose(t *testing.T) {
	mx := audiopcm.NewMixer(audiopcm.L16Mono16K)

	readDone := make(chan error, 1)
	go func() {
		_, err := mx.Read(make([]byte, 320))
		readDone <- err
	}()

	select {
	case err := <-readDone:
		t.Fatalf("no tracks: Read returned unexpectedly: %v", err)
	case <-time.After(150 * time.Millisecond):
		// expected: blocked
	}

	if err := mx.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error: %v", err)
	}

	select {
	case err := <-readDone:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("Read() err = %v, want EOF", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked Read() was not released by CloseWrite")
	}
}

func TestMixerReadBlockedCanBeWokenByCloseWrite(t *testing.T) {
	mx := audiopcm.NewMixer(audiopcm.L16Mono16K)

	readDone := make(chan error, 1)
	go func() {
		_, err := mx.Read(make([]byte, 320))
		readDone <- err
	}()

	time.Sleep(20 * time.Millisecond)
	if err := mx.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error: %v", err)
	}

	select {
	case err := <-readDone:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("blocked Read() err = %v, want EOF", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked Read() was not woken up by CloseWrite (possible deadlock)")
	}
}

func TestMixerTrackExistsButNoDataReturnsSilenceChunk(t *testing.T) {
	mx := audiopcm.NewMixer(audiopcm.L16Mono16K)
	_, ctrl, err := mx.CreateTrack(audiopcm.WithTrackLabel("stalled"))
	if err != nil {
		t.Fatalf("CreateTrack() error: %v", err)
	}
	defer func() {
		_ = ctrl.CloseWrite()
		_ = mx.CloseWrite()
	}()

	readDone := make(chan struct {
		n   int
		err error
		buf []byte
	}, 1)
	go func() {
		buf := make([]byte, 320)
		n, err := mx.Read(buf)
		readDone <- struct {
			n   int
			err error
			buf []byte
		}{n: n, err: err, buf: buf}
	}()

	select {
	case r := <-readDone:
		if r.err != nil {
			t.Fatalf("Read() error: %v", r.err)
		}
		if r.n != 320 {
			t.Fatalf("Read() n = %d, want 320", r.n)
		}
		for i, b := range r.buf {
			if b != 0 {
				t.Fatalf("silence chunk byte[%d] = %d, want 0", i, b)
			}
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("track exists but no data: Read should return silence chunk, got blocked")
	}
}

func TestMixerCloseWithBlockedReadersAllUnblocked(t *testing.T) {
	mx := audiopcm.NewMixer(audiopcm.L16Mono16K)

	const readers = 8
	errCh := make(chan error, readers)
	for range readers {
		go func() {
			_, err := mx.Read(make([]byte, 320))
			errCh <- err
		}()
	}

	time.Sleep(20 * time.Millisecond)
	if err := mx.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for i := range readers {
		select {
		case err := <-errCh:
			if !errors.Is(err, io.EOF) {
				t.Fatalf("reader[%d] err = %v, want EOF", i, err)
			}
		case <-deadline:
			t.Fatalf("reader[%d] did not exit after CloseWrite (possible goroutine leak)", i)
		}
	}
}

func TestMixerOnTrackCreatedCallbackCanReenterMixer(t *testing.T) {
	var mx *audiopcm.Mixer
	done := make(chan struct{})

	mx = audiopcm.NewMixer(audiopcm.L16Mono16K, audiopcm.WithOnTrackCreated(func() {
		_ = mx.CloseWrite()
		close(done)
	}))

	createDone := make(chan error, 1)
	go func() {
		_, _, err := mx.CreateTrack()
		createDone <- err
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("onTrackCreated callback reentry blocked (possible lock deadlock)")
	}

	select {
	case err := <-createDone:
		if err != nil {
			t.Fatalf("CreateTrack() error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CreateTrack did not return (possible lock deadlock)")
	}
}

func TestMixerOnTrackClosedCallbackCanReenterMixer(t *testing.T) {
	var mx *audiopcm.Mixer
	var once sync.Once
	done := make(chan struct{})

	mx = audiopcm.NewMixer(audiopcm.L16Mono16K, audiopcm.WithOnTrackClosed(func() {
		_ = mx.CloseWrite()
		once.Do(func() { close(done) })
	}))

	tr, ctrl, err := mx.CreateTrack()
	if err != nil {
		t.Fatalf("CreateTrack() error: %v", err)
	}
	if err := tr.Write(audiopcm.L16Mono16K.DataChunk(bytes.Repeat([]byte{1, 2}, 50))); err != nil {
		t.Fatalf("track write error: %v", err)
	}
	if err := ctrl.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error: %v", err)
	}

	readDone := make(chan struct{})
	go func() {
		_, _ = io.ReadAll(mx)
		close(readDone)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("onTrackClosed callback reentry blocked (possible lock deadlock)")
	}

	select {
	case <-readDone:
	case <-time.After(2 * time.Second):
		t.Fatal("ReadAll did not return (possible lock deadlock)")
	}
}

func TestMixerRealtimeExactMixOneTrackPartialChunkPadsSilence(t *testing.T) {
	format := audiopcm.L16Mono16K
	mx := audiopcm.NewMixer(format, audiopcm.WithAutoClose())

	trA, ctrlA, err := mx.CreateTrack(audiopcm.WithTrackLabel("A"))
	if err != nil {
		t.Fatalf("CreateTrack(A) error: %v", err)
	}
	trB, ctrlB, err := mx.CreateTrack(audiopcm.WithTrackLabel("B"))
	if err != nil {
		t.Fatalf("CreateTrack(B) error: %v", err)
	}

	const samples = 160
	half := samples / 2
	if err := trA.Write(format.DataChunk(makeConstantChunk(1000, samples))); err != nil {
		t.Fatalf("track A write error: %v", err)
	}
	if err := trB.Write(format.DataChunk(makeConstantChunk(2000, half))); err != nil {
		t.Fatalf("track B write error: %v", err)
	}
	_ = ctrlA.CloseWrite()
	_ = ctrlB.CloseWrite()
	_ = mx.CloseWrite()

	buf := make([]byte, samples*2)
	n, err := mx.Read(buf)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("Read() n=%d want=%d", n, len(buf))
	}

	decoded := decodePCM16LE(buf[:n])
	for i := range half {
		if d := absInt16Diff(decoded[i], 3000); d > 1 {
			t.Fatalf("front sample[%d]=%d want~=3000 (diff=%d)", i, decoded[i], d)
		}
	}
	for i := half; i < samples; i++ {
		if d := absInt16Diff(decoded[i], 1000); d > 1 {
			t.Fatalf("tail sample[%d]=%d want~=1000 (diff=%d)", i, decoded[i], d)
		}
	}
}

func makeConstantChunk(sample int16, sampleCount int) []byte {
	data := make([]byte, sampleCount*2)
	for i := range sampleCount {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(sample))
	}
	return data
}

func decodePCM16LE(data []byte) []int16 {
	out := make([]int16, len(data)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return out
}

func absInt16Diff(a, b int16) int {
	d := int(a) - int(b)
	if d < 0 {
		d = -d
	}
	return d
}
