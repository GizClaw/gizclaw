package timestretch

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	frameDuration     = 0.030 // analysis window length in seconds
	toleranceDuration = 0.008 // similarity search radius in seconds
	correlationStride = 2     // sample stride of the similarity measure
)

// Stretcher time-stretches one continuous PCM16 stream. It is not safe for
// concurrent use.
type Stretcher struct {
	channels int
	speed    float64
	frame    int     // window length N in samples per channel
	hop      int     // synthesis hop, N/2
	analysis float64 // analysis hop, hop*speed
	tol      int     // search radius
	window   []float32

	in      [][]float32 // per-channel input starting at absolute index inBase
	inBase  int
	out     [][]float32 // per-channel overlap-add output starting at outBase
	outBase int

	next      int // index of the next frame
	prevStart int // chosen input start of the previous frame
	emitted   int // absolute output index of the next sample to return
	partial   []byte
}

// New creates a Stretcher. speed is the playback-rate factor: 0.5 produces
// output twice as long as the input and 2 produces half as long.
func New(sampleRate, channels int, speed float64) (*Stretcher, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("timestretch: sample rate must be positive")
	}
	if channels <= 0 {
		return nil, fmt.Errorf("timestretch: channels must be positive")
	}
	if !(speed > 0) || math.IsInf(speed, 0) {
		return nil, fmt.Errorf("timestretch: speed must be positive")
	}
	frame := max(int(float64(sampleRate)*frameDuration)&^1, 4)
	hop := frame / 2
	window := make([]float32, frame)
	for i := range window {
		// A periodic Hann window sums to one at 50% overlap.
		window[i] = float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(frame)))
	}
	s := &Stretcher{
		channels: channels,
		speed:    speed,
		frame:    frame,
		hop:      hop,
		analysis: float64(hop) * speed,
		tol:      max(1, int(float64(sampleRate)*toleranceDuration)),
		window:   window,
	}
	s.reset()
	return s, nil
}

// Passthrough reports whether the Stretcher leaves audio unchanged.
func (s *Stretcher) Passthrough() bool {
	return s.speed == 1
}

// Write consumes interleaved little-endian PCM16 and returns the stretched
// audio that is final so far. A trailing odd byte is kept for the next call.
func (s *Stretcher) Write(pcm []byte) []byte {
	if s.Passthrough() {
		return s.passthrough(pcm)
	}
	s.appendPCM(pcm)
	s.process()
	return s.drain(s.next * s.hop)
}

// Flush returns the remaining stretched audio and resets the Stretcher for a
// new stream. The total output length is the input length divided by speed.
func (s *Stretcher) Flush() []byte {
	if s.Passthrough() {
		s.partial = nil
		return nil
	}
	// The input starts with hop samples of silence so the first frame fades in
	// over padding instead of speech; that padding is excluded from the length.
	inputSamples := s.inBase + len(s.in[0]) - s.hop
	end := s.hop + int(math.Round(float64(inputSamples)/s.speed))
	s.process()
	for s.next*s.hop < end {
		s.padInput(s.requiredInput(s.next))
		s.process()
	}
	result := s.drain(end)
	s.reset()
	return result
}

func (s *Stretcher) reset() {
	s.in = make([][]float32, s.channels)
	s.out = make([][]float32, s.channels)
	for ch := range s.in {
		s.in[ch] = make([]float32, s.hop)
	}
	s.inBase = 0
	s.outBase = 0
	s.next = 0
	s.prevStart = 0
	s.emitted = s.hop
	s.partial = nil
}

func (s *Stretcher) passthrough(pcm []byte) []byte {
	data := append(s.partial, pcm...)
	whole := len(data) - len(data)%(2*s.channels)
	s.partial = append([]byte(nil), data[whole:]...)
	return append([]byte(nil), data[:whole]...)
}

func (s *Stretcher) appendPCM(pcm []byte) {
	data := append(s.partial, pcm...)
	frameBytes := 2 * s.channels
	whole := len(data) - len(data)%frameBytes
	for offset := 0; offset < whole; offset += frameBytes {
		for ch := range s.channels {
			sample := int16(binary.LittleEndian.Uint16(data[offset+2*ch:]))
			s.in[ch] = append(s.in[ch], float32(sample)/32768)
		}
	}
	s.partial = append([]byte(nil), data[whole:]...)
}

func (s *Stretcher) requiredInput(frame int) int {
	return s.analysisStart(frame) + s.tol + s.frame
}

func (s *Stretcher) analysisStart(frame int) int {
	return int(math.Round(float64(frame) * s.analysis))
}

func (s *Stretcher) padInput(until int) {
	for s.inBase+len(s.in[0]) < until {
		for ch := range s.in {
			s.in[ch] = append(s.in[ch], 0)
		}
	}
}

// process overlap-adds every frame whose search range is fully available. At
// flush time the caller pads the input so trailing frames can complete.
func (s *Stretcher) process() {
	for s.requiredInput(s.next) <= s.inBase+len(s.in[0]) {
		start := 0
		if s.next > 0 {
			start = s.bestStart(s.analysisStart(s.next), s.prevStart+s.hop)
		}
		s.overlapAdd(start, s.next*s.hop)
		s.prevStart = start
		s.next++
		s.trimInput()
	}
}

// bestStart searches around the ideal analysis position for the segment whose
// first half best continues the previous frame's natural continuation.
func (s *Stretcher) bestStart(ideal, target int) int {
	lo := max(ideal-s.tol, s.inBase)
	hi := ideal + s.tol
	best, bestScore := ideal, math.Inf(-1)
	for candidate := lo; candidate <= hi; candidate++ {
		var dot, energy float64
		for i := 0; i < s.hop; i += correlationStride {
			a := s.mono(target + i)
			b := s.mono(candidate + i)
			dot += a * b
			energy += b * b
		}
		score := dot
		if energy > 0 {
			score = dot / math.Sqrt(energy)
		}
		if score > bestScore {
			best, bestScore = candidate, score
		}
	}
	return best
}

func (s *Stretcher) mono(index int) float64 {
	var sum float64
	for ch := range s.in {
		sum += float64(s.in[ch][index-s.inBase])
	}
	return sum
}

func (s *Stretcher) overlapAdd(start, at int) {
	for ch := range s.out {
		need := at + s.frame - s.outBase
		for len(s.out[ch]) < need {
			s.out[ch] = append(s.out[ch], 0)
		}
		in := s.in[ch][start-s.inBase:]
		out := s.out[ch][at-s.outBase:]
		for i, w := range s.window {
			out[i] += w * in[i]
		}
	}
}

// trimInput drops input no future frame can reach: the next search range and
// the next natural continuation both start at or after keep.
func (s *Stretcher) trimInput() {
	keep := min(s.analysisStart(s.next)-s.tol, s.prevStart+s.hop)
	if drop := keep - s.inBase; drop > 0 {
		for ch := range s.in {
			s.in[ch] = s.in[ch][drop:]
		}
		s.inBase = keep
	}
}

// drain returns output samples in [emitted, until) as interleaved PCM16.
func (s *Stretcher) drain(until int) []byte {
	if until <= s.emitted {
		return nil
	}
	count := until - s.emitted
	result := make([]byte, 0, count*2*s.channels)
	for index := s.emitted; index < until; index++ {
		for ch := range s.out {
			var value float32
			if offset := index - s.outBase; offset < len(s.out[ch]) {
				value = s.out[ch][offset]
			}
			result = binary.LittleEndian.AppendUint16(result, uint16(toInt16(value)))
		}
	}
	s.emitted = until
	if drop := s.emitted - s.outBase; drop > 0 {
		for ch := range s.out {
			s.out[ch] = s.out[ch][min(drop, len(s.out[ch])):]
		}
		s.outBase = s.emitted
	}
	return result
}

func toInt16(value float32) int16 {
	scaled := math.Round(float64(value) * 32768)
	switch {
	case scaled > math.MaxInt16:
		return math.MaxInt16
	case scaled < math.MinInt16:
		return math.MinInt16
	default:
		return int16(scaled)
	}
}
