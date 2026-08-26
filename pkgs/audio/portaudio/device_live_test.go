//go:build cgo && portaudio_device && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)))

package portaudio

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const liveLifecycleModeEnvironment = "GIZCLAW_PORTAUDIO_LIFECYCLE_MODE"

func TestListDevicesLive(t *testing.T) {
	devices, err := ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("no audio devices detected in current runtime")
	}
}

func TestStreamLifecycleLive(t *testing.T) {
	if mode := os.Getenv(liveLifecycleModeEnvironment); mode != "" {
		runStreamLifecycleLiveChild(t, mode)
		return
	}
	for _, mode := range []string{"capture", "playback"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStreamLifecycleLive$", "-test.v")
			command.Env = append(os.Environ(), liveLifecycleModeEnvironment+"="+mode)
			output, err := command.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("%s lifecycle helper did not cancel blocked native I/O before timeout:\n%s", mode, output)
			}
			if strings.Contains(string(output), "PORTAUDIO_LIVE_UNSUPPORTED") {
				t.Skipf("%s", strings.TrimSpace(string(output)))
			}
			if err != nil {
				t.Fatalf("%s lifecycle helper: %v\n%s", mode, err, output)
			}
		})
	}
}

func runStreamLifecycleLiveChild(t *testing.T, mode string) {
	driver := NewDriver()
	direction := directionInput
	device, err := driver.DefaultInputDevice()
	if mode == "playback" {
		direction = directionOutput
		device, err = driver.DefaultOutputDevice()
	} else if mode != "capture" {
		t.Fatalf("unknown lifecycle mode %q", mode)
	}
	if err != nil {
		t.Skipf("PORTAUDIO_LIVE_UNSUPPORTED: %s default device: %v", mode, err)
	}
	if device.DefaultSampleRate <= 0 {
		t.Skipf("PORTAUDIO_LIVE_UNSUPPORTED: %s device has invalid sample rate", mode)
	}
	frames := uint32(device.DefaultSampleRate * 10)
	config := StreamConfig{DeviceID: device.ID, SampleRate: device.DefaultSampleRate, Channels: 1, FramesPerBuffer: frames}
	handle, err := driver.open(direction, config)
	if err != nil {
		t.Skipf("PORTAUDIO_LIVE_UNSUPPORTED: open %s stream: %v", mode, err)
	}
	defer func() { _ = driver.release() }()

	buffer := make([]byte, int(frames)*config.frameBytes())
	ioStarted := make(chan struct{})
	ioDone := make(chan error, 1)
	go func() {
		close(ioStarted)
		var ioErr error
		if direction == directionInput {
			_, ioErr = handle.Read(buffer)
		} else {
			_, ioErr = handle.Write(buffer)
		}
		ioDone <- ioErr
	}()
	<-ioStarted
	select {
	case ioErr := <-ioDone:
		_ = handle.Close()
		t.Skipf("PORTAUDIO_LIVE_UNSUPPORTED: %s I/O did not block: %v", mode, ioErr)
	case <-time.After(150 * time.Millisecond):
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- handle.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close %s stream: %v", mode, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("Close %s stream did not cancel blocked I/O", mode)
	}
	select {
	case <-ioDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s I/O did not drain after Close", mode)
	}
}
