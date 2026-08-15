package server

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	pprofprofile "github.com/google/pprof/profile"
)

func newProfilingTestStore(t *testing.T) objectstore.ObjectStore {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	store, err := objectstore.NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestProcessProfilerPublishesReadableBaseline(t *testing.T) {
	store := newProfilingTestStore(t)
	now := time.Date(2026, 8, 15, 1, 2, 3, 4, time.UTC)
	profiler, err := newProcessProfiler(store, profilingOptions{now: func() time.Time { return now }, pid: 42})
	if err != nil {
		t.Fatal(err)
	}
	if err := profiler.baseline(); err != nil {
		t.Fatal(err)
	}
	items, err := store.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("objects = %#v", items)
	}
	prefix := "runs/20260815T010203.000000004Z-pid-42/000000-baseline"
	manifest, err := profiler.readManifest(prefix + "/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || len(manifest.Profiles) != 3 {
		t.Fatalf("manifest = %#v", manifest)
	}
	for _, file := range manifest.Profiles {
		reader, err := store.Get(prefix + "/" + file.Name)
		if err != nil {
			t.Fatal(err)
		}
		parsed, parseErr := pprofprofile.Parse(reader)
		closeErr := reader.Close()
		if parseErr != nil || closeErr != nil || len(parsed.SampleType) == 0 {
			t.Fatalf("parse %s = %v, close = %v", file.Name, parseErr, closeErr)
		}
	}
}

func TestProcessProfilerCleansFailedSet(t *testing.T) {
	store := newProfilingTestStore(t)
	now := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	profiler, err := newProcessProfiler(store, profilingOptions{
		now: func() time.Time { return now }, pid: 7,
		capture: func(kind string, writer io.Writer) error {
			if kind == "allocs" {
				return errors.New("capture failed")
			}
			_, err := io.WriteString(writer, kind)
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := profiler.baseline(); err == nil {
		t.Fatal("baseline succeeded")
	}
	items, err := store.List("")
	if err != nil || len(items) != 0 {
		t.Fatalf("objects after failure = %#v, %v", items, err)
	}
}

type failingProfilingDeleteStore struct {
	objectstore.ObjectStore
	failures int
}

func (s *failingProfilingDeleteStore) DeletePrefix(prefix string) error {
	if s.failures > 0 {
		s.failures--
		return errors.New("delete failed")
	}
	return s.ObjectStore.DeletePrefix(prefix)
}

func TestProcessProfilerRetriesFailedCleanupBeforeNextCapture(t *testing.T) {
	base := newProfilingTestStore(t)
	store := &failingProfilingDeleteStore{ObjectStore: base, failures: 1}
	now := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	failCapture := true
	profiler, err := newProcessProfiler(store, profilingOptions{
		now: func() time.Time { return now }, pid: 8, maxBytes: 1024,
		capture: func(kind string, writer io.Writer) error {
			if failCapture && kind == "allocs" {
				return errors.New("capture failed")
			}
			_, err := io.WriteString(writer, kind)
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := profiler.baseline(); err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("first capture error = %v", err)
	}
	failCapture = false
	if err := profiler.captureSet(1, now.Add(time.Second), false); err != nil {
		t.Fatal(err)
	}
	items, err := base.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("objects after retry = %#v", items)
	}
	for _, item := range items {
		if strings.Contains(item.Name, "000000-baseline") {
			t.Fatalf("failed partial set survived retry: %q", item.Name)
		}
	}
}

func TestProcessProfilerRotatesOldestSets(t *testing.T) {
	store := newProfilingTestStore(t)
	start := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	profiler, err := newProcessProfiler(store, profilingOptions{
		now: func() time.Time { return start }, pid: 9, maxSets: 2, maxBytes: 1024,
		capture: func(kind string, writer io.Writer) error {
			_, err := io.WriteString(writer, kind)
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := profiler.captureSet(0, start, true); err != nil {
		t.Fatal(err)
	}
	if err := profiler.captureSet(1, start.Add(time.Second), false); err != nil {
		t.Fatal(err)
	}
	if err := profiler.captureSet(2, start.Add(2*time.Second), false); err != nil {
		t.Fatal(err)
	}
	sets, err := profiler.loadCompletedSets("")
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 2 {
		t.Fatalf("sets = %#v", sets)
	}
	for _, set := range sets {
		if strings.Contains(set.prefix, "000000-baseline") {
			t.Fatalf("oldest baseline retained: %#v", sets)
		}
	}
}

func TestProcessProfilerRejectsManifestPathMismatch(t *testing.T) {
	store := newProfilingTestStore(t)
	now := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	profiler, err := newProcessProfiler(store, profilingOptions{
		now: func() time.Time { return now }, pid: 12, maxBytes: 1024,
		capture: func(kind string, writer io.Writer) error {
			_, err := io.WriteString(writer, kind)
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := profiler.baseline(); err != nil {
		t.Fatal(err)
	}
	prefix := "runs/20260815T010203.000000000Z-pid-12/000000-baseline"
	manifest, err := profiler.readManifest(prefix + "/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Sequence = 1
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(prefix+"/manifest.json", strings.NewReader(string(data))); err != nil {
		t.Fatal(err)
	}
	if _, err := profiler.loadCompletedSets(""); err == nil || !strings.Contains(err.Error(), "invalid profiling manifest") {
		t.Fatalf("loadCompletedSets() error = %v", err)
	}
}

func TestProcessProfilerRejectsSameSizeDigestMismatch(t *testing.T) {
	store := newProfilingTestStore(t)
	now := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	profiler, err := newProcessProfiler(store, profilingOptions{
		now: func() time.Time { return now }, pid: 14, maxBytes: 1024,
		capture: func(kind string, writer io.Writer) error {
			_, err := io.WriteString(writer, kind)
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := profiler.baseline(); err != nil {
		t.Fatal(err)
	}
	name := "runs/20260815T010203.000000000Z-pid-14/000000-baseline/heap.pprof"
	if err := store.Put(name, strings.NewReader("xxxx")); err != nil {
		t.Fatal(err)
	}
	if _, err := profiler.loadCompletedSets(""); err == nil || !strings.Contains(err.Error(), "does not match manifest") {
		t.Fatalf("loadCompletedSets() error = %v", err)
	}
}

func TestProcessProfilerRejectsRunCollisionWithIncompleteSet(t *testing.T) {
	store := newProfilingTestStore(t)
	now := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	name := "runs/20260815T010203.000000000Z-pid-13/000000-baseline/heap.pprof"
	if err := store.Put(name, strings.NewReader("partial")); err != nil {
		t.Fatal(err)
	}
	if _, err := newProcessProfiler(store, profilingOptions{now: func() time.Time { return now }, pid: 13}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("newProcessProfiler() error = %v", err)
	}
	if _, err := store.Get(name); err != nil {
		t.Fatalf("collision evidence was deleted: %v", err)
	}
}

func TestProcessProfilerEnforcesSharedByteLimit(t *testing.T) {
	store := newProfilingTestStore(t)
	now := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	profiler, err := newProcessProfiler(store, profilingOptions{
		now: func() time.Time { return now }, pid: 10, maxBytes: 5,
		capture: func(_ string, writer io.Writer) error {
			_, err := io.WriteString(writer, "1234")
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := profiler.baseline(); !errors.Is(err, errProfileLimit) {
		t.Fatalf("baseline error = %v", err)
	}
	items, err := store.List("")
	if err != nil || len(items) != 0 {
		t.Fatalf("objects after limit = %#v, %v", items, err)
	}
}

func TestProcessProfilerWorkerStopsWithoutFinalSnapshot(t *testing.T) {
	store := newProfilingTestStore(t)
	var captures atomic.Int64
	now := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	profiler, err := newProcessProfiler(store, profilingOptions{
		now: func() time.Time { return now.Add(time.Duration(captures.Load()) * time.Second) },
		pid: 11, interval: 5 * time.Millisecond, maxBytes: 1024,
		capture: func(kind string, writer io.Writer) error {
			captures.Add(1)
			_, err := io.WriteString(writer, kind)
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	profiler.start(t.Context())
	deadline := time.Now().Add(time.Second)
	for captures.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	profiler.stop()
	got := captures.Load()
	time.Sleep(15 * time.Millisecond)
	if captures.Load() != got {
		t.Fatalf("captures continued after stop: %d -> %d", got, captures.Load())
	}
}
