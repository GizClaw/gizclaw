//go:build cgo && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)))

package portaudio

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

func TestNativeBackendNameAndDeviceQueries(t *testing.T) {
	b := nativeBackend{}
	if got := b.Name(); !strings.HasPrefix(got, "portaudio/native") {
		t.Fatalf("Name=%q, want prefix portaudio/native", got)
	}

	if _, err := b.ListDevices(); err != nil && !strings.Contains(err.Error(), "portaudio") {
		t.Fatalf("ListDevices unexpected error: %v", err)
	}

	if _, err := b.DefaultInputDevice(); err != nil && !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("DefaultInputDevice unexpected error: %v", err)
	}

	if _, err := b.DefaultOutputDevice(); err != nil && !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("DefaultOutputDevice unexpected error: %v", err)
	}
}

func TestNativeBackendInitTerminate(t *testing.T) {
	b := nativeBackend{}
	if err := b.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := b.ListDevices(); err != nil {
		t.Fatalf("ListDevices after init: %v", err)
	}
	if err := b.Terminate(); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
}

func TestBuildPaStreamParametersWithDefaultDevice(t *testing.T) {
	b := nativeBackend{}
	if err := b.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() {
		_ = b.Terminate()
	})

	cfg := StreamConfig{
		DeviceID:        DefaultDeviceID,
		SampleRate:      16000,
		Channels:        1,
		FramesPerBuffer: 320,
	}

	in, out, err := buildPaStreamParameters(directionInput, cfg)
	if err != nil {
		if errors.Is(err, ErrDeviceNotFound) {
			t.Skipf("no default input device available: %v", err)
		}
		t.Fatalf("buildPaStreamParameters(input): %v", err)
	}
	if in == nil || out != nil {
		t.Fatalf("input params mismatch: in=%v out=%v", in, out)
	}

	in, out, err = buildPaStreamParameters(directionOutput, cfg)
	if err != nil {
		if errors.Is(err, ErrDeviceNotFound) {
			t.Skipf("no default output device available: %v", err)
		}
		t.Fatalf("buildPaStreamParameters(output): %v", err)
	}
	if in != nil || out == nil {
		t.Fatalf("output params mismatch: in=%v out=%v", in, out)
	}
}

func TestNativeBackendFormatAndOpenWithDefaultDevice(t *testing.T) {
	b := nativeBackend{}
	if err := b.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() {
		_ = b.Terminate()
	})

	cfg := StreamConfig{
		DeviceID:        DefaultDeviceID,
		SampleRate:      16000,
		Channels:        1,
		FramesPerBuffer: 320,
	}

	_ = b.IsFormatSupported(directionOutput, cfg)

	h, err := b.OpenStream(directionOutput, cfg)
	if err != nil {
		if errors.Is(err, ErrDeviceNotFound) || strings.Contains(strings.ToLower(err.Error()), "invalid device") {
			t.Skipf("default output device unavailable: %v", err)
		}
		t.Fatalf("OpenStream: %v", err)
	}
	t.Cleanup(func() {
		_ = h.Close()
	})

	if err := h.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := h.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestNativeBackendInvalidConfigPaths(t *testing.T) {
	b := nativeBackend{}

	if err := b.IsFormatSupported(directionInput, StreamConfig{}); err == nil {
		t.Fatal("IsFormatSupported should fail on invalid config")
	}

	if _, err := b.OpenStream(directionOutput, StreamConfig{}); err == nil {
		t.Fatal("OpenStream should fail on invalid config")
	}
}

func TestBuildPaStreamParametersInvalidDevice(t *testing.T) {
	cfg := StreamConfig{
		DeviceID:        1 << 30,
		SampleRate:      16000,
		Channels:        1,
		FramesPerBuffer: 320,
	}

	if _, _, err := buildPaStreamParameters(directionInput, cfg); err == nil {
		t.Fatal("buildPaStreamParameters(input) should fail on invalid device")
	}

	if _, _, err := buildPaStreamParameters(directionOutput, cfg); err == nil {
		t.Fatal("buildPaStreamParameters(output) should fail on invalid device")
	}
}

func TestNativeBackendOpenStreamErrorPath(t *testing.T) {
	b := nativeBackend{}
	cfg := StreamConfig{
		DeviceID:        1 << 30,
		SampleRate:      16000,
		Channels:        1,
		FramesPerBuffer: 320,
	}

	if _, err := b.OpenStream(directionOutput, cfg); err == nil {
		t.Fatal("OpenStream should fail on invalid device")
	}
}

func TestNativeStreamGuardBranches(t *testing.T) {
	in := &nativeStream{direction: directionInput, frameSize: 2}
	out := &nativeStream{direction: directionOutput, frameSize: 2}

	if n, err := in.Read(nil); n != 0 || err != nil {
		t.Fatalf("Read(nil)=(%d,%v), want (0,nil)", n, err)
	}
	if n, err := out.Write(nil); n != 0 || err != nil {
		t.Fatalf("Write(nil)=(%d,%v), want (0,nil)", n, err)
	}

	if _, err := in.Write([]byte{0, 0}); err == nil {
		t.Fatal("Write on input stream should fail")
	}
	if _, err := out.Read([]byte{0, 0}); err == nil {
		t.Fatal("Read on output stream should fail")
	}

	if err := in.Close(); err != nil {
		t.Fatalf("Close input stream: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close output stream: %v", err)
	}

	if err := in.Start(); err == nil {
		t.Fatal("Start should fail for closed stream")
	}
	if err := out.Start(); err == nil {
		t.Fatal("Start should fail for closed stream")
	}

	if err := in.Stop(); err != nil {
		t.Fatalf("Stop on closed stream should be nil, got %v", err)
	}
	if err := out.Stop(); err != nil {
		t.Fatalf("Stop on closed stream should be nil, got %v", err)
	}

	if _, err := in.Read([]byte{0, 0}); err == nil {
		t.Fatal("Read on closed input stream should fail")
	}
	if _, err := out.Write([]byte{0, 0}); err == nil {
		t.Fatal("Write on closed output stream should fail")
	}
}

func TestNativeStreamLifecycleCancelsBlockedReadBeforeHandleClose(t *testing.T) {
	for _, lifecycle := range []string{"stop", "close"} {
		t.Run(lifecycle, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			aborted := make(chan struct{})
			var releaseOnce sync.Once
			var active atomic.Bool
			var freedWhileActive atomic.Bool
			operations := &nativeStreamOperations{
				read: func(unsafe.Pointer, unsafe.Pointer, int) int {
					active.Store(true)
					close(entered)
					<-release
					active.Store(false)
					return 0
				},
				abort: func(unsafe.Pointer) int {
					close(aborted)
					releaseOnce.Do(func() { close(release) })
					return 0
				},
				stop: func(unsafe.Pointer) int { return 0 },
				close: func(unsafe.Pointer) int {
					freedWhileActive.Store(active.Load())
					return 0
				},
			}
			handle := new(byte)
			stream := &nativeStream{
				stream: unsafe.Pointer(handle), direction: directionInput, frameSize: 2,
				operations: operations,
			}
			readDone := make(chan error, 1)
			go func() {
				_, err := stream.Read(make([]byte, 2))
				readDone <- err
			}()
			<-entered
			lifecycleDone := make(chan error, 1)
			go func() {
				if lifecycle == "stop" {
					lifecycleDone <- stream.Stop()
					return
				}
				lifecycleDone <- stream.Close()
			}()

			select {
			case <-aborted:
			case <-time.After(time.Second):
				releaseOnce.Do(func() { close(release) })
				<-readDone
				<-lifecycleDone
				t.Fatalf("%s could not signal cancellation while native Read was blocked", lifecycle)
			}
			if err := <-readDone; err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			if err := <-lifecycleDone; err != nil {
				t.Fatalf("%s error = %v", lifecycle, err)
			}
			if freedWhileActive.Load() {
				t.Fatal("native handle was closed while Read still used it")
			}
		})
	}
}

func TestNativeStreamSerializesSameDirectionIO(t *testing.T) {
	entered := make(chan int, 2)
	release := make(chan struct{}, 2)
	var calls atomic.Int32
	stream := &nativeStream{
		stream: unsafe.Pointer(new(byte)), direction: directionInput, frameSize: 2,
		operations: &nativeStreamOperations{
			read: func(unsafe.Pointer, unsafe.Pointer, int) int {
				call := int(calls.Add(1))
				entered <- call
				<-release
				return 0
			},
		},
	}
	readDone := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := stream.Read(make([]byte, 2))
			readDone <- err
		}()
	}
	if call := <-entered; call != 1 {
		t.Fatalf("first native read call = %d", call)
	}
	select {
	case call := <-entered:
		release <- struct{}{}
		release <- struct{}{}
		<-readDone
		<-readDone
		t.Fatalf("second native read entered concurrently as call %d", call)
	default:
	}
	release <- struct{}{}
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if call := <-entered; call != 2 {
		t.Fatalf("second native read call = %d", call)
	}
	release <- struct{}{}
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
}
