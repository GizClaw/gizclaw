package pendingdeletion

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	storemetrics "github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

type scanTestSource struct {
	Source
	name    string
	pages   [][]Reference
	mu      sync.Mutex
	calls   int
	cursors []string
}

func (s *scanTestSource) Name() string  { return s.name }
func (s *scanTestSource) Kinds() []Kind { return []Kind{KindPeer} }
func (s *scanTestSource) ScanDue(_ context.Context, _ time.Time, _ int, cursor string) ([]Reference, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursors = append(s.cursors, cursor)
	page := s.calls
	s.calls++
	if page >= len(s.pages) {
		return nil, "", nil
	}
	next := ""
	if page+1 < len(s.pages) {
		next = s.pages[page][len(s.pages[page])-1].DeletionID
	}
	return s.pages[page], next, nil
}

func TestProcessorScanReadsSourcesRoundRobin(t *testing.T) {
	first := &scanTestSource{name: "first", pages: [][]Reference{
		{{Source: "first", DeletionID: "first-1"}},
		{{Source: "first", DeletionID: "first-2"}},
	}}
	second := &scanTestSource{name: "second", pages: [][]Reference{
		{{Source: "second", DeletionID: "second-1"}},
	}}
	registry := NewRegistry()
	for _, source := range []*scanTestSource{first, second} {
		if err := registry.Register(source, &registryTestHandler{kind: KindPeer}); err != nil {
			t.Fatal(err)
		}
	}
	processor := &Processor{registry: registry, config: Config{PageSize: 1}, now: time.Now}
	dispatch := make(chan dispatchItem, 3)
	processor.scan(context.Background(), dispatch)
	if got := len(dispatch); got != 2 {
		t.Fatalf("first scan dispatch count = %d, want 2", got)
	}
	processor.scan(context.Background(), dispatch)
	got := []string{(<-dispatch).ref.DeletionID, (<-dispatch).ref.DeletionID, (<-dispatch).ref.DeletionID}
	want := []string{"first-1", "second-1", "first-2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dispatch order = %v, want %v", got, want)
		}
	}
	if len(first.cursors) != 2 || first.cursors[0] != "" || first.cursors[1] != "first-1" {
		t.Fatalf("first source cursors = %v, want [empty first-1]", first.cursors)
	}
}

type finalizingKVHandler struct {
	kind   Kind
	source KVSource
	calls  atomic.Int64
}

type notifyingKVSource struct {
	KVSource
	once    sync.Once
	scanned chan struct{}
}

func (s *notifyingKVSource) ScanDue(ctx context.Context, now time.Time, limit int, cursor string) ([]Reference, string, error) {
	refs, next, err := s.KVSource.ScanDue(ctx, now, limit, cursor)
	s.once.Do(func() { close(s.scanned) })
	return refs, next, err
}

func (*notifyingKVSource) ActiveStats(context.Context, time.Time) (int64, time.Time, error) {
	return 2, time.Now().Add(-time.Minute), nil
}

func (h *finalizingKVHandler) Kind() Kind { return h.kind }
func (h *finalizingKVHandler) Handle(ctx context.Context, claim Claim) error {
	marker, err := Get(ctx, h.source.Store, claim.Record.DeletionID)
	if err != nil {
		return err
	}
	fingerprint, err := Fingerprint(marker)
	if err != nil || fingerprint != claim.MarkerFingerprint {
		return ErrConflict
	}
	raw, err := h.source.Store.Get(ctx, kvTaskKey(claim.Record.DeletionID))
	if err != nil {
		return err
	}
	current, _, err := h.source.decodeTask(claim.Record, raw)
	if err != nil {
		return err
	}
	if current.MarkerFingerprint != claim.MarkerFingerprint || current.Status != StatusRunning ||
		current.LeaseToken != claim.LeaseToken || !current.LeaseDeadline.After(time.Now().UTC()) {
		return ErrConflict
	}
	matched, err := kv.CompareAndMutate(ctx, h.source.Store, kvTaskKey(claim.Record.DeletionID), raw, nil, []kv.Key{
		kvTaskKey(claim.Record.DeletionID), byIDKey(claim.Record.DeletionID), byLocatorKey(claim.Record.Kind, claim.Record.ResourceID),
	})
	if err != nil {
		return err
	}
	if !matched {
		return ErrConflict
	}
	h.calls.Add(1)
	return nil
}

func TestTwoProcessorsCannotFinalizeOneKVTaskTwice(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	record, err := New(KindPeer, "peer-one", nil, ReasonPeerDelete, struct{}{}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateOrGet(ctx, store, record); err != nil {
		t.Fatal(err)
	}
	source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	handler := &finalizingKVHandler{kind: KindPeer, source: source}
	registry := NewRegistry()
	if err := registry.Register(source, handler); err != nil {
		t.Fatal(err)
	}
	config := Config{
		ScanInterval: 5 * time.Millisecond, PageSize: 1, DispatchCapacity: 2, Workers: 1,
		LeaseDuration: time.Second, AttemptTimeout: 500 * time.Millisecond,
		RetryInitial: time.Millisecond, RetryMax: time.Second, MaxAttempts: 3,
	}
	first, err := NewProcessor(registry, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewProcessor(registry, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	first.Start(ctx)
	second.Start(ctx)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err := source.GetTask(ctx, record.DeletionID)
		if errors.Is(err, ErrNotFound) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("task was not finalized")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := handler.calls.Load(); got != 1 {
		t.Fatalf("handler finalizations = %d, want 1", got)
	}
}

func TestProcessorWakeRunsCommittedTaskAndEmitsBoundedMetrics(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	baseSource := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	source := &notifyingKVSource{KVSource: baseSource, scanned: make(chan struct{})}
	handler := &finalizingKVHandler{kind: KindPeer, source: baseSource}
	registry := NewRegistry()
	if err := registry.Register(source, handler); err != nil {
		t.Fatal(err)
	}
	metricStore := storemetrics.NewMemoryStore()
	processor, err := NewProcessor(registry, Config{
		ScanInterval: time.Hour, PageSize: 1, DispatchCapacity: 1, Workers: 1,
		LeaseDuration: time.Second, AttemptTimeout: 500 * time.Millisecond,
		RetryInitial: time.Millisecond, RetryMax: time.Second, MaxAttempts: 3,
	}, metricStore)
	if err != nil {
		t.Fatal(err)
	}
	processor.Start(ctx)
	t.Cleanup(processor.Close)
	select {
	case <-source.scanned:
	case <-time.After(time.Second):
		t.Fatal("initial scan did not run")
	}
	record, err := New(KindPeer, "peer-wake", nil, ReasonPeerDelete, struct{}{}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateOrGet(ctx, store, record); err != nil {
		t.Fatal(err)
	}
	processor.Wake()
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err := baseSource.GetTask(ctx, record.DeletionID)
		if errors.Is(err, ErrNotFound) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("woken task was not finalized")
		}
		time.Sleep(5 * time.Millisecond)
	}
	for _, name := range []string{
		"gizclaw_pending_deletion_active_depth",
		"gizclaw_pending_deletion_oldest_age_seconds",
		"gizclaw_pending_deletion_events_total",
		"gizclaw_pending_deletion_active_workers",
		"gizclaw_pending_deletion_phase_latency_seconds",
	} {
		for {
			series, err := metricStore.Latest(ctx, storemetrics.LatestQuery{
				Selector: storemetrics.Selector{Name: name}, At: time.Now().Add(time.Second), Lookback: time.Minute,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(series) > 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("metric %q was not emitted", name)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

type panicHandler struct{}

func (*panicHandler) Kind() Kind { return KindPeer }
func (*panicHandler) Handle(context.Context, Claim) error {
	panic("sensitive panic payload")
}

type timeoutHandler struct{}

func (*timeoutHandler) Kind() Kind { return KindPeer }
func (*timeoutHandler) Handle(ctx context.Context, _ Claim) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestProcessorPersistsAttemptTimeoutAsRetryableFailure(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	record, err := New(KindPeer, "peer-timeout", nil, ReasonPeerDelete, struct{}{}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateOrGet(ctx, store, record); err != nil {
		t.Fatal(err)
	}
	source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	registry := NewRegistry()
	if err := registry.Register(source, &timeoutHandler{}); err != nil {
		t.Fatal(err)
	}
	processor, err := NewProcessor(registry, Config{
		ScanInterval: time.Second, PageSize: 1, DispatchCapacity: 1, Workers: 1,
		LeaseDuration: 200 * time.Millisecond, AttemptTimeout: 20 * time.Millisecond,
		RetryInitial: time.Second, RetryMax: time.Second, MaxAttempts: 3,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	processor.Start(ctx)
	t.Cleanup(processor.Close)
	deadline := time.Now().Add(time.Second)
	for {
		task, err := source.GetTask(ctx, record.DeletionID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status == StatusRetryWait {
			if task.FailureCount != 1 || task.LastErrorCode != "attempt_timeout" {
				t.Fatalf("timeout task = %#v", task)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout task remained in status %q", task.Status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestProcessorContainsHandlerPanicAsSafeTerminalFailure(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	record, err := New(KindPeer, "peer-panic", nil, ReasonPeerDelete, struct{}{}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateOrGet(ctx, store, record); err != nil {
		t.Fatal(err)
	}
	source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	registry := NewRegistry()
	if err := registry.Register(source, &panicHandler{}); err != nil {
		t.Fatal(err)
	}
	processor, err := NewProcessor(registry, Config{
		ScanInterval: 5 * time.Millisecond, PageSize: 1, DispatchCapacity: 1, Workers: 1,
		LeaseDuration: time.Second, AttemptTimeout: 500 * time.Millisecond,
		RetryInitial: time.Millisecond, RetryMax: time.Second, MaxAttempts: 3,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	processor.Start(ctx)
	t.Cleanup(processor.Close)
	deadline := time.Now().Add(3 * time.Second)
	for {
		task, err := source.GetTask(ctx, record.DeletionID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status == StatusFailed {
			if task.LastErrorCode != "handler_panic" || task.LastErrorMessage != "cleanup handler panicked" {
				t.Fatalf("safe failure = %q, %q", task.LastErrorCode, task.LastErrorMessage)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("panic task did not fail")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
