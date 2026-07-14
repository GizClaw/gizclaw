package resampler

import (
	"bytes"
	"errors"
	"io"
	"math"
	"testing"
)

func TestResamplerPassthroughMono(t *testing.T) {
	src := Format{SampleRate: 16000, Stereo: false}
	dst := Format{SampleRate: 16000, Stereo: false}

	input := []byte{1, 0, 2, 0, 3, 0, 4, 0}
	r, err := New(bytes.NewReader(input), src, dst)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer r.Close()

	out := make([]byte, len(input))
	n, err := r.Read(out)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if n != len(input) {
		t.Fatalf("Read() n = %d, want %d", n, len(input))
	}
	if !bytes.Equal(out[:n], input) {
		t.Fatalf("passthrough mismatch: got %v want %v", out[:n], input)
	}
}

func TestResamplerUpmixMonoToStereo(t *testing.T) {
	src := Format{SampleRate: 16000, Stereo: false}
	dst := Format{SampleRate: 16000, Stereo: true}

	// mono samples: [1000, -1000]
	input := int16ToBytes([]int16{1000, -1000})
	r, err := New(bytes.NewReader(input), src, dst)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer r.Close()

	buf := make([]byte, 8)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if n != 8 {
		t.Fatalf("Read() n = %d, want 8", n)
	}

	got := bytesToInt16(buf[:n])
	want := []int16{1000, 1000, -1000, -1000}
	if !equalInt16(got, want) {
		t.Fatalf("upmix result mismatch: got %v want %v", got, want)
	}
}

func TestResamplerDownmixStereoToMono(t *testing.T) {
	src := Format{SampleRate: 16000, Stereo: true}
	dst := Format{SampleRate: 16000, Stereo: false}

	// stereo frames: [(1000,3000),(-1000,-3000)] => mono [2000,-2000]
	input := int16ToBytes([]int16{1000, 3000, -1000, -3000})
	r, err := New(bytes.NewReader(input), src, dst)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer r.Close()

	buf := make([]byte, 4)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if n != 4 {
		t.Fatalf("Read() n = %d, want 4", n)
	}

	got := bytesToInt16(buf[:n])
	want := []int16{2000, -2000}
	if !equalInt16(got, want) {
		t.Fatalf("downmix result mismatch: got %v want %v", got, want)
	}
}

func TestResamplerDifferentSampleRate(t *testing.T) {
	src := Format{SampleRate: 16000, Stereo: false}
	dst := Format{SampleRate: 8000, Stereo: false}

	input := generateSinePCM(16000, 440, 1.0) // 1 second 16k mono
	r, err := New(bytes.NewReader(input), src, dst)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer r.Close()

	out, err := readAll(r, 30)
	if err != nil {
		t.Fatalf("readAll() error: %v", err)
	}

	if len(out) == 0 {
		t.Fatal("resampled output is empty")
	}
	if len(out)%2 != 0 {
		t.Fatalf("output length should align to int16, got %d", len(out))
	}

	// Downsampled output should be significantly smaller than input.
	if !(len(out) < len(input) && len(out) > len(input)/4) {
		t.Fatalf("unexpected downsample size: input=%d output=%d", len(input), len(out))
	}
}

func TestResamplerDrainsLeftoverBeforeEOF(t *testing.T) {
	src := Format{SampleRate: 16000, Stereo: false}
	dst := Format{SampleRate: 8000, Stereo: false}

	input := generateSinePCM(16000, 440, 0.1)
	r, err := New(bytes.NewReader(input), src, dst)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer r.Close()

	out, err := readAll(r, 2)
	if err != nil {
		t.Fatalf("readAll() error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("resampled output is empty")
	}
	if len(out)%2 != 0 {
		t.Fatalf("output length should align to int16, got %d", len(out))
	}
}

func TestSoxrFlushesOnceAndDefersEOFUntilOutputIsDrained(t *testing.T) {
	engine := &recordingResampler{
		processOutput: []float64{0.25},
		flushOutput:   []float64{0.5, 0.75},
	}
	r := &Soxr{
		srcFmt:        Format{SampleRate: 2},
		src:           &dataEOFReader{data: []byte{1, 0, 2, 0}},
		dstFmt:        Format{SampleRate: 1},
		resampler:     engine,
		needsResample: true,
	}

	var got []byte
	buf := make([]byte, 2)
	for {
		n, err := r.Read(buf)
		got = append(got, buf[:n]...)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
	}
	if engine.flushCalls != 1 {
		t.Fatalf("Flush calls = %d, want 1", engine.flushCalls)
	}
	if len(got) != 6 {
		t.Fatalf("output bytes = %d, want 6", len(got))
	}
}

func TestSoxrPassthroughDefersEOFReturnedWithData(t *testing.T) {
	r := &Soxr{
		srcFmt: Format{SampleRate: 16000},
		src:    &dataEOFReader{data: []byte{1, 0, 2, 0}},
		dstFmt: Format{SampleRate: 16000},
	}
	buf := make([]byte, 4)
	n, err := r.Read(buf)
	if err != nil || n != 4 {
		t.Fatalf("first Read() = (%d, %v), want (4, nil)", n, err)
	}
	n, err = r.Read(buf)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("second Read() = (%d, %v), want (0, EOF)", n, err)
	}
}

func TestResamplerShortBuffer(t *testing.T) {
	src := Format{SampleRate: 16000, Stereo: false}
	dst := Format{SampleRate: 16000, Stereo: false}
	r, err := New(bytes.NewReader(make([]byte, 4)), src, dst)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer r.Close()

	_, err = r.Read(make([]byte, 1))
	if !errors.Is(err, io.ErrShortBuffer) {
		t.Fatalf("Read() err = %v, want %v", err, io.ErrShortBuffer)
	}
}

func TestResamplerCloseWithError(t *testing.T) {
	src := Format{SampleRate: 16000, Stereo: false}
	dst := Format{SampleRate: 16000, Stereo: false}
	r, err := New(bytes.NewReader(nil), src, dst)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	wantErr := errors.New("custom-close-error")
	if err := r.CloseWithError(wantErr); err != nil {
		t.Fatalf("CloseWithError() err = %v", err)
	}

	_, err = r.Read(make([]byte, 2))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Read() err = %v, want %v", err, wantErr)
	}
}

func TestResamplerClose(t *testing.T) {
	src := Format{SampleRate: 16000, Stereo: false}
	dst := Format{SampleRate: 16000, Stereo: false}
	r, err := New(bytes.NewReader(nil), src, dst)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}

	_, err = r.Read(make([]byte, 2))
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Read() err = %v, want closed pipe", err)
	}
}

func TestResamplerCloseWithNilError(t *testing.T) {
	src := Format{SampleRate: 16000, Stereo: false}
	dst := Format{SampleRate: 8000, Stereo: false}

	input := generateSinePCM(16000, 440, 0.1)
	r, err := New(bytes.NewReader(input), src, dst)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := r.CloseWithError(nil); err != nil {
		t.Fatalf("CloseWithError(nil) err = %v", err)
	}

	_, err = r.Read(make([]byte, 160))
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Read() err = %v, want closed pipe", err)
	}
}

func int16ToBytes(samples []int16) []byte {
	out := make([]byte, len(samples)*2)
	for i, s := range samples {
		out[i*2] = byte(s)
		out[i*2+1] = byte(s >> 8)
	}
	return out
}

func bytesToInt16(b []byte) []int16 {
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(b[i*2]) | int16(b[i*2+1])<<8
	}
	return out
}

func equalInt16(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func generateSinePCM(sampleRate int, freq float64, seconds float64) []byte {
	n := int(float64(sampleRate) * seconds)
	out := make([]byte, n*2)
	for i := range n {
		v := math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
		s := int16(v * 30000)
		out[i*2] = byte(s)
		out[i*2+1] = byte(s >> 8)
	}
	return out
}

func readAll(r io.Reader, bufSize int) ([]byte, error) {
	buf := make([]byte, bufSize)
	var out []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		return out, err
	}
}

type dataEOFReader struct {
	data []byte
}

func (r *dataEOFReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, io.EOF
}

type recordingResampler struct {
	processOutput []float64
	flushOutput   []float64
	flushCalls    int
}

func (r *recordingResampler) Process([]float64) ([]float64, error) {
	return append([]float64(nil), r.processOutput...), nil
}

func (*recordingResampler) ProcessFloat32([]float32) ([]float32, error)   { return nil, nil }
func (*recordingResampler) ProcessMulti([][]float64) ([][]float64, error) { return nil, nil }
func (r *recordingResampler) Flush() ([]float64, error) {
	r.flushCalls++
	return append([]float64(nil), r.flushOutput...), nil
}
func (*recordingResampler) GetLatency() int   { return 0 }
func (*recordingResampler) Reset()            {}
func (*recordingResampler) GetRatio() float64 { return 0.5 }
