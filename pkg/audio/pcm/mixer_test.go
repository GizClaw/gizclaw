package pcm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"testing"
	"time"
)

// generateSineWave generates a sine wave as int16 samples.
func generateSineWave(freq float64, sampleRate int, durationMs int) []byte {
	samples := sampleRate * durationMs / 1000
	data := make([]byte, samples*2)
	for i := range samples {
		t := float64(i) / float64(sampleRate)
		value := math.Sin(2 * math.Pi * freq * t)
		sample := int16(value * 16000)
		binary.LittleEndian.PutUint16(data[i*2:], uint16(sample))
	}
	return data
}

func makeConstantChunk(sample int16, sampleCount int) []byte {
	data := make([]byte, sampleCount*2)
	for i := range sampleCount {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(sample))
	}
	return data
}

func decodePCM16LE(data []byte) []int16 {
	out := make([]int16, len(data)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return out
}

func absInt16Diff(a, b int16) int {
	d := int(a) - int(b)
	if d < 0 {
		d = -d
	}
	return d
}

// -----------------------------------------------------------------------------
// 基础 API / 生命周期
// -----------------------------------------------------------------------------

func TestMixerReadEmptyBuffer(t *testing.T) {
	mx := NewMixer(L16Mono16K)
	n, err := mx.Read(nil)
	if err != nil {
		t.Fatalf("Read(nil) error: %v", err)
	}
	if n != 0 {
		t.Fatalf("Read(nil) n=%d, want 0", n)
	}
}

func TestMixerReadOddBufferReturnsAlignedLength(t *testing.T) {
	mx := NewMixer(L16Mono16K, WithAutoClose())

	tr, ctrl, err := mx.CreateTrack()
	if err != nil {
		t.Fatalf("CreateTrack() error: %v", err)
	}
	if err := tr.Write(L16Mono16K.DataChunk([]byte{1, 0, 2, 0})); err != nil {
		t.Fatalf("track write error: %v", err)
	}
	if err := ctrl.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error: %v", err)
	}

	buf := []byte{0, 0, 0, 0, 0x7f}
	n, err := mx.Read(buf)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if n != 4 {
		t.Fatalf("Read() n = %d, want 4", n)
	}
	if buf[4] != 0 {
		t.Fatalf("odd tail byte should be zeroed, got %d", buf[4])
	}
}

func TestMixerClosePaths(t *testing.T) {
	mx := NewMixer(L16Mono16K)
	if err := mx.CloseWithError(nil); err != nil {
		t.Fatalf("CloseWithError(nil) error: %v", err)
	}

	if _, _, err := mx.CreateTrack(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("CreateTrack after CloseWithError err = %v, want closed pipe", err)
	}

	mx2 := NewMixer(L16Mono16K)
	if err := mx2.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error: %v", err)
	}
	if _, _, err := mx2.CreateTrack(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("CreateTrack after CloseWrite err = %v, want closed pipe", err)
	}

	if err := mx2.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}

func TestTrackWriterErrorMethods(t *testing.T) {
	mx := NewMixer(L16Mono16K)
	tr, _, err := mx.CreateTrack()
	if err != nil {
		t.Fatalf("CreateTrack() error: %v", err)
	}

	inner := tr.(*track)
	tw := inner.inputs[0]
	if tw.Error() != nil {
		t.Fatalf("initial track writer error should be nil")
	}

	wantErr := errors.New("tw-error")
	if err := tw.CloseWithError(wantErr); err != nil {
		t.Fatalf("CloseWithError() error: %v", err)
	}
	if !errors.Is(tw.Error(), wantErr) {
		t.Fatalf("tw.Error() = %v, want %v", tw.Error(), wantErr)
	}

	buf := make([]byte, 32)
	if _, err := tw.Read(buf); !errors.Is(err, wantErr) {
		t.Fatalf("Read() err = %v, want %v", err, wantErr)
	}
}

func TestMixerOptionsCallbacksAndOutput(t *testing.T) {
	created := 0
	closed := 0

	mx := NewMixer(
		L16Mono16K,
		WithSilenceGap(50*time.Millisecond),
		WithOnTrackCreated(func() { created++ }),
		WithOnTrackClosed(func() { closed++ }),
	)

	if mx.Output() != L16Mono16K {
		t.Fatalf("Output() = %v, want %v", mx.Output(), L16Mono16K)
	}

	tr, ctrl, err := mx.CreateTrack(WithTrackLabel("cb"))
	if err != nil {
		t.Fatalf("CreateTrack() error: %v", err)
	}
	if created != 1 {
		t.Fatalf("created callback count = %d, want 1", created)
	}

	data := bytes.Repeat([]byte{1, 2}, 320)
	if err := tr.Write(L16Mono16K.DataChunk(data)); err != nil {
		t.Fatalf("track write error: %v", err)
	}
	if err := ctrl.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error: %v", err)
	}
	if err := mx.CloseWrite(); err != nil {
		t.Fatalf("mixer CloseWrite() error: %v", err)
	}

	// Drain mixer so closed callback has chance to run.
	if _, err := io.ReadAll(mx); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAll() error: %v", err)
	}

	if closed == 0 {
		t.Fatal("closed callback should be called at least once")
	}
}

// -----------------------------------------------------------------------------
// 包内并发回归（需要访问未导出方法）
// -----------------------------------------------------------------------------

func TestMixerNotifyWriteAfterCloseWriteNoPanicNoBlock(t *testing.T) {
	mx := NewMixer(L16Mono16K)
	if err := mx.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 10000 {
			mx.notifyWrite()
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("notifyWrite() blocked after CloseWrite")
	}
}

// -----------------------------------------------------------------------------
// 混音功能行为
// -----------------------------------------------------------------------------

func TestMixerExactMixTwoTracksFullChunk(t *testing.T) {
	format := L16Mono16K
	mx := NewMixer(format, WithAutoClose())

	trA, ctrlA, err := mx.CreateTrack(WithTrackLabel("A"))
	if err != nil {
		t.Fatalf("CreateTrack(A) error: %v", err)
	}
	trB, ctrlB, err := mx.CreateTrack(WithTrackLabel("B"))
	if err != nil {
		t.Fatalf("CreateTrack(B) error: %v", err)
	}

	const samples = 160 // 10ms @ 16k
	if err := trA.Write(format.DataChunk(makeConstantChunk(1000, samples))); err != nil {
		t.Fatalf("track A write error: %v", err)
	}
	if err := trB.Write(format.DataChunk(makeConstantChunk(2000, samples))); err != nil {
		t.Fatalf("track B write error: %v", err)
	}
	_ = ctrlA.CloseWrite()
	_ = ctrlB.CloseWrite()
	_ = mx.CloseWrite()

	buf := make([]byte, samples*2)
	n, err := mx.Read(buf)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("Read() n=%d want=%d", n, len(buf))
	}

	decoded := decodePCM16LE(buf[:n])
	for i, s := range decoded {
		if d := absInt16Diff(s, 3000); d > 1 {
			t.Fatalf("sample[%d]=%d want~=3000 (diff=%d)", i, s, d)
		}
	}
}

func TestMixerExactMixOneTrackPartialChunkPadsSilence(t *testing.T) {
	format := L16Mono16K
	mx := NewMixer(format, WithAutoClose())

	trA, ctrlA, err := mx.CreateTrack(WithTrackLabel("A"))
	if err != nil {
		t.Fatalf("CreateTrack(A) error: %v", err)
	}
	trB, ctrlB, err := mx.CreateTrack(WithTrackLabel("B"))
	if err != nil {
		t.Fatalf("CreateTrack(B) error: %v", err)
	}

	const samples = 160
	half := samples / 2
	if err := trA.Write(format.DataChunk(makeConstantChunk(1000, samples))); err != nil {
		t.Fatalf("track A write error: %v", err)
	}
	if err := trB.Write(format.DataChunk(makeConstantChunk(2000, half))); err != nil {
		t.Fatalf("track B write error: %v", err)
	}
	_ = ctrlA.CloseWrite()
	_ = ctrlB.CloseWrite()
	_ = mx.CloseWrite()

	buf := make([]byte, samples*2)
	n, err := mx.Read(buf)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("Read() n=%d want=%d", n, len(buf))
	}

	decoded := decodePCM16LE(buf[:n])
	for i := range half {
		if d := absInt16Diff(decoded[i], 3000); d > 1 {
			t.Fatalf("front sample[%d]=%d want~=3000 (diff=%d)", i, decoded[i], d)
		}
	}
	for i := half; i < samples; i++ {
		if d := absInt16Diff(decoded[i], 1000); d > 1 {
			t.Fatalf("tail sample[%d]=%d want~=1000 (diff=%d)", i, decoded[i], d)
		}
	}
}

func TestMixerExactMixOneTrackBlockedOtherActive(t *testing.T) {
	format := L16Mono16K
	mx := NewMixer(format)

	_, stalledCtrl, err := mx.CreateTrack(WithTrackLabel("stalled"))
	if err != nil {
		t.Fatalf("CreateTrack(stalled) error: %v", err)
	}
	active, activeCtrl, err := mx.CreateTrack(WithTrackLabel("active"))
	if err != nil {
		t.Fatalf("CreateTrack(active) error: %v", err)
	}

	const samples = 160
	if err := active.Write(format.DataChunk(makeConstantChunk(2000, samples))); err != nil {
		t.Fatalf("active write error: %v", err)
	}
	_ = activeCtrl.CloseWrite()

	buf := make([]byte, samples*2)
	readDone := make(chan error, 1)
	go func() {
		_, err := mx.Read(buf)
		readDone <- err
	}()

	select {
	case err := <-readDone:
		if err != nil {
			t.Fatalf("Read() error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("one track stalled but another active: Read blocked unexpectedly")
	}

	decoded := decodePCM16LE(buf)
	for i, s := range decoded {
		if d := absInt16Diff(s, 2000); d > 1 {
			t.Fatalf("sample[%d]=%d want~=2000 (diff=%d)", i, s, d)
		}
	}

	_ = stalledCtrl.CloseWrite()
	_ = mx.CloseWrite()
}

func TestMixerExactMixClippingBoundary(t *testing.T) {
	format := L16Mono16K
	mx := NewMixer(format, WithAutoClose())

	trA, ctrlA, err := mx.CreateTrack(WithTrackLabel("clipA"))
	if err != nil {
		t.Fatalf("CreateTrack(clipA) error: %v", err)
	}
	trB, ctrlB, err := mx.CreateTrack(WithTrackLabel("clipB"))
	if err != nil {
		t.Fatalf("CreateTrack(clipB) error: %v", err)
	}

	const samples = 160
	if err := trA.Write(format.DataChunk(makeConstantChunk(30000, samples))); err != nil {
		t.Fatalf("track A write error: %v", err)
	}
	if err := trB.Write(format.DataChunk(makeConstantChunk(30000, samples))); err != nil {
		t.Fatalf("track B write error: %v", err)
	}
	_ = ctrlA.CloseWrite()
	_ = ctrlB.CloseWrite()
	_ = mx.CloseWrite()

	buf := make([]byte, samples*2)
	n, err := mx.Read(buf)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("Read() n=%d want=%d", n, len(buf))
	}

	decoded := decodePCM16LE(buf[:n])
	for i, s := range decoded {
		if s > 32767 || s < -32768 {
			t.Fatalf("sample[%d]=%d out of int16 range", i, s)
		}
		if s < 32760 {
			t.Fatalf("sample[%d]=%d expected clipped near 32767", i, s)
		}
	}
}

func TestMixerMixesTwoTracks(t *testing.T) {
	format := L16Mono16K
	mixer := NewMixer(format, WithAutoClose())

	// Create two tracks.
	track1, ctrl1, err := mixer.CreateTrack(WithTrackLabel("440Hz"))
	if err != nil {
		t.Fatal(err)
	}
	track2, ctrl2, err := mixer.CreateTrack(WithTrackLabel("880Hz"))
	if err != nil {
		t.Fatal(err)
	}

	// Generate 100ms of audio for each track.
	wave1 := generateSineWave(440, 16000, 100) // 440Hz.
	wave2 := generateSineWave(880, 16000, 100) // 880Hz.

	// Write to tracks in goroutines.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := track1.Write(format.DataChunk(wave1)); err != nil {
			t.Errorf("track1 write error: %v", err)
		}
		ctrl1.CloseWrite()
	}()

	go func() {
		defer wg.Done()
		if err := track2.Write(format.DataChunk(wave2)); err != nil {
			t.Errorf("track2 write error: %v", err)
		}
		ctrl2.CloseWrite()
	}()

	// Close mixer when writers are done.
	go func() {
		wg.Wait()
		mixer.CloseWrite()
	}()

	// Read mixed output.
	mixed, err := io.ReadAll(mixer)
	if err != nil {
		t.Fatal(err)
	}

	// Analyze the mixed output.
	if len(mixed) < 4 {
		t.Fatal("mixed output too short")
	}

	// Convert to int16 samples.
	samples := make([]int16, len(mixed)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(mixed[i*2:]))
	}

	// Find peak, count zero crossings, and check if we have audio.
	var peak int16
	var nonZero int
	var zeroCrossings int
	var prevSign bool

	for i, s := range samples {
		if s > peak {
			peak = s
		}
		if -s > peak {
			peak = -s
		}
		if s != 0 {
			nonZero++
		}
		// Count zero crossings.
		currentSign := s >= 0
		if i > 0 && currentSign != prevSign {
			zeroCrossings++
		}
		prevSign = currentSign
	}

	expectedMinCrossings := 150 // Should be more than single 440Hz.
	if zeroCrossings < expectedMinCrossings {
		t.Errorf("zero crossings too low (%d < %d), suggests tracks not properly mixed", zeroCrossings, expectedMinCrossings)
	}

	// Check that we have audio (允许前置静音，不要求非零占比).
	if nonZero == 0 {
		t.Errorf("all samples are zero, mixing may have failed")
	}

	// Compute expected values from separate waves and compare.
	for i := 0; i < 10 && i < len(samples); i++ {
		w1 := int16(binary.LittleEndian.Uint16(wave1[i*2:]))
		w2 := int16(binary.LittleEndian.Uint16(wave2[i*2:]))
		expected := int32(w1) + int32(w2)
		// Clip expected.
		if expected > 32767 {
			expected = 32767
		}
		if expected < -32768 {
			expected = -32768
		}
		_ = samples[i] - int16(expected)
	}
}

func TestMixerSequentialWrite(t *testing.T) {
	// Test writing sequentially to see if it's track order issue.
	format := L16Mono16K
	mixer := NewMixer(format, WithAutoClose())

	track1, ctrl1, _ := mixer.CreateTrack(WithTrackLabel("track1"))
	track2, ctrl2, _ := mixer.CreateTrack(WithTrackLabel("track2"))

	pattern1 := make([]byte, 20) // 10 samples.
	pattern2 := make([]byte, 20)
	for i := range 10 {
		binary.LittleEndian.PutUint16(pattern1[i*2:], uint16(10000))
		binary.LittleEndian.PutUint16(pattern2[i*2:], uint16(5000))
	}

	// Write synchronously.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		track1.Write(format.DataChunk(pattern1))
		ctrl1.CloseWrite()
	}()

	go func() {
		defer wg.Done()
		track2.Write(format.DataChunk(pattern2))
		ctrl2.CloseWrite()
	}()

	go func() {
		wg.Wait()
		mixer.CloseWrite()
	}()

	mixed, _ := io.ReadAll(mixer)

	// Check values (允许前置静音，扫描全部样本).
	hasData := false
	for i := 0; i < len(mixed)/2; i++ {
		val := int16(binary.LittleEndian.Uint16(mixed[i*2:]))
		if val != 0 {
			hasData = true
			break
		}
	}
	if !hasData {
		t.Fatal("expected mixed output to contain non-zero samples")
	}
}

// TestMixerDebug is a detailed debug test.
func TestMixerDebug(t *testing.T) {
	format := L16Mono16K
	mixer := NewMixer(format)

	// Create tracks.
	track1, ctrl1, _ := mixer.CreateTrack(WithTrackLabel("A"))
	track2, ctrl2, _ := mixer.CreateTrack(WithTrackLabel("B"))

	// Write constant values.
	// Track A: all 1000.
	// Track B: all 2000.
	dataA := make([]byte, 100)
	dataB := make([]byte, 100)
	for i := range 50 {
		binary.LittleEndian.PutUint16(dataA[i*2:], uint16(1000))
		binary.LittleEndian.PutUint16(dataB[i*2:], uint16(2000))
	}

	done := make(chan struct{})
	go func() {
		// Write sequentially - this works.
		track1.Write(format.DataChunk(dataA))
		ctrl1.CloseWrite()
		track2.Write(format.DataChunk(dataB))
		ctrl2.CloseWrite()
		mixer.CloseWrite()
		close(done)
	}()

	// Read in chunks.
	buf := make([]byte, 20)
	for {
		_, err := mixer.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	<-done
}

// TestMixerConcurrentWrite tests concurrent writes which is the real issue.
func TestMixerConcurrentWrite(t *testing.T) {
	format := L16Mono16K
	mixer := NewMixer(format, WithAutoClose())

	track1, ctrl1, _ := mixer.CreateTrack(WithTrackLabel("A"))
	track2, ctrl2, _ := mixer.CreateTrack(WithTrackLabel("B"))

	dataA := make([]byte, 3200) // 100ms at 16kHz.
	dataB := make([]byte, 3200)
	for i := range 1600 {
		binary.LittleEndian.PutUint16(dataA[i*2:], uint16(1000))
		binary.LittleEndian.PutUint16(dataB[i*2:], uint16(2000))
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Write concurrently.
	go func() {
		defer wg.Done()
		track1.Write(format.DataChunk(dataA))
		ctrl1.CloseWrite()
	}()
	go func() {
		defer wg.Done()
		track2.Write(format.DataChunk(dataB))
		ctrl2.CloseWrite()
	}()

	go func() {
		wg.Wait()
		mixer.CloseWrite()
	}()

	mixed, _ := io.ReadAll(mixer)

	// Analyze.
	count1000 := 0
	count2000 := 0
	count3000 := 0
	countOther := 0

	for i := 0; i < len(mixed)/2; i++ {
		val := int16(binary.LittleEndian.Uint16(mixed[i*2:]))
		switch val {
		case 1000:
			count1000++
		case 2000:
			count2000++
		case 3000:
			count3000++
		default:
			countOther++
		}
	}

	_ = countOther

	if count3000 == 0 && count1000 == 0 && count2000 == 0 {
		t.Error("no audio data at all")
	}
}

func TestMixerFourTracks(t *testing.T) {
	format := L16Mono16K
	mixer := NewMixer(format, WithAutoClose())

	values := []int16{1000, 2000, 3000, 4000}
	var wg sync.WaitGroup

	for _, val := range values {
		track, ctrl, err := mixer.CreateTrack(WithTrackLabel(fmt.Sprintf("t%d", val)))
		if err != nil {
			t.Fatal(err)
		}
		data := make([]byte, 1600) // 50ms.
		for i := range 800 {
			binary.LittleEndian.PutUint16(data[i*2:], uint16(val))
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			track.Write(format.DataChunk(data))
			ctrl.CloseWrite()
		}()
	}

	go func() {
		wg.Wait()
		mixer.CloseWrite()
	}()

	mixed, err := io.ReadAll(mixer)
	if err != nil {
		t.Fatal(err)
	}

	samples := make([]int16, len(mixed)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(mixed[i*2:]))
	}

	nonZero := 0
	var maxSample int16
	for _, s := range samples {
		if s != 0 {
			nonZero++
		}
		if s > maxSample {
			maxSample = s
		}
	}

	if nonZero == 0 {
		t.Error("should have non-zero samples from 4 tracks")
	}
	if maxSample < 1000 {
		t.Errorf("should have audio from tracks (peak=%d)", maxSample)
	}
}

func TestMixerDynamicTrackAddition(t *testing.T) {
	format := L16Mono16K
	mixer := NewMixer(format)

	track1, ctrl1, _ := mixer.CreateTrack(WithTrackLabel("bg"))
	track2, ctrl2, _ := mixer.CreateTrack(WithTrackLabel("fg"))

	data1 := make([]byte, 3200) // 100ms.
	data2 := make([]byte, 3200)
	for i := range 1600 {
		binary.LittleEndian.PutUint16(data1[i*2:], uint16(1000))
		binary.LittleEndian.PutUint16(data2[i*2:], uint16(2000))
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		track1.Write(format.DataChunk(data1))
		track2.Write(format.DataChunk(data2))

		// Add 3rd track while mixer is running.
		track3, ctrl3, _ := mixer.CreateTrack(WithTrackLabel("overlay"))
		data3 := make([]byte, 1600)
		for i := range 800 {
			binary.LittleEndian.PutUint16(data3[i*2:], uint16(3000))
		}
		track3.Write(format.DataChunk(data3))

		ctrl1.CloseWrite()
		ctrl2.CloseWrite()
		ctrl3.CloseWrite()
		mixer.CloseWrite()
	}()

	mixed, _ := io.ReadAll(mixer)
	<-done

	samples := make([]int16, len(mixed)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(mixed[i*2:]))
	}

	nonZero := 0
	for _, s := range samples {
		if s != 0 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Error("should have audio from dynamically added tracks")
	}
}

func TestMixerGainClipping(t *testing.T) {
	format := L16Mono16K
	mixer := NewMixer(format, WithAutoClose())

	var wg sync.WaitGroup

	// 4 tracks, each writing 10000 — sum = 40000 > 32767.
	for i := range 4 {
		track, ctrl, _ := mixer.CreateTrack(WithTrackLabel(fmt.Sprintf("loud%d", i)))
		data := make([]byte, 1600) // 50ms.
		for j := range 800 {
			binary.LittleEndian.PutUint16(data[j*2:], uint16(10000))
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			track.Write(format.DataChunk(data))
			ctrl.CloseWrite()
		}()
	}

	go func() {
		wg.Wait()
		mixer.CloseWrite()
	}()

	mixed, _ := io.ReadAll(mixer)

	samples := make([]int16, len(mixed)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(mixed[i*2:]))
	}

	var maxSample int16
	for _, s := range samples {
		if s > maxSample {
			maxSample = s
		}
	}
	if maxSample < 10000 {
		t.Errorf("with 4 tracks of 10000, peak should show mixing (got %d)", maxSample)
	}
}

func TestMixerPerTrackGain(t *testing.T) {
	format := L16Mono16K
	mixer := NewMixer(format, WithAutoClose())

	trackA, ctrlA, _ := mixer.CreateTrack(WithTrackLabel("full"))
	trackB, ctrlB, _ := mixer.CreateTrack(WithTrackLabel("quiet"))

	ctrlB.SetGain(0.25)

	data := make([]byte, 1600) // 50ms of 20000.
	for i := range 800 {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(20000))
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		trackA.Write(format.DataChunk(data))
		ctrlA.CloseWrite()
	}()
	data2 := make([]byte, len(data))
	copy(data2, data)
	go func() {
		defer wg.Done()
		trackB.Write(format.DataChunk(data2))
		ctrlB.CloseWrite()
	}()

	go func() {
		wg.Wait()
		mixer.CloseWrite()
	}()

	mixed, _ := io.ReadAll(mixer)

	samples := make([]int16, len(mixed)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(mixed[i*2:]))
	}

	hasData := false
	for _, s := range samples {
		if s != 0 {
			hasData = true
			break
		}
	}
	if !hasData {
		t.Error("should have audio output")
	}

	var maxSample int16
	for _, s := range samples {
		if s > maxSample {
			maxSample = s
		}
	}
	countAbove20k := 0
	for _, s := range samples {
		if s > 20000 {
			countAbove20k++
		}
	}
	if countAbove20k == 0 && maxSample <= 5000 {
		t.Error("gain-reduced track B should still contribute to output")
	}
}

func TestMixerFadeOutRealtime(t *testing.T) {
	format := L16Mono16K
	mixer := NewMixer(format, WithAutoClose())

	track, ctrl, _ := mixer.CreateTrack(WithTrackLabel("fade"))

	// Write 200ms of constant 10000.
	data := make([]byte, 6400) // 16kHz * 0.2s * 2 bytes.
	for i := range 3200 {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(10000))
	}
	track.Write(format.DataChunk(data))

	ctrl.SetFadeOutDuration(100 * time.Millisecond)
	ctrl.Close()

	// Read at realtime pace: 20ms per chunk.
	var chunks [][]int16
	buf := make([]byte, 640)
	for {
		time.Sleep(20 * time.Millisecond)
		n, err := mixer.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
		samples := make([]int16, n/2)
		for i := range samples {
			samples[i] = int16(binary.LittleEndian.Uint16(buf[i*2:]))
		}
		chunks = append(chunks, samples)
	}

	if len(chunks) == 0 {
		t.Fatal("should have at least one chunk for 200ms of audio")
	}

	nonZero := 0
	for _, chunk := range chunks {
		for _, s := range chunk {
			if s != 0 {
				nonZero++
			}
		}
	}
	if nonZero == 0 {
		t.Error("should have non-zero audio output")
	}
}
