package portaudio

import (
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
)

type fakeStream struct {
	startErr error
	stopErr  error
	closeErr error
	readErr  error
	writeErr error

	startCalls int
	stopCalls  int
	closeCalls int
	readCalls  int
	writeCalls int
}

func (s *fakeStream) Start() error {
	s.startCalls++
	return s.startErr
}

func (s *fakeStream) Stop() error {
	s.stopCalls++
	return s.stopErr
}

func (s *fakeStream) Close() error {
	s.closeCalls++
	return s.closeErr
}

func (s *fakeStream) Read(p []byte) (int, error) {
	s.readCalls++
	if s.readErr != nil {
		return 0, s.readErr
	}
	for i := range p {
		p[i] = byte(i % 251)
	}
	return len(p), nil
}

func (s *fakeStream) Write(p []byte) (int, error) {
	s.writeCalls++
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	return len(p), nil
}

type fakeBackend struct {
	devices       []DeviceInfo
	defaultInput  int
	defaultOutput int

	initErr      error
	terminateErr error
	formatErr    error
	openErr      error

	streamToOpen *fakeStream

	initCalls      int
	terminateCalls int
	listCalls      int
	openCalls      int
	formatCalls    int
}

func (b *fakeBackend) Name() string { return "fake" }

func (b *fakeBackend) Init() error {
	b.initCalls++
	return b.initErr
}

func (b *fakeBackend) Terminate() error {
	b.terminateCalls++
	return b.terminateErr
}

func (b *fakeBackend) ListDevices() ([]DeviceInfo, error) {
	b.listCalls++
	return copyDevices(b.devices), nil
}

func (b *fakeBackend) DefaultInputDevice() (int, error) {
	return b.defaultInput, nil
}

func (b *fakeBackend) DefaultOutputDevice() (int, error) {
	return b.defaultOutput, nil
}

func (b *fakeBackend) IsFormatSupported(direction streamDirection, cfg StreamConfig) error {
	b.formatCalls++
	_ = direction
	if err := cfg.Validate(); err != nil {
		return err
	}
	return b.formatErr
}

func (b *fakeBackend) OpenStream(direction streamDirection, cfg StreamConfig) (streamHandle, error) {
	b.openCalls++
	_ = direction
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if b.openErr != nil {
		return nil, b.openErr
	}
	if b.streamToOpen == nil {
		b.streamToOpen = &fakeStream{}
	}
	return b.streamToOpen, nil
}

func TestPlatformMatrix(t *testing.T) {
	if !strings.Contains(supportedPlatformDescription, "darwin/arm64") {
		t.Fatalf("supportedPlatformDescription missing darwin/arm64: %q", supportedPlatformDescription)
	}

	cases := []struct {
		goos   string
		goarch string
		want   bool
	}{
		{goos: "linux", goarch: "amd64", want: true},
		{goos: "linux", goarch: "arm64", want: true},
		{goos: "darwin", goarch: "amd64", want: true},
		{goos: "darwin", goarch: "arm64", want: true},
		{goos: "windows", goarch: "amd64", want: false},
		{goos: "linux", goarch: "386", want: false},
	}

	for _, tc := range cases {
		if got := isSupportedPlatform(tc.goos, tc.goarch); got != tc.want {
			t.Fatalf("isSupportedPlatform(%q,%q)=%v, want %v", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestNativeRuntimeSupportedMarker(t *testing.T) {
	if !nativeCGOEnabled && NativeRuntimeSupported() {
		t.Fatal("NativeRuntimeSupported should be false when cgo is disabled")
	}

	if NativeRuntimeSupported() {
		if !isSupportedPlatform(runtime.GOOS, runtime.GOARCH) {
			t.Fatalf("NativeRuntimeSupported=true on unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
		}
	}
}

func TestBackendNameAndDefaultDriverWrappers(t *testing.T) {
	prev := defaultDriver
	b := &fakeBackend{
		devices: []DeviceInfo{
			{ID: 1, Name: "mic"},
			{ID: 2, Name: "speaker"},
		},
		defaultInput:  1,
		defaultOutput: 2,
		streamToOpen:  &fakeStream{},
	}
	defaultDriver = newDriverWithBackend(b)
	t.Cleanup(func() {
		defaultDriver = prev
	})

	if got := BackendName(); got != "fake" {
		t.Fatalf("BackendName=%q, want fake", got)
	}

	devices, err := ListDevices()
	if err != nil {
		t.Fatalf("ListDevices wrapper: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("ListDevices wrapper len=%d, want 2", len(devices))
	}

	in, err := DefaultInputDevice()
	if err != nil {
		t.Fatalf("DefaultInputDevice wrapper: %v", err)
	}
	if in.ID != 1 {
		t.Fatalf("DefaultInputDevice wrapper id=%d, want 1", in.ID)
	}

	out, err := DefaultOutputDevice()
	if err != nil {
		t.Fatalf("DefaultOutputDevice wrapper: %v", err)
	}
	if out.ID != 2 {
		t.Fatalf("DefaultOutputDevice wrapper id=%d, want 2", out.ID)
	}

	cap, err := OpenCapture(pcm.L16Mono16K, CaptureOptions{})
	if err != nil {
		t.Fatalf("OpenCapture wrapper: %v", err)
	}
	if err := cap.Close(); err != nil {
		t.Fatalf("Close capture stream: %v", err)
	}

	play, err := OpenPlayback(pcm.L16Mono16K, PlaybackOptions{})
	if err != nil {
		t.Fatalf("OpenPlayback wrapper: %v", err)
	}
	if err := play.Close(); err != nil {
		t.Fatalf("Close playback stream: %v", err)
	}

	pw, err := OpenPCMPlaybackWriter(pcm.L16Mono16K, PlaybackOptions{})
	if err != nil {
		t.Fatalf("OpenPCMPlaybackWriter: %v", err)
	}
	if err := pw.Write(pcm.L16Mono16K.DataChunk([]byte{0, 0})); err != nil {
		t.Fatalf("PCMPlaybackWriter.Write: %v", err)
	}
	if err := pw.Close(); err != nil {
		t.Fatalf("PCMPlaybackWriter.Close: %v", err)
	}
}

func TestBackendNameWithNilDefaultDriver(t *testing.T) {
	prev := defaultDriver
	defaultDriver = nil
	t.Cleanup(func() {
		defaultDriver = prev
	})

	if got := BackendName(); got != "unknown" {
		t.Fatalf("BackendName=%q, want unknown when default driver is nil", got)
	}
}

func TestNewDriverWithNilBackend(t *testing.T) {
	d := newDriverWithBackend(nil)
	if d == nil || d.backend == nil {
		t.Fatalf("newDriverWithBackend(nil) should fallback to compile-time backend, got %+v", d)
	}
}

func TestStreamConfigAndPCMConversions(t *testing.T) {
	cfg := StreamConfig{DeviceID: DefaultDeviceID, SampleRate: 16000, Channels: 1, FramesPerBuffer: 320}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if cfg.frameBytes() != 2 {
		t.Fatalf("frameBytes=%d, want 2", cfg.frameBytes())
	}

	if err := (StreamConfig{SampleRate: 0, Channels: 1, FramesPerBuffer: 1}).Validate(); err == nil {
		t.Fatal("expected invalid sample rate error")
	}
	if err := (StreamConfig{SampleRate: 16000, Channels: 0, FramesPerBuffer: 1}).Validate(); err == nil {
		t.Fatal("expected invalid channels error")
	}
	if err := (StreamConfig{SampleRate: 16000, Channels: 1, FramesPerBuffer: 0}).Validate(); err == nil {
		t.Fatal("expected invalid frames_per_buffer error")
	}

	if got := defaultFramesPerBuffer(0); got == 0 {
		t.Fatal("defaultFramesPerBuffer(0) should fallback to non-zero")
	}

	fromPCM := StreamConfigFromPCM(pcm.L16Mono16K, DefaultDeviceID, 0)
	if fromPCM.SampleRate != 16000 || fromPCM.Channels != 1 || fromPCM.FramesPerBuffer == 0 {
		t.Fatalf("StreamConfigFromPCM mismatch: %+v", fromPCM)
	}

	if fmt16k, err := PCMFormatFromSampleRate(16000, 1); err != nil || fmt16k != pcm.L16Mono16K {
		t.Fatalf("PCMFormatFromSampleRate(16k,1)=(%v,%v)", fmt16k, err)
	}
	if _, err := PCMFormatFromSampleRate(16000, 2); err == nil {
		t.Fatal("expected channels unsupported error")
	}
	if _, err := PCMFormatFromSampleRate(44100, 1); err == nil {
		t.Fatal("expected sample rate unsupported error")
	}
}

func TestDriverListAndDefaultDevices(t *testing.T) {
	b := &fakeBackend{
		devices: []DeviceInfo{
			{ID: 1, Name: "mic"},
			{ID: 2, Name: "speaker"},
		},
		defaultInput:  1,
		defaultOutput: 2,
	}
	d := newDriverWithBackend(b)

	devices, err := d.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("ListDevices len=%d, want 2", len(devices))
	}

	// Ensure copy behavior.
	devices[0].Name = "mutated"
	again, err := d.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices second call: %v", err)
	}
	if again[0].Name != "mic" {
		t.Fatalf("device list should be copied, got %q", again[0].Name)
	}

	inDev, err := d.DefaultInputDevice()
	if err != nil {
		t.Fatalf("DefaultInputDevice: %v", err)
	}
	if inDev.ID != 1 {
		t.Fatalf("DefaultInputDevice id=%d, want 1", inDev.ID)
	}

	outDev, err := d.DefaultOutputDevice()
	if err != nil {
		t.Fatalf("DefaultOutputDevice: %v", err)
	}
	if outDev.ID != 2 {
		t.Fatalf("DefaultOutputDevice id=%d, want 2", outDev.ID)
	}

	b.defaultInput = 42
	if _, err := d.DefaultInputDevice(); err == nil || !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("expected ErrDeviceNotFound, got %v", err)
	}
}

func TestOpenCaptureReadClose(t *testing.T) {
	stream := &fakeStream{}
	b := &fakeBackend{streamToOpen: stream}
	d := newDriverWithBackend(b)

	cap, err := d.OpenCapture(pcm.L16Mono16K, CaptureOptions{})
	if err != nil {
		t.Fatalf("OpenCapture: %v", err)
	}
	if cap.Config().DeviceID != DefaultDeviceID {
		t.Fatalf("default device id=%d, want %d", cap.Config().DeviceID, DefaultDeviceID)
	}

	if _, err := cap.Read(make([]byte, 3)); err == nil {
		t.Fatal("expected frame alignment error")
	}

	buf := make([]byte, 320)
	n, err := cap.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("Read n=%d, want %d", n, len(buf))
	}

	if err := cap.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := cap.Close(); err != nil {
		t.Fatalf("Close second call: %v", err)
	}
	if _, err := cap.Read(buf); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("expected closed read error, got %v", err)
	}

	if stream.startCalls != 1 || stream.stopCalls != 1 || stream.closeCalls != 1 {
		t.Fatalf("stream lifecycle calls mismatch: start=%d stop=%d close=%d", stream.startCalls, stream.stopCalls, stream.closeCalls)
	}
	if b.initCalls == 0 || b.terminateCalls == 0 {
		t.Fatalf("backend lifecycle not triggered: init=%d terminate=%d", b.initCalls, b.terminateCalls)
	}
}

func TestOpenPlaybackWriteAndPCMBridge(t *testing.T) {
	stream := &fakeStream{}
	b := &fakeBackend{streamToOpen: stream}
	d := newDriverWithBackend(b)

	play, err := d.OpenPlayback(pcm.L16Mono24K, PlaybackOptions{HasDeviceID: true, DeviceID: 7})
	if err != nil {
		t.Fatalf("OpenPlayback: %v", err)
	}
	if play.Config().DeviceID != 7 {
		t.Fatalf("device id=%d, want 7", play.Config().DeviceID)
	}

	stereo, err := d.OpenPlaybackConfig(StreamConfig{DeviceID: 7, SampleRate: 48000, Channels: 2, FramesPerBuffer: 960})
	if err != nil {
		t.Fatalf("OpenPlaybackConfig: %v", err)
	}
	if stereo.Config().DeviceID != 7 || stereo.Config().Channels != 2 || stereo.Config().SampleRate != 48000 {
		t.Fatalf("stereo playback config = %+v", stereo.Config())
	}
	if err := stereo.Close(); err != nil {
		t.Fatalf("stereo Close: %v", err)
	}

	if _, err := play.Write(make([]byte, 3)); err == nil {
		t.Fatal("expected frame alignment error")
	}
	if _, err := play.Write(make([]byte, 480)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	pw := &PCMPlaybackWriter{stream: play, format: pcm.L16Mono24K}
	if err := pw.Write(nil); err == nil {
		t.Fatal("expected nil chunk error")
	}
	if err := pw.Write(pcm.L16Mono16K.DataChunk([]byte{0, 0})); err == nil {
		t.Fatal("expected chunk format mismatch")
	}
	if err := pw.Write(pcm.L16Mono24K.DataChunk([]byte{0, 0, 1, 1})); err != nil {
		t.Fatalf("PCMPlaybackWriter.Write: %v", err)
	}
	if err := pw.Close(); err != nil {
		t.Fatalf("PCMPlaybackWriter.Close: %v", err)
	}
}

func TestOpenFailurePathsReleaseLifecycle(t *testing.T) {
	b := &fakeBackend{openErr: errors.New("open failed")}
	d := newDriverWithBackend(b)

	if _, err := d.OpenCapture(pcm.L16Mono16K, CaptureOptions{}); err == nil || !strings.Contains(err.Error(), "open failed") {
		t.Fatalf("expected open failed, got %v", err)
	}
	if b.initCalls != 1 || b.terminateCalls != 1 {
		t.Fatalf("expected init/terminate once after open failure, got init=%d terminate=%d", b.initCalls, b.terminateCalls)
	}

	b2 := &fakeBackend{streamToOpen: &fakeStream{startErr: errors.New("start failed")}}
	d2 := newDriverWithBackend(b2)
	if _, err := d2.OpenPlayback(pcm.L16Mono16K, PlaybackOptions{}); err == nil || !strings.Contains(err.Error(), "start failed") {
		t.Fatalf("expected start failed, got %v", err)
	}
	if b2.terminateCalls != 1 {
		t.Fatalf("expected terminate after start failure, got %d", b2.terminateCalls)
	}
}

func TestReadPCMChunk(t *testing.T) {
	if _, err := ReadPCMChunk(nil, pcm.L16Mono16K, 20*time.Millisecond); err == nil {
		t.Fatal("expected nil reader error")
	}
	if _, err := ReadPCMChunk(strings.NewReader(""), pcm.L16Mono16K, 0); err == nil {
		t.Fatal("expected invalid duration error")
	}

	data := make([]byte, pcm.L16Mono16K.BytesInDuration(20*time.Millisecond))
	chunk, err := ReadPCMChunk(strings.NewReader(string(data)), pcm.L16Mono16K, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("ReadPCMChunk: %v", err)
	}
	if chunk.Len() != int64(len(data)) {
		t.Fatalf("chunk len=%d, want %d", chunk.Len(), len(data))
	}
}

func TestDefaultDriverOnUnsupportedRuntime(t *testing.T) {
	if NativeRuntimeSupported() {
		t.Skipf("native runtime enabled on %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	if _, err := ListDevices(); err == nil || !strings.Contains(err.Error(), "unsupported platform") {
		t.Fatalf("ListDevices expected unsupported error, got %v", err)
	}
	if _, err := OpenCapture(pcm.L16Mono16K, CaptureOptions{}); err == nil || !strings.Contains(err.Error(), "unsupported platform") {
		t.Fatalf("OpenCapture expected unsupported error, got %v", err)
	}
	if _, err := OpenPlayback(pcm.L16Mono16K, PlaybackOptions{}); err == nil || !strings.Contains(err.Error(), "unsupported platform") {
		t.Fatalf("OpenPlayback expected unsupported error, got %v", err)
	}

	writer, err := OpenPCMPlaybackWriter(pcm.L16Mono16K, PlaybackOptions{})
	if err == nil || writer != nil {
		t.Fatalf("OpenPCMPlaybackWriter expected unsupported error, writer=%v err=%v", writer, err)
	}
}

func TestNilReceivers(t *testing.T) {
	var cap *CaptureStream
	if err := cap.Close(); err != nil {
		t.Fatalf("nil CaptureStream Close: %v", err)
	}
	if _, err := cap.Read(make([]byte, 2)); err == nil || !strings.Contains(err.Error(), "nil capture stream") {
		t.Fatalf("nil CaptureStream Read error mismatch: %v", err)
	}

	var play *PlaybackStream
	if err := play.Close(); err != nil {
		t.Fatalf("nil PlaybackStream Close: %v", err)
	}
	if _, err := play.Write(make([]byte, 2)); err == nil || !strings.Contains(err.Error(), "nil playback stream") {
		t.Fatalf("nil PlaybackStream Write error mismatch: %v", err)
	}

	var pw *PCMPlaybackWriter
	if err := pw.Close(); err != nil {
		t.Fatalf("nil PCMPlaybackWriter Close: %v", err)
	}
	if err := pw.Write(pcm.L16Mono16K.DataChunk([]byte{0, 0})); err == nil || !strings.Contains(err.Error(), "nil pcm playback writer") {
		t.Fatalf("nil PCMPlaybackWriter Write error mismatch: %v", err)
	}
}

func TestFindDeviceByID(t *testing.T) {
	devices := []DeviceInfo{{ID: 10, Name: "a"}}
	dev, err := findDeviceByID(devices, 10)
	if err != nil {
		t.Fatalf("findDeviceByID: %v", err)
	}
	if dev.ID != 10 || dev.Name != "a" {
		t.Fatalf("device mismatch: %+v", dev)
	}
	if _, err := findDeviceByID(devices, 11); err == nil || !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("expected ErrDeviceNotFound, got %v", err)
	}
}

func TestDriverAcquireAndReleaseErrors(t *testing.T) {
	b := &fakeBackend{initErr: io.ErrUnexpectedEOF}
	d := newDriverWithBackend(b)

	if err := d.acquire(); err == nil || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("acquire expected init error, got %v", err)
	}

	b2 := &fakeBackend{terminateErr: io.ErrClosedPipe}
	d2 := newDriverWithBackend(b2)
	if err := d2.acquire(); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := d2.release(); err == nil || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("release expected terminate error, got %v", err)
	}
}
