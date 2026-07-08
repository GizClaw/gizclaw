package stampedopus

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func FuzzUnpack(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		{},
		make([]byte, HeaderSize-1),
		Pack(0, nil),
		Pack(0, []byte{0x99}),
		Pack(0x01020304050607, []byte{0xf8, 0xff, 0x10, 0x20}),
		{Version + 1, 0, 0, 0, 0, 0, 0, 1, 2},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			return
		}
		timestamp, frame, ok := Unpack(data)
		if ok {
			if len(frame) == 0 {
				t.Fatal("Unpack returned ok=true with empty frame")
			}
			if timestamp > timestampMask {
				t.Fatalf("timestamp = %#x exceeds low-56-bit mask", timestamp)
			}
			original := append([]byte(nil), frame...)
			if len(data) > HeaderSize {
				data[HeaderSize] ^= 0xff
			}
			if !bytes.Equal(frame, original) {
				t.Fatal("Unpack returned a frame that aliases input data")
			}
		}

		if len(data) == 0 {
			return
		}
		var tsBuf [8]byte
		copy(tsBuf[8-min(len(data), 8):], data[:min(len(data), 8)])
		wantTS := binary.BigEndian.Uint64(tsBuf[:]) & timestampMask
		packed := Pack(wantTS, data)
		gotTS, gotFrame, ok := Unpack(packed)
		if !ok {
			t.Fatal("Unpack(Pack(...)) returned ok=false for non-empty frame")
		}
		if gotTS != wantTS {
			t.Fatalf("timestamp = %#x, want %#x", gotTS, wantTS)
		}
		if !bytes.Equal(gotFrame, data) {
			t.Fatalf("frame = %x, want %x", gotFrame, data)
		}
	})
}
