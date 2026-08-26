//go:build cgo && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)))

package portaudio

/*
#include <portaudio.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
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
	ioChunkFrames := max(1, int(cfg.SampleRate/50))
	code := C.Pa_OpenStream(
		(*unsafe.Pointer)(unsafe.Pointer(&stream)),
		input,
		output,
		C.double(cfg.SampleRate),
		C.ulong(min(cfg.FramesPerBuffer, uint32(ioChunkFrames))),
		C.paNoFlag,
		nil,
		nil,
	)
	if code != C.paNoError {
		return nil, paErr(code, "open stream")
	}

	return &nativeStream{
		stream:        stream,
		direction:     direction,
		frameSize:     cfg.frameBytes(),
		ioChunkFrames: ioChunkFrames,
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
	lifecycleMu sync.Mutex
	ioMu        sync.Mutex
	mu          sync.Mutex
	cond        *sync.Cond
	activeIO    int
	draining    bool

	stream        unsafe.Pointer
	direction     streamDirection
	frameSize     int
	ioChunkFrames int
	closed        bool
	operations    *nativeStreamOperations
}

type nativeStreamOperations struct {
	start          func(unsafe.Pointer) int
	stop           func(unsafe.Pointer) int
	abort          func(unsafe.Pointer) int
	close          func(unsafe.Pointer) int
	read           func(unsafe.Pointer, unsafe.Pointer, int) int
	write          func(unsafe.Pointer, unsafe.Pointer, int) int
	readAvailable  func(unsafe.Pointer) int
	writeAvailable func(unsafe.Pointer) int
}

var defaultNativeStreamOperations = nativeStreamOperations{
	start: func(stream unsafe.Pointer) int { return int(C.Pa_StartStream(stream)) },
	stop:  func(stream unsafe.Pointer) int { return int(C.Pa_StopStream(stream)) },
	abort: func(stream unsafe.Pointer) int { return int(C.Pa_AbortStream(stream)) },
	close: func(stream unsafe.Pointer) int { return int(C.Pa_CloseStream(stream)) },
	read: func(stream unsafe.Pointer, buffer unsafe.Pointer, frames int) int {
		return int(C.Pa_ReadStream(stream, buffer, C.ulong(frames)))
	},
	write: func(stream unsafe.Pointer, buffer unsafe.Pointer, frames int) int {
		return int(C.Pa_WriteStream(stream, buffer, C.ulong(frames)))
	},
	readAvailable:  func(stream unsafe.Pointer) int { return int(C.Pa_GetStreamReadAvailable(stream)) },
	writeAvailable: func(stream unsafe.Pointer) int { return int(C.Pa_GetStreamWriteAvailable(stream)) },
}

func (s *nativeStream) ops() nativeStreamOperations {
	if s.operations != nil {
		return *s.operations
	}
	return defaultNativeStreamOperations
}

func (s *nativeStream) Start() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.stream == nil {
		return fmt.Errorf("portaudio: stream is closed")
	}
	if s.activeIO != 0 || s.draining {
		return errors.New("portaudio: stream I/O is active")
	}
	if code := s.ops().start(s.stream); code != int(C.paNoError) {
		return paErr(C.PaError(code), "start stream")
	}
	return nil
}

func (s *nativeStream) Stop() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	if s.closed || s.stream == nil {
		s.mu.Unlock()
		return nil
	}
	s.draining = true
	stream := s.stream
	operations := s.ops()
	active := s.activeIO
	s.mu.Unlock()

	operation := "stop stream"
	code := 0
	if active > 0 {
		operation = "abort stream"
		code = operations.abort(stream)
	} else {
		code = operations.stop(stream)
	}
	s.mu.Lock()
	s.waitForIOLocked()
	s.mu.Unlock()
	s.ioMu.Lock()
	s.mu.Lock()
	s.draining = false
	s.mu.Unlock()
	s.ioMu.Unlock()
	if code != int(C.paNoError) {
		return paErr(C.PaError(code), operation)
	}
	return nil
}

func (s *nativeStream) Close() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.draining = true
	if s.stream == nil {
		s.mu.Unlock()
		return nil
	}
	stream := s.stream
	operations := s.ops()
	active := s.activeIO
	s.mu.Unlock()

	var abortErr error
	if active > 0 {
		if code := operations.abort(stream); code != int(C.paNoError) {
			abortErr = paErr(C.PaError(code), "abort stream")
		}
	}
	s.mu.Lock()
	s.waitForIOLocked()
	s.stream = nil
	s.mu.Unlock()
	code := operations.close(stream)
	if code != int(C.paNoError) {
		return errors.Join(abortErr, paErr(C.PaError(code), "close stream"))
	}
	return abortErr
}

func (s *nativeStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if s.direction != directionInput {
		return 0, fmt.Errorf("portaudio: read called on output stream")
	}

	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	stream, operations, err := s.beginIO()
	if err != nil {
		return 0, err
	}
	defer s.endIO()
	return s.transfer(p, stream, operations.read, operations.readAvailable, "read stream")
}

func (s *nativeStream) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if s.direction != directionOutput {
		return 0, fmt.Errorf("portaudio: write called on input stream")
	}

	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	stream, operations, err := s.beginIO()
	if err != nil {
		return 0, err
	}
	defer s.endIO()
	return s.transfer(p, stream, operations.write, operations.writeAvailable, "write stream")
}

func (s *nativeStream) transfer(
	p []byte,
	stream unsafe.Pointer,
	operation func(unsafe.Pointer, unsafe.Pointer, int) int,
	available func(unsafe.Pointer) int,
	name string,
) (int, error) {
	totalFrames := len(p) / s.frameSize
	chunkFrames := s.ioChunkFrames
	if chunkFrames <= 0 {
		chunkFrames = totalFrames
	}
	completedFrames := 0
	for completedFrames < totalFrames {
		s.mu.Lock()
		stopping := s.draining || s.closed
		s.mu.Unlock()
		if stopping {
			return completedFrames * s.frameSize, errors.New("portaudio: stream is stopping")
		}
		frames := min(chunkFrames, totalFrames-completedFrames)
		if available != nil {
			ready := available(stream)
			if ready < 0 {
				return completedFrames * s.frameSize, paErr(C.PaError(ready), name+" availability")
			}
			if ready == 0 {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			frames = min(frames, ready)
		}
		offset := completedFrames * s.frameSize
		if code := operation(stream, unsafe.Pointer(&p[offset]), frames); !successfulTransferCode(name, code) {
			return completedFrames * s.frameSize, paErr(C.PaError(code), name)
		}
		completedFrames += frames
		s.mu.Lock()
		stopping = s.draining || s.closed
		s.mu.Unlock()
		if stopping && completedFrames < totalFrames {
			return completedFrames * s.frameSize, errors.New("portaudio: stream is stopping")
		}
	}
	return completedFrames * s.frameSize, nil
}

func successfulTransferCode(operation string, code int) bool {
	return code == int(C.paNoError) ||
		operation == "read stream" && code == int(C.paInputOverflowed) ||
		operation == "write stream" && code == int(C.paOutputUnderflowed)
}

func (s *nativeStream) beginIO() (unsafe.Pointer, nativeStreamOperations, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.stream == nil {
		return nil, nativeStreamOperations{}, errors.New("portaudio: stream is closed")
	}
	if s.draining {
		return nil, nativeStreamOperations{}, errors.New("portaudio: stream is stopping")
	}
	s.activeIO++
	return s.stream, s.ops(), nil
}

func (s *nativeStream) endIO() {
	s.mu.Lock()
	s.activeIO--
	if s.cond != nil {
		s.cond.Broadcast()
	}
	s.mu.Unlock()
}

func (s *nativeStream) waitForIOLocked() {
	if s.cond == nil {
		s.cond = sync.NewCond(&s.mu)
	}
	for s.activeIO > 0 {
		s.cond.Wait()
	}
}

func paErr(code C.PaError, op string) error {
	errText := strings.TrimSpace(C.GoString(C.Pa_GetErrorText(code)))
	if errText == "" {
		errText = "unknown"
	}
	return fmt.Errorf("portaudio: %s: %s (%d)", op, errText, int(code))
}
