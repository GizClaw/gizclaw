package timestretch

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

const testSampleRate = 24000

func sinePCM(frequency float64, samples, channels int) []byte {
	out := make([]byte, 0, samples*2*channels)
	for i := range samples {
		value := int16(0.5 * 32767 * math.Sin(2*math.Pi*frequency*float64(i)/testSampleRate))
		for range channels {
			out = binary.LittleEndian.AppendUint16(out, uint16(value))
		}
	}
	return out
}

func stretchAll(t *testing.T, speed float64, channels int, pcm []byte, chunk int) []byte {
	t.Helper()
	stretcher, err := New(testSampleRate, channels, speed)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var out []byte
	for offset := 0; offset < len(pcm); offset += chunk {
		out = append(out, stretcher.Write(pcm[offset:min(offset+chunk, len(pcm))])...)
	}
	return append(out, stretcher.Flush()...)
}

// zeroCrossingRate counts sign changes per second of the first channel,
// ignoring the edges where the stretched signal fades in and out.
func zeroCrossingRate(pcm []byte, channels int) float64 {
	samples := len(pcm) / (2 * channels)
	margin := samples / 10
	var crossings int
	previous := int16(binary.LittleEndian.Uint16(pcm[margin*2*channels:]))
	for i := margin + 1; i < samples-margin; i++ {
		current := int16(binary.LittleEndian.Uint16(pcm[i*2*channels:]))
		if (previous < 0) != (current < 0) {
			crossings++
		}
		previous = current
	}
	return float64(crossings) / (float64(samples-2*margin) / testSampleRate)
}

// rms measures the level of the first channel away from the fade edges.
func rms(pcm []byte) float64 {
	samples := len(pcm) / 2
	margin := samples / 10
	var sum float64
	for i := margin; i < samples-margin; i++ {
		value := float64(int16(binary.LittleEndian.Uint16(pcm[i*2:])))
		sum += value * value
	}
	return math.Sqrt(sum / float64(samples-2*margin))
}

func TestStretcherScalesDurationAndKeepsPitch(t *testing.T) {
	t.Parallel()
	input := sinePCM(220, testSampleRate, 1)
	inputRate := zeroCrossingRate(input, 1)
	for _, speed := range []float64{0.5, 0.7, 1.5, 2} {
		output := stretchAll(t, speed, 1, input, 960)
		wantSamples := int(math.Round(float64(testSampleRate) / speed))
		if got := len(output) / 2; got != wantSamples {
			t.Fatalf("speed %.1f output samples = %d, want %d", speed, got, wantSamples)
		}
		if rate := zeroCrossingRate(output, 1); math.Abs(rate-inputRate)/inputRate > 0.05 {
			t.Fatalf("speed %.1f zero crossings/s = %.1f, want about %.1f", speed, rate, inputRate)
		}
		if level, want := rms(output), rms(input); math.Abs(level-want)/want > 0.1 {
			t.Fatalf("speed %.1f RMS = %.1f, want about %.1f", speed, level, want)
		}
	}
}

func TestStretcherOutputDoesNotDependOnChunking(t *testing.T) {
	t.Parallel()
	input := sinePCM(330, testSampleRate/2, 2)
	whole := stretchAll(t, 0.6, 2, input, len(input))
	for _, chunk := range []int{1, 3, 480, 4099} {
		if got := stretchAll(t, 0.6, 2, input, chunk); !bytes.Equal(got, whole) {
			t.Fatalf("chunk %d output differs from single write (%d vs %d bytes)", chunk, len(got), len(whole))
		}
	}
}

func TestStretcherPassthroughAtNormalSpeed(t *testing.T) {
	t.Parallel()
	input := sinePCM(440, 1000, 1)
	stretcher, err := New(testSampleRate, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !stretcher.Passthrough() {
		t.Fatal("Passthrough() = false at speed 1")
	}
	got := append(stretcher.Write(input[:101]), stretcher.Write(input[101:])...)
	if !bytes.Equal(got, input) {
		t.Fatal("speed 1 changed the audio")
	}
	if flushed := stretcher.Flush(); len(flushed) != 0 {
		t.Fatalf("Flush() = %d bytes, want none", len(flushed))
	}
}

func TestStretcherResetsAfterFlush(t *testing.T) {
	t.Parallel()
	input := sinePCM(220, testSampleRate/4, 1)
	stretcher, err := New(testSampleRate, 1, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	first := append(stretcher.Write(input), stretcher.Flush()...)
	second := append(stretcher.Write(input), stretcher.Flush()...)
	if !bytes.Equal(first, second) {
		t.Fatal("second stream differs after Flush reset")
	}
	if empty := stretcher.Flush(); len(empty) != 0 {
		t.Fatalf("Flush() on empty stream = %d bytes, want none", len(empty))
	}
}

func TestNewRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		rate, channels int
		speed          float64
	}{
		{0, 1, 1}, {testSampleRate, 0, 1}, {testSampleRate, 1, 0}, {testSampleRate, 1, -1},
		{testSampleRate, 1, math.NaN()}, {testSampleRate, 1, math.Inf(1)},
	} {
		if _, err := New(tc.rate, tc.channels, tc.speed); err == nil {
			t.Fatalf("New(%d, %d, %v) error = nil", tc.rate, tc.channels, tc.speed)
		}
	}
}
