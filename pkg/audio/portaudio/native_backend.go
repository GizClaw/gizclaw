//go:build cgo && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)))

package portaudio

/*
#include <portaudio.h>
*/
import "C"

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

type nativeBackend struct{}

func newBackend() backend {
	return nativeBackend{}
}

func (nativeBackend) Name() string {
	version := strings.TrimSpace(C.GoString(C.Pa_GetVersionText()))
	if version == "" {
		return "portaudio/native"
	}
	return "portaudio/native:" + version
}

func (nativeBackend) Init() error {
	if code := C.Pa_Initialize(); code != C.paNoError {
		return paErr(code, "initialize")
	}
	return nil
}

func (nativeBackend) Terminate() error {
	if code := C.Pa_Terminate(); code != C.paNoError {
		return paErr(code, "terminate")
	}
	return nil
}

func (nativeBackend) ListDevices() ([]DeviceInfo, error) {
	deviceCount := int(C.Pa_GetDeviceCount())
	if deviceCount < 0 {
		return nil, paErr(C.PaError(deviceCount), "get device count")
	}

	devices := make([]DeviceInfo, 0, deviceCount)
	for i := range deviceCount {
		idx := C.PaDeviceIndex(i)
		info := C.Pa_GetDeviceInfo(idx)
		if info == nil {
			continue
		}

		hostAPIName := ""
		hostAPI := C.Pa_GetHostApiInfo(info.hostApi)
		if hostAPI != nil {
			hostAPIName = C.GoString(hostAPI.name)
		}

		devices = append(devices, DeviceInfo{
			ID:                     i,
			Name:                   C.GoString(info.name),
			HostAPI:                hostAPIName,
			MaxInputChannels:       int(info.maxInputChannels),
			MaxOutputChannels:      int(info.maxOutputChannels),
			DefaultSampleRate:      float64(info.defaultSampleRate),
			DefaultInputLatencyMs:  float64(info.defaultLowInputLatency) * 1000,
			DefaultOutputLatencyMs: float64(info.defaultLowOutputLatency) * 1000,
		})
	}

	return devices, nil
}

func (nativeBackend) DefaultInputDevice() (int, error) {
	id := int(C.Pa_GetDefaultInputDevice())
	if id < 0 {
		return 0, ErrDeviceNotFound
	}
	return id, nil
}

func (nativeBackend) DefaultOutputDevice() (int, error) {
	id := int(C.Pa_GetDefaultOutputDevice())
	if id < 0 {
		return 0, ErrDeviceNotFound
	}
	return id, nil
}

func (nativeBackend) IsFormatSupported(direction streamDirection, cfg StreamConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	input, output, err := buildPaStreamParameters(direction, cfg)
	if err != nil {
		return err
	}

	code := C.Pa_IsFormatSupported(input, output, C.double(cfg.SampleRate))
	if code == C.paFormatIsSupported {
		return nil
	}
	return paErr(code, "format not supported")
}

func (nativeBackend) OpenStream(direction streamDirection, cfg StreamConfig) (streamHandle, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	input, output, err := buildPaStreamParameters(direction, cfg)
	if err != nil {
		return nil, err
	}

	var stream unsafe.Pointer
	code := C.Pa_OpenStream(
		(*unsafe.Pointer)(unsafe.Pointer(&stream)),
		input,
		output,
		C.double(cfg.SampleRate),
		C.ulong(cfg.FramesPerBuffer),
		C.paNoFlag,
		nil,
		nil,
	)
	if code != C.paNoError {
		return nil, paErr(code, "open stream")
	}

	return &nativeStream{
		stream:    stream,
		direction: direction,
		frameSize: cfg.frameBytes(),
	}, nil
}

func buildPaStreamParameters(direction streamDirection, cfg StreamConfig) (*C.PaStreamParameters, *C.PaStreamParameters, error) {
	deviceID := cfg.DeviceID
	if deviceID < 0 {
		if direction == directionInput {
			deviceID = int(C.Pa_GetDefaultInputDevice())
		} else {
			deviceID = int(C.Pa_GetDefaultOutputDevice())
		}
	}
	if deviceID < 0 {
		return nil, nil, ErrDeviceNotFound
	}

	info := C.Pa_GetDeviceInfo(C.PaDeviceIndex(deviceID))
	if info == nil {
		return nil, nil, fmt.Errorf("%w: id=%d", ErrDeviceNotFound, deviceID)
	}

	params := &C.PaStreamParameters{
		device:                    C.PaDeviceIndex(deviceID),
		channelCount:              C.int(cfg.Channels),
		sampleFormat:              C.paInt16,
		hostApiSpecificStreamInfo: nil,
	}
	if direction == directionInput {
		params.suggestedLatency = info.defaultLowInputLatency
		return params, nil, nil
	}
	params.suggestedLatency = info.defaultLowOutputLatency
	return nil, params, nil
}

type nativeStream struct {
	mu sync.Mutex

	stream    unsafe.Pointer
	direction streamDirection
	frameSize int
	closed    bool
}

func (s *nativeStream) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.stream == nil {
		return fmt.Errorf("portaudio: stream is closed")
	}
	if code := C.Pa_StartStream(s.stream); code != C.paNoError {
		return paErr(code, "start stream")
	}
	return nil
}

func (s *nativeStream) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.stream == nil {
		return nil
	}
	if code := C.Pa_StopStream(s.stream); code != C.paNoError {
		return paErr(code, "stop stream")
	}
	return nil
}

func (s *nativeStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.stream == nil {
		return nil
	}
	code := C.Pa_CloseStream(s.stream)
	s.stream = nil
	if code != C.paNoError {
		return paErr(code, "close stream")
	}
	return nil
}

func (s *nativeStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if s.direction != directionInput {
		return 0, fmt.Errorf("portaudio: read called on output stream")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.stream == nil {
		return 0, fmt.Errorf("portaudio: stream is closed")
	}

	frames := len(p) / s.frameSize
	if code := C.Pa_ReadStream(s.stream, unsafe.Pointer(&p[0]), C.ulong(frames)); code != C.paNoError {
		return 0, paErr(code, "read stream")
	}
	return frames * s.frameSize, nil
}

func (s *nativeStream) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if s.direction != directionOutput {
		return 0, fmt.Errorf("portaudio: write called on input stream")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.stream == nil {
		return 0, fmt.Errorf("portaudio: stream is closed")
	}

	frames := len(p) / s.frameSize
	if code := C.Pa_WriteStream(s.stream, unsafe.Pointer(&p[0]), C.ulong(frames)); code != C.paNoError {
		return 0, paErr(code, "write stream")
	}
	return frames * s.frameSize, nil
}

func paErr(code C.PaError, op string) error {
	errText := strings.TrimSpace(C.GoString(C.Pa_GetErrorText(code)))
	if errText == "" {
		errText = "unknown"
	}
	return fmt.Errorf("portaudio: %s: %s (%d)", op, errText, int(code))
}
