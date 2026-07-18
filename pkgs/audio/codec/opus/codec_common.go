package opus

import (
	"fmt"
	"runtime"
)

// Application maps to libopus encoder application modes.
type Application int

// OpusSampleRate represents one of the discrete sample rates supported by Opus.
type OpusSampleRate int

const (
	// ApplicationVoIP optimizes for voice speech.
	ApplicationVoIP Application = 2048

	// ApplicationAudio optimizes for general audio fidelity.
	ApplicationAudio Application = 2049

	// ApplicationRestrictedLowDelay optimizes for lowest delay.
	ApplicationRestrictedLowDelay Application = 2051

	// DefaultMaxPacketSize is the recommended upper bound for one Opus packet.
	DefaultMaxPacketSize = 4000
)

const (
	SampleRate8K  OpusSampleRate = 8000
	SampleRate12K OpusSampleRate = 12000
	SampleRate16K OpusSampleRate = 16000
	SampleRate24K OpusSampleRate = 24000
	SampleRate48K OpusSampleRate = 48000
)

var supportedSampleRates = map[int]struct{}{
	8000:  {},
	12000: {},
	16000: {},
	24000: {},
	48000: {},
}

// IsRuntimeSupported reports whether native cgo-backed Opus is available.
func IsRuntimeSupported() bool {
	return nativeCGOEnabled && isSupportedPlatform(runtime.GOOS, runtime.GOARCH)
}

func (r OpusSampleRate) Int() int {
	return int(r)
}

func (r OpusSampleRate) Validate() error {
	return validateSampleRate(int(r))
}

func validateSampleRate(sampleRate int) error {
	if _, ok := supportedSampleRates[sampleRate]; !ok {
		return fmt.Errorf("opus: unsupported sample rate %d (allowed: 8000, 12000, 16000, 24000, 48000)", sampleRate)
	}
	return nil
}

func validateChannels(channels int) error {
	if channels != 1 && channels != 2 {
		return fmt.Errorf("opus: invalid channel count %d (allowed: 1 or 2)", channels)
	}
	return nil
}

func validateApplication(app Application) error {
	switch app {
	case ApplicationVoIP, ApplicationAudio, ApplicationRestrictedLowDelay:
		return nil
	default:
		return fmt.Errorf("opus: unsupported application %d", app)
	}
}

func validateComplexity(complexity int) error {
	if complexity < 0 || complexity > 10 {
		return fmt.Errorf("opus: complexity must be between 0 and 10, got %d", complexity)
	}
	return nil
}

func validateFrameSize(sampleRate, frameSize int) error {
	if frameSize <= 0 {
		return fmt.Errorf("opus: frame size must be positive, got %d", frameSize)
	}

	allowed := [...]int{
		sampleRate / 400,      // 2.5 ms
		sampleRate / 200,      // 5 ms
		sampleRate / 100,      // 10 ms
		sampleRate / 50,       // 20 ms
		sampleRate / 25,       // 40 ms
		(sampleRate * 3) / 50, // 60 ms
	}

	for _, v := range allowed {
		if frameSize == v {
			return nil
		}
	}

	return fmt.Errorf("opus: invalid frame size %d for sample rate %d", frameSize, sampleRate)
}

func validateDecodeFrameSize(sampleRate, frameSize int) error {
	if frameSize == sampleRate*3/25 { // 120 ms maximum Opus packet duration.
		return nil
	}
	return validateFrameSize(sampleRate, frameSize)
}

func validatePCM(pcm []int16, frameSize, channels int) error {
	if len(pcm) == 0 {
		return fmt.Errorf("opus: empty pcm input")
	}
	need, ok := checkedMul(frameSize, channels)
	if !ok {
		return fmt.Errorf("opus: pcm frame shape overflow: frameSize=%d channels=%d", frameSize, channels)
	}
	if len(pcm) != need {
		return fmt.Errorf("opus: pcm length mismatch: got %d, want %d", len(pcm), need)
	}
	return nil
}

func validateMaxDataBytes(maxDataBytes int) error {
	if maxDataBytes <= 0 {
		return fmt.Errorf("opus: max data bytes must be positive, got %d", maxDataBytes)
	}
	if maxDataBytes > DefaultMaxPacketSize {
		return fmt.Errorf("opus: max data bytes %d exceeds limit %d", maxDataBytes, DefaultMaxPacketSize)
	}
	return nil
}

func unsupportedRuntimeError(goos, goarch string, cgoEnabled bool) error {
	if !cgoEnabled {
		if isSupportedPlatform(goos, goarch) {
			return fmt.Errorf("opus: cgo is disabled on %s/%s; rebuild with CGO_ENABLED=1", goos, goarch)
		}
		return fmt.Errorf("opus: cgo is disabled and platform %s/%s is unsupported; phase 1 supports %s", goos, goarch, supportedPlatformDescription)
	}

	return fmt.Errorf("opus: unsupported platform %s/%s: phase 1 supports %s", goos, goarch, supportedPlatformDescription)
}
