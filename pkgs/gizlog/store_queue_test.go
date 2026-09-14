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
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type gatedLogStore struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	records []logstore.Record
	batches []int
	closes  int
}

func (s *gatedLogStore) Append(ctx context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
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

func TestStoreQueueBackpressurePreservesAcceptedRecords(t *testing.T) {
	store := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{})}
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
	if err := appendRecord(t.Context(), "1"); err != nil {
		t.Fatal(err)
	}
	<-store.entered
	for _, id := range []string{"2", "3"} {
		if err := appendRecord(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := appendRecord(ctx, "rejected"); !errors.Is(err, context.Canceled) {
		t.Fatalf("full queue error=%v", err)
	}
	unblock()
	if err := q.close(); err != nil {
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
	if !slices.Equal(store.batches, []int{1, 2}) {
		t.Fatalf("batches=%v", store.batches)
	}
	if !slices.Equal(ids, []string{"1", "2", "3"}) {
		t.Fatalf("accepted records=%v", ids)
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
			} else if !errors.Is(err, errStoreQueueClosed) {
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
	if err := <-closed; err != nil {
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
	if store.records[0].Attributes["value"] != "original" {
		t.Fatal("queued record retained mutable attributes")
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
