package pcm

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestReadFullKeepsDataWhenReadReturnsDataAndEOF(t *testing.T) {
	p := make([]byte, 6)
	r := &dataEOFReader{data: []byte{1, 2, 3}}

	n, err := readFull(r, p)
	if err != nil {
		t.Fatalf("readFull() error: %v", err)
	}
	if n != 6 {
		t.Fatalf("readFull() n = %d, want 6", n)
	}

	if !bytes.Equal(p[:3], []byte{1, 2, 3}) {
		t.Fatalf("readFull() data prefix = %v, want [1 2 3]", p[:3])
	}
	if !bytes.Equal(p[3:], []byte{0, 0, 0}) {
		t.Fatalf("readFull() zero padded tail = %v, want [0 0 0]", p[3:])
	}
}

func TestTrackCloseWritePreventsFurtherWrites(t *testing.T) {
	mx := NewMixer(L16Mono16K)
	tr, ctrl, err := mx.CreateTrack()
	if err != nil {
		t.Fatalf("CreateTrack() error: %v", err)
	}

	if err := ctrl.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error: %v", err)
	}

	err = tr.Write(L16Mono16K.DataChunk([]byte{1, 2, 3, 4}))
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write after CloseWrite (same format) err = %v, want closed pipe", err)
	}

	err = tr.Write(L16Mono24K.DataChunk([]byte{1, 2, 3, 4}))
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write after CloseWrite (format switch) err = %v, want closed pipe", err)
	}
}

func TestTrackMatchingFormatBypassesResampler(t *testing.T) {
	mx := NewMixer(L16Mono16K)
	tk := &track{mx: mx}
	writer, err := tk.newWriter(L16Mono16K)
	if err != nil {
		t.Fatalf("newWriter(16 kHz) error: %v", err)
	}
	if writer.resampler != nil {
		t.Fatal("newWriter(16 kHz) created a resampler for a matching mixer format")
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}

type dataEOFReader struct {
	data []byte
	read bool
}

func (r *dataEOFReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	n := copy(p, r.data)
	return n, io.EOF
}
