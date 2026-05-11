package voiceprint

import (
	"math"
	"testing"
)

func TestComputeFbankBasic(t *testing.T) {
	cfg := eres2netFbankConfig()

	nSamples := 800
	audio := makeSineWavePCM(440, nSamples, cfg.SampleRate)

	result := computeFbank(audio, cfg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	expectedFrames := (nSamples-cfg.FrameLength)/cfg.FrameShift + 1
	if len(result) != expectedFrames {
		t.Fatalf("expected %d frames, got %d", expectedFrames, len(result))
	}

	for i, frame := range result {
		if len(frame) != cfg.NumMels {
			t.Fatalf("frame %d: expected %d mels, got %d", i, cfg.NumMels, len(frame))
		}
		for j, v := range frame {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				t.Fatalf("frame %d mel %d: non-finite value %f", i, j, v)
			}
		}
	}
}

func TestComputeFbankTooShort(t *testing.T) {
	cfg := eres2netFbankConfig()
	audio := make([]byte, cfg.FrameLength*2-2)
	result := computeFbank(audio, cfg)
	if result != nil {
		t.Fatalf("expected nil for too-short audio, got %d frames", len(result))
	}
}

func TestComputeFbankSineVsSilence(t *testing.T) {
	cfg := eres2netFbankConfig()
	nSamples := 1600

	sine := makeSineWavePCM(440, nSamples, cfg.SampleRate)
	sineFbank := computeFbank(sine, cfg)
	silence := make([]byte, nSamples*2)
	silenceFbank := computeFbank(silence, cfg)
	if sineFbank == nil || silenceFbank == nil {
		t.Fatal("expected non-nil results")
	}

	var sineSum, silenceSum float64
	for _, frame := range sineFbank {
		for _, v := range frame {
			sineSum += float64(v)
		}
	}
	for _, frame := range silenceFbank {
		for _, v := range frame {
			silenceSum += float64(v)
		}
	}

	if sineSum <= silenceSum {
		t.Fatalf("sine energy (%f) should be > silence energy (%f)", sineSum, silenceSum)
	}
}

func TestComputeFbankDifferentFreqs(t *testing.T) {
	cfg := eres2netFbankConfig()
	nSamples := 3200

	lowFreq := makeSineWavePCM(200, nSamples, cfg.SampleRate)
	highFreq := makeSineWavePCM(4000, nSamples, cfg.SampleRate)

	lowFbank := computeFbank(lowFreq, cfg)
	highFbank := computeFbank(highFreq, cfg)
	if lowFbank == nil || highFbank == nil {
		t.Fatal("expected non-nil results")
	}

	var lowLow, highLow float64
	for _, frame := range lowFbank {
		for i := range 20 {
			lowLow += float64(frame[i])
		}
	}
	for _, frame := range highFbank {
		for i := range 20 {
			highLow += float64(frame[i])
		}
	}
	if lowLow <= highLow {
		t.Fatal("200Hz should have more energy in low mel bins than 4kHz")
	}
}

func TestComputeFbankInvalidConfig(t *testing.T) {
	if result := computeFbank([]byte{0, 0, 0, 0}, fbankConfig{}); result != nil {
		t.Fatal("expected nil result for invalid config")
	}
}

func TestFFTPowerOfTwo(t *testing.T) {
	n := 8
	x := make([]complex128, n)
	x[0] = 1
	fft(x)

	for i, v := range x {
		if math.Abs(real(v)-1.0) > 1e-10 || math.Abs(imag(v)) > 1e-10 {
			t.Fatalf("FFT[%d] = %v, expected 1+0i", i, v)
		}
	}
}

func TestNextPow2(t *testing.T) {
	tests := []struct{ in, want int }{
		{1, 1}, {2, 2}, {3, 4}, {4, 4}, {5, 8},
		{400, 512}, {512, 512}, {513, 1024},
	}
	for _, tt := range tests {
		if got := nextPow2(tt.in); got != tt.want {
			t.Fatalf("nextPow2(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestMelConversion(t *testing.T) {
	for _, hz := range []float64{0, 100, 1000, 4000, 8000} {
		mel := hzToMel(hz)
		back := melToHz(mel)
		if math.Abs(back-hz) > 0.01 {
			t.Fatalf("round-trip failed: %f Hz -> %f mel -> %f Hz", hz, mel, back)
		}
	}
}

func TestCMVNAndFlatten(t *testing.T) {
	features := [][]float32{
		{1, 2},
		{3, 4},
	}
	cmvn(features, eres2netFbankConfig())
	flat := flatten(features)
	if len(flat) != 4 {
		t.Fatalf("flatten len = %d", len(flat))
	}
	if math.Abs(float64(features[0][0]+features[1][0])) > 1e-5 {
		t.Fatalf("cmvn should center the first column: %v", features)
	}
}

func TestCMVNMeanOnly(t *testing.T) {
	features := [][]float32{
		{1, 10},
		{3, 14},
	}
	cfg := fbankConfig{
		SampleRate:   16000,
		NumMels:      80,
		FrameLength:  400,
		FrameShift:   160,
		FFTSize:      400,
		PreEmphasis:  0,
		EnergyFloor:  1e-10,
		LowFreq:      0,
		HighFreq:     8000,
		RemoveDC:     false,
		Center:       true,
		PoveyWindow:  false,
		UseLog10:     true,
		TopDB:        80,
		MeanNorm:     true,
		StdNorm:      false,
		NormalizePCM: true,
	}
	cmvn(features, cfg)
	if got := features[0][0] + features[1][0]; math.Abs(float64(got)) > 1e-5 {
		t.Fatalf("mean-only cmvn should center first column: %v", features)
	}
	if got := features[0][1] + features[1][1]; math.Abs(float64(got)) > 1e-5 {
		t.Fatalf("mean-only cmvn should center second column: %v", features)
	}
	if math.Abs(float64(features[0][0]+1)) > 1e-5 || math.Abs(float64(features[1][0]-1)) > 1e-5 {
		t.Fatalf("mean-only cmvn should not scale variance: %v", features)
	}
}
