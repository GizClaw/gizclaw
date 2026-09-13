package opus

import (
	"math"
	"runtime"
	"strings"
	"testing"
)

func isNativeOpusSupportedRuntime() bool {
	return nativeCGOEnabled && isSupportedPlatform(runtime.GOOS, runtime.GOARCH)
}

func requireNativeOpusSupportedRuntime(t *testing.T) {
	t.Helper()
	if !isNativeOpusSupportedRuntime() {
		t.Skipf("requires native opus runtime, got %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func mustPCM16kMono(frameSize int) []int16 {
	p := make([]int16, frameSize)
	for i := range p {
		x := math.Sin(2 * math.Pi * 440 * float64(i) / 16000)
		p[i] = int16(x * 12000)
	}
	return p
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
		{goos: "linux", goarch: "riscv64", want: false},
	}

	for _, tc := range cases {
		got := isSupportedPlatform(tc.goos, tc.goarch)
		if got != tc.want {
			t.Fatalf("isSupportedPlatform(%q, %q)=%v, want %v", tc.goos, tc.goarch, got, tc.want)
		}
	}

	if IsRuntimeSupported() != isNativeOpusSupportedRuntime() {
		t.Fatalf("IsRuntimeSupported mismatch: got=%v want=%v", IsRuntimeSupported(), isNativeOpusSupportedRuntime())
	}
}

func TestCheckedMul(t *testing.T) {
	if got, ok := checkedMul(7, 9); !ok || got != 63 {
		t.Fatalf("checkedMul(7,9)=(%d,%v), want (63,true)", got, ok)
	}
	if got, ok := checkedMul(math.MaxInt, 2); ok || got != 0 {
		t.Fatalf("checkedMul(MaxInt,2)=(%d,%v), want (0,false)", got, ok)
	}
	if got, ok := checkedMul(0, 123); !ok || got != 0 {
		t.Fatalf("checkedMul(0,123)=(%d,%v), want (0,true)", got, ok)
	}
	if got, ok := checkedMul(-1, 2); ok || got != 0 {
		t.Fatalf("checkedMul(-1,2)=(%d,%v), want (0,false)", got, ok)
	}
}

func TestValidationHelpers(t *testing.T) {
	if err := validateSampleRate(16000); err != nil {
		t.Fatalf("validateSampleRate(16000): %v", err)
	}
	if err := validateSampleRate(44100); err == nil {
		t.Fatal("validateSampleRate(44100) expected error")
	}
	if got := SampleRate16K.Int(); got != 16000 {
		t.Fatalf("SampleRate16K.Int() = %d", got)
	}
	if err := SampleRate16K.Validate(); err != nil {
		t.Fatalf("SampleRate16K.Validate(): %v", err)
	}
	if err := OpusSampleRate(0).Validate(); err == nil {
		t.Fatal("OpusSampleRate(0).Validate() expected error")
	}

	if err := validateChannels(1); err != nil {
		t.Fatalf("validateChannels(1): %v", err)
	}
	if err := validateChannels(3); err == nil {
		t.Fatal("validateChannels(3) expected error")
	}

	if err := validateApplication(ApplicationAudio); err != nil {
		t.Fatalf("validateApplication(ApplicationAudio): %v", err)
	}
	if err := validateApplication(Application(9999)); err == nil {
		t.Fatal("validateApplication(9999) expected error")
	}

	if err := validateFrameSize(16000, 320); err != nil {
		t.Fatalf("validateFrameSize(16000,320): %v", err)
	}
	if err := validateFrameSize(16000, 123); err == nil {
		t.Fatal("validateFrameSize(16000,123) expected error")
	}

	if err := validatePCM([]int16{1, 2}, 2, 1); err != nil {
		t.Fatalf("validatePCM(valid): %v", err)
	}
	if err := validatePCM(nil, 2, 1); err == nil {
		t.Fatal("validatePCM(empty) expected error")
	}
	if err := validatePCM([]int16{1}, 2, 1); err == nil {
		t.Fatal("validatePCM(short) expected error")
	}

	if err := validateMaxDataBytes(1200); err != nil {
		t.Fatalf("validateMaxDataBytes(1200): %v", err)
	}
	if err := validateMaxDataBytes(0); err == nil {
		t.Fatal("validateMaxDataBytes(0) expected error")
	}
	if err := validateMaxDataBytes(DefaultMaxPacketSize + 1); err == nil {
		t.Fatal("validateMaxDataBytes(>limit) expected error")
	}
}

func TestVersionMarker(t *testing.T) {
	v := strings.TrimSpace(Version())
	if isNativeOpusSupportedRuntime() {
		if v == "" || v == "unsupported" {
			t.Fatalf("Version()=%q, expected non-empty libopus version", v)
		}
		return
	}

	if v != "unsupported" {
		t.Fatalf("Version()=%q, want unsupported on unsupported runtime", v)
	}
}

func TestNilReceiverSafety(t *testing.T) {
	var nilEncoder *Encoder
	if out, err := nilEncoder.Encode(nil, 320); err == nil || out != nil {
		t.Fatalf("nil encoder Encode should fail, out=%v err=%v", out, err)
	}
	if out, err := nilEncoder.EncodeWithMaxDataBytes(nil, 320, 1200); err == nil || out != nil {
		t.Fatalf("nil encoder EncodeWithMaxDataBytes should fail, out=%v err=%v", out, err)
	}
	if err := nilEncoder.Close(); err != nil {
		t.Fatalf("nil encoder Close error: %v", err)
	}
	if nilEncoder.SampleRate() != 0 || nilEncoder.Channels() != 0 {
		t.Fatalf("nil encoder accessors should return 0")
	}

	var nilDecoder *Decoder
	if out, err := nilDecoder.Decode(nil, 320, false); err == nil || out != nil {
		t.Fatalf("nil decoder Decode should fail, out=%v err=%v", out, err)
	}
	if err := nilDecoder.Close(); err != nil {
		t.Fatalf("nil decoder Close error: %v", err)
	}
	if nilDecoder.SampleRate() != 0 || nilDecoder.Channels() != 0 {
		t.Fatalf("nil decoder accessors should return 0")
	}
}

func TestNewEncoderAndDecoderValidation(t *testing.T) {
	if _, err := NewEncoder(44100, 1, ApplicationAudio); err == nil {
		t.Fatal("NewEncoder invalid sample rate expected error")
	}
	if _, err := NewEncoder(16000, 3, ApplicationAudio); err == nil {
		t.Fatal("NewEncoder invalid channels expected error")
	}
	if _, err := NewEncoder(16000, 1, Application(9999)); err == nil {
		t.Fatal("NewEncoder invalid application expected error")
	}

	if _, err := NewDecoder(44100, 1); err == nil {
		t.Fatal("NewDecoder invalid sample rate expected error")
	}
	if _, err := NewDecoder(16000, 3); err == nil {
		t.Fatal("NewDecoder invalid channels expected error")
	}
}

func TestEncodeDecodeRoundTripAndClose(t *testing.T) {
	requireNativeOpusSupportedRuntime(t)

	enc, err := NewEncoder(16000, 1, ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	if enc.SampleRate() != 16000 || enc.Channels() != 1 {
		t.Fatalf("encoder accessors mismatch: sampleRate=%d channels=%d", enc.SampleRate(), enc.Channels())
	}
	if err := enc.SetComplexity(0); err != nil {
		t.Fatalf("SetComplexity(0): %v", err)
	}
	if got, err := enc.Complexity(); err != nil || got != 0 {
		t.Fatalf("Complexity() = %d, %v; want 0, nil", got, err)
	}
	if err := enc.SetComplexity(11); err == nil {
		t.Fatal("SetComplexity(11) expected error")
	}

	dec, err := NewDecoder(16000, 1)
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	if dec.SampleRate() != 16000 || dec.Channels() != 1 {
		t.Fatalf("decoder accessors mismatch: sampleRate=%d channels=%d", dec.SampleRate(), dec.Channels())
	}

	frameSize := 320 // 20ms @ 16kHz
	pcm := mustPCM16kMono(frameSize)

	packet, err := enc.Encode(pcm, frameSize)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(packet) == 0 {
		t.Fatal("Encode produced empty packet")
	}

	decoded, err := dec.Decode(packet, frameSize, false)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded) == 0 || len(decoded) > frameSize {
		t.Fatalf("Decode output size mismatch: got=%d want in (0,%d]", len(decoded), frameSize)
	}

	plc, err := dec.Decode(nil, frameSize, false)
	if err != nil {
		t.Fatalf("Decode PLC(nil packet): %v", err)
	}
	if len(plc) == 0 {
		t.Fatal("Decode PLC produced empty output")
	}

	if _, err := enc.EncodeWithMaxDataBytes(pcm, frameSize, 0); err == nil {
		t.Fatal("EncodeWithMaxDataBytes max=0 expected error")
	}
	if _, err := enc.Encode(pcm[:len(pcm)-1], frameSize); err == nil {
		t.Fatal("Encode short pcm expected error")
	}
	if _, err := enc.Encode(pcm, 123); err == nil {
		t.Fatal("Encode invalid frame size expected error")
	}

	if _, err := dec.Decode(packet, 123, false); err == nil {
		t.Fatal("Decode invalid frame size expected error")
	}

	if err := enc.Close(); err != nil {
		t.Fatalf("encoder Close: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("encoder Close second call: %v", err)
	}
	if _, err := enc.Encode(pcm, frameSize); err == nil || !strings.Contains(err.Error(), "encoder is nil") {
		t.Fatalf("encode after close expected encoder nil error, got: %v", err)
	}

	if err := dec.Close(); err != nil {
		t.Fatalf("decoder Close: %v", err)
	}
	if err := dec.Close(); err != nil {
		t.Fatalf("decoder Close second call: %v", err)
	}
	if _, err := dec.Decode(packet, frameSize, false); err == nil || !strings.Contains(err.Error(), "decoder is nil") {
		t.Fatalf("decode after close expected decoder nil error, got: %v", err)
	}
}

func TestUnsupportedRuntimeErrorMessage(t *testing.T) {
	if isNativeOpusSupportedRuntime() {
		t.Skipf("native opus is supported on %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	expectedSubstr := "unsupported platform"
	if !nativeCGOEnabled && isSupportedPlatform(runtime.GOOS, runtime.GOARCH) {
		expectedSubstr = "CGO_ENABLED=1"
	}

	_, err := NewEncoder(16000, 1, ApplicationAudio)
	if err == nil || !strings.Contains(err.Error(), expectedSubstr) {
		t.Fatalf("NewEncoder expected %q error, got: %v", expectedSubstr, err)
	}

	_, err = NewDecoder(16000, 1)
	if err == nil || !strings.Contains(err.Error(), expectedSubstr) {
		t.Fatalf("NewDecoder expected %q error, got: %v", expectedSubstr, err)
	}
}

func TestUnsupportedRuntimeErrorHelper(t *testing.T) {
	err := unsupportedRuntimeError("darwin", "arm64", false)
	if !strings.Contains(err.Error(), "CGO_ENABLED=1") {
		t.Fatalf("expected cgo-disabled hint, got: %v", err)
	}

	err = unsupportedRuntimeError("windows", "amd64", false)
	if !strings.Contains(err.Error(), "cgo is disabled and platform windows/amd64 is unsupported") {
		t.Fatalf("expected cgo+platform reason, got: %v", err)
	}

	err = unsupportedRuntimeError("windows", "amd64", true)
	if !strings.Contains(err.Error(), "unsupported platform windows/amd64") {
		t.Fatalf("expected platform-only reason, got: %v", err)
	}
}
