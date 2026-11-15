package codecconv

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
)

func TestPCM16ConverterContinuousFormats(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target resampler.Format
	}{
		{"8k_mono", resampler.Format{SampleRate: 8000}},
		{"16k_stereo", resampler.Format{SampleRate: 16000, Stereo: true}},
		{"8k_stereo", resampler.Format{SampleRate: 8000, Stereo: true}},
		{"24k_stereo", resampler.Format{SampleRate: 24000, Stereo: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, 16000*2/10)
			for i := 0; i < len(data)/2; i++ {
				binary.LittleEndian.PutUint16(data[i*2:], uint16(i%200*100))
			}
			convert := func(chunkBytes int) []byte {
				c := NewPCM16Converter(tc.target)
				defer c.Close()
				var out []byte
				for offset := 0; offset < len(data); offset += chunkBytes {
					part, err := c.Convert("audio/pcm;rate=16000;channels=1", data[offset:min(offset+chunkBytes, len(data))])
					if err != nil {
						t.Fatal(err)
					}
					out = append(out, part...)
				}
				tail, err := c.Flush()
				if err != nil {
					t.Fatal(err)
				}
				out = append(out, tail...)
				if again, err := c.Flush(); err != nil || len(again) != 0 {
					t.Fatalf("second flush = %d bytes, %v", len(again), err)
				}
				return out
			}
			want := convert(len(data))
			channels := 1
			if tc.target.Stereo {
				channels = 2
			}
			if len(want) != tc.target.SampleRate*channels*2/10 {
				t.Fatalf("100ms output = %d bytes", len(want))
			}
			for _, chunkBytes := range []int{80, 320, 3} {
				if got := convert(chunkBytes); !bytes.Equal(got, want) {
					t.Fatalf("%d-byte chunks changed continuous resampling: got %d bytes, want %d", chunkBytes, len(got), len(want))
				}
			}
		})
	}
}

func TestPCM16ConverterRetainsConvertedOutput(t *testing.T) {
	c := NewPCM16Converter(resampler.Format{SampleRate: 8000})
	defer c.Close()
	const mimeType = "audio/pcm;rate=16000;channels=1"
	input := bytes.Repeat([]byte{0x10, 0x20}, 1600)
	first, err := c.Convert(mimeType, input)
	if err != nil || len(first) == 0 {
		t.Fatalf("first conversion = %d bytes, %v", len(first), err)
	}
	want := bytes.Clone(first)
	clear(input)
	if _, err := c.Convert(mimeType, input); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Convert(mimeType, input); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("convert after flush = %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, want) {
		t.Fatal("later conversion or cleanup changed retained output")
	}
}

func TestPCM16ConverterSourceValidationAndDiscard(t *testing.T) {
	fallback := resampler.Format{SampleRate: 16000}
	for _, mimeType := range []string{
		"audio/pcm;rate=0", "audio/pcm;rate=-1", "audio/pcm;rate=bad",
		"audio/pcm;channels=3", "audio/pcm;format=f32le", "audio/pcm;bits=24", "audio/opus",
	} {
		if _, err := ParsePCM16Format(mimeType, fallback); err == nil {
			t.Fatalf("accepted invalid format %q", mimeType)
		}
	}
	c := NewPCM16Converter(resampler.Format{SampleRate: 8000})
	if _, err := c.Convert("audio/pcm;rate=16000;channels=1", []byte{1, 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Convert("audio/pcm;rate=8000;channels=1", []byte{1, 0}); err == nil {
		t.Fatal("accepted a source format change within a route")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if tail, err := c.Flush(); err != nil || len(tail) != 0 {
		t.Fatalf("closed route flushed %d bytes, %v", len(tail), err)
	}
	if _, err := c.Convert("audio/pcm", nil); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("convert after close = %v", err)
	}

	partial := NewPCM16Converter(fallback)
	defer partial.Close()
	if _, err := partial.Convert("audio/pcm", []byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := partial.Flush(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("incomplete PCM sample flush = %v", err)
	}
}
