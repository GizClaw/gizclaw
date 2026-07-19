package memory

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestScopedFlowcraftStoreIsolatesFacts(t *testing.T) {
	base, err := OpenFlowcraftStore(t.Context(), FlowcraftConfig{RuntimeID: "app", UserID: "user"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Close() })
	first := Scoped(base, "workspace-a")
	second := Scoped(base, "workspace-b")
	if _, err := first.Observe(t.Context(), Observation{Text: "the brass key opens the observatory"}); err != nil {
		t.Fatal(err)
	}
	firstResult, err := first.Recall(t.Context(), Query{Text: "brass key", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstResult.Matches) != 1 {
		t.Fatalf("first scope matches = %d, want 1", len(firstResult.Matches))
	}
	secondResult, err := second.Recall(t.Context(), Query{Text: "brass key", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondResult.Matches) != 0 {
		t.Fatalf("second scope matches = %d, want 0", len(secondResult.Matches))
	}
}

func TestScopedFlowcraftStoreWaitKeepsAsyncStatusInScope(t *testing.T) {
	loader := &testFlowcraftLoader{model: testLLM{response: `{"facts":[{"text":"Alice prefers tea.","kind":"preference","subject":"Alice","predicate":"prefers","object":"tea","entities":["Alice","tea"],"evidence_refs":[{"id":"turn","text":"I prefer tea."}]}]}`}}
	config := FlowcraftConfig{
		Dir: t.TempDir(), RuntimeID: "app", UserID: "user", ExtractionModel: "extract",
		Async: FlowcraftAsyncConfig{Enabled: true, WorkerID: "test"},
	}
	base, err := OpenFlowcraftStore(t.Context(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Close() })
	first := Scoped(base, "workspace-a")
	observed, err := first.Observe(t.Context(), Observation{
		ID: "observation", Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "I prefer tea.", ObservedAt: time.Now()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Operation == nil || observed.Operation.Status != OperationPending {
		t.Fatalf("Observe() = %+v", observed)
	}
	if _, err := Scoped(base, "workspace-b").(OperationWaiter).Wait(t.Context(), observed.Operation.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second scope Wait() error = %v, want ErrNotFound", err)
	}
	waiter, ok := first.(OperationWaiter)
	if !ok {
		t.Fatal("scoped Flowcraft store does not implement OperationWaiter")
	}
	completed, err := waiter.Wait(t.Context(), observed.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Operation == nil || completed.Operation.Status != OperationSucceeded || len(completed.Facts) != 1 {
		t.Fatalf("Wait() = %+v", completed)
	}
	secondResult, err := Scoped(base, "workspace-b").Recall(t.Context(), Query{Text: "tea", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondResult.Matches) != 0 {
		t.Fatalf("second scope matches = %d, want 0", len(secondResult.Matches))
	}
	if err := base.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenFlowcraftStore(t.Context(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	rehydrated, err := Scoped(reopened, "workspace-a").(OperationWaiter).Wait(t.Context(), observed.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rehydrated.Operation == nil || rehydrated.Operation.Status != OperationSucceeded || len(rehydrated.Facts) != 1 {
		t.Fatalf("reopened Wait() = %+v", rehydrated)
	}
}

func TestScopedMem0StoreUsesOneOpaqueEntityScope(t *testing.T) {
	base, err := NewMem0Store(Mem0Config{
		Endpoint: "https://example.test", APIKey: "key", AppID: "app", UserID: "user", AgentID: "agent", RunID: "run",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	first := Scoped(base, "workspace-a").(*Mem0Store)
	second := Scoped(base, "workspace-b").(*Mem0Store)
	if first.config.UserID == second.config.UserID || first.config.UserID == base.config.UserID {
		t.Fatalf("scoped user IDs were not isolated: base=%q first=%q second=%q", base.config.UserID, first.config.UserID, second.config.UserID)
	}
	if first.config.AppID != "" || first.config.AgentID != "" || first.config.RunID != "" {
		t.Fatalf("scoped Mem0 config retained broad entity scopes: %+v", first.config)
	}
}

func TestScopedCustomStoreAddsMandatoryScope(t *testing.T) {
	base := &scopeRecordingStore{}
	store := Scoped(base, "workspace-a")
	if _, err := store.Observe(t.Context(), Observation{Text: "remember"}); err != nil {
		t.Fatal(err)
	}
	if base.observation.Context[runtimeScopeAttribute] != "workspace-a" {
		t.Fatalf("observation context = %+v", base.observation.Context)
	}
	if _, err := store.Recall(t.Context(), Query{Text: "remember", Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if len(base.query.Filters) != 1 || base.query.Filters[0].Field != runtimeScopeAttribute || base.query.Filters[0].Value != "workspace-a" {
		t.Fatalf("recall filters = %+v", base.query.Filters)
	}
	if _, err := store.Update(t.Context(), UpdateRequest{ID: "fact", Text: stringPointer("updated")}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Update() error = %v, want ErrUnsupported", err)
	}
}

type scopeRecordingStore struct {
	observation Observation
	query       Query
}

func (s *scopeRecordingStore) Observe(_ context.Context, observation Observation) (ObserveResult, error) {
	s.observation = observation
	return ObserveResult{}, nil
}

func (s *scopeRecordingStore) Recall(_ context.Context, query Query) (RecallResult, error) {
	s.query = query
	return RecallResult{}, nil
}

func (*scopeRecordingStore) Update(context.Context, UpdateRequest) (Fact, error) {
	return Fact{}, nil
}

func (*scopeRecordingStore) Delete(context.Context, DeleteRequest) error { return nil }

func stringPointer(value string) *string { return &value }
