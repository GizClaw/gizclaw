package portaudio

import (
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
)

const (
	// DefaultDeviceID tells PortAudio to use system default input/output device.
	DefaultDeviceID = -1

	bytesPerSample = 2 // int16 LE
)

var (
	ErrDeviceNotFound        = errors.New("portaudio: device not found")
	ErrInvalidSampleRate     = errors.New("portaudio: invalid sample rate")
	ErrInvalidChannelCount   = errors.New("portaudio: invalid channel count")
	ErrInvalidFramesPerBatch = errors.New("portaudio: invalid frames_per_buffer")
)

// DeviceInfo describes an audio IO device discovered by backend.
type DeviceInfo struct {
	ID                     int
	Name                   string
	HostAPI                string
	MaxInputChannels       int
	MaxOutputChannels      int
	DefaultSampleRate      float64
	DefaultInputLatencyMs  float64
	DefaultOutputLatencyMs float64
}

// StreamConfig is the runtime stream configuration for capture/playback.
type StreamConfig struct {
	DeviceID        int
	SampleRate      float64
	Channels        int
	FramesPerBuffer uint32
}

// Validate checks config constraints.
func (cfg StreamConfig) Validate() error {
	if cfg.SampleRate <= 0 {
		return fmt.Errorf("%w: %.2f", ErrInvalidSampleRate, cfg.SampleRate)
	}
	if cfg.Channels <= 0 {
		return fmt.Errorf("%w: %d", ErrInvalidChannelCount, cfg.Channels)
	}
	if cfg.FramesPerBuffer == 0 {
		return fmt.Errorf("%w: %d", ErrInvalidFramesPerBatch, cfg.FramesPerBuffer)
	}
	return nil
}

func (cfg StreamConfig) frameBytes() int {
	return cfg.Channels * bytesPerSample
}

func defaultFramesPerBuffer(sampleRate int) uint32 {
	if sampleRate <= 0 {
		return 320
	}
	// 20ms block for low-latency streaming.
	frames := max(sampleRate/50, 64)
	return uint32(frames)
}

// StreamConfigFromPCM maps pcm.Format into a stream config.
func StreamConfigFromPCM(format pcm.Format, deviceID int, framesPerBuffer uint32) StreamConfig {
	if framesPerBuffer == 0 {
		framesPerBuffer = defaultFramesPerBuffer(format.SampleRate())
	}
	return StreamConfig{
		DeviceID:        deviceID,
		SampleRate:      float64(format.SampleRate()),
		Channels:        format.Channels(),
		FramesPerBuffer: framesPerBuffer,
	}
}

// PCMFormatFromSampleRate converts sample-rate/channels to repository PCM format.
func PCMFormatFromSampleRate(sampleRate, channels int) (pcm.Format, error) {
	if channels != 1 {
		return 0, fmt.Errorf("portaudio: unsupported channels=%d for pcm package", channels)
	}

	switch sampleRate {
	case 16000:
		return pcm.L16Mono16K, nil
	case 24000:
		return pcm.L16Mono24K, nil
	case 48000:
		return pcm.L16Mono48K, nil
	default:
		return 0, fmt.Errorf("portaudio: unsupported sample rate for pcm package: %d", sampleRate)
	}
}
