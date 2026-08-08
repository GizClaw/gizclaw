package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/voiceprint"
)

const (
	defaultPrefix    = "voice"
	silenceThreshold = int16(300)
)

type config struct {
	dir    string
	files  []string
	prefix string
}

type audioSample struct {
	path  string
	name  string
	audio []byte
}

type fileResult struct {
	Path  string
	Name  string
	Label string
}

type fileGroup struct {
	Label string
	Files []string
}

type analysis struct {
	Dynamic []fileResult
}

type detectorRunner struct {
	detector voiceprint.Detector
}

var (
	newDetectorFn = func(cfg config) (voiceprint.Detector, error) {
		return voiceprint.NewECAPA(voiceprint.DetectorConfig{
			VoiceLabelPrefix: cfg.prefix,
		})
	}
	decodeOGGFileFn           = decodeOGGFile
	progressWriter  io.Writer = os.Stderr
)

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse args failed: %v\n", err)
		os.Exit(2)
	}
	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		os.Exit(1)
	}
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("voiceprint", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	cfg := config{
		prefix: defaultPrefix,
	}
	fs.StringVar(&cfg.dir, "dir", "", "Directory containing .ogg files")
	fs.StringVar(&cfg.prefix, "prefix", cfg.prefix, "Voice label prefix")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cfg.files = append(cfg.files, fs.Args()...)
	if err := cfg.validate(); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func (cfg config) validate() error {
	hasDir := strings.TrimSpace(cfg.dir) != ""
	hasFiles := len(cfg.files) > 0

	switch {
	case !hasDir && !hasFiles:
		return errors.New("either -dir or one or more input files is required")
	case hasDir && hasFiles:
		return errors.New("use either -dir or explicit input files, not both")
	}

	for _, path := range cfg.files {
		if strings.TrimSpace(path) == "" {
			return errors.New("input file path is empty")
		}
	}
	return nil
}

func run(cfg config, w io.Writer) error {
	paths, err := collectInputPaths(cfg)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		if cfg.dir != "" {
			return fmt.Errorf("no .ogg files found in %s", cfg.dir)
		}
		return errors.New("no input files provided")
	}
	progressf("found %d input files\n", len(paths))

	runner, err := newDetectorRunner(cfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = runner.Close()
	}()

	result, err := runner.AnalyzeFiles(paths)
	if err != nil {
		return err
	}
	return renderAnalysis(w, result)
}

func newDetectorRunner(cfg config) (*detectorRunner, error) {
	detector, err := newDetectorFn(cfg)
	if err != nil {
		return nil, fmt.Errorf("create voiceprint detector: %w", err)
	}
	return &detectorRunner{detector: detector}, nil
}

func (r *detectorRunner) AnalyzeFiles(paths []string) (analysis, error) {
	samples, err := loadSamples(paths)
	if err != nil {
		return analysis{}, err
	}

	dynamic, err := r.runPass(samples, true)
	if err != nil {
		return analysis{}, err
	}

	return analysis{
		Dynamic: dynamic,
	}, nil
}

func (r *detectorRunner) runPass(samples []audioSample, update bool) ([]fileResult, error) {
	mode := "detect"
	if update {
		mode = "detect-and-update"
	}

	results := make([]fileResult, 0, len(samples))
	for i, sample := range samples {
		progressf("[%s %d/%d] %s\n", mode, i+1, len(samples), sample.name)

		var (
			result voiceprint.DetectResult
			err    error
		)
		if update {
			result, err = r.detector.DetectAndUpdate(pcm.L16Mono16K, bytes.NewReader(sample.audio), nil)
		} else {
			result, err = r.detector.Detect(pcm.L16Mono16K, bytes.NewReader(sample.audio), nil)
		}
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", mode, sample.name, err)
		}

		results = append(results, fileResult{
			Path:  sample.path,
			Name:  sample.name,
			Label: result.Label,
		})
		progressf("[%s %d/%d] %s => %s\n", mode, i+1, len(samples), sample.name, displayLabel(result.Label))
	}
	return results, nil
}

func (r *detectorRunner) Close() error {
	if r == nil || r.detector == nil {
		return nil
	}
	if closer, ok := r.detector.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func loadSamples(paths []string) ([]audioSample, error) {
	samples := make([]audioSample, 0, len(paths))
	for i, path := range paths {
		name := filepath.Base(path)
		progressf("[load %d/%d] %s\n", i+1, len(paths), name)

		audio, err := decodeOGGFileFn(path)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", name, err)
		}
		audio = trimSilence(audio, silenceThreshold)

		samples = append(samples, audioSample{
			path:  path,
			name:  name,
			audio: audio,
		})
	}
	return samples, nil
}

func collectInputPaths(cfg config) ([]string, error) {
	if cfg.dir != "" {
		return collectOGGFiles(cfg.dir)
	}
	out := make([]string, len(cfg.files))
	copy(out, cfg.files)
	return out, nil
}

func collectOGGFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".ogg") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func renderAnalysis(w io.Writer, result analysis) error {
	return renderGroupSection(w, "dynamic detect-and-update groups:", result.Dynamic)
}

func renderGroupSection(w io.Writer, title string, results []fileResult) error {
	if _, err := fmt.Fprintln(w, title); err != nil {
		return err
	}
	for _, group := range groupResults(results) {
		if _, err := fmt.Fprintf(w, "%s (%d)\n", group.Label, len(group.Files)); err != nil {
			return err
		}
		for _, name := range group.Files {
			if _, err := fmt.Fprintf(w, "  - %s\n", name); err != nil {
				return err
			}
		}
	}
	return nil
}

func groupResults(results []fileResult) []fileGroup {
	indexByLabel := make(map[string]int, len(results))
	groups := make([]fileGroup, 0, len(results))

	for _, result := range results {
		label := displayLabel(result.Label)
		idx, ok := indexByLabel[label]
		if !ok {
			idx = len(groups)
			indexByLabel[label] = idx
			groups = append(groups, fileGroup{Label: label})
		}
		groups[idx].Files = append(groups[idx].Files, result.Name)
	}
	return groups
}

func displayLabel(label string) string {
	if label == "" {
		return "(unmatched)"
	}
	return label
}

func progressf(format string, args ...any) {
	if progressWriter == nil {
		return
	}
	_, _ = fmt.Fprintf(progressWriter, format, args...)
}

func decodeOGGFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	var pcmBytes bytes.Buffer
	if _, err := codecconv.OggToPCM(&pcmBytes, f, opus.SampleRate16K); err != nil {
		return nil, err
	}
	return pcmBytes.Bytes(), nil
}

func trimSilence(audio []byte, threshold int16) []byte {
	const frameSamples = 320
	const frameBytes = frameSamples * 2

	numFrames := len(audio) / frameBytes
	if numFrames < 3 {
		return audio
	}

	rms := func(start int) float64 {
		var sum float64
		for i := range frameSamples {
			offset := start + i*2
			if offset+1 >= len(audio) {
				break
			}
			sample := int16(audio[offset]) | int16(audio[offset+1])<<8
			sum += float64(sample) * float64(sample)
		}
		return math.Sqrt(sum / frameSamples)
	}

	thresh := float64(threshold)
	first := 0
	for frame := range numFrames {
		if rms(frame*frameBytes) > thresh {
			first = frame
			break
		}
	}
	last := numFrames - 1
	for frame := numFrames - 1; frame >= first; frame-- {
		if rms(frame*frameBytes) > thresh {
			last = frame
			break
		}
	}

	if first > 0 {
		first--
	}
	if last < numFrames-1 {
		last++
	}

	startByte := first * frameBytes
	endByte := min((last+1)*frameBytes, len(audio))
	return audio[startByte:endByte]
}
