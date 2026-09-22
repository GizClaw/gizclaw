//go:build !js

package resampler

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"testing"
)

// fragmentedReader models a transport that may split an interleaved frame.
type fragmentedReader struct {
	data []byte
	size int
}

func (r *fragmentedReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data[:min(len(r.data), r.size)])
	r.data = r.data[n:]
	return n, nil
}
func convertTest(t *testing.T, data []byte, src, dst Format, fragment, readSize int) []byte {
	t.Helper()
	var input io.Reader = bytes.NewReader(data)
	if fragment > 0 {
		input = &fragmentedReader{data: data, size: fragment}
	}
	r, err := New(input, src, dst)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out, err := readAll(r, readSize)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestFIRChunkBoundaries(t *testing.T) {
	for _, rates := range [][2]int{{24000, 16000}, {48000, 16000}, {44100, 16000}, {8000, 16000}, {16000, 48000}, {24000, 48000}, {44100, 48000}, {16000, 24000}, {48000, 24000}, {44100, 24000}, {16000, 8000}, {12000, 16000}, {22050, 16000}, {32000, 16000}, {47999, 16000}, {256, 1}, {1, 256}} {
		t.Run(fmt.Sprintf("%d-%d", rates[0], rates[1]), func(t *testing.T) {
			input := generateSinePCM(rates[0], float64(rates[0])*.07, .02)
			if len(input) == 0 {
				input = []byte{100, 0}
			}
			for _, stereo := range []bool{false, true} {
				src := Format{SampleRate: rates[0]}
				dst := Format{SampleRate: rates[1], Stereo: stereo}
				expected := convertTest(t, input, src, dst, 0, 4096)
				frames := (int64(len(input)/2)*int64(rates[1]) + int64(rates[0]) - 1) / int64(rates[0])
				if len(expected) != int(frames)*dst.sampleBytes() {
					t.Fatalf("frames: got %d bytes, want %d frames", len(expected), frames)
				}
				for _, fragment := range []int{1, 3, 127} {
					got := convertTest(t, input, src, dst, fragment, 7)
					if !bytes.Equal(got, expected) {
						t.Fatalf("fragment=%d stereo=%v differs: got %d bytes want %d", fragment, stereo, len(got), len(expected))
					}
				}
			}
		})
	}
}
func TestFIRStereoIsolation(t *testing.T) {
	input := generateSinePCM(24000, 1000, .1)
	stereo := make([]byte, len(input)*2)
	for i := 0; i < len(input)/2; i++ {
		copy(stereo[i*4:i*4+2], input[i*2:i*2+2])
	}
	out := convertTest(t, stereo, Format{SampleRate: 24000, Stereo: true}, Format{SampleRate: 16000, Stereo: true}, 3, 13)
	mono := convertTest(t, input, Format{SampleRate: 24000}, Format{SampleRate: 16000}, 0, 4096)
	for i := 0; i < len(out)/4; i++ {
		if out[i*4+2] != 0 || out[i*4+3] != 0 {
			t.Fatalf("crosstalk at %d", i)
		}
		// Scalar stereo and unrolled mono summation may differ at a quantization boundary.
		a := int16(out[i*4]) | int16(out[i*4+1])<<8
		b := int16(mono[i*2]) | int16(mono[i*2+1])<<8
		if math.Abs(float64(a)-float64(b)) > 1 {
			t.Fatalf("left channel differs at %d: %d %d", i, a, b)
		}
	}
}
func TestFIRFrequencyResponse(t *testing.T) {
	// Measure the signal itself, including interpolation phases; no dependency on
	// filter coefficients or a second copy of the implementation.
	for _, rates := range [][2]int{{24000, 16000}, {48000, 16000}, {44100, 16000}, {47999, 16000}, {16000, 48000}} {
		for _, stop := range []bool{false, true} {
			freq := 1000.0
			if stop {
				if rates[0] <= rates[1] {
					continue
				}
				freq = float64(rates[1]) * .52
			}
			input := make([]float64, rates[0]/5)
			for i := range input {
				input[i] = math.Sin(2 * math.Pi * freq * float64(i) / float64(rates[0]))
			}
			f := newFIR(rates[0], rates[1], 1)
			out, err := f.Process(input)
			if err != nil {
				t.Fatal(err)
			}
			tail, err := f.Flush()
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, tail...)
			start, end := rates[1]/50, len(out)-rates[1]/50
			energy, errorEnergy := 0.0, 0.0
			for i := start; i < end; i++ {
				energy += out[i] * out[i]
				d := out[i] - math.Sin(2*math.Pi*freq*float64(i)/float64(rates[1]))
				errorEnergy += d * d
			}
			rms := math.Sqrt(energy / float64(end-start))
			errRMS := math.Sqrt(errorEnergy / float64(end-start))
			if stop && rms > 1e-4 {
				t.Fatalf("%v stopband RMS=%g (> -80 dBFS)", rates, rms)
			}
			if !stop && errRMS > 1e-4 {
				t.Fatalf("%v passband RMS error=%g", rates, errRMS)
			}
		}
	}
}
func TestFIREmptyAndPartialFrame(t *testing.T) {
	for _, stereo := range []bool{false, true} {
		src := Format{SampleRate: 24000, Stereo: stereo}
		dst := Format{SampleRate: 16000, Stereo: stereo}
		if got := convertTest(t, nil, src, dst, 0, 64); len(got) != 0 {
			t.Fatalf("empty stream generated %d bytes", len(got))
		}
		r, err := New(bytes.NewReader([]byte{1}), src, dst)
		if err != nil {
			t.Fatal(err)
		}
		_, err = readAll(r, 64)
		r.Close()
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("partial frame: %v", err)
		}
	}
}
func TestFIRRejectsInvalidRates(t *testing.T) {
	for _, rates := range [][2]int{{0, 16000}, {16000, 0}, {-1, -1}, {1, 257}, {257, 1}} {
		if r, err := New(bytes.NewReader(nil), Format{SampleRate: rates[0]}, Format{SampleRate: rates[1]}); err == nil {
			r.Close()
			t.Fatalf("accepted %v", rates)
		}
	}
}

func TestFIRPhaseBankResponse(t *testing.T) {
	for _, rates := range [][2]int{{24000, 16000}, {48000, 16000}, {44100, 16000}, {47999, 16000}, {8000, 16000}, {44100, 48000}} {
		f := newFIR(rates[0], rates[1], 1)
		nyquist := .5 * math.Min(1, float64(rates[1])/float64(rates[0]))
		for p := 0; p < len(f.bank); p += max(1, f.phases/7) {
			h := f.bank[p]
			for step := 0; step <= 100; step++ {
				freq := nyquist * .88 * float64(step) / 100
				re, im := 0.0, 0.0
				for k, v := range h {
					phase := 2 * math.Pi * freq * float64(k)
					re += v * math.Cos(phase)
					im += v * math.Sin(phase)
				}
				gain := math.Hypot(re, im)
				if math.Abs(gain-1) > 1e-4 {
					t.Fatalf("%v phase=%d frequency=%g passband gain=%g", rates, p, freq, gain)
				}
			}
			if rates[0] <= rates[1] {
				continue
			}
			for step := 0; step <= 100; step++ {
				freq := nyquist + (.5-nyquist)*float64(step)/100
				re, im := 0.0, 0.0
				for k, v := range h {
					phase := 2 * math.Pi * freq * float64(k)
					re += v * math.Cos(phase)
					im += v * math.Sin(phase)
				}
				gain := math.Hypot(re, im)
				if gain > 1e-4 {
					t.Fatalf("%v phase=%d frequency=%g stopband gain=%g", rates, p, freq, gain)
				}
			}
		}
	}
}

func TestFIRPreservesLeadingSilence(t *testing.T) {
	input := append(make([]byte, 24000*2*80/1000), generateSinePCM(24000, 1000, .1)...)
	out := convertTest(t, input, Format{SampleRate: 24000}, Format{SampleRate: 16000}, 7, 37)
	// The linear-phase filter can ring up to 3 ms before the onset; it must not
	// trim the input's leading samples as an implicit codec-delay correction.
	silence := out[:16000*2*77/1000]
	if bytes.Count(silence, []byte{0}) != len(silence) {
		t.Fatal("leading silence was trimmed")
	}
	if bytes.Count(out, []byte{0}) == len(out) {
		t.Fatal("lost the audible signal")
	}
}

func TestFIRFlushWhenSourceReturnsDataAndEOF(t *testing.T) {
	input := generateSinePCM(24000, 1000, .1)
	src, dst := Format{SampleRate: 24000}, Format{SampleRate: 16000}
	want := convertTest(t, input, src, dst, 0, 65536)
	r, err := New(&dataEOFReader{data: input}, src, dst)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, err := readAll(r, 65536)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("data+EOF output differs: got %d bytes, want %d", len(got), len(want))
	}
}
