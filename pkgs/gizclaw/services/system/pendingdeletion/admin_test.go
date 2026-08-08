package pendingdeletion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type invalidRetrySource struct{ KVSource }

func (invalidRetrySource) Retry(context.Context, string, time.Time) (Task, error) {
	return Task{}, nil
}

type invalidReadSource struct{ KVSource }

func (invalidReadSource) GetTask(context.Context, string) (Task, error) {
	return Task{}, nil
}

func (invalidReadSource) ListTasks(context.Context, SourceListOptions) ([]Task, error) {
	return []Task{{}}, nil
}

func TestAdminAggregatesFilterBoundPagesAndRetriesFailedTask(t *testing.T) {
	ctx := context.Background()
	registry := NewRegistry()
	sources := []KVSource{
		{Store: kv.NewMemory(nil), SourceName: "source_a", OwnedKinds: []Kind{KindPeer}},
		{Store: kv.NewMemory(nil), SourceName: "source_b", OwnedKinds: []Kind{KindPeer}},
	}
	var records []Record
	for i, source := range sources {
		if err := registry.Register(source, &registryTestHandler{kind: KindPeer}); err != nil {
			t.Fatalf("Register(%s) error = %v", source.Name(), err)
		}
		record, err := New(KindPeer, source.Name(), nil, ReasonPeerDelete, map[string]string{"public_key": source.Name()}, time.Unix(int64(i+1), 0))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := CreateOrGet(ctx, source.Store, record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}

	wakeCount := 0
	admin := NewAdmin(registry, func() { wakeCount++ })
	first, err := admin.List(ctx, ListRequest{Limit: 1})
	if err != nil || len(first.Tasks) != 1 || first.NextCursor == "" {
		t.Fatalf("List(first) = %#v, %v", first, err)
	}
	second, err := admin.List(ctx, ListRequest{Limit: 2, Cursor: first.NextCursor})
	if err != nil || len(second.Tasks) != 1 || second.Tasks[0].Record.DeletionID != records[1].DeletionID {
		t.Fatalf("List(second) = %#v, %v", second, err)
	}
	if _, err := admin.List(ctx, ListRequest{Limit: 1, Status: StatusQueued, Cursor: first.NextCursor}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("List(cursor mismatch) error = %v, want ErrInvalid", err)
	}
	if got, err := admin.Get(ctx, sources[0].Name(), records[0].DeletionID); err != nil || got.Source != sources[0].Name() {
		t.Fatalf("Get() = %#v, %v", got, err)
	}
	if _, err := admin.Get(ctx, "unknown", records[0].DeletionID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Get(unknown source) error = %v, want ErrInvalid", err)
	}
	if _, err := admin.Retry(ctx, sources[0].Name(), records[0].DeletionID); !errors.Is(err, ErrConflict) {
		t.Fatalf("Retry(queued) error = %v, want ErrConflict", err)
	}

	refs, _, err := sources[0].ScanDue(ctx, time.Unix(10, 0), 1, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ScanDue() = %#v, %v", refs, err)
	}
	claim, claimed, err := sources[0].Claim(ctx, refs[0], time.Unix(10, 0), time.Minute)
	if err != nil || !claimed {
		t.Fatalf("Claim() = %#v, %v, %v", claim, claimed, err)
	}
	if err := sources[0].Fail(ctx, claim, "terminal", "safe failure", true, time.Unix(10, 0), time.Unix(10, 0), 3); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	retried, err := admin.Retry(ctx, sources[0].Name(), records[0].DeletionID)
	if err != nil {
		t.Fatalf("Retry(failed) error = %v", err)
	}
	if retried.Status != StatusQueued || retried.FailureCount != 0 || retried.LastErrorMessage != "" {
		t.Fatalf("Retry(failed) task = %#v", retried)
	}
	if wakeCount != 1 {
		t.Fatalf("retry wakes = %d, want 1", wakeCount)
	}
	task, err := admin.Get(ctx, sources[0].Name(), records[0].DeletionID)
	if err != nil || task.Status != StatusQueued || task.FailureCount != 0 || task.LastErrorMessage != "" {
		t.Fatalf("Get(after retry) = %#v, %v", task, err)
	}
}

func TestAdminEmptyRegistry(t *testing.T) {
	result, err := NewAdmin(NewRegistry(), nil).List(context.Background(), ListRequest{})
	if err != nil || len(result.Tasks) != 0 || result.NextCursor != "" {
		t.Fatalf("List() = %#v, %v", result, err)
	}
}

func TestAdminRejectsInvalidRetriedTaskWithoutWaking(t *testing.T) {
	registry := NewRegistry()
	source := invalidRetrySource{KVSource: KVSource{
		Store: kv.NewMemory(nil), SourceName: "invalid_retry", OwnedKinds: []Kind{KindPeer},
	}}
	if err := registry.Register(source, &registryTestHandler{kind: KindPeer}); err != nil {
		t.Fatal(err)
	}
	wakes := 0
	_, err := NewAdmin(registry, func() { wakes++ }).Retry(t.Context(), source.Name(), "10000000-0000-4000-8000-000000000001")
	if err == nil || errors.Is(err, ErrInvalid) {
		t.Fatalf("Retry() error = %v, want internal source error", err)
	}
	if wakes != 0 {
		t.Fatalf("retry wakes = %d, want 0", wakes)
	}
}

func TestAdminTreatsInvalidPersistedTasksAsInternalErrors(t *testing.T) {
	registry := NewRegistry()
	source := invalidReadSource{KVSource: KVSource{
		Store: kv.NewMemory(nil), SourceName: "invalid_read", OwnedKinds: []Kind{KindPeer},
	}}
	if err := registry.Register(source, &registryTestHandler{kind: KindPeer}); err != nil {
		t.Fatal(err)
	}
	admin := NewAdmin(registry, nil)
	if _, err := admin.List(t.Context(), ListRequest{Source: source.Name()}); err == nil || errors.Is(err, ErrInvalid) {
		t.Fatalf("List() error = %v, want internal source error", err)
	}
	if _, err := admin.Get(t.Context(), source.Name(), "10000000-0000-4000-8000-000000000001"); err == nil || errors.Is(err, ErrInvalid) {
		t.Fatalf("Get() error = %v, want internal source error", err)
	}
}
