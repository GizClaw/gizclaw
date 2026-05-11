package voiceprint

import (
	"fmt"
	"math"
	"math/cmplx"

	"gonum.org/v1/gonum/dsp/fourier"
)

// fbankConfig configures the speaker-embedding filterbank frontend.
//
// This frontend matches the historic voiceprint pipeline rather than the
// generic shared audio frontend because small preprocessing changes can
// noticeably shift embedding quality.
type fbankConfig struct {
	SampleRate   int
	NumMels      int
	FrameLength  int
	FrameShift   int
	FFTSize      int
	PreEmphasis  float64
	EnergyFloor  float64
	LowFreq      float64
	HighFreq     float64
	RemoveDC     bool
	Center       bool
	PoveyWindow  bool
	UseLog10     bool
	TopDB        float64
	MeanNorm     bool
	StdNorm      bool
	NormalizePCM bool
}

func validateFbankConfig(cfg fbankConfig) error {
	if cfg.SampleRate <= 0 {
		return fmt.Errorf("voiceprint: invalid SampleRate %d", cfg.SampleRate)
	}
	if cfg.NumMels <= 0 {
		return fmt.Errorf("voiceprint: invalid NumMels %d", cfg.NumMels)
	}
	if cfg.FrameLength <= 0 {
		return fmt.Errorf("voiceprint: invalid FrameLength %d", cfg.FrameLength)
	}
	if cfg.FrameShift <= 0 {
		return fmt.Errorf("voiceprint: invalid FrameShift %d", cfg.FrameShift)
	}
	if cfg.FFTSize > 0 && cfg.FFTSize < cfg.FrameLength {
		return fmt.Errorf("voiceprint: invalid FFTSize %d for FrameLength %d", cfg.FFTSize, cfg.FrameLength)
	}
	if cfg.LowFreq < 0 {
		return fmt.Errorf("voiceprint: invalid LowFreq %f", cfg.LowFreq)
	}

	highFreq := cfg.HighFreq
	if highFreq <= 0 {
		highFreq = float64(cfg.SampleRate)/2.0 + highFreq
	}
	if highFreq <= cfg.LowFreq {
		return fmt.Errorf("voiceprint: invalid freq range low=%f high=%f", cfg.LowFreq, highFreq)
	}

	if cfg.EnergyFloor <= 0 {
		return fmt.Errorf("voiceprint: invalid EnergyFloor %f", cfg.EnergyFloor)
	}
	if cfg.TopDB < 0 {
		return fmt.Errorf("voiceprint: invalid TopDB %f", cfg.TopDB)
	}
	return nil
}

// computeFbank extracts log mel filterbank features from PCM16 audio.
func computeFbank(audio []byte, cfg fbankConfig) [][]float32 {
	if err := validateFbankConfig(cfg); err != nil {
		return nil
	}

	nSamples := len(audio) / 2
	if nSamples < cfg.FrameLength {
		return nil
	}

	samples := make([]float64, nSamples)
	for i := 0; i < nSamples; i++ {
		lo := audio[2*i]
		hi := audio[2*i+1]
		s := int16(lo) | int16(hi)<<8
		samples[i] = float64(s)
	}
	if cfg.NormalizePCM {
		for i := range samples {
			samples[i] /= 32768.0
		}
	}

	if cfg.Center {
		pad := cfg.FrameLength / 2
		padded := make([]float64, len(samples)+pad*2)
		copy(padded[pad:], samples)
		samples = padded
		nSamples = len(samples)
	}

	numFrames := (nSamples-cfg.FrameLength)/cfg.FrameShift + 1
	if numFrames <= 0 {
		return nil
	}

	fftSize := cfg.FFTSize
	if fftSize <= 0 {
		fftSize = nextPow2(cfg.FrameLength)
	}
	halfFFT := fftSize/2 + 1

	var window []float64
	if cfg.PoveyWindow {
		window = poveyWindow(cfg.FrameLength)
	} else {
		window = hammingWindow(cfg.FrameLength)
	}

	highFreq := cfg.HighFreq
	if highFreq <= 0 {
		highFreq = float64(cfg.SampleRate)/2.0 + highFreq
	}
	filterbank := melFilterbank(cfg.NumMels, fftSize, cfg.SampleRate, cfg.LowFreq, highFreq)

	result := make([][]float32, numFrames)
	fftEngine := fourier.NewFFT(fftSize)
	for f := range numFrames {
		offset := f * cfg.FrameShift
		frameBuf := make([]float64, cfg.FrameLength)
		copy(frameBuf, samples[offset:offset+cfg.FrameLength])

		if cfg.RemoveDC {
			var sum float64
			for _, v := range frameBuf {
				sum += v
			}
			mean := sum / float64(cfg.FrameLength)
			for i := range frameBuf {
				frameBuf[i] -= mean
			}
		}

		if cfg.PreEmphasis > 0 {
			for i := cfg.FrameLength - 1; i > 0; i-- {
				frameBuf[i] -= cfg.PreEmphasis * frameBuf[i-1]
			}
			frameBuf[0] *= 1.0 - cfg.PreEmphasis
		}

		fftInput := make([]float64, fftSize)
		for i := 0; i < cfg.FrameLength; i++ {
			fftInput[i] = frameBuf[i] * window[i]
		}
		coeffs := fftEngine.Coefficients(nil, fftInput)

		powerSpec := make([]float64, halfFFT)
		for k := range halfFFT {
			r := real(coeffs[k])
			im := imag(coeffs[k])
			powerSpec[k] = r*r + im*im
		}

		frame := make([]float32, cfg.NumMels)
		for m := 0; m < cfg.NumMels; m++ {
			var energy float64
			for k, w := range filterbank[m] {
				energy += w * powerSpec[k]
			}
			if energy < cfg.EnergyFloor {
				energy = cfg.EnergyFloor
			}
			if cfg.UseLog10 {
				frame[m] = float32(10.0 * math.Log10(energy))
			} else {
				frame[m] = float32(math.Log(energy))
			}
		}
		result[f] = frame
	}

	if cfg.UseLog10 && cfg.TopDB > 0 {
		maxVal := float32(-math.MaxFloat32)
		for _, frame := range result {
			for _, v := range frame {
				if v > maxVal {
					maxVal = v
				}
			}
		}
		floor := maxVal - float32(cfg.TopDB)
		for i := range result {
			for j := range result[i] {
				if result[i][j] < floor {
					result[i][j] = floor
				}
			}
		}
	}

	return result
}

func nextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

func hammingWindow(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(n-1))
	}
	return w
}

func poveyWindow(n int) []float64 {
	w := hammingWindow(n)
	for i := range w {
		w[i] = math.Pow(w[i], 0.85)
	}
	return w
}

func hzToMel(hz float64) float64 {
	return 2595.0 * math.Log10(1.0+hz/700.0)
}

func melToHz(mel float64) float64 {
	return 700.0 * (math.Pow(10.0, mel/2595.0) - 1.0)
}

func melFilterbank(numMels, fftSize, sampleRate int, lowFreq, highFreq float64) [][]float64 {
	halfFFT := fftSize/2 + 1

	melLow := hzToMel(lowFreq)
	melHigh := hzToMel(highFreq)

	melPoints := make([]float64, numMels+2)
	for i := range melPoints {
		melPoints[i] = melLow + float64(i)*(melHigh-melLow)/float64(numMels+1)
	}

	binIndices := make([]int, numMels+2)
	for i := range melPoints {
		hz := melToHz(melPoints[i])
		binIndices[i] = int(math.Floor(hz * float64(fftSize) / float64(sampleRate)))
		if binIndices[i] >= halfFFT {
			binIndices[i] = halfFFT - 1
		}
		if binIndices[i] < 0 {
			binIndices[i] = 0
		}
	}

	fb := make([][]float64, numMels)
	for m := range numMels {
		fb[m] = make([]float64, halfFFT)
		left := binIndices[m]
		center := binIndices[m+1]
		right := binIndices[m+2]

		for k := left; k <= center; k++ {
			if center > left {
				fb[m][k] = float64(k-left) / float64(center-left)
			}
		}
		for k := center; k <= right; k++ {
			if right > center {
				fb[m][k] = float64(right-k) / float64(right-center)
			}
		}
	}
	return fb
}

func fft(x []complex128) {
	n := len(x)
	if n <= 1 {
		return
	}

	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}

	for size := 2; size <= n; size <<= 1 {
		half := size / 2
		wn := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := range half {
				u := x[start+k]
				t := w * x[start+k+half]
				x[start+k] = u + t
				x[start+k+half] = u - t
				w *= wn
			}
		}
	}
}

func cmvn(features [][]float32, cfg fbankConfig) {
	if len(features) == 0 {
		return
	}
	if !cfg.MeanNorm && !cfg.StdNorm {
		return
	}
	numMels := len(features[0])
	T := float64(len(features))

	for m := range numMels {
		var sum float64
		for _, f := range features {
			sum += float64(f[m])
		}
		mean := sum / T

		std := 1.0
		if cfg.StdNorm {
			var varSum float64
			for _, f := range features {
				d := float64(f[m]) - mean
				varSum += d * d
			}
			std = math.Sqrt(varSum / T)
			if std < 1e-10 {
				std = 1e-10
			}
		}

		for _, f := range features {
			v := float64(f[m])
			if cfg.MeanNorm {
				v -= mean
			}
			if cfg.StdNorm {
				v /= std
			}
			f[m] = float32(v)
		}
	}
}

func flatten(features [][]float32) []float32 {
	if len(features) == 0 {
		return nil
	}
	cols := len(features[0])
	flat := make([]float32, len(features)*cols)
	for t, row := range features {
		copy(flat[t*cols:], row)
	}
	return flat
}
