package pendingdeletion

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type automaticKVHandler struct {
	finalizingKVHandler
	probe func(context.Context, Task) (bool, error)
}

func (h *automaticKVHandler) CanRecoverFailed(ctx context.Context, task Task) (bool, error) {
	return h.probe(ctx, task)
}

func seedFailedRecoveryTask(t *testing.T, source KVSource, id string, at time.Time) Task {
	t.Helper()
	record, err := New(KindPeer, id, nil, ReasonPeerDelete, struct{}{}, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateOrGet(t.Context(), source.Store, record); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := Fingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	claim, claimed, err := source.Claim(t.Context(), Reference{Source: source.Name(), DeletionID: record.DeletionID, MarkerFingerprint: fingerprint}, at.Add(time.Second), time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	claim, err = source.Checkpoint(t.Context(), claim, PhaseFinalize, at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Fail(t.Context(), claim, "legacy_projection", "safe diagnostic", true, at.Add(time.Second), at.Add(time.Second), 3); err != nil {
		t.Fatal(err)
	}
	task, err := source.GetTask(t.Context(), record.DeletionID)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func recoveryTestConfig() Config {
	return Config{ScanInterval: time.Hour, PageSize: 2, DispatchCapacity: 2, Workers: 1, LeaseDuration: 5 * time.Second, AttemptTimeout: 3 * time.Second, RetryInitial: time.Millisecond, RetryMax: time.Second, MaxAttempts: 3}
}

func awaitRecovery(t *testing.T, source KVSource, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, err := source.GetTask(t.Context(), id)
		if errors.Is(err, ErrNotFound) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("automatic recovery did not complete")
}

func TestTwoProcessorsRecoverOneFailedObservationOnce(t *testing.T) {
	source := KVSource{Store: kv.NewMemory(nil), SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	task := seedFailedRecoveryTask(t, source, "automatic-one", time.Now().Add(-time.Minute))
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	handler := &automaticKVHandler{finalizingKVHandler: finalizingKVHandler{kind: KindPeer, source: source}, probe: func(ctx context.Context, _ Task) (bool, error) {
		entered <- struct{}{}
		select {
		case <-release:
			return true, nil
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}}
	registry := NewRegistry()
	if err := registry.Register(source, handler); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		processor, err := NewProcessor(registry, recoveryTestConfig(), nil)
		if err != nil {
			t.Fatal(err)
		}
		processor.Start(t.Context())
		t.Cleanup(processor.Close)
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("competing probes did not enter")
		}
	}
	close(release)
	awaitRecovery(t, source, task.Record.DeletionID)
	if handler.calls.Load() != 1 {
		t.Fatalf("finalizations=%d", handler.calls.Load())
	}
}

type boundedRecoverySource struct {
	KVSource
	lists    atomic.Int64
	advanced atomic.Int64
	maxLimit atomic.Int64
}

func (s *boundedRecoverySource) ListTasks(ctx context.Context, options SourceListOptions) ([]Task, error) {
	s.lists.Add(1)
	if options.AfterCreatedAt != nil {
		s.advanced.Add(1)
	}
	for previous := s.maxLimit.Load(); int64(options.Limit) > previous; previous = s.maxLimit.Load() {
		if s.maxLimit.CompareAndSwap(previous, int64(options.Limit)) {
			break
		}
	}
	return s.KVSource.ListTasks(ctx, options)
}

func TestRecoveryPagesPastRetainedUnsafeFailures(t *testing.T) {
	base := KVSource{Store: kv.NewMemory(nil), SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	var retained []Task
	for i := range 8 {
		retained = append(retained, seedFailedRecoveryTask(t, base, "unsafe-"+string(rune('a'+i)), time.Unix(int64(100+i), 0)))
	}
	last := seedFailedRecoveryTask(t, base, "recover-tail", time.Unix(200, 0))
	source := &boundedRecoverySource{KVSource: base}
	handler := &automaticKVHandler{finalizingKVHandler: finalizingKVHandler{kind: KindPeer, source: base}, probe: func(_ context.Context, task Task) (bool, error) { return task.Record.ResourceID == "recover-tail", nil }}
	registry := NewRegistry()
	if err := registry.Register(source, handler); err != nil {
		t.Fatal(err)
	}
	cfg := recoveryTestConfig()
	cfg.ScanInterval = 5 * time.Millisecond
	cfg.PageSize = 2
	cfg.DispatchCapacity = 1
	processor, err := NewProcessor(registry, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	processor.Start(t.Context())
	defer processor.Close()
	awaitRecovery(t, base, last.Record.DeletionID)
	if source.maxLimit.Load() != 2 || source.advanced.Load() == 0 {
		t.Fatalf("limit=%d advanced=%d", source.maxLimit.Load(), source.advanced.Load())
	}
	for _, before := range retained {
		after, err := base.GetTask(t.Context(), before.Record.DeletionID)
		if err != nil {
			t.Fatal(err)
		}
		if !sameFailedObservation(after, before) {
			t.Fatal("rejected failure changed")
		}
	}
}

func TestRecoveryProbeCancellationPreservesFailure(t *testing.T) {
	source := KVSource{Store: kv.NewMemory(nil), SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	before := seedFailedRecoveryTask(t, source, "cancel-recovery", time.Now().Add(-time.Minute))
	entered := make(chan struct{}, 1)
	handler := &automaticKVHandler{finalizingKVHandler: finalizingKVHandler{kind: KindPeer, source: source}, probe: func(ctx context.Context, _ Task) (bool, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return false, ctx.Err()
	}}
	registry := NewRegistry()
	if err := registry.Register(source, handler); err != nil {
		t.Fatal(err)
	}
	processor, err := NewProcessor(registry, recoveryTestConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	processor.Start(t.Context())
	select {
	case <-entered:
	case <-time.After(time.Second):
		processor.Close()
		t.Fatal("probe did not start")
	}
	processor.Close()
	after, err := source.GetTask(t.Context(), before.Record.DeletionID)
	if err != nil {
		t.Fatal(err)
	}
	if !sameFailedObservation(after, before) {
		t.Fatal("canceled read-only probe changed failed state")
	}
}

func TestHandlerWithoutRecoveryPolicyLeavesFailedTasksAlone(t *testing.T) {
	base := KVSource{Store: kv.NewMemory(nil), SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	before := seedFailedRecoveryTask(t, base, "ordinary-terminal", time.Now().Add(-time.Minute))
	source := &boundedRecoverySource{KVSource: base}
	registry := NewRegistry()
	if err := registry.Register(source, &finalizingKVHandler{kind: KindPeer, source: base}); err != nil {
		t.Fatal(err)
	}
	processor, err := NewProcessor(registry, recoveryTestConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	processor.scan(t.Context(), make(chan dispatchItem, 2))
	after, err := base.GetTask(t.Context(), before.Record.DeletionID)
	if err != nil {
		t.Fatal(err)
	}
	if source.lists.Load() != 0 || !sameFailedObservation(after, before) {
		t.Fatal("ordinary terminal failure was automatically retried")
	}
}

func TestRecoveryProbePanicPreservesFailureAndWorkerProgress(t *testing.T) {
	source := KVSource{Store: kv.NewMemory(nil), SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	failed := seedFailedRecoveryTask(t, source, "panic-probe", time.Now().Add(-time.Minute))
	good := seedFailedRecoveryTask(t, source, "good-after-panic", time.Now().Add(-time.Minute))
	handler := &automaticKVHandler{finalizingKVHandler: finalizingKVHandler{kind: KindPeer, source: source}, probe: func(_ context.Context, task Task) (bool, error) {
		if task.Record.ResourceID == "panic-probe" {
			panic("probe failure")
		}
		return true, nil
	}}
	registry := NewRegistry()
	if err := registry.Register(source, handler); err != nil {
		t.Fatal(err)
	}
	processor, err := NewProcessor(registry, recoveryTestConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range []Task{failed, good} {
		observed := task
		processor.process(t.Context(), dispatchItem{source: source, ref: Reference{Source: task.Source, DeletionID: task.Record.DeletionID, MarkerFingerprint: task.MarkerFingerprint}, failed: &observed})
	}
	after, err := source.GetTask(t.Context(), failed.Record.DeletionID)
	if err != nil {
		t.Fatal(err)
	}
	if !sameFailedObservation(after, failed) {
		t.Fatal("panicked read-only probe changed failure")
	}
	if _, err := source.GetTask(t.Context(), good.Record.DeletionID); !errors.Is(err, ErrNotFound) {
		t.Fatal("worker did not progress after probe panic")
	}
}

type ordinaryRetrySource struct{ Source }

func TestRecoveryRegistrationRequiresConditionalRetry(t *testing.T) {
	base := KVSource{Store: kv.NewMemory(nil), SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	handler := &automaticKVHandler{finalizingKVHandler: finalizingKVHandler{kind: KindPeer, source: base}, probe: func(context.Context, Task) (bool, error) { return true, nil }}
	if err := NewRegistry().Register(ordinaryRetrySource{Source: base}, handler); err == nil {
		t.Fatal("automatic recovery accepted an unconditional retry source")
	}
}
