package flowcraft

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/flowcraft/memory/recall"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestStorePurgeScopeRemovesOnlyThePartition(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	backend := newWorkspaceBackend(t)
	store := newTestStore(t, Config{
		TemporalStore: backend.TemporalStore(), EvidenceStore: backend.EvidenceStore(),
		SideEffectOutbox: backend.SideEffectOutbox(),
	})
	purged := Scope{AppID: "workspace-a"}
	kept := []Scope{{AppID: "workspace-a", UserID: "user"}, {AppID: "workspace-b"}}
	for index, scope := range append([]Scope{purged}, kept...) {
		if _, err := store.Observe(ctx, Observation{
			Scope: scope, ID: "observation-" + string(rune('a'+index)),
			Facts: []memorystore.FactCandidate{{Text: "GIZCLAWPURGE remembered fact"}},
		}); err != nil {
			t.Fatalf("Observe(%+v) error = %v", scope, err)
		}
	}
	bound, err := memorystore.BindApp(store, purged.AppID)
	if err != nil {
		t.Fatal(err)
	}
	if empty, err := memorystore.ScopeEmpty(ctx, bound, Scope{}); err != nil || empty {
		t.Fatalf("ScopeEmpty() before purge = %v, %v; want false", empty, err)
	}
	for range 2 {
		if err := memorystore.PurgeScope(ctx, bound, Scope{}); err != nil {
			t.Fatalf("PurgeScope() error = %v", err)
		}
	}
	if empty, err := memorystore.ScopeEmpty(ctx, bound, Scope{}); err != nil || !empty {
		t.Fatalf("ScopeEmpty() after purge = %v, %v; want true", empty, err)
	}
	result, err := store.Recall(ctx, Query{Scope: purged, Text: "GIZCLAWPURGE", Limit: 10})
	if err != nil || len(result.Matches) != 0 {
		t.Fatalf("Recall(purged) = %+v, %v; want no matches", result, err)
	}
	for _, scope := range kept {
		result, err := store.Recall(ctx, Query{Scope: scope, Text: "GIZCLAWPURGE", Limit: 10})
		if err != nil || len(result.Matches) != 1 {
			t.Fatalf("Recall(%+v) = %+v, %v; want the kept fact", scope, result, err)
		}
		if empty, err := store.ScopeEmpty(ctx, scope); err != nil || empty {
			t.Fatalf("ScopeEmpty(%+v) = %v, %v; want false", scope, empty, err)
		}
	}
}

func TestMaintenanceStorePurgesPendingExtraction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	backend := newWorkspaceBackend(t)
	durable := Config{
		TemporalStore: backend.TemporalStore(), EvidenceStore: backend.EvidenceStore(),
		AsyncQueue: backend.AsyncSemanticQueue(), SideEffectOutbox: backend.SideEffectOutbox(),
	}
	runtime := durable
	runtime.Loader = &testFlowcraftLoader{model: testLLM{response: `{"facts":[{"text":"Alice prefers tea.","kind":"preference"}]}`}}
	runtime.Extraction = ExtractionConfig{Model: "extract"}
	writer := newTestStore(t, runtime)
	scope := Scope{AppID: "workspace-a", UserID: "conversation-a"}
	observed, err := writer.Observe(ctx, Observation{Scope: scope, ID: "pending", Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "Alice prefers tea."}}})
	if err != nil || observed.Operation == nil || observed.Operation.Status != OperationPending {
		t.Fatalf("Observe() = %+v, %v; want a pending operation", observed, err)
	}
	native, err := nativeScope(scope)
	if err != nil {
		t.Fatal(err)
	}
	if stats, err := backend.AsyncSemanticQueue().Stats(ctx, recall.AsyncSemanticStatsFilter{Scope: native}); err != nil || stats.Pending == 0 {
		t.Fatalf("queue Stats() before purge = %+v, %v; want a pending job", stats, err)
	}

	maintenance, err := NewMaintenance(ctx, durable)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = maintenance.Close() })
	if err := maintenance.PurgeScope(ctx, scope); err != nil {
		t.Fatalf("PurgeScope() error = %v", err)
	}
	if empty, err := maintenance.ScopeEmpty(ctx, scope); err != nil || !empty {
		t.Fatalf("ScopeEmpty() = %v, %v; want true", empty, err)
	}
	stats, err := backend.AsyncSemanticQueue().Stats(ctx, recall.AsyncSemanticStatsFilter{Scope: native})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Pending != 0 || stats.Leased != 0 || stats.Failed != 0 || stats.DeadLetter != 0 || stats.Completed != 0 {
		t.Fatalf("queue Stats() after purge = %+v; want no retained job", stats)
	}
}

func TestStorePurgeScopeRejectsUnsupportedScopes(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, Config{})
	for _, scope := range []Scope{
		{AppID: "workspace", AgentID: "assistant"},
		{AppID: "workspace", RunID: "run"},
	} {
		if err := store.PurgeScope(context.Background(), scope); !errors.Is(err, memorystore.ErrUnsupported) {
			t.Fatalf("PurgeScope(%+v) error = %v; want ErrUnsupported", scope, err)
		}
		if _, err := store.ScopeEmpty(context.Background(), scope); !errors.Is(err, memorystore.ErrUnsupported) {
			t.Fatalf("ScopeEmpty(%+v) error = %v; want ErrUnsupported", scope, err)
		}
	}
	if err := store.PurgeScope(context.Background(), Scope{}); !errors.Is(err, memorystore.ErrInvalidInput) {
		t.Fatalf("PurgeScope(empty) error = %v; want ErrInvalidInput", err)
	}
}

func TestNewMaintenanceLoadsNoModelsAndRejectsWrites(t *testing.T) {
	t.Parallel()
	if _, err := NewMaintenance(context.Background(), Config{Embedding: EmbeddingConfig{Model: "embedding"}}); !errors.Is(err, memorystore.ErrInvalidInput) {
		t.Fatalf("NewMaintenance(models) error = %v; want ErrInvalidInput", err)
	}
	store, err := NewMaintenance(context.Background(), Config{AsyncQueue: recall.NewInMemoryAsyncSemanticQueue()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.Observe(context.Background(), Observation{Scope: Scope{AppID: "workspace"}, Text: "remember"}); !errors.Is(err, memorystore.ErrUnsupported) {
		t.Fatalf("Observe() error = %v; want ErrUnsupported", err)
	}
	if _, err := store.ProcessAsync(context.Background(), memorystore.OperationRequest{Scope: Scope{AppID: "workspace"}, ID: "operation"}); !errors.Is(err, memorystore.ErrUnsupported) {
		t.Fatalf("ProcessAsync() error = %v; want ErrUnsupported", err)
	}
}
