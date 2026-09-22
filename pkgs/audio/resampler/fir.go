//go:build !js

package resampler

import "math"

// The 96-sample window at the lower sample rate and beta 9 preserve 88% of
// the lower Nyquist band with at least 80 dB stopband rejection. Downsampling
// scales the input-domain window so the transition width stays constant.
const (
	firHalfWindow      = 48
	firMaxPhases       = 256
	firMaxCoefficients = 262144
	firCutoff          = 0.94
	firWindowBeta      = 9
)

// fir uses an exact integer phase clock and a windowed sinc bank. Each stream
// owns its sample history. Large denominators use interpolated phases.
type fir struct {
	inRate, outRate, channels, half, phases int
	bank                                    [][]float64
	history                                 []float64
	// Absolute frame coordinates permit compaction without resetting the clock.
	base, received, position, phase int64
}

func newFIR(inRate, outRate, channels int) *fir {
	g := inRate
	for b := outRate; b != 0; {
		g, b = b, g%b
	}
	phases := min(outRate/g, firMaxPhases)
	half := int(math.Ceil(firHalfWindow * math.Max(1, float64(inRate)/float64(outRate))))
	phases = min(phases, max(1, firMaxCoefficients/(half*2+1)-1))
	f := &fir{inRate: inRate, outRate: outRate, channels: channels, half: half, phases: phases, base: int64(-half), history: make([]float64, half*channels)}
	f.bank = make([][]float64, phases+1)
	cutoff := firCutoff * math.Min(1, float64(outRate)/float64(inRate))
	windowScale := 1 / besselI0(firWindowBeta)
	for p := range f.bank {
		h := make([]float64, half*2+1)
		sum := 0.0
		for k := range h {
			x := float64(k-half) - float64(p)/float64(phases)
			w := 0.0
			if math.Abs(x) <= float64(half) {
				w = besselI0(firWindowBeta*math.Sqrt(1-x*x/float64(half*half))) * windowScale
			}
			v := cutoff
			if x != 0 {
				v = math.Sin(math.Pi*cutoff*x) / (math.Pi * x)
			}
			h[k] = v * w
			sum += h[k]
		}
		for k := range h {
			h[k] /= sum
		}
		f.bank[p] = h
	}
	return f
}

// Process copies complete interleaved frames into private history and emits
// output for which the lookahead is available.
func (f *fir) Process(input []float64) ([]float64, error) {
	f.history = append(f.history, input...)
	f.received += int64(len(input) / f.channels)
	return f.output(false), nil
}

// Flush zero-extends the final window but emits only the input duration.
// The owning reader calls it exactly once, at EOF.
func (f *fir) Flush() ([]float64, error) {
	f.history = append(f.history, make([]float64, f.half*f.channels)...)
	return f.output(true), nil
}

func (f *fir) output(final bool) []float64 {
	var out []float64
	limit := f.received - int64(f.half)
	if final {
		limit = f.received
	}
	if limit > f.position {
		out = make([]float64, 0, int((limit-f.position)*int64(f.outRate)/int64(f.inRate)+1)*f.channels)
	}
	for f.position < limit {
		phase := f.phase * int64(f.phases)
		p := int(phase / int64(f.outRate))
		blend := float64(phase%int64(f.outRate)) / float64(f.outRate)
		a, b := f.bank[p], f.bank[p+1]
		start := int(f.position-int64(f.half)-f.base) * f.channels
		for c := range f.channels {
			sum := 0.0
			if blend == 0 && f.channels == 1 {
				sum = dot(f.history[start:start+len(a)], a)
			} else if blend == 0 {
				for k, v := range a {
					sum += f.history[start+k*f.channels+c] * v
				}
			} else if f.channels == 1 {
				x := f.history[start : start+len(a)]
				left, right := dot(x, a), dot(x, b)
				sum = left + (right-left)*blend
			} else {
				for k, v := range a {
					sum += f.history[start+k*f.channels+c] * (v + (b[k]-v)*blend)
				}
			}
			out = append(out, sum)
		}
		f.phase += int64(f.inRate)
		f.position += f.phase / int64(f.outRate)
		f.phase %= int64(f.outRate)
	}
	discard := min(int(f.position-int64(f.half)-f.base), len(f.history)/f.channels)
	if discard > 0 {
		copy(f.history, f.history[discard*f.channels:])
		f.history = f.history[:len(f.history)-discard*f.channels]
		f.base += int64(discard)
	}
	return out
}

// Four independent accumulators avoid a serial floating-point dependency
// across the entire FIR window. No architecture-specific assembly is needed.
func dot(x, h []float64) float64 {
	a, b, c, d := 0.0, 0.0, 0.0, 0.0
	n := len(h)
	x = x[:n]
	k := 0
	for ; k+3 < n; k += 4 {
		a += x[k] * h[k]
		b += x[k+1] * h[k+1]
		c += x[k+2] * h[k+2]
		d += x[k+3] * h[k+3]
	}
	sum := (a + b) + (c + d)
	for ; k < n; k++ {
		sum += x[k] * h[k]
	}
	return sum
}

// besselI0 evaluates the modified Bessel function used by the Kaiser window.
// Its argument is bounded by the window beta (9).
func besselI0(x float64) float64 {
	sum, term := 1.0, 1.0
	for k := 1; k < 40; k++ {
		term *= x * x / (4 * float64(k*k))
		sum += term
		if term < sum*1e-16 {
			break
		}
	}
	return sum
}
