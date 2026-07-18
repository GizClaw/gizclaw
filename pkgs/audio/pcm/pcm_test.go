package pcm

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

func TestFormatProperties(t *testing.T) {
	tests := []struct {
		format     Format
		sampleRate int
		rateString string
	}{
		{L16Mono16K, 16000, "audio/L16; rate=16000; channels=1"},
		{L16Mono24K, 24000, "audio/L16; rate=24000; channels=1"},
		{L16Mono48K, 48000, "audio/L16; rate=48000; channels=1"},
	}

	for _, tt := range tests {
		if got := tt.format.SampleRate(); got != tt.sampleRate {
			t.Fatalf("SampleRate() = %d, want %d", got, tt.sampleRate)
		}
		if got := tt.format.Channels(); got != 1 {
			t.Fatalf("Channels() = %d, want 1", got)
		}
		if got := tt.format.Depth(); got != 16 {
			t.Fatalf("Depth() = %d, want 16", got)
		}
		if got := tt.format.String(); got != tt.rateString {
			t.Fatalf("String() = %q, want %q", got, tt.rateString)
		}
		if got := tt.format.BitsRate(); got != tt.sampleRate*16 {
			t.Fatalf("BitsRate() = %d", got)
		}
		if got := tt.format.BytesRate(); got != tt.sampleRate*2 {
			t.Fatalf("BytesRate() = %d", got)
		}
	}
}

func TestL16Format(t *testing.T) {
	tests := []struct {
		rate     int
		channels int
		want     Format
	}{
		{16000, 1, L16Mono16K},
		{24000, 1, L16Mono24K},
		{48000, 1, L16Mono48K},
		{16000, 2, L16Stereo16K},
		{24000, 2, L16Stereo24K},
		{48000, 2, L16Stereo48K},
	}
	for _, tt := range tests {
		got, err := L16Format(tt.rate, tt.channels)
		if err != nil || got != tt.want {
			t.Fatalf("L16Format(%d, %d) = (%v, %v), want (%v, nil)", tt.rate, tt.channels, got, err, tt.want)
		}
		if got.SampleRate() != tt.rate || got.Channels() != tt.channels || got.Depth() != 16 {
			t.Fatalf("format %v shape = %d/%d/%d", got, got.SampleRate(), got.Channels(), got.Depth())
		}
	}
	if _, err := L16Format(44100, 1); err == nil {
		t.Fatal("L16Format(44100, 1) expected error")
	}
}

func TestFormatDurationConversions(t *testing.T) {
	f := L16Mono16K

	bytesN := int64(320) // 10ms @ 16k mono l16
	if got := f.Samples(bytesN); got != 160 {
		t.Fatalf("Samples() = %d, want 160", got)
	}

	d := f.Duration(bytesN)
	if d != 10*time.Millisecond {
		t.Fatalf("Duration() = %v, want 10ms", d)
	}

	if got := f.SamplesInDuration(25 * time.Millisecond); got != 400 {
		t.Fatalf("SamplesInDuration() = %d, want 400", got)
	}

	if got := f.BytesInDuration(25 * time.Millisecond); got != 800 {
		t.Fatalf("BytesInDuration() = %d, want 800", got)
	}
}

func TestFormatReadChunkAndDataChunkReadFrom(t *testing.T) {
	f := L16Mono16K
	chunkBytes := f.BytesInDuration(20 * time.Millisecond)

	buf := bytes.Repeat([]byte{1, 2}, int(chunkBytes/2))
	chunk, err := f.ReadChunk(bytes.NewReader(buf), 20*time.Millisecond)
	if err != nil {
		t.Fatalf("ReadChunk() error: %v", err)
	}
	if chunk.Len() != chunkBytes {
		t.Fatalf("chunk.Len() = %d, want %d", chunk.Len(), chunkBytes)
	}
	if chunk.Format() != f {
		t.Fatalf("chunk.Format() = %v, want %v", chunk.Format(), f)
	}

	if _, err := f.ReadChunk(bytes.NewReader([]byte{1, 2, 3}), 20*time.Millisecond); err == nil {
		t.Fatal("ReadChunk() expected error on short reader")
	}

	dc := &DataChunk{Data: make([]byte, 0, 8), fmt: f}
	n, err := dc.ReadFrom(bytes.NewReader([]byte{9, 8, 7, 6}))
	if err != nil {
		t.Fatalf("DataChunk.ReadFrom() error: %v", err)
	}
	if n != 4 || len(dc.Data) != 4 {
		t.Fatalf("DataChunk.ReadFrom() n=%d len=%d", n, len(dc.Data))
	}
	if dc.Len() != 4 {
		t.Fatalf("DataChunk.Len() = %d, want 4", dc.Len())
	}
}

func TestDataChunkReadFromKeepsPartialDataOnError(t *testing.T) {
	f := L16Mono16K

	dc := &DataChunk{Data: make([]byte, 0, 8), fmt: f}
	n, err := dc.ReadFrom(&partialErrReader{data: []byte{1, 2, 3}, err: io.ErrUnexpectedEOF})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadFrom() err = %v, want %v", err, io.ErrUnexpectedEOF)
	}
	if n != 3 {
		t.Fatalf("ReadFrom() n = %d, want 3", n)
	}
	if !bytes.Equal(dc.Data, []byte{1, 2, 3}) {
		t.Fatalf("ReadFrom() data = %v, want [1 2 3]", dc.Data)
	}

	dc2 := &DataChunk{Data: make([]byte, 0, 8), fmt: f}
	n, err = dc2.ReadFrom(&partialErrReader{data: []byte{9, 8}, err: io.EOF})
	if err != nil {
		t.Fatalf("ReadFrom() EOF should be nil, got %v", err)
	}
	if n != 2 || !bytes.Equal(dc2.Data, []byte{9, 8}) {
		t.Fatalf("ReadFrom() EOF partial result n=%d data=%v", n, dc2.Data)
	}
}

func TestSilenceChunkWriteTo(t *testing.T) {
	f := L16Mono24K
	s := f.SilenceChunk(30 * time.Millisecond)

	if s.Format() != f {
		t.Fatalf("silence format mismatch: got %v want %v", s.Format(), f)
	}
	if s.Len() != f.BytesInDuration(30*time.Millisecond) {
		t.Fatalf("silence len mismatch: got %d want %d", s.Len(), f.BytesInDuration(30*time.Millisecond))
	}

	var out bytes.Buffer
	n, err := s.WriteTo(&out)
	if err != nil {
		t.Fatalf("silence WriteTo() error: %v", err)
	}
	if n != s.Len() {
		t.Fatalf("silence WriteTo() n=%d want %d", n, s.Len())
	}

	for i, b := range out.Bytes() {
		if b != 0 {
			t.Fatalf("expected zero at %d, got %d", i, b)
		}
	}
}

func TestChunkWriteToHandlesShortWrite(t *testing.T) {
	f := L16Mono16K

	dc := f.DataChunk([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	w := &shortWriter{limit: 3}
	n, err := dc.WriteTo(w)
	if err != nil {
		t.Fatalf("DataChunk.WriteTo() error: %v", err)
	}
	if n != 8 {
		t.Fatalf("DataChunk.WriteTo() n=%d, want 8", n)
	}
	if !bytes.Equal(w.buf.Bytes(), []byte{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Fatalf("DataChunk.WriteTo() wrote %v", w.buf.Bytes())
	}

	sc := f.SilenceChunk(20 * time.Millisecond)
	w2 := &shortWriter{limit: 5}
	n, err = sc.WriteTo(w2)
	if err != nil {
		t.Fatalf("SilenceChunk.WriteTo() error: %v", err)
	}
	if n != sc.Len() {
		t.Fatalf("SilenceChunk.WriteTo() n=%d, want %d", n, sc.Len())
	}
	for i, b := range w2.buf.Bytes() {
		if b != 0 {
			t.Fatalf("SilenceChunk.WriteTo() byte[%d]=%d, want 0", i, b)
		}
	}
}

func TestChunkWriteToErrShortWrite(t *testing.T) {
	f := L16Mono16K
	dc := f.DataChunk([]byte{1, 2, 3, 4})

	n, err := dc.WriteTo(zeroWriter{})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("DataChunk.WriteTo() err=%v, want io.ErrShortWrite", err)
	}
	if n != 0 {
		t.Fatalf("DataChunk.WriteTo() n=%d, want 0", n)
	}
}

func TestTrackControlHelpers(t *testing.T) {
	mx := NewMixer(L16Mono16K)
	tr, ctrl, err := mx.CreateTrack(WithTrackLabel("test-track"))
	if err != nil {
		t.Fatalf("CreateTrack() error: %v", err)
	}
	if ctrl.Label() != "test-track" {
		t.Fatalf("Label() = %q", ctrl.Label())
	}

	if err := tr.Write(L16Mono16K.DataChunk(bytes.Repeat([]byte{1, 2}, 320))); err != nil {
		t.Fatalf("track write error: %v", err)
	}
	if err := ctrl.CloseWriteWithSilence(10 * time.Millisecond); err != nil {
		t.Fatalf("CloseWriteWithSilence() error: %v", err)
	}

	buf := make([]byte, 640)
	for {
		_, err := mx.Read(buf)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("mixer read error: %v", err)
		}
		if ctrl.ReadBytes() > 0 {
			break
		}
	}

	if ctrl.ReadBytes() == 0 {
		t.Fatal("ReadBytes() should be > 0 after mixer read")
	}
}

type partialErrReader struct {
	data []byte
	err  error

	read bool
}

func (r *partialErrReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	n := copy(p, r.data)
	return n, r.err
}

type shortWriter struct {
	buf   bytes.Buffer
	limit int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if w.limit <= 0 {
		w.limit = 1
	}
	if len(p) > w.limit {
		p = p[:w.limit]
	}
	return w.buf.Write(p)
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) {
	return 0, nil
}
