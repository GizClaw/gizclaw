package mp3

import (
	"bytes"
	"errors"
	"io"
	"math"
	"runtime"
	"strings"
	"testing"
)

type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

type partialErrReader struct {
	data []byte
	read bool
}

func (r *partialErrReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, errors.New("stream read failed")
	}
	r.read = true
	n := copy(p, r.data)
	return n, nil
}

func nativeEncoderRuntimeSupported() bool {
	return nativeEncoderEnabled && isSupportedPlatform(runtime.GOOS, runtime.GOARCH)
}

func requireNativeEncoderRuntime(t *testing.T) {
	t.Helper()
	if !nativeEncoderRuntimeSupported() {
		t.Skipf("requires native mp3 encoder runtime, got %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func generatePCM16Sine(sampleRate, channels int, seconds float64, hz float64) []byte {
	numSamples := int(float64(sampleRate) * seconds)
	pcm := make([]byte, numSamples*channels*2)
	for i := range numSamples {
		timeAt := float64(i) / float64(sampleRate)
		sample := int16(math.Sin(2*math.Pi*hz*timeAt) * 16000)
		for ch := range channels {
			off := i*channels*2 + ch*2
			pcm[off] = byte(sample)
			pcm[off+1] = byte(sample >> 8)
		}
	}
	return pcm
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

func TestNewEncoderValidation(t *testing.T) {
	if _, err := NewEncoder(nil, 44100, 2); err == nil || !strings.Contains(err.Error(), "writer is nil") {
		t.Fatalf("expected writer is nil error, got %v", err)
	}

	if _, err := NewEncoder(io.Discard, 44100, 3); err == nil || !strings.Contains(err.Error(), "channels") {
		t.Fatalf("expected channels validation error, got %v", err)
	}

	if nativeEncoderRuntimeSupported() {
		if _, err := NewEncoder(io.Discard, 0, 2); err == nil || !strings.Contains(err.Error(), "sample rate") {
			t.Fatalf("expected sample rate validation error, got %v", err)
		}
	} else {
		if _, err := NewEncoder(io.Discard, 44100, 2); err == nil || !strings.Contains(err.Error(), "unsupported platform") {
			t.Fatalf("expected unsupported platform error, got %v", err)
		}
	}
}

func TestEncodeDecodeRoundTripStereo(t *testing.T) {
	requireNativeEncoderRuntime(t)

	pcm := generatePCM16Sine(44100, 2, 0.5, 440)
	var mp3Buf bytes.Buffer

	enc, err := NewEncoder(&mp3Buf, 44100, 2, WithQuality(QualityMedium))
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}

	if n, err := enc.Write(pcm); err != nil {
		t.Fatalf("Write: %v", err)
	} else if n != len(pcm) {
		t.Fatalf("Write n=%d, want %d", n, len(pcm))
	}

	if err := enc.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("Close second call: %v", err)
	}

	if mp3Buf.Len() == 0 {
		t.Fatal("encoded mp3 is empty")
	}

	decoded, sampleRate, channels, err := DecodeFull(bytes.NewReader(mp3Buf.Bytes()))
	if err != nil {
		t.Fatalf("DecodeFull: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("decoded pcm is empty")
	}
	if sampleRate != 44100 {
		t.Fatalf("sampleRate=%d, want 44100", sampleRate)
	}
	if channels != 2 {
		t.Fatalf("channels=%d, want 2", channels)
	}

	ratio := float64(len(decoded)) / float64(len(pcm))
	if ratio < 0.7 || ratio > 1.3 {
		t.Fatalf("decoded ratio=%f out of range", ratio)
	}
}

func TestEncodePCMStreamMono(t *testing.T) {
	requireNativeEncoderRuntime(t)

	pcm := generatePCM16Sine(16000, 1, 0.3, 660)
	var out bytes.Buffer
	written, err := EncodePCMStream(&out, bytes.NewReader(pcm), 16000, 1, WithBitrate(64))
	if err != nil {
		t.Fatalf("EncodePCMStream: %v", err)
	}
	if written != int64(len(pcm)) {
		t.Fatalf("written=%d, want %d", written, len(pcm))
	}
	if out.Len() == 0 {
		t.Fatal("encoded stream is empty")
	}
}

func TestEncodePCMStreamReadError(t *testing.T) {
	requireNativeEncoderRuntime(t)

	payload := generatePCM16Sine(8000, 1, 0.1, 440)
	var out bytes.Buffer
	written, err := EncodePCMStream(&out, &partialErrReader{data: payload}, 8000, 1)
	if err == nil || !strings.Contains(err.Error(), "stream read failed") {
		t.Fatalf("expected stream read failed error, got %v", err)
	}
	if written != int64(len(payload)) {
		t.Fatalf("written=%d, want %d", written, len(payload))
	}
}

func TestEncodePCMStreamNilReader(t *testing.T) {
	requireNativeEncoderRuntime(t)

	_, err := EncodePCMStream(io.Discard, nil, 44100, 2)
	if err == nil || !strings.Contains(err.Error(), "reader is nil") {
		t.Fatalf("expected reader is nil error, got %v", err)
	}
}

func TestEncoderWriteRemainderBuffering(t *testing.T) {
	requireNativeEncoderRuntime(t)

	payload := generatePCM16Sine(44100, 2, 0.2, 440)
	if len(payload) < 10 {
		t.Fatalf("unexpected payload length: %d", len(payload))
	}

	var out bytes.Buffer
	enc, err := NewEncoder(&out, 44100, 2)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}

	first := payload[:3]
	second := payload[3:]

	if n, err := enc.Write(first); err != nil {
		t.Fatalf("Write first: %v", err)
	} else if n != len(first) {
		t.Fatalf("Write first n=%d, want %d", n, len(first))
	}
	if out.Len() != 0 {
		t.Fatalf("encoded output after unaligned first write should be empty, got %d", out.Len())
	}

	if n, err := enc.Write(second); err != nil {
		t.Fatalf("Write second: %v", err)
	} else if n != len(second) {
		t.Fatalf("Write second n=%d, want %d", n, len(second))
	}

	if err := enc.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected encoded output")
	}
}

func TestEncoderFlushWithTrailingBytes(t *testing.T) {
	requireNativeEncoderRuntime(t)

	enc, err := NewEncoder(io.Discard, 44100, 2)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer func() {
		_ = enc.Close()
	}()

	if _, err := enc.Write([]byte{1}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := enc.Flush(); err == nil || !strings.Contains(err.Error(), "incomplete pcm frame") {
		t.Fatalf("expected incomplete frame error, got %v", err)
	}
}

func TestEncoderWriterError(t *testing.T) {
	requireNativeEncoderRuntime(t)

	enc, err := NewEncoder(errWriter{}, 44100, 2)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer func() {
		_ = enc.Close()
	}()

	pcm := generatePCM16Sine(44100, 2, 0.1, 440)
	if _, err := enc.Write(pcm); err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected writer error, got %v", err)
	}
}

func TestDecoderCloseAndErrors(t *testing.T) {
	dec := NewDecoder(bytes.NewReader(nil))
	if dec == nil {
		t.Fatal("NewDecoder returned nil")
	}

	if _, err := dec.Read(make([]byte, 16)); err == nil {
		t.Fatal("expected invalid mp3 decode error")
	}

	if err := dec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := dec.Close(); err != nil {
		t.Fatalf("Close second call: %v", err)
	}
	if _, err := dec.Read(make([]byte, 16)); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("expected closed error, got %v", err)
	}

	var nilDec *Decoder
	if err := nilDec.Close(); err != nil {
		t.Fatalf("nil decoder close: %v", err)
	}
}

func TestDecodeFullInvalidData(t *testing.T) {
	if _, _, _, err := DecodeFull(bytes.NewReader([]byte("not-an-mp3"))); err == nil {
		t.Fatal("expected decode error for invalid data")
	}
}

func TestDecodeFullNilReader(t *testing.T) {
	if _, _, _, err := DecodeFull(nil); err == nil || !strings.Contains(err.Error(), "nil reader") {
		t.Fatalf("expected nil reader error, got %v", err)
	}
}

func TestDecoderNilReader(t *testing.T) {
	dec := NewDecoder(nil)
	if _, err := dec.Read(make([]byte, 32)); err == nil || !strings.Contains(err.Error(), "nil reader") {
		t.Fatalf("expected nil reader error, got %v", err)
	}
}

func TestDecoderMetadataBeforeInit(t *testing.T) {
	dec := NewDecoder(bytes.NewReader(nil))
	if got := dec.SampleRate(); got != 0 {
		t.Fatalf("SampleRate before init=%d, want 0", got)
	}
	if got := dec.Bitrate(); got != 0 {
		t.Fatalf("Bitrate before init=%d, want 0", got)
	}
}

func TestEncoderOptionsAndEdgeBranches(t *testing.T) {
	requireNativeEncoderRuntime(t)

	WithQuality(QualityLow)(nil)
	WithBitrate(128)(nil)

	encBest, err := NewEncoder(io.Discard, 16000, 1, WithQuality(Quality(-1)), WithBitrate(-64))
	if err != nil {
		t.Fatalf("NewEncoder best clamp: %v", err)
	}
	if encBest.quality != QualityBest {
		t.Fatalf("quality=%d, want %d", encBest.quality, QualityBest)
	}
	if encBest.bitrate != 0 {
		t.Fatalf("bitrate=%d, want 0", encBest.bitrate)
	}
	if err := encBest.Flush(); err != nil {
		t.Fatalf("Flush before init: %v", err)
	}
	if n, err := encBest.Write(nil); err != nil || n != 0 {
		t.Fatalf("Write empty got n=%d err=%v", n, err)
	}
	if err := encBest.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := encBest.Write([]byte{1, 2}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("expected closed error, got %v", err)
	}

	encWorst, err := NewEncoder(io.Discard, 16000, 1, WithQuality(Quality(42)))
	if err != nil {
		t.Fatalf("NewEncoder worst clamp: %v", err)
	}
	if encWorst.quality != QualityWorst {
		t.Fatalf("quality=%d, want %d", encWorst.quality, QualityWorst)
	}
	if err := encWorst.Close(); err != nil {
		t.Fatalf("Close worst encoder: %v", err)
	}
}

func TestUnsupportedRuntimeError(t *testing.T) {
	if nativeEncoderRuntimeSupported() {
		t.Skipf("native encoder supported on %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	_, err := NewEncoder(io.Discard, 44100, 2)
	if err == nil || !strings.Contains(err.Error(), "unsupported platform") {
		t.Fatalf("expected unsupported platform error, got %v", err)
	}

	_, err = EncodePCMStream(io.Discard, bytes.NewReader(nil), 44100, 2)
	if err == nil || !strings.Contains(err.Error(), "unsupported platform") {
		t.Fatalf("expected unsupported platform error, got %v", err)
	}
}
