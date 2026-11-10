package codecconv

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
)

func TestPCM16ConverterDrainsLargeUpsamplingRatio(t *testing.T) {
	input := make([]byte, 200)
	for i := range 100 {
		binary.LittleEndian.PutUint16(input[i*2:], uint16(i*100))
	}
	source := resampler.Format{SampleRate: 100}
	target := resampler.Format{SampleRate: 16000}
	reader, err := resampler.New(bytes.NewReader(input), source, target)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	want, err := io.ReadAll(reader)
	if err != nil || len(want) != 32000 {
		t.Fatalf("reference conversion = %d bytes, %v", len(want), err)
	}
	for _, chunkBytes := range []int{200, 3, 1} {
		converter := NewPCM16Converter(target)
		var got []byte
		for offset := 0; offset < len(input); offset += chunkBytes {
			part, err := converter.Convert("audio/pcm;rate=100;channels=1", input[offset:min(offset+chunkBytes, len(input))])
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, part...)
		}
		tail, err := converter.Flush()
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, tail...)
		if !bytes.Equal(got, want) {
			t.Fatalf("%d-byte chunks produced %d bytes, want %d continuous samples", chunkBytes, len(got), len(want))
		}
		if tail, err := converter.Flush(); err != nil || len(tail) != 0 {
			t.Fatalf("second flush = %d bytes, %v", len(tail), err)
		}
		if err := converter.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
