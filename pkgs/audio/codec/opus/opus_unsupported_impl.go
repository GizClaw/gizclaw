//go:build !cgo || !(linux || darwin) || ((linux || darwin) && cgo && !amd64 && !arm64)

package opus

import (
	"fmt"
	"runtime"
)

func unsupportedErr() error {
	return unsupportedRuntimeError(runtime.GOOS, runtime.GOARCH, nativeCGOEnabled)
}

// Version returns a static marker on unsupported platforms.
func Version() string {
	return "unsupported"
}

// Encoder is a compatibility wrapper for unsupported targets.
type Encoder struct {
	sampleRate int
	channels   int
	closed     bool
}

// NewEncoder validates parameters then returns unsupported error.
func NewEncoder(sampleRate, channels int, app Application) (*Encoder, error) {
	if err := validateSampleRate(sampleRate); err != nil {
		return nil, err
	}
	if err := validateChannels(channels); err != nil {
		return nil, err
	}
	if err := validateApplication(app); err != nil {
		return nil, err
	}
	return nil, unsupportedErr()
}

// SampleRate returns encoder sample rate.
func (e *Encoder) SampleRate() int {
	if e == nil {
		return 0
	}
	return e.sampleRate
}

// Channels returns encoder channel count.
func (e *Encoder) Channels() int {
	if e == nil {
		return 0
	}
	return e.channels
}

// SetComplexity validates input then returns unsupported error.
func (e *Encoder) SetComplexity(complexity int) error {
	if e == nil {
		return fmt.Errorf("opus: encoder is nil")
	}
	if e.closed {
		return fmt.Errorf("opus: encoder is nil")
	}
	if err := validateComplexity(complexity); err != nil {
		return err
	}
	return unsupportedErr()
}

// Encode validates input then returns unsupported error.
func (e *Encoder) Encode(pcm []int16, frameSize int) ([]byte, error) {
	return e.EncodeWithMaxDataBytes(pcm, frameSize, DefaultMaxPacketSize)
}

// EncodeWithMaxDataBytes validates input then returns unsupported error.
func (e *Encoder) EncodeWithMaxDataBytes(pcm []int16, frameSize, maxDataBytes int) ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("opus: encoder is nil")
	}
	if e.closed {
		return nil, fmt.Errorf("opus: encoder is nil")
	}
	if err := validateFrameSize(e.sampleRate, frameSize); err != nil {
		return nil, err
	}
	if err := validateMaxDataBytes(maxDataBytes); err != nil {
		return nil, err
	}
	if err := validatePCM(pcm, frameSize, e.channels); err != nil {
		return nil, err
	}
	return nil, unsupportedErr()
}

// Close is safe to call repeatedly.
func (e *Encoder) Close() error {
	if e != nil {
		e.closed = true
	}
	return nil
}

// Decoder is a compatibility wrapper for unsupported targets.
type Decoder struct {
	sampleRate int
	channels   int
	closed     bool
}

// NewDecoder validates parameters then returns unsupported error.
func NewDecoder(sampleRate, channels int) (*Decoder, error) {
	if err := validateSampleRate(sampleRate); err != nil {
		return nil, err
	}
	if err := validateChannels(channels); err != nil {
		return nil, err
	}
	return nil, unsupportedErr()
}

// SampleRate returns decoder sample rate.
func (d *Decoder) SampleRate() int {
	if d == nil {
		return 0
	}
	return d.sampleRate
}

// Channels returns decoder channel count.
func (d *Decoder) Channels() int {
	if d == nil {
		return 0
	}
	return d.channels
}

// Decode validates input then returns unsupported error.
func (d *Decoder) Decode(packet []byte, frameSize int, fec bool) ([]int16, error) {
	_ = packet
	_ = fec
	if d == nil {
		return nil, fmt.Errorf("opus: decoder is nil")
	}
	if d.closed {
		return nil, fmt.Errorf("opus: decoder is nil")
	}
	if err := validateDecodeFrameSize(d.sampleRate, frameSize); err != nil {
		return nil, err
	}
	return nil, unsupportedErr()
}

// Close is safe to call repeatedly.
func (d *Decoder) Close() error {
	if d != nil {
		d.closed = true
	}
	return nil
}
