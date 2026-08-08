package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/voiceprint"
)

func patchDeps(t *testing.T) {
	t.Helper()

	origNewDetector := newDetectorFn
	origDecodeOGGFile := decodeOGGFileFn
	origProgressWriter := progressWriter

	t.Cleanup(func() {
		newDetectorFn = origNewDetector
		decodeOGGFileFn = origDecodeOGGFile
		progressWriter = origProgressWriter
	})
}

type scriptedDetector struct {
	detectResults []voiceprint.DetectResult
	updateResults []voiceprint.DetectResult
	detectErr     error
	updateErr     error
	detectCalls   int
	updateCalls   int
	payloads      []string
	closed        bool
}

func (d *scriptedDetector) Detect(_ pcm.Format, r io.Reader, _ voiceprint.DetectCallback) (voiceprint.DetectResult, error) {
	payload, err := io.ReadAll(r)
	if err != nil {
		return voiceprint.DetectResult{}, err
	}
	d.payloads = append(d.payloads, string(payload))
	if d.detectErr != nil {
		return voiceprint.DetectResult{}, d.detectErr
	}
	if d.detectCalls >= len(d.detectResults) {
		return voiceprint.DetectResult{}, errors.New("missing scripted detect result")
	}
	result := d.detectResults[d.detectCalls]
	d.detectCalls++
	return result, nil
}

func (d *scriptedDetector) DetectAndUpdate(_ pcm.Format, r io.Reader, _ voiceprint.DetectCallback) (voiceprint.DetectResult, error) {
	payload, err := io.ReadAll(r)
	if err != nil {
		return voiceprint.DetectResult{}, err
	}
	d.payloads = append(d.payloads, string(payload))
	if d.updateErr != nil {
		return voiceprint.DetectResult{}, d.updateErr
	}
	if d.updateCalls >= len(d.updateResults) {
		return voiceprint.DetectResult{}, errors.New("missing scripted detect-and-update result")
	}
	result := d.updateResults[d.updateCalls]
	d.updateCalls++
	return result, nil
}

func (d *scriptedDetector) Reset() {}

func (d *scriptedDetector) Close() error {
	d.closed = true
	return nil
}

func buildOpusHeadPacket(sampleRate, channels int) []byte {
	packet := make([]byte, 19)
	copy(packet[:8], "OpusHead")
	packet[8] = 1
	packet[9] = byte(channels)
	binary.LittleEndian.PutUint32(packet[12:16], uint32(sampleRate))
	return packet
}

func buildOpusTagsPacket(vendor string) []byte {
	vendorBytes := []byte(vendor)
	packet := make([]byte, 8+4+len(vendorBytes)+4)
	copy(packet[:8], "OpusTags")
	binary.LittleEndian.PutUint32(packet[8:12], uint32(len(vendorBytes)))
	copy(packet[12:12+len(vendorBytes)], vendorBytes)
	return packet
}

func buildAudioFrame(frameSize int) []int16 {
	frame := make([]int16, frameSize)
	for i := range frame {
		frame[i] = int16((i * 113) % 30000)
	}
	return frame
}

func buildOGGOpusFile(t *testing.T, path string) {
	t.Helper()
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}

	enc, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer func() {
		_ = enc.Close()
	}()

	packet, err := enc.Encode(buildAudioFrame(320), 320)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	sw, err := ogg.NewStreamWriter(f, 66)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}
	if _, err := sw.WritePacket(buildOpusHeadPacket(16000, 1), 0, false); err != nil {
		t.Fatalf("WritePacket head: %v", err)
	}
	if _, err := sw.WritePacket(buildOpusTagsPacket("voiceprint-test"), 0, false); err != nil {
		t.Fatalf("WritePacket tags: %v", err)
	}
	if _, err := sw.WritePacket(packet, 320, true); err != nil {
		t.Fatalf("WritePacket audio: %v", err)
	}
}

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig([]string{"a.ogg", "b.ogg"})
	if err != nil {
		t.Fatalf("parseConfig files: %v", err)
	}
	if len(cfg.files) != 2 || cfg.files[0] != "a.ogg" || cfg.files[1] != "b.ogg" {
		t.Fatalf("files = %#v", cfg.files)
	}
	if cfg.prefix != defaultPrefix {
		t.Fatalf("prefix = %q", cfg.prefix)
	}

	cfg, err = parseConfig([]string{"-dir", "/tmp/in"})
	if err != nil {
		t.Fatalf("parseConfig dir: %v", err)
	}
	if cfg.dir != "/tmp/in" {
		t.Fatalf("dir = %q", cfg.dir)
	}
	if _, err := parseConfig(nil); err == nil {
		t.Fatal("expected missing input error")
	}
	if _, err := parseConfig([]string{"-dir", "/tmp/in", "a.ogg"}); err == nil {
		t.Fatal("expected mixed input error")
	}
}

func TestConfigValidateErrors(t *testing.T) {
	cases := []config{
		{},
		{dir: "/tmp/in", files: []string{"a.ogg"}},
		{files: []string{""}},
	}
	for _, tc := range cases {
		if err := tc.validate(); err == nil {
			t.Fatalf("expected validate error for %+v", tc)
		}
	}
}

func TestCollectOGGFilesFiltersOnlyOGG(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.ogg", "a.OGG", "skip.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	files, err := collectOGGFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join([]string{filepath.Base(files[0]), filepath.Base(files[1])}, ",")
	if got != "a.OGG,b.ogg" {
		t.Fatalf("collectOGGFiles = %q", got)
	}
}

func TestCollectInputPathsUsesExplicitOrder(t *testing.T) {
	paths, err := collectInputPaths(config{
		files: []string{"b.ogg", "a.ogg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(paths, ",")
	if got != "b.ogg,a.ogg" {
		t.Fatalf("collectInputPaths = %q", got)
	}
}

func TestRenderAnalysis(t *testing.T) {
	var out bytes.Buffer
	err := renderAnalysis(&out, analysis{
		Dynamic: []fileResult{
			{Name: "a.ogg", Label: "voice:001"},
			{Name: "b.ogg", Label: "voice:001"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	want := "" +
		"dynamic detect-and-update groups:\n" +
		"voice:001 (2)\n" +
		"  - a.ogg\n" +
		"  - b.ogg\n"
	if got != want {
		t.Fatalf("unexpected render output:\n%s", got)
	}
}

func TestTrimSilence(t *testing.T) {
	silenceFrame := make([]byte, 640)
	voiceFrame := make([]byte, 640)
	for i := range 320 {
		sample := int16(1000)
		binary.LittleEndian.PutUint16(voiceFrame[i*2:], uint16(sample))
	}

	audio := append([]byte{}, silenceFrame...)
	audio = append(audio, voiceFrame...)
	audio = append(audio, voiceFrame...)
	audio = append(audio, silenceFrame...)

	trimmed := trimSilence(audio, silenceThreshold)
	if len(trimmed) != 4*640 {
		t.Fatalf("trimmed len = %d, want %d", len(trimmed), 4*640)
	}
	if !bytes.Equal(trimmed[640:1280], voiceFrame) {
		t.Fatal("expected first voiced frame to be preserved")
	}
}

func TestDecodeOGGFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.ogg")
	buildOGGOpusFile(t, path)

	pcmData, err := decodeOGGFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(pcmData) == 0 {
		t.Fatal("expected decoded pcm data")
	}
}

func TestDecodeOGGFileOpenError(t *testing.T) {
	if _, err := decodeOGGFile(filepath.Join(t.TempDir(), "missing.ogg")); err == nil || !strings.Contains(err.Error(), "open file") {
		t.Fatalf("expected open file error, got %v", err)
	}
}

func TestNewDetectorRunnerError(t *testing.T) {
	patchDeps(t)

	newDetectorFn = func(config) (voiceprint.Detector, error) {
		return nil, errors.New("boom")
	}
	if _, err := newDetectorRunner(config{prefix: "voice"}); err == nil {
		t.Fatal("expected newDetectorRunner error")
	}
}

func TestDetectorRunnerAnalyzeFiles(t *testing.T) {
	patchDeps(t)

	detector := &scriptedDetector{
		updateResults: []voiceprint.DetectResult{
			{Label: "voice:001"},
			{Label: "voice:001"},
		},
	}
	decodeOGGFileFn = func(path string) ([]byte, error) {
		switch filepath.Base(path) {
		case "a.ogg":
			return []byte("a"), nil
		case "b.ogg":
			return []byte("b"), nil
		default:
			return nil, errors.New("unexpected path")
		}
	}

	runner := &detectorRunner{detector: detector}
	result, err := runner.AnalyzeFiles([]string{"/tmp/a.ogg", "/tmp/b.ogg"})
	if err != nil {
		t.Fatal(err)
	}
	if detector.updateCalls != 2 || detector.detectCalls != 0 {
		t.Fatalf("calls = detect:%d update:%d", detector.detectCalls, detector.updateCalls)
	}
	if strings.Join(detector.payloads, ",") != "a,b" {
		t.Fatalf("payloads = %#v", detector.payloads)
	}
	if len(result.Dynamic) != 2 {
		t.Fatalf("unexpected result lengths: %+v", result)
	}
	if result.Dynamic[0].Label != "voice:001" || result.Dynamic[1].Label != "voice:001" {
		t.Fatalf("unexpected dynamic labels: %+v", result.Dynamic)
	}
}

func TestDetectorRunnerAnalyzeFilesErrors(t *testing.T) {
	patchDeps(t)

	runner := &detectorRunner{detector: &scriptedDetector{}}
	if _, err := runner.AnalyzeFiles([]string{"/tmp/demo.ogg"}); err == nil {
		t.Fatal("expected decode error")
	}

	decodeOGGFileFn = func(path string) ([]byte, error) { return []byte("a"), nil }
	runner.detector = &scriptedDetector{
		updateErr: errors.New("update failed"),
	}
	if _, err := runner.AnalyzeFiles([]string{"/tmp/demo.ogg"}); err == nil || !strings.Contains(err.Error(), "update failed") {
		t.Fatalf("expected update error, got %v", err)
	}

	runner.detector = &scriptedDetector{
		updateResults: []voiceprint.DetectResult{{Label: "voice:001"}},
	}
}

func TestDetectorRunnerClose(t *testing.T) {
	detector := &scriptedDetector{}
	runner := &detectorRunner{detector: detector}
	if err := runner.Close(); err != nil {
		t.Fatal(err)
	}
	if !detector.closed {
		t.Fatal("expected detector to be closed")
	}
	if err := (*detectorRunner)(nil).Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRunOutputsGroups(t *testing.T) {
	patchDeps(t)

	dir := t.TempDir()
	for _, name := range []string{"a.ogg", "b.ogg", "skip.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	detector := &scriptedDetector{
		updateResults: []voiceprint.DetectResult{
			{Label: "voice:001"},
			{Label: "voice:001"},
		},
	}
	newDetectorFn = func(cfg config) (voiceprint.Detector, error) {
		if cfg.dir != dir {
			t.Fatalf("unexpected cfg.dir: %s", cfg.dir)
		}
		if cfg.prefix != defaultPrefix {
			t.Fatalf("unexpected cfg.prefix: %s", cfg.prefix)
		}
		return detector, nil
	}
	decodeOGGFileFn = func(path string) ([]byte, error) {
		return []byte(filepath.Base(path)[:1]), nil
	}

	var out bytes.Buffer
	var progress bytes.Buffer
	progressWriter = &progress

	err := run(config{
		dir:    dir,
		prefix: defaultPrefix,
	}, &out)
	if err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if !strings.Contains(got, "dynamic detect-and-update groups:\nvoice:001 (2)\n  - a.ogg\n  - b.ogg\n") {
		t.Fatalf("missing dynamic groups:\n%s", got)
	}
	if !strings.Contains(progress.String(), "found 2 input files") {
		t.Fatalf("missing progress output: %s", progress.String())
	}
	if !detector.closed {
		t.Fatal("expected detector to be closed")
	}
}

func TestRunErrorsAndRenderWriterError(t *testing.T) {
	patchDeps(t)

	dir := t.TempDir()
	if err := run(config{dir: dir, prefix: defaultPrefix}, io.Discard); err == nil {
		t.Fatal("expected no-files error")
	}

	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.ogg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	newDetectorFn = func(config) (voiceprint.Detector, error) {
		return nil, errors.New("boom")
	}
	if err := run(config{dir: dir, prefix: defaultPrefix}, io.Discard); err == nil {
		t.Fatal("expected detector creation error")
	}

	fail := failWriter{}
	if err := renderAnalysis(fail, analysis{
		Dynamic: []fileResult{{Name: "a.ogg", Label: "voice:001"}},
	}); err == nil {
		t.Fatal("expected render error")
	}
}

type failWriter struct{}

func (failWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}
