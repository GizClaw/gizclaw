package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/flowcraft/memory/recall"
	"github.com/GizClaw/flowcraft/sdk/errdefs"
	"github.com/GizClaw/flowcraft/sdk/llm"
)

func TestFlowcraftStoreLifecycleAndPersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	config := FlowcraftConfig{Dir: t.TempDir(), RuntimeID: "app", AgentID: "agent", UserID: "user"}
	store, err := OpenFlowcraftStore(ctx, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := store.Observe(ctx, Observation{
		ID: "observation", Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "The brass key opens the observatory."}}, Context: map[string]any{"lane": "clues"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.Facts) != 1 {
		t.Fatalf("Observe() facts = %+v", observed.Facts)
	}
	fact := observed.Facts[0]
	if fact.Text != "The brass key opens the observatory." || fact.Attributes["lane"] != "clues" {
		t.Fatalf("Observe() fact = %+v", fact)
	}
	result, err := store.Recall(ctx, Query{Text: "observatory key", Limit: 5, Filters: []Filter{{Field: "kind", Operator: FilterEqual, Value: "note"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) == 0 || result.Matches[0].Fact.ID != fact.ID {
		t.Fatalf("Recall() = %+v, want fact %q", result, fact.ID)
	}
	updatedText := "The brass key opens the northern observatory."
	updated, err := store.Update(ctx, UpdateRequest{ID: fact.ID, ExpectedRevision: fact.Revision, Text: &updatedText, Attributes: AttributePatch{Set: map[string]any{"verified": true}}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != fact.ID || updated.Revision == fact.Revision || updated.Text != updatedText || updated.Attributes["verified"] != true || !updated.CreatedAt.Equal(fact.CreatedAt) {
		t.Fatalf("Update() = %+v", updated)
	}
	if _, err := store.Update(ctx, UpdateRequest{ID: updated.ID, ExpectedRevision: fact.Revision, Text: &updatedText}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale Update() error = %v, want ErrConflict", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenFlowcraftStore(ctx, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.Delete(ctx, DeleteRequest{ID: fact.ID, ExpectedRevision: updated.Revision}); err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.Recall(ctx, Query{Text: "northern observatory", Limit: 5}); err != nil {
		t.Fatal(err)
	} else if len(got.Matches) != 0 {
		t.Fatalf("Recall() after Delete = %+v, want no matches", got)
	}
}

func TestFlowcraftDeterministicObserveIncludesTextAndTurns(t *testing.T) {
	t.Parallel()
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{RuntimeID: "app", UserID: "user"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	result, err := store.Observe(context.Background(), Observation{Text: "summary", Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "detail"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Facts) != 1 || !strings.Contains(result.Facts[0].Text, "summary") || !strings.Contains(result.Facts[0].Text, "detail") {
		t.Fatalf("Observe() = %+v", result)
	}
}

func TestFlowcraftObserveRejectsProviderOwnedAttributes(t *testing.T) {
	t.Parallel()
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{RuntimeID: "app", UserID: "user"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, key := range []string{flowcraftRootIDAttribute, "observation_id", "kind", "subject", "predicate", "object", "entities"} {
		if _, err := store.Observe(context.Background(), Observation{Text: "remember", Context: map[string]any{key: "value"}}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("Observe() attribute %q error = %v", key, err)
		}
	}
}

func TestFlowcraftFactIgnoresRootIDOnInitialFact(t *testing.T) {
	t.Parallel()
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{RuntimeID: "app", UserID: "user"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	result, err := store.memory.Save(context.Background(), store.scope, recall.SaveRequest{Facts: []recall.TemporalFact{{Kind: recall.FactNote, Content: "remember", Metadata: map[string]any{flowcraftRootIDAttribute: "spoofed"}}}})
	if err != nil {
		t.Fatal(err)
	}
	facts, err := store.loadFacts(context.Background(), result.FactIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].ID == "spoofed" || facts[0].ID != facts[0].Revision {
		t.Fatalf("facts = %+v", facts)
	}
}

func TestFlowcraftStoreAsyncWait(t *testing.T) {
	t.Parallel()
	model := testLLM{response: `{"facts":[{"text":"Alice prefers tea.","kind":"preference","subject":"Alice","predicate":"prefers","object":"tea","entities":["Alice","tea"],"evidence_refs":[{"id":"turn","text":"I prefer tea."}]}]}`}
	loader := &testFlowcraftLoader{model: model}
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{
		Dir: t.TempDir(), RuntimeID: "app", UserID: "user", ExtractionModel: "extract", Async: FlowcraftAsyncConfig{Enabled: true, WorkerID: "test"},
	}, loader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	observed, err := store.Observe(context.Background(), Observation{ID: "observation", Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "I prefer tea.", ObservedAt: time.Now()}}})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Operation == nil || observed.Operation.Status != OperationPending {
		t.Fatalf("Observe() = %+v", observed)
	}
	completed, err := store.Wait(context.Background(), observed.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Operation == nil || completed.Operation.Status != OperationSucceeded || len(completed.Facts) != 1 || completed.Facts[0].Text != "Alice prefers tea." {
		t.Fatalf("Wait() = %+v", completed)
	}
}

func TestFlowcraftStoreAsyncWaitAfterRestart(t *testing.T) {
	t.Parallel()
	model := testLLM{response: `{"facts":[{"text":"Alice prefers tea.","kind":"preference","subject":"Alice","predicate":"prefers","object":"tea","entities":["Alice","tea"],"evidence_refs":[{"id":"turn","text":"I prefer tea."}]}]}`}
	loader := &testFlowcraftLoader{model: model}
	config := FlowcraftConfig{Dir: t.TempDir(), RuntimeID: "app", UserID: "user", ExtractionModel: "extract", Async: FlowcraftAsyncConfig{Enabled: true}}
	store, err := OpenFlowcraftStore(context.Background(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Observe(context.Background(), Observation{ObservedAt: time.Now(), Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "I prefer tea."}}})
	if err != nil {
		t.Fatal(err)
	}
	observed, err := store.Observe(context.Background(), Observation{ObservedAt: time.Now().Add(-time.Hour), Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "I prefer tea."}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenFlowcraftStore(context.Background(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	completed, err := reopened.Wait(context.Background(), observed.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Operation == nil || completed.Operation.Status != OperationSucceeded || len(completed.Facts) != 0 {
		t.Fatalf("Wait() facts = %+v, operation = %+v", completed.Facts, completed.Operation)
	}
	firstCompleted, err := reopened.Wait(context.Background(), first.Operation.ID)
	if err != nil || firstCompleted.Operation == nil || firstCompleted.Operation.Status != OperationSucceeded || len(firstCompleted.Facts) != 1 {
		t.Fatalf("first Wait() = %+v, %v", firstCompleted, err)
	}
}

type failingTestLLM struct{}

func (failingTestLLM) Generate(context.Context, []llm.Message, ...llm.GenerateOption) (llm.Message, llm.TokenUsage, error) {
	return llm.Message{}, llm.TokenUsage{}, errors.New("provider unavailable")
}
func (failingTestLLM) GenerateStream(context.Context, []llm.Message, ...llm.GenerateOption) (llm.StreamMessage, error) {
	return nil, errors.New("provider unavailable")
}

func TestFlowcraftAsyncFailureIsTerminal(t *testing.T) {
	t.Parallel()
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{
		Dir: t.TempDir(), RuntimeID: "app", UserID: "user", ExtractionModel: "extract", Async: FlowcraftAsyncConfig{Enabled: true},
	}, &testFlowcraftLoader{model: failingTestLLM{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	observed, err := store.Observe(context.Background(), Observation{Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "remember"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Wait(context.Background(), observed.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation == nil || result.Operation.Status != OperationFailed || result.Operation.Error == "" {
		t.Fatalf("Wait() = %+v", result)
	}
	again, err := store.Wait(context.Background(), observed.Operation.ID)
	if err != nil || again.Operation == nil || again.Operation.Status != OperationFailed {
		t.Fatalf("second Wait() = %+v, %v", again, err)
	}
}

func TestFlowcraftUpdateRejectsProviderOwnedAttributePatches(t *testing.T) {
	t.Parallel()
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{RuntimeID: "app", UserID: "user"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	observed, err := store.Observe(context.Background(), Observation{Text: "remember"})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []UpdateRequest{
		{ID: observed.Facts[0].ID, Attributes: AttributePatch{Set: map[string]any{"kind": "preference"}}},
		{ID: observed.Facts[0].ID, Attributes: AttributePatch{Delete: []string{"entities"}}},
	} {
		if _, err := store.Update(context.Background(), request); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("Update(%+v) error = %v", request.Attributes, err)
		}
	}
}

func TestFlowcraftWaitGateHonorsCancellation(t *testing.T) {
	t.Parallel()
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{RuntimeID: "app", UserID: "user"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	<-store.waitGate
	defer func() { store.waitGate <- struct{}{} }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Wait(ctx, "operation"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestFlowcraftRecallRejectsUnsupportedFilters(t *testing.T) {
	t.Parallel()
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{RuntimeID: "app", UserID: "user"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, filter := range []Filter{
		{Field: "kind", Operator: FilterNotEqual, Value: "note"},
		{Field: "metadata.lane", Operator: FilterEqual, Value: "clues"},
	} {
		if _, err := store.Recall(context.Background(), Query{Text: "x", Limit: 1, Filters: []Filter{filter}}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("Recall(%+v) error = %v", filter, err)
		}
	}
}

func TestMapFlowcraftError(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		input error
		want  error
	}{
		"validation":  {errdefs.Validationf("bad"), ErrInvalidInput},
		"not found":   {errdefs.NotFound(errors.New("missing")), ErrNotFound},
		"conflict":    {errdefs.Conflict(errors.New("stale")), ErrConflict},
		"unavailable": {errdefs.NotAvailable(errors.New("down")), ErrUnavailable},
		"canceled":    {context.Canceled, context.Canceled},
	} {
		t.Run(name, func(t *testing.T) {
			if err := mapFlowcraftError("test", test.input); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
