package gizlog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type gatedLogStore struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	active  atomic.Int32
	maximum atomic.Int32
	calls   chan struct{}
	mu      sync.Mutex
	records []logstore.Record
	batches []int
	closes  int
}

func (s *gatedLogStore) Append(ctx context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	active := s.active.Add(1)
	defer s.active.Add(-1)
	for old := s.maximum.Load(); old < active; old = s.maximum.Load() {
		if s.maximum.CompareAndSwap(old, active) {
			break
		}
	}
	if s.calls != nil {
		select {
		case s.calls <- struct{}{}:
		default:
		}
	}
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, records...)
	s.batches = append(s.batches, len(records))
	keys := make([]logstore.RecordKey, len(records))
	for i, record := range records {
		keys[i] = record.Key()
	}
	return keys, nil
}
func (*gatedLogStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}
func (s *gatedLogStore) Close() error { s.mu.Lock(); defer s.mu.Unlock(); s.closes++; return nil }

type gatedLogResolver struct{ store *gatedLogStore }

func (r gatedLogResolver) Log(string) (logstore.ImmutableStore, error) { return r.store, nil }

func TestStoreSinkDoesNotBlockFirstResponseOnNetwork(t *testing.T) {
	store := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	unblock := func() { release.Do(func() { close(store.release) }) }
	defer unblock()
	logger, cleanup, err := NewLogger(Config{Sinks: []SinkConfig{{Kind: SinkStore, Store: "logs"}}}, gatedLogResolver{store})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unblock(); _ = cleanup() }()
	returned := make(chan struct{})
	go func() { logger.Info("first response"); close(returned) }()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("store was not called")
	}
	select {
	case <-returned:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("first response waited for log storage")
	}
	unblock()
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.records) != 1 || store.records[0].Message != "first response" {
		t.Fatalf("records=%v", store.records)
	}
}

func TestStoreQueueRejectsOverloadAndPreservesAcceptedRecords(t *testing.T) {
	store := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{}), calls: make(chan struct{}, storeQueueWorkers)}
	q := newStoreQueue(slog.NewTextHandler(io.Discard, nil))
	q.jobs = make(chan storeJob, 2)
	go q.run()
	var release sync.Once
	unblock := func() { release.Do(func() { close(store.release) }) }
	defer func() { unblock(); _ = q.close() }()
	appender := queuedStoreAppender{queue: q, store: store, name: "logs"}
	appendRecord := func(ctx context.Context, id string) error {
		_, err := appender.Append(ctx, []logstore.Record{{ID: id, Attributes: map[string]string{"captured": id}}})
		return err
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var accepted []string
	for i := range storeQueueWorkers {
		id := fmt.Sprint(i)
		if err := appendRecord(ctx, id); err != nil {
			t.Fatal(err)
		}
		accepted = append(accepted, id)
		select {
		case <-store.calls:
		case <-time.After(time.Second):
			t.Fatal("worker did not enter Store")
		}
	}
	for i := storeQueueWorkers; ; i++ {
		id := fmt.Sprint(i)
		err := appendRecord(ctx, id)
		if errors.Is(err, ErrStoreQueueFull) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		accepted = append(accepted, id)
	}
	unblock()
	if err := q.close(); !errors.Is(err, ErrStoreQueueFull) {
		t.Fatal(err)
	}
	if err := appendRecord(t.Context(), "closed"); !errors.Is(err, errStoreQueueClosed) {
		t.Fatalf("closed queue error=%v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	var ids []string
	for _, record := range store.records {
		ids = append(ids, record.ID)
	}
	slices.Sort(ids)
	slices.Sort(accepted)
	if !slices.Equal(ids, accepted) {
		t.Fatalf("accepted records=%v", ids)
	}
}

func TestStoreQueueRunsFourWorkersAndJoinsThemOnClose(t *testing.T) {
	store := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{}), calls: make(chan struct{}, storeQueueWorkers)}
	q := newStoreQueue(slog.NewTextHandler(io.Discard, nil))
	go q.run()
	var release sync.Once
	unblock := func() { release.Do(func() { close(store.release) }) }
	defer func() { unblock(); _ = q.close() }()
	for i := range storeQueueWorkers {
		if err := q.enqueue(t.Context(), storeJob{store: store, name: "logs", records: []logstore.Record{{ID: fmt.Sprint(i)}}}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-store.calls:
		case <-time.After(time.Second):
			t.Fatal("workers did not write concurrently")
		}
	}
	if err := q.enqueue(t.Context(), storeJob{store: store, name: "logs", records: []logstore.Record{{ID: "queued"}}}); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- q.close() }()
	<-q.stopping
	select {
	case err := <-closed:
		t.Fatalf("close returned with workers still blocked: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if active, maximum := store.active.Load(), store.maximum.Load(); active != 0 || maximum != 4 {
		t.Fatalf("active=%d maximum=%d; want 0 and 4", active, maximum)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	var ids []string
	for _, record := range store.records {
		ids = append(ids, record.ID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"0", "1", "2", "3", "queued"}) {
		t.Fatalf("drained records=%v", ids)
	}
}

func TestStoreQueueFreeWorkersProgressWhileAnotherStoreBlocks(t *testing.T) {
	slow := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{})}
	fast := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{})}
	close(fast.release)
	q := newStoreQueue(slog.NewTextHandler(io.Discard, nil))
	go q.run()
	defer func() { close(slow.release); _ = q.close() }()
	if err := q.enqueue(t.Context(), storeJob{store: slow, name: "slow", records: []logstore.Record{{ID: "slow"}}}); err != nil {
		t.Fatal(err)
	}
	<-slow.entered
	if err := q.enqueue(t.Context(), storeJob{store: fast, name: "fast", records: []logstore.Record{{ID: "fast"}}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fast.entered:
	case <-time.After(time.Second):
		t.Fatal("available workers waited for the blocked Store")
	}
}

func TestStoreQueueBatchesAdjacentRecordsUpTo64(t *testing.T) {
	store := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{})}
	close(store.release)
	q := newStoreQueue(slog.NewTextHandler(io.Discard, nil))
	for i := range 140 {
		if err := q.enqueue(t.Context(), storeJob{store: store, name: "logs", records: []logstore.Record{{ID: fmt.Sprint(i)}}}); err != nil {
			t.Fatal(err)
		}
	}
	go q.run()
	if err := q.close(); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	slices.Sort(store.batches)
	if !slices.Equal(store.batches, []int{12, 64, 64}) || len(store.records) != 140 {
		t.Fatalf("batches=%v records=%d", store.batches, len(store.records))
	}
}

type gatedReportHandler struct {
	slog.Handler
	entered chan struct{}
	release chan struct{}
}

func (h *gatedReportHandler) Handle(ctx context.Context, record slog.Record) error {
	close(h.entered)
	<-h.release
	return h.Handler.Handle(ctx, record)
}

func TestStoreQueueOverloadCoalescesWithoutBlockingOnFallback(t *testing.T) {
	var output bytes.Buffer
	fallback := &gatedReportHandler{Handler: slog.NewTextHandler(&output, nil), entered: make(chan struct{}), release: make(chan struct{})}
	q := newStoreQueue(fallback)
	q.jobs = make(chan storeJob, 1)
	job := storeJob{name: "logs", records: []logstore.Record{{ID: "accepted"}}}
	if err := q.enqueue(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	for range 1000 {
		if err := q.enqueue(t.Context(), job); !errors.Is(err, ErrStoreQueueFull) {
			t.Fatalf("overload: %v", err)
		}
	}
	// Block a real rejection summary, then prove producers still finish.
	done := make(chan struct{})
	go func() { q.reportRejections(); close(done) }()
	<-fallback.entered
	var release sync.Once
	unblock := func() { release.Do(func() { close(fallback.release) }) }
	defer func() { unblock(); <-done; q.cancel() }()
	returned := make(chan error, 1)
	go func() { returned <- q.enqueue(t.Context(), job) }()
	select {
	case err := <-returned:
		if !errors.Is(err, ErrStoreQueueFull) {
			t.Fatalf("overload: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("producer waited for stderr reporting")
	}
	unblock()
	<-done
	if !strings.Contains(output.String(), "records=1000") || !strings.Contains(output.String(), "reason=full") {
		t.Fatalf("missing aggregate rejection count: %s", output.String())
	}
}

func TestStoreQueueShutdownCancelsBlockedWriteAndReportsFailure(t *testing.T) {
	store := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{})}
	var fallback bytes.Buffer
	q := newStoreQueue(slog.NewTextHandler(&fallback, nil))
	q.timeout = 50 * time.Millisecond
	go q.run()
	if err := q.enqueue(t.Context(), storeJob{store: store, name: "logs", records: []logstore.Record{{ID: "blocked"}}}); err != nil {
		t.Fatal(err)
	}
	<-store.entered
	if err := q.close(); !errors.Is(err, errStoreQueueWrite) {
		t.Fatalf("close error=%v", err)
	}
	if !strings.Contains(fallback.String(), "system log store sink failed") || !strings.Contains(fallback.String(), "store=logs") {
		t.Fatalf("missing failure: %s", fallback.String())
	}
}

func TestStoreQueueConcurrentCloseKeepsEveryAcceptedRecord(t *testing.T) {
	store := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{})}
	q := newStoreQueue(slog.NewTextHandler(io.Discard, nil))
	q.jobs = make(chan storeJob, 2)
	go q.run()
	var release sync.Once
	unblock := func() { release.Do(func() { close(store.release) }) }
	defer func() { unblock(); _ = q.close() }()
	appender := queuedStoreAppender{queue: q, store: store, name: "logs"}
	attrs := map[string]string{"value": "original"}
	if _, err := appender.Append(t.Context(), []logstore.Record{{ID: "first", Attributes: attrs}}); err != nil {
		t.Fatal(err)
	}
	<-store.entered
	attrs["value"] = "mutated"
	results := make(chan string, 32)
	var writers sync.WaitGroup
	for i := range 32 {
		writers.Go(func() {
			id := fmt.Sprint(i)
			_, err := appender.Append(t.Context(), []logstore.Record{{ID: id}})
			if err == nil {
				results <- id
			} else if !errors.Is(err, errStoreQueueClosed) && !errors.Is(err, ErrStoreQueueFull) {
				t.Errorf("append: %v", err)
			}
		})
	}
	closed := make(chan error, 1)
	go func() { closed <- q.close() }()
	<-q.stopping
	unblock()
	writers.Wait()
	close(results)
	if err := <-closed; err != nil && !errors.Is(err, ErrStoreQueueFull) {
		t.Fatal(err)
	}
	want := []string{"first"}
	for id := range results {
		want = append(want, id)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	var got []string
	for _, record := range store.records {
		got = append(got, record.ID)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("persisted=%v accepted=%v", got, want)
	}
	for _, record := range store.records {
		if record.ID == "first" && record.Attributes["value"] != "original" {
			t.Fatal("queued record retained mutable attributes")
		}
	}
}

type partialLogAppender struct {
	calls      [][]string
	deadlines  []time.Time
	fail       bool
	firstError bool
	block      bool
}

func (s *partialLogAppender) Append(ctx context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	ids := make([]string, len(records))
	for i, record := range records {
		ids[i] = record.ID
	}
	s.calls = append(s.calls, ids)
	deadline, _ := ctx.Deadline()
	s.deadlines = append(s.deadlines, deadline)
	if len(s.calls) == 1 {
		if s.firstError {
			return []logstore.RecordKey{records[0].Key()}, errors.New("private provider payload")
		}
		return []logstore.RecordKey{records[0].Key()}, nil
	}
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.fail {
		return nil, errors.New("private provider payload")
	}
	return []logstore.RecordKey{records[0].Key()}, nil
}

func TestStoreQueuePartialAppend(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		fail, block, firstError bool
	}{
		{name: "success"}, {name: "partial error recovery", firstError: true}, {name: "error", fail: true}, {name: "shutdown", block: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &partialLogAppender{fail: tc.fail, block: tc.block, firstError: tc.firstError}
			var fallback bytes.Buffer
			q := newStoreQueue(slog.NewTextHandler(&fallback, nil))
			q.timeout = 50 * time.Millisecond
			if err := q.enqueue(t.Context(), storeJob{store: store, name: "logs", records: []logstore.Record{{ID: "1"}, {ID: "2"}, {ID: "3"}}}); err != nil {
				t.Fatal(err)
			}
			go q.run()
			err := q.close()
			if tc.fail || tc.block {
				if !errors.Is(err, errStoreQueueWrite) {
					t.Fatalf("close=%v", err)
				}
				if !strings.Contains(fallback.String(), "records=2") || strings.Contains(fallback.String(), "private") {
					t.Fatalf("failure=%s", fallback.String())
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(store.calls) < 2 || !slices.Equal(store.calls[0], []string{"1", "2", "3"}) || !slices.Equal(store.calls[1], []string{"2", "3"}) {
				t.Fatalf("calls=%v", store.calls)
			}
			for i := 1; i < len(store.calls); i++ {
				if store.calls[i][0] == "1" {
					t.Fatalf("replayed prefix: %v", store.calls)
				}
				if !store.deadlines[i].Equal(store.deadlines[0]) {
					t.Fatal("retry renewed deadline")
				}
			}
			if !tc.fail && !tc.block && (len(store.calls) != 3 || !slices.Equal(store.calls[2], []string{"3"})) {
				t.Fatalf("calls=%v", store.calls)
			}
		})
	}
}

func TestIndependentLoggerCleanup(t *testing.T) {
	store := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{})}
	close(store.release)
	cfg := Config{Sinks: []SinkConfig{{Kind: SinkStore, Store: "first"}, {Kind: SinkStore, Store: "second"}}}
	logger1, close1, err := NewLogger(cfg, gatedLogResolver{store})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = close1() }()
	logger2, close2, err := NewLogger(cfg, gatedLogResolver{store})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = close2() }()
	logger1.Info("one")
	var workers sync.WaitGroup
	workers.Go(func() {
		if err := close1(); err != nil {
			t.Error(err)
		}
	})
	workers.Go(func() { logger2.Info("two") })
	workers.Wait()
	logger2.Info("after first cleanup")
	if err := close2(); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closes != 0 {
		t.Fatalf("logger closed registry Store %d times", store.closes)
	}
	counts := map[string]int{}
	for _, record := range store.records {
		counts[record.Message]++
	}
	if counts["one"] != 2 || counts["two"] != 2 || counts["after first cleanup"] != 2 || len(store.records) != 6 {
		t.Fatalf("records=%v", store.records)
	}
}
