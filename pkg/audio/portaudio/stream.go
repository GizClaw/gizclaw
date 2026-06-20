package portaudio

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkg/audio/pcm"
)

// CaptureOptions controls input stream creation.
type CaptureOptions struct {
	HasDeviceID     bool
	DeviceID        int
	FramesPerBuffer uint32
}

// PlaybackOptions controls output stream creation.
type PlaybackOptions struct {
	HasDeviceID     bool
	DeviceID        int
	FramesPerBuffer uint32
}

func (o CaptureOptions) deviceIDOrDefault() int {
	if !o.HasDeviceID {
		return DefaultDeviceID
	}
	return o.DeviceID
}

func (o PlaybackOptions) deviceIDOrDefault() int {
	if !o.HasDeviceID {
		return DefaultDeviceID
	}
	return o.DeviceID
}

// CaptureStream reads PCM bytes from an input device.
type CaptureStream struct {
	driver    *Driver
	handle    streamHandle
	config    StreamConfig
	frameSize int
	closed    atomic.Bool
}

// Config returns stream config.
func (s *CaptureStream) Config() StreamConfig {
	if s == nil {
		return StreamConfig{}
	}
	return s.config
}

// Read reads PCM bytes from capture stream.
func (s *CaptureStream) Read(p []byte) (int, error) {
	if s == nil || s.handle == nil {
		return 0, fmt.Errorf("portaudio: nil capture stream: %w", io.ErrClosedPipe)
	}
	if s.closed.Load() {
		return 0, fmt.Errorf("portaudio: capture stream closed: %w", io.ErrClosedPipe)
	}
	if len(p) == 0 {
		return 0, nil
	}
	if len(p)%s.frameSize != 0 {
		return 0, fmt.Errorf("portaudio: read buffer size %d must align to frame bytes %d", len(p), s.frameSize)
	}
	return s.handle.Read(p)
}

// Close stops and releases capture stream.
func (s *CaptureStream) Close() error {
	if s == nil {
		return nil
	}
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	var errs []error
	if s.handle != nil {
		if err := s.handle.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("portaudio: capture stop failed: %w", err))
		}
		if err := s.handle.Close(); err != nil {
			errs = append(errs, fmt.Errorf("portaudio: capture close failed: %w", err))
		}
	}
	if s.driver != nil {
		if err := s.driver.release(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// PlaybackStream writes PCM bytes to an output device.
type PlaybackStream struct {
	driver    *Driver
	handle    streamHandle
	config    StreamConfig
	frameSize int
	closed    atomic.Bool
}

// Config returns stream config.
func (s *PlaybackStream) Config() StreamConfig {
	if s == nil {
		return StreamConfig{}
	}
	return s.config
}

// Write writes PCM bytes into playback stream.
func (s *PlaybackStream) Write(p []byte) (int, error) {
	if s == nil || s.handle == nil {
		return 0, fmt.Errorf("portaudio: nil playback stream: %w", io.ErrClosedPipe)
	}
	if s.closed.Load() {
		return 0, fmt.Errorf("portaudio: playback stream closed: %w", io.ErrClosedPipe)
	}
	if len(p) == 0 {
		return 0, nil
	}
	if len(p)%s.frameSize != 0 {
		return 0, fmt.Errorf("portaudio: write buffer size %d must align to frame bytes %d", len(p), s.frameSize)
	}
	return s.handle.Write(p)
}

// Close stops and releases playback stream.
func (s *PlaybackStream) Close() error {
	if s == nil {
		return nil
	}
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	var errs []error
	if s.handle != nil {
		if err := s.handle.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("portaudio: playback stop failed: %w", err))
		}
		if err := s.handle.Close(); err != nil {
			errs = append(errs, fmt.Errorf("portaudio: playback close failed: %w", err))
		}
	}
	if s.driver != nil {
		if err := s.driver.release(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (d *Driver) open(direction streamDirection, cfg StreamConfig) (streamHandle, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if err := d.acquire(); err != nil {
		return nil, err
	}

	if err := d.backend.IsFormatSupported(direction, cfg); err != nil {
		_ = d.release()
		return nil, err
	}

	handle, err := d.backend.OpenStream(direction, cfg)
	if err != nil {
		_ = d.release()
		return nil, err
	}

	if err := handle.Start(); err != nil {
		_ = handle.Close()
		_ = d.release()
		return nil, err
	}

	return handle, nil
}

// OpenCapture opens an input stream according to pcm format.
func (d *Driver) OpenCapture(format pcm.Format, opts CaptureOptions) (*CaptureStream, error) {
	cfg := StreamConfigFromPCM(format, opts.deviceIDOrDefault(), opts.FramesPerBuffer)
	handle, err := d.open(directionInput, cfg)
	if err != nil {
		return nil, err
	}

	return &CaptureStream{
		driver:    d,
		handle:    handle,
		config:    cfg,
		frameSize: cfg.frameBytes(),
	}, nil
}

// OpenPlayback opens an output stream according to pcm format.
func (d *Driver) OpenPlayback(format pcm.Format, opts PlaybackOptions) (*PlaybackStream, error) {
	cfg := StreamConfigFromPCM(format, opts.deviceIDOrDefault(), opts.FramesPerBuffer)
	return d.OpenPlaybackConfig(cfg)
}

// OpenPlaybackConfig opens an output stream according to an explicit stream config.
func (d *Driver) OpenPlaybackConfig(cfg StreamConfig) (*PlaybackStream, error) {
	handle, err := d.open(directionOutput, cfg)
	if err != nil {
		return nil, err
	}

	return &PlaybackStream{
		driver:    d,
		handle:    handle,
		config:    cfg,
		frameSize: cfg.frameBytes(),
	}, nil
}

// OpenCapture opens an input stream via default driver.
func OpenCapture(format pcm.Format, opts CaptureOptions) (*CaptureStream, error) {
	return defaultDriver.OpenCapture(format, opts)
}

// OpenPlayback opens an output stream via default driver.
func OpenPlayback(format pcm.Format, opts PlaybackOptions) (*PlaybackStream, error) {
	return defaultDriver.OpenPlayback(format, opts)
}

// OpenPlaybackConfig opens an output stream via default driver.
func OpenPlaybackConfig(cfg StreamConfig) (*PlaybackStream, error) {
	return defaultDriver.OpenPlaybackConfig(cfg)
}
