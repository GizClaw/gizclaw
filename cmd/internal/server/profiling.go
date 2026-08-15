package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

const (
	profilingInterval      = 5 * time.Minute
	profilingMaxSets       = 576
	profilingMaxBytes      = int64(1 << 30)
	profilingManifestLimit = int64(1 << 20)
)

var (
	profilingObjectPattern = regexp.MustCompile(`^runs/([0-9]{8}T[0-9]{6}\.[0-9]{9}Z-pid-[0-9]+)/([0-9]{6,}-(?:baseline|[0-9]{8}T[0-9]{6}\.[0-9]{9}Z))/(heap\.pprof|allocs\.pprof|goroutine\.pprof|manifest\.json)$`)
	errProfileLimit        = errors.New("profiling: snapshot exceeds byte limit")
)

type profilingManifest struct {
	Version    int                     `json:"version"`
	Run        string                  `json:"run"`
	Sequence   uint64                  `json:"sequence"`
	CapturedAt time.Time               `json:"captured_at"`
	Profiles   []profilingManifestFile `json:"profiles"`
}

type profilingManifestFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type profilingOptions struct {
	now      func() time.Time
	pid      int
	interval time.Duration
	maxSets  int
	maxBytes int64
	capture  func(string, io.Writer) error
}

type processProfiler struct {
	store    objectstore.ObjectStore
	run      string
	options  profilingOptions
	cancel   context.CancelFunc
	wait     sync.WaitGroup
	sequence uint64
}

func newProcessProfiler(store objectstore.ObjectStore, options profilingOptions) (*processProfiler, error) {
	if store == nil {
		return nil, errors.New("profiling: ObjectStore is nil")
	}
	if options.now == nil {
		options.now = time.Now
	}
	if options.pid == 0 {
		options.pid = os.Getpid()
	}
	if options.interval == 0 {
		options.interval = profilingInterval
	}
	if options.maxSets == 0 {
		options.maxSets = profilingMaxSets
	}
	if options.maxBytes == 0 {
		options.maxBytes = profilingMaxBytes
	}
	if options.capture == nil {
		options.capture = captureRuntimeProfile
	}
	if options.pid <= 0 || options.interval <= 0 || options.maxSets <= 0 || options.maxBytes <= 0 {
		return nil, errors.New("profiling: invalid internal limits")
	}
	now := options.now().UTC()
	run := fmt.Sprintf("%s-pid-%d", profilingTimestamp(now), options.pid)
	profiler := &processProfiler{store: store, run: run, options: options}
	runItems, err := store.List("runs/" + run)
	if err != nil {
		return nil, fmt.Errorf("profiling: check run collision: %w", err)
	}
	if len(runItems) != 0 {
		return nil, fmt.Errorf("profiling: run %q already exists", run)
	}
	items, err := profiler.loadCompletedSets("")
	if err != nil {
		return nil, fmt.Errorf("profiling: prepare retained objects: %w", err)
	}
	for _, item := range items {
		if item.run == run {
			return nil, fmt.Errorf("profiling: run %q already exists", run)
		}
	}
	return profiler, nil
}

func captureRuntimeProfile(kind string, writer io.Writer) error {
	profile := pprof.Lookup(kind)
	if profile == nil {
		return fmt.Errorf("profiling: runtime profile %q is unavailable", kind)
	}
	return profile.WriteTo(writer, 0)
}

func (p *processProfiler) baseline() error {
	return p.captureSet(0, p.options.now().UTC(), true)
}

func (p *processProfiler) start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	p.wait.Go(func() {
		for {
			timer := time.NewTimer(p.options.interval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
			p.sequence++
			capturedAt := p.options.now().UTC()
			if err := p.captureSet(p.sequence, capturedAt, false); err != nil {
				slog.Warn("server profiling snapshot failed", "operation", "capture", "path", p.setPrefix(p.sequence, capturedAt, false), "error", err)
			}
		}
	})
}

func (p *processProfiler) stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wait.Wait()
}

func (p *processProfiler) captureSet(sequence uint64, capturedAt time.Time, baseline bool) (err error) {
	prefix := p.setPrefix(sequence, capturedAt, baseline)
	remaining := p.options.maxBytes
	manifest := profilingManifest{Version: 1, Run: p.run, Sequence: sequence, CapturedAt: capturedAt}
	complete := false
	defer func() {
		if !complete {
			err = errors.Join(err, p.store.DeletePrefix(prefix))
		}
	}()
	for _, kind := range []string{"heap", "allocs", "goroutine"} {
		file, writeErr := p.uploadProfile(prefix, kind, &remaining)
		if writeErr != nil {
			return fmt.Errorf("profiling: capture %s: %w", kind, writeErr)
		}
		manifest.Profiles = append(manifest.Profiles, file)
	}
	if err := p.rotate(manifest, prefix); err != nil {
		return err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("profiling: encode manifest: %w", err)
	}
	if err := p.store.Put(prefix+"/manifest.json", strings.NewReader(string(data))); err != nil {
		return fmt.Errorf("profiling: publish manifest: %w", err)
	}
	complete = true
	return nil
}

func (p *processProfiler) uploadProfile(prefix, kind string, remaining *int64) (profilingManifestFile, error) {
	name := prefix + "/" + kind + ".pprof"
	reader, writer := io.Pipe()
	type captureResult struct {
		size int64
		sum  string
		err  error
	}
	done := make(chan captureResult, 1)
	go func() {
		hasher := sha256.New()
		limited := &profileLimitWriter{writer: io.MultiWriter(writer, hasher), remaining: remaining}
		err := p.options.capture(kind, limited)
		_ = writer.CloseWithError(err)
		done <- captureResult{size: limited.written, sum: hex.EncodeToString(hasher.Sum(nil)), err: err}
	}()
	putErr := p.store.Put(name, reader)
	_ = reader.CloseWithError(putErr)
	outcome := <-done
	if outcome.err != nil || putErr != nil {
		return profilingManifestFile{}, errors.Join(outcome.err, putErr)
	}
	return profilingManifestFile{Name: kind + ".pprof", Size: outcome.size, SHA256: outcome.sum}, nil
}

type profileLimitWriter struct {
	writer    io.Writer
	remaining *int64
	written   int64
}

func (w *profileLimitWriter) Write(data []byte) (int, error) {
	if *w.remaining <= 0 {
		return 0, errProfileLimit
	}
	exceeded := int64(len(data)) > *w.remaining
	if exceeded {
		data = data[:*w.remaining]
	}
	written, err := w.writer.Write(data)
	w.written += int64(written)
	*w.remaining -= int64(written)
	if err == nil && written < len(data) {
		err = io.ErrShortWrite
	}
	if err == nil && exceeded {
		err = errProfileLimit
	}
	return written, err
}

type completedProfileSet struct {
	run      string
	prefix   string
	captured time.Time
	bytes    int64
	manifest profilingManifest
}

func (p *processProfiler) rotate(candidate profilingManifest, preservePrefix string) error {
	candidateBytes := int64(0)
	for _, profile := range candidate.Profiles {
		candidateBytes += profile.Size
	}
	if candidateBytes > p.options.maxBytes {
		return errProfileLimit
	}
	sets, err := p.loadCompletedSets(preservePrefix)
	if err != nil {
		return fmt.Errorf("profiling: list retained sets: %w", err)
	}
	slices.SortFunc(sets, func(a, b completedProfileSet) int {
		if order := a.captured.Compare(b.captured); order != 0 {
			return order
		}
		return cmpString(a.prefix, b.prefix)
	})
	totalBytes := candidateBytes
	for _, set := range sets {
		totalBytes += set.bytes
	}
	for len(sets)+1 > p.options.maxSets || totalBytes > p.options.maxBytes {
		oldest := sets[0]
		if err := p.deleteCompletedSet(oldest); err != nil {
			return fmt.Errorf("profiling: rotate %q: %w", oldest.prefix, err)
		}
		totalBytes -= oldest.bytes
		sets = sets[1:]
	}
	return nil
}

func (p *processProfiler) loadCompletedSets(preservePrefix string) ([]completedProfileSet, error) {
	items, err := p.store.List("")
	if err != nil {
		return nil, err
	}
	type group struct {
		run, prefix string
		sequence    uint64
		captured    time.Time
		baseline    bool
		files       map[string]objectstore.ObjectInfo
	}
	groups := map[string]*group{}
	for _, item := range items {
		matches := profilingObjectPattern.FindStringSubmatch(item.Name)
		if matches == nil {
			return nil, fmt.Errorf("unrecognized profiling object %q", item.Name)
		}
		prefix := "runs/" + matches[1] + "/" + matches[2]
		entry := groups[prefix]
		if entry == nil {
			sequenceText, suffix, _ := strings.Cut(matches[2], "-")
			sequence, parseErr := strconv.ParseUint(sequenceText, 10, 64)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid profiling object %q", item.Name)
			}
			baseline := suffix == "baseline"
			captured := time.Time{}
			if baseline {
				if sequence != 0 {
					return nil, fmt.Errorf("invalid profiling baseline object %q", item.Name)
				}
			} else {
				captured, parseErr = time.Parse("20060102T150405.000000000Z", suffix)
				if parseErr != nil {
					return nil, fmt.Errorf("invalid profiling object %q", item.Name)
				}
			}
			entry = &group{run: matches[1], prefix: prefix, sequence: sequence, captured: captured, baseline: baseline, files: map[string]objectstore.ObjectInfo{}}
			groups[prefix] = entry
		}
		entry.files[matches[3]] = item
	}
	var completed []completedProfileSet
	for _, group := range groups {
		_, hasManifest := group.files["manifest.json"]
		if !hasManifest {
			if group.prefix != preservePrefix {
				if err := p.store.DeletePrefix(group.prefix); err != nil {
					return nil, fmt.Errorf("clean incomplete set %q: %w", group.prefix, err)
				}
			}
			continue
		}
		manifest, err := p.readManifest(group.prefix + "/manifest.json")
		if err != nil {
			return nil, err
		}
		set, err := validateProfilingManifest(group.run, group.prefix, group.sequence, group.captured, group.baseline, manifest, group.files)
		if err != nil {
			return nil, err
		}
		completed = append(completed, set)
	}
	return completed, nil
}

func (p *processProfiler) readManifest(name string) (profilingManifest, error) {
	reader, err := p.store.Get(name)
	if err != nil {
		return profilingManifest{}, fmt.Errorf("read manifest %q: %w", name, err)
	}
	data, err := io.ReadAll(io.LimitReader(reader, profilingManifestLimit+1))
	err = errors.Join(err, reader.Close())
	if err != nil {
		return profilingManifest{}, fmt.Errorf("read manifest %q: %w", name, err)
	}
	if int64(len(data)) > profilingManifestLimit {
		return profilingManifest{}, fmt.Errorf("manifest %q exceeds size limit", name)
	}
	var manifest profilingManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return profilingManifest{}, fmt.Errorf("decode manifest %q: %w", name, err)
	}
	return manifest, nil
}

func validateProfilingManifest(run, prefix string, sequence uint64, captured time.Time, baseline bool, manifest profilingManifest, files map[string]objectstore.ObjectInfo) (completedProfileSet, error) {
	if manifest.Version != 1 || manifest.Run != run || manifest.Sequence != sequence || manifest.CapturedAt.IsZero() || len(manifest.Profiles) != 3 {
		return completedProfileSet{}, fmt.Errorf("invalid profiling manifest %q", prefix+"/manifest.json")
	}
	if (!baseline && !manifest.CapturedAt.Equal(captured)) || (baseline && sequence != 0) {
		return completedProfileSet{}, fmt.Errorf("invalid profiling manifest %q", prefix+"/manifest.json")
	}
	want := map[string]struct{}{"heap.pprof": {}, "allocs.pprof": {}, "goroutine.pprof": {}}
	bytes := int64(0)
	for _, profile := range manifest.Profiles {
		if _, ok := want[profile.Name]; !ok || profile.Size < 0 || len(profile.SHA256) != sha256.Size*2 {
			return completedProfileSet{}, fmt.Errorf("invalid profiling manifest entry in %q", prefix+"/manifest.json")
		}
		if _, err := hex.DecodeString(profile.SHA256); err != nil {
			return completedProfileSet{}, fmt.Errorf("invalid profiling manifest digest in %q", prefix+"/manifest.json")
		}
		info, ok := files[profile.Name]
		if !ok || info.Size != profile.Size {
			return completedProfileSet{}, fmt.Errorf("profiling manifest %q does not match stored profiles", prefix+"/manifest.json")
		}
		delete(want, profile.Name)
		bytes += profile.Size
	}
	if len(want) != 0 || len(files) != 4 {
		return completedProfileSet{}, fmt.Errorf("profiling set %q has unexpected files", prefix)
	}
	return completedProfileSet{run: run, prefix: prefix, captured: manifest.CapturedAt, bytes: bytes, manifest: manifest}, nil
}

func (p *processProfiler) deleteCompletedSet(set completedProfileSet) error {
	var errs []error
	if err := p.store.Delete(set.prefix + "/manifest.json"); err != nil {
		return err
	}
	for _, profile := range set.manifest.Profiles {
		if err := p.store.Delete(set.prefix + "/" + profile.Name); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *processProfiler) setPrefix(sequence uint64, capturedAt time.Time, baseline bool) string {
	suffix := profilingTimestamp(capturedAt)
	if baseline {
		suffix = "baseline"
	}
	return fmt.Sprintf("runs/%s/%06d-%s", p.run, sequence, suffix)
}

func profilingTimestamp(value time.Time) string {
	return value.UTC().Format("20060102T150405.000000000Z")
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
