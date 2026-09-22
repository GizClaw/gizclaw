package resampler

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkSpeech measures complete streams, including construction and drain,
// with 60 ms output reads as used by the peer mixer. Set
// GIZCLAW_RESAMPLER_SPEECH_DIR to PCM16LE speech fixtures named RATE-CHANNELS.pcm.
// Fixture preparation and measurement commands are described in the audio guide.
func BenchmarkSpeech(b *testing.B) {
	dir := os.Getenv("GIZCLAW_RESAMPLER_SPEECH_DIR")
	if dir == "" {
		b.Skip("set GIZCLAW_RESAMPLER_SPEECH_DIR to PCM16LE speech fixtures")
	}
	for _, tc := range []struct{ input, output, channels int }{
		{24000, 16000, 1}, {48000, 16000, 1}, {44100, 16000, 2},
		{8000, 16000, 1}, {16000, 48000, 1}, {24000, 48000, 1},
		{44100, 48000, 2}, {16000, 16000, 1},
		{16000, 24000, 1}, {48000, 24000, 1}, {44100, 24000, 2},
		{16000, 8000, 1}, {12000, 16000, 1}, {22050, 16000, 1},
		{32000, 16000, 1}, {47999, 16000, 1},
	} {
		b.Run(fmt.Sprintf("%d-%d-%dch", tc.input, tc.output, tc.channels), func(b *testing.B) {
			data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%d-%d.pcm", tc.input, tc.channels)))
			if err != nil {
				b.Fatal(err)
			}
			seconds := float64(len(data)) / float64(tc.input*tc.channels*2)
			if seconds == 0 {
				b.Fatal("empty speech fixture")
			}
			buf := make([]byte, tc.output*2*60/1000)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				r, err := New(bytes.NewReader(data), Format{SampleRate: tc.input, Stereo: tc.channels == 2}, Format{SampleRate: tc.output})
				if err != nil {
					b.Fatal(err)
				}
				var total int
				for {
					n, err := r.Read(buf)
					total += n
					if err == io.EOF {
						break
					}
					if err != nil {
						b.Fatal(err)
					}
				}
				if total == 0 {
					b.Fatal("empty resampler output")
				}
				if err := r.Close(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/seconds/1e6, "ms/audio-s")
		})
	}
}

// BenchmarkFIRKernel measures the arithmetic floor for one 24→16 kHz output
// sample, separately from stream construction, PCM conversion and buffering.
func BenchmarkFIRKernel(b *testing.B) {
	f := newFIR(24000, 16000, 1)
	h := f.bank[0]
	input := make([]float64, len(h))
	for i := range input {
		input[i] = float64(i%17) / 17
	}
	sum := 0.0
	b.ResetTimer()
	for b.Loop() {
		sum += dot(input, h)
	}
	if sum == 0 {
		b.Fatal("empty convolution")
	}
	b.ReportMetric(float64(len(h)), "MAC/output")
}
