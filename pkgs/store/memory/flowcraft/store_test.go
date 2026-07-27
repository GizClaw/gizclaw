package flowcraft

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/flowcraft/memory/recall"
	flowworkspace "github.com/GizClaw/flowcraft/memory/recall/store/workspace"
	"github.com/GizClaw/flowcraft/sdk/errdefs"
	"github.com/GizClaw/flowcraft/sdk/workspace"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

var testScope = Scope{AppID: "app-a", UserID: "conversation-a", AgentID: "assistant"}

type recallMemoryWithHits struct {
	recall.Memory
	hits  []recall.Hit
	query recall.Query
}

func (m *recallMemoryWithHits) Recall(_ context.Context, _ recall.Scope, query recall.Query) ([]recall.Hit, error) {
	m.query = query
	return m.hits, nil
}

func TestExtractedLaneRecognizesOnlyConfiguredExactPrefix(t *testing.T) {
	t.Parallel()
	lanes := []string{"story-facts", "story-questions"}
	if got := extractedLane("story-facts: Monkey found the cave.", lanes); got != "story-facts" {
		t.Fatalf("extractedLane() = %q", got)
	}
	for _, content := range []string{"story-fact: wrong", "prefix story-facts: wrong", "story-facts wrong"} {
		if got := extractedLane(content, lanes); got != "" {
			t.Fatalf("extractedLane(%q) = %q", content, got)
		}
	}
}

func TestStoreScopesAreIsolated(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, Config{})
	for _, observation := range []Observation{
		{Scope: Scope{AppID: "app-a", UserID: "conversation-a"}, Text: "Alice prefers tea."},
		{Scope: Scope{AppID: "app-a", UserID: "conversation-b"}, Text: "Bob prefers coffee."},
	} {
		if _, err := store.Observe(context.Background(), observation); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		scope Scope
		want  string
		not   string
	}{
		{Scope{AppID: "app-a", UserID: "conversation-a"}, "tea", "coffee"},
		{Scope{AppID: "app-a", UserID: "conversation-b"}, "coffee", "tea"},
	} {
		result, err := store.Recall(context.Background(), Query{Scope: test.scope, Text: "preference", Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Matches) != 1 || !strings.Contains(result.Matches[0].Fact.Text, test.want) || strings.Contains(result.Matches[0].Fact.Text, test.not) {
			t.Fatalf("scope %q result = %+v", test.scope, result)
		}
		if _, leaked := result.Matches[0].Fact.Attributes["scope"]; leaked {
			t.Fatalf("scope leaked into fact attributes: %+v", result.Matches[0].Fact.Attributes)
		}
	}
}

func TestStorePersistsStructuredFactCandidatesWithoutExtraction(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, Config{})
	result, err := store.Observe(context.Background(), Observation{
		Scope: testScope,
		Facts: []memorystore.FactCandidate{{
			Text: "story_progress: progress: current_beat=origin",
			Attributes: map[string]any{
				"kind": "state", "subject": "story_progress", "predicate": "progress",
				"object": "origin", "entities": []string{"wukong", "origin"}, "lane": "story_progress",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Facts) != 1 {
		t.Fatalf("Observe() facts = %d, want 1", len(result.Facts))
	}
	fact := result.Facts[0]
	if fact.Text != "story_progress: progress: current_beat=origin" || fact.Attributes["kind"] != "state" || fact.Attributes["subject"] != "story_progress" || fact.Attributes["predicate"] != "progress" || fact.Attributes["object"] != "origin" || fact.Attributes["lane"] != "story_progress" {
		t.Fatalf("Observe() fact = %#v", fact)
	}
	if entities, ok := fact.Attributes["entities"].([]string); !ok || !slices.Contains(entities, "wukong") || !slices.Contains(entities, "origin") {
		t.Fatalf("Observe() entities = %#v, want wukong and origin", fact.Attributes["entities"])
	}
}

func TestStorePersistsStructuredFactCandidatesWithExtractionConfigured(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, Config{
		Loader:     &testFlowcraftLoader{model: testLLM{response: `{"facts":[]}`}},
		Extraction: ExtractionConfig{Model: "extract"},
	})
	const text = "The assistant remembered GIZCLAWMEMORY123."
	result, err := store.Observe(context.Background(), Observation{
		Scope: testScope,
		Facts: []memorystore.FactCandidate{{Text: text}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Facts) != 1 || result.Facts[0].Text != text {
		t.Fatalf("Observe() facts = %#v, want direct fact %q", result.Facts, text)
	}
	recallResult, err := store.Recall(context.Background(), Query{
		Scope: testScope,
		Text:  "GIZCLAWMEMORY123",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recallResult.Matches) != 1 || recallResult.Matches[0].Fact.Text != text {
		t.Fatalf("Recall() matches = %#v, want direct fact %q", recallResult.Matches, text)
	}
}

func TestStoreRecallReturnsStableDescendingScores(t *testing.T) {
	t.Parallel()
	base := newTestStore(t, Config{})
	memory := &recallMemoryWithHits{
		Memory: base.memory,
		hits: []recall.Hit{
			{Fact: recall.TemporalFact{ID: "first", Content: "first"}, Score: 0.2},
			{Fact: recall.TemporalFact{ID: "second", Content: "second"}, Score: 0.8},
			{Fact: recall.TemporalFact{ID: "third", Content: "third"}, Score: 0.8},
			{Fact: recall.TemporalFact{ID: "fourth", Content: "fourth"}, Score: 0.1},
		},
	}
	store := newStore(Config{}, memory, base.temporal, nil)
	result, err := store.Recall(context.Background(), Query{Scope: testScope, Text: "query", Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	if memory.query.Limit != 4 {
		t.Fatalf("native query limit = %d, want 4", memory.query.Limit)
	}
	want := []string{"second", "third", "first", "fourth"}
	if len(result.Matches) != len(want) {
		t.Fatalf("Recall() returned %d matches, want %d", len(result.Matches), len(want))
	}
	for i, text := range want {
		if result.Matches[i].Fact.Text != text {
			t.Fatalf("Recall().Matches[%d].Fact.Text = %q, want %q", i, result.Matches[i].Fact.Text, text)
		}
	}
}

func TestStoreUpdateDeleteUseOpaqueFactLocator(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, Config{})
	observed, err := store.Observe(context.Background(), Observation{Scope: testScope, Text: "Alice prefers tea."})
	if err != nil || len(observed.Facts) != 1 {
		t.Fatalf("Observe() = %+v, %v", observed, err)
	}
	fact := observed.Facts[0]
	updatedText := "Alice prefers coffee."
	updated, err := store.Update(context.Background(), UpdateRequest{Scope: testScope, ID: fact.ID, ExpectedRevision: fact.Revision, Text: &updatedText})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != fact.ID || updated.Text != updatedText || updated.Revision == fact.Revision {
		t.Fatalf("Update() = %+v, original = %+v", updated, fact)
	}
	if err := store.Delete(context.Background(), DeleteRequest{Scope: testScope, ID: updated.ID, ExpectedRevision: updated.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(context.Background(), UpdateRequest{Scope: testScope, ID: "native-id", Text: &updatedText}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("raw native id error = %v", err)
	}
}

func TestStoreRejectsCrossScopeFactAndOperationLocators(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, Config{})
	scopeA := Scope{AppID: "app-a", UserID: "user", AgentID: "agent"}
	scopeB := Scope{AppID: "app-b", UserID: "user", AgentID: "agent"}
	observed, err := store.Observe(context.Background(), Observation{Scope: scopeA, Text: "private"})
	if err != nil || len(observed.Facts) != 1 {
		t.Fatalf("Observe() = %+v, %v", observed, err)
	}
	text := "changed"
	if _, err := store.Update(context.Background(), UpdateRequest{Scope: scopeB, ID: observed.Facts[0].ID, Text: &text}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-scope Update() error = %v, want ErrInvalidInput", err)
	}
	if err := store.Delete(context.Background(), DeleteRequest{Scope: scopeB, ID: observed.Facts[0].ID}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-scope Delete() error = %v, want ErrInvalidInput", err)
	}

	nativeA, err := nativeScope(scopeA)
	if err != nil {
		t.Fatal(err)
	}
	operationID := encodeLocator(nativeA, "operation")
	if _, err := store.Wait(context.Background(), memorystore.OperationRequest{Scope: scopeB, ID: operationID}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-scope Wait() error = %v, want ErrInvalidInput", err)
	}
}

func TestLocatorRoundTripsCompleteScopeAndRejectsLegacyV1(t *testing.T) {
	t.Parallel()
	scope := recall.Scope{RuntimeID: "app", UserID: "user", AgentID: "agent"}
	locator := encodeLocator(scope, "native")
	gotScope, gotID, err := decodeLocator(locator)
	if err != nil || !reflect.DeepEqual(gotScope, scope) || gotID != "native" {
		t.Fatalf("decodeLocator() = %#v, %q, %v", gotScope, gotID, err)
	}
	if _, _, err := decodeLocator("flowcraft:v1:dXNlcg:bmF0aXZl"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("legacy locator error = %v, want ErrInvalidInput", err)
	}
}

func TestStoreAsyncWaitAndRestart(t *testing.T) {
	t.Parallel()
	backend := newWorkspaceBackend(t)
	loader := &testFlowcraftLoader{model: testLLM{response: `{"facts":[{"text":"Alice prefers tea.","kind":"preference","subject":"Alice","predicate":"prefers","object":"tea","entities":["Alice","tea"],"evidence_refs":[{"id":"turn","text":"Alice prefers tea."}]}]}`}}
	config := Config{
		Loader: loader, Extraction: ExtractionConfig{Model: "extract"},
		TemporalStore: backend.TemporalStore(), EvidenceStore: backend.EvidenceStore(),
		AsyncQueue: backend.AsyncSemanticQueue(), SideEffectOutbox: backend.SideEffectOutbox(),
	}
	store := newTestStore(t, config)
	observed, err := store.Observe(context.Background(), Observation{Scope: testScope, ID: "obs", Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "Alice prefers tea."}}})
	if err != nil || observed.Operation == nil || observed.Operation.Status != OperationPending {
		t.Fatalf("Observe() = %+v, %v", observed, err)
	}
	completed, err := store.Wait(context.Background(), memorystore.OperationRequest{Scope: testScope, ID: observed.Operation.ID})
	if err != nil || completed.Operation == nil || completed.Operation.Status != OperationSucceeded || len(completed.Facts) != 1 {
		t.Fatalf("Wait() = %+v, %v", completed, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	rehydrated, err := reopened.Wait(context.Background(), memorystore.OperationRequest{Scope: testScope, ID: observed.Operation.ID})
	if err != nil || rehydrated.Operation == nil || rehydrated.Operation.Status != OperationSucceeded || len(rehydrated.Facts) != 1 {
		t.Fatalf("reopened Wait() = %+v, %v", rehydrated, err)
	}
}

type temporalWithoutScopeEnumerator struct{ recall.TemporalStore }

func TestStoreWaitRehydratesDecodedScopeWithoutEnumerator(t *testing.T) {
	t.Parallel()
	backend := newWorkspaceBackend(t)
	loader := &testFlowcraftLoader{model: testLLM{response: `{"facts":[{"text":"Alice prefers tea.","kind":"preference"}]}`}}
	config := Config{
		Loader: loader, Extraction: ExtractionConfig{Model: "extract"},
		TemporalStore: temporalWithoutScopeEnumerator{TemporalStore: backend.TemporalStore()},
		EvidenceStore: backend.EvidenceStore(), AsyncQueue: backend.AsyncSemanticQueue(), SideEffectOutbox: backend.SideEffectOutbox(),
	}
	store, err := New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := store.Observe(context.Background(), Observation{Scope: testScope, Text: "Alice prefers tea."})
	if err != nil || observed.Operation == nil || observed.Operation.Status != OperationPending {
		t.Fatalf("Observe() = %+v, %v", observed, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	result, err := reopened.Wait(context.Background(), memorystore.OperationRequest{Scope: testScope, ID: observed.Operation.ID})
	if err != nil || result.Operation == nil || result.Operation.Status != OperationSucceeded || len(result.Facts) != 1 {
		t.Fatalf("Wait() = %+v, %v", result, err)
	}
}

func TestStoreAsyncOperationsAreIsolatedAcrossScopes(t *testing.T) {
	t.Parallel()
	loader := &testFlowcraftLoader{model: testLLM{response: `{"facts":[{"text":"remembered","kind":"note"}]}`}}
	store := newTestStore(t, Config{
		Loader: loader, Extraction: ExtractionConfig{Model: "extract"},
		AsyncQueue: recall.NewInMemoryAsyncSemanticQueue(),
	})
	operations := make(map[Scope]string)
	for _, scope := range []Scope{
		{AppID: "app-a", UserID: "conversation-a"},
		{AppID: "app-b", UserID: "conversation-b"},
	} {
		result, err := store.Observe(context.Background(), Observation{Scope: scope, Text: "remember separately"})
		if err != nil || result.Operation == nil || result.Operation.Status != OperationPending {
			t.Fatalf("Observe(%q) = %+v, %v", scope, result, err)
		}
		operations[scope] = result.Operation.ID
	}
	if operations[Scope{AppID: "app-a", UserID: "conversation-a"}] == operations[Scope{AppID: "app-b", UserID: "conversation-b"}] {
		t.Fatal("operations across scopes share an opaque locator")
	}

	var wg sync.WaitGroup
	errorsByScope := make(chan error, len(operations))
	for scope, operationID := range operations {
		wg.Add(1)
		go func(scope Scope, operationID string) {
			defer wg.Done()
			result, err := store.Wait(context.Background(), memorystore.OperationRequest{Scope: scope, ID: operationID})
			if err != nil {
				errorsByScope <- fmt.Errorf("Wait(%q): %w", scope, err)
				return
			}
			if result.Operation == nil || result.Operation.Status != OperationSucceeded || len(result.Facts) != 1 {
				errorsByScope <- fmt.Errorf("Wait(%q) = %+v", scope, result)
				return
			}
			factScope, _, err := decodeLocator(result.Facts[0].ID)
			if err != nil || factScope.RuntimeID != scope.AppID || factScope.UserID != scope.UserID || factScope.AgentID != scope.AgentID {
				errorsByScope <- fmt.Errorf("Wait(%q) fact locator scope = %+v, %v", scope, factScope, err)
			}
		}(scope, operationID)
	}
	wg.Wait()
	close(errorsByScope)
	for err := range errorsByScope {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestStoreValidatesScopeAndProviderOwnedMetadata(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, Config{})
	if _, err := store.Observe(context.Background(), Observation{Text: "missing scope"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing scope error = %v", err)
	}
	if _, err := store.Observe(context.Background(), Observation{Scope: testScope, Text: "x", Context: map[string]any{"kind": "note"}}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("reserved metadata error = %v", err)
	}
	if _, err := store.Recall(context.Background(), Query{Scope: testScope, Text: "x", Limit: 1, Filters: []Filter{{Field: "unknown", Operator: FilterEqual, Value: "x"}}}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported filter error = %v", err)
	}
}

func TestStoreWaitHonorsCancellation(t *testing.T) {
	t.Parallel()
	store := newTestStore(t, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	native, err := nativeScope(testScope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Wait(ctx, memorystore.OperationRequest{Scope: testScope, ID: encodeLocator(native, "operation")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestMapFlowcraftError(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		input error
		want  error
	}{
		{errdefs.Validation(errors.New("bad")), ErrInvalidInput},
		{errdefs.NotFound(errors.New("missing")), ErrNotFound},
		{errdefs.Conflict(errors.New("conflict")), ErrConflict},
		{errdefs.NotAvailable(errors.New("down")), ErrUnavailable},
		{context.DeadlineExceeded, context.DeadlineExceeded},
	} {
		if err := mapFlowcraftError("test", test.input); !errors.Is(err, test.want) {
			t.Fatalf("mapFlowcraftError(%v) = %v, want %v", test.input, err, test.want)
		}
	}
}

func newTestStore(t *testing.T, config Config) *Store {
	t.Helper()
	store, err := New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newWorkspaceBackend(t *testing.T) *flowworkspace.Backend {
	t.Helper()
	ws, err := workspace.NewLocalWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backend, err := flowworkspace.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	return backend
}

func TestConfigHasNoSerializationTags(t *testing.T) {
	t.Parallel()
	configType := reflect.TypeFor[Config]()
	for field := range configType.Fields() {
		if field.Tag.Get("yaml") != "" || field.Tag.Get("json") != "" {
			t.Fatalf("Config.%s contains serialization tags", field.Name)
		}
	}
}
