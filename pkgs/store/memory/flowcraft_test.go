package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
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
	if _, err := reopened.Update(ctx, UpdateRequest{ID: fact.ID, Text: &updatedText}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update() after Delete error = %v, want ErrNotFound", err)
	}
	if err := reopened.Delete(ctx, DeleteRequest{ID: fact.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Delete() error = %v, want ErrNotFound", err)
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

func TestFlowcraftDeterministicObserveSkipsEmptyTurnIDs(t *testing.T) {
	t.Parallel()
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{RuntimeID: "app", UserID: "user"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	result, err := store.Observe(context.Background(), Observation{Turns: []Turn{{Role: RoleUser, Text: "remember"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Facts) != 1 || len(result.Facts[0].Sources) != 0 {
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
	for _, key := range []string{flowcraftRootIDAttribute, flowcraftOperationStatusAttribute, flowcraftProvenanceMarkerAttribute, "observation_id", "kind", "subject", "predicate", "object", "entities"} {
		if _, err := store.Observe(context.Background(), Observation{Text: "remember", Context: map[string]any{key: "value"}}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("Observe() attribute %q error = %v", key, err)
		}
	}
}

func TestFlowcraftModelObserveRejectsUnsupportedContext(t *testing.T) {
	t.Parallel()
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{
		RuntimeID: "app", UserID: "user", ExtractionModel: "extract",
	}, &testFlowcraftLoader{model: testLLM{response: `{"facts":[]}`}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.Observe(context.Background(), Observation{Text: "remember", Context: map[string]any{"lane": "clues"}}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Observe() error = %v, want ErrUnsupported", err)
	}
}

func TestFlowcraftModelObservePreservesObservationSource(t *testing.T) {
	t.Parallel()
	model := testLLM{response: `{"facts":[{"text":"Alice prefers tea.","kind":"preference","subject":"Alice","predicate":"prefers","object":"tea","entities":["Alice","tea"],"evidence_refs":[{"id":"turn","text":"I prefer tea."}]}]}`}
	config := FlowcraftConfig{Dir: t.TempDir(), RuntimeID: "app", UserID: "user", ExtractionModel: "extract"}
	loader := &testFlowcraftLoader{model: model}
	store, err := OpenFlowcraftStore(context.Background(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	result, err := store.Observe(context.Background(), Observation{ID: "observation", Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "I prefer tea."}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Facts) != 1 || len(result.Facts[0].Sources) != 1 || result.Facts[0].Sources[0].ObservationID != "observation" {
		t.Fatalf("Observe() = %+v", result)
	}
	updatedText := "Alice strongly prefers tea."
	updated, err := store.Update(context.Background(), UpdateRequest{ID: result.Facts[0].ID, Text: &updatedText})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Sources) != 1 || updated.Sources[0].ObservationID != "observation" {
		t.Fatalf("Update() sources = %+v", updated.Sources)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenFlowcraftStore(context.Background(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	persisted, err := reopened.factByID(context.Background(), result.Facts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.Sources) != 1 || persisted.Sources[0].ObservationID != "observation" {
		t.Fatalf("reopened sources = %+v", persisted.Sources)
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
	if len(completed.Facts[0].Sources) != 1 || completed.Facts[0].Sources[0].ObservationID != "observation" {
		t.Fatalf("Wait() sources = %+v", completed.Facts[0].Sources)
	}
	updatedText := "Alice strongly prefers tea."
	updated, err := store.Update(context.Background(), UpdateRequest{ID: completed.Facts[0].ID, Text: &updatedText})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Sources) != 1 || updated.Sources[0].ObservationID != "observation" {
		t.Fatalf("Update() sources = %+v", updated.Sources)
	}
}

type failClaimOnceSideEffectOutbox struct {
	recall.SideEffectOutbox
	mu     sync.Mutex
	failed bool
}

func (o *failClaimOnceSideEffectOutbox) Claim(ctx context.Context, options recall.SideEffectClaimOptions) ([]recall.SideEffectJob, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.failed {
		o.failed = true
		return nil, errors.New("temporary outbox failure")
	}
	return o.SideEffectOutbox.Claim(ctx, options)
}

func TestFlowcraftAsyncWaitRetriesSideEffectsBeforeSuccess(t *testing.T) {
	t.Parallel()
	outbox := &failClaimOnceSideEffectOutbox{SideEffectOutbox: recall.NewInMemorySideEffectOutbox()}
	store, err := OpenFlowcraftStore(context.Background(), FlowcraftConfig{
		RuntimeID: "app", UserID: "user", ExtractionModel: "extract", Async: FlowcraftAsyncConfig{Enabled: true},
	}, &testFlowcraftLoader{model: testLLM{response: `{"facts":[{"text":"Alice prefers tea.","kind":"preference","subject":"Alice","predicate":"prefers","object":"tea","entities":["Alice","tea"],"evidence_refs":[{"id":"observation","text":"I prefer tea."}]}]}`}}, WithFlowcraftRecallOptions(recall.WithSideEffectOutbox(outbox)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	observed, err := store.Observe(context.Background(), Observation{ID: "observation", Text: "I prefer tea."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Wait(context.Background(), observed.Operation.ID); err == nil {
		t.Fatal("first Wait() should report the temporary side-effect failure")
	}
	completed, err := store.Wait(context.Background(), observed.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Operation == nil || completed.Operation.Status != OperationSucceeded || len(completed.Facts) != 1 {
		t.Fatalf("second Wait() = %+v", completed)
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
	first, err := store.Observe(context.Background(), Observation{ID: "first-observation", ObservedAt: time.Now(), Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "I prefer tea."}}})
	if err != nil {
		t.Fatal(err)
	}
	observed, err := store.Observe(context.Background(), Observation{ID: "zero-observation", ObservedAt: time.Now().Add(-time.Hour), Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "I prefer tea."}}})
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
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	terminal, err := OpenFlowcraftStore(context.Background(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = terminal.Close() })
	completed, err = terminal.Wait(context.Background(), observed.Operation.ID)
	if err != nil || completed.Operation == nil || completed.Operation.Status != OperationSucceeded || len(completed.Facts) != 0 {
		t.Fatalf("terminal zero-fact Wait() = %+v, %v", completed, err)
	}
	firstCompleted, err = terminal.Wait(context.Background(), first.Operation.ID)
	if err != nil || firstCompleted.Operation == nil || firstCompleted.Operation.Status != OperationSucceeded || len(firstCompleted.Facts) != 1 {
		t.Fatalf("terminal fact Wait() = %+v, %v", firstCompleted, err)
	}
	if len(firstCompleted.Facts[0].Sources) != 1 || firstCompleted.Facts[0].Sources[0].ObservationID != "first-observation" {
		t.Fatalf("terminal fact sources = %+v", firstCompleted.Facts[0].Sources)
	}
}

func TestFlowcraftZeroFactCompletionSurvivesPreMarkerCrashWindow(t *testing.T) {
	t.Parallel()
	config := FlowcraftConfig{Dir: t.TempDir(), RuntimeID: "app", UserID: "user", ExtractionModel: "extract", Async: FlowcraftAsyncConfig{Enabled: true}}
	loader := &testFlowcraftLoader{model: testLLM{response: `{"facts":[]}`}}
	store, err := OpenFlowcraftStore(context.Background(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := store.Observe(context.Background(), Observation{ID: "observation", Text: "nothing durable"})
	if err != nil {
		t.Fatal(err)
	}
	processor, ok := recall.NewAsyncSemanticProcessor(store.memory)
	if !ok {
		t.Fatal("async processor is unavailable")
	}
	processed, err := processor.ProcessAsyncSemantic(context.Background(), recall.AsyncSemanticProcessOptions{Scope: store.scope, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if processed.Completed != 1 {
		t.Fatalf("ProcessAsyncSemantic() = %+v", processed)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenFlowcraftStore(context.Background(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	result, err := reopened.Wait(context.Background(), observed.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation == nil || result.Operation.Status != OperationSucceeded || len(result.Facts) != 0 {
		t.Fatalf("Wait() = %+v", result)
	}
}

func TestFlowcraftRehydratePrefersTerminalMarker(t *testing.T) {
	t.Parallel()
	config := FlowcraftConfig{Dir: t.TempDir(), RuntimeID: "app", UserID: "user", ExtractionModel: "extract", Async: FlowcraftAsyncConfig{Enabled: true}}
	loader := &testFlowcraftLoader{model: testLLM{response: `{"facts":[]}`}}
	store, err := OpenFlowcraftStore(context.Background(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	operationID := "completed-operation"
	statuses := []struct {
		value      string
		observedAt time.Time
	}{
		{value: flowcraftOperationStatusSucceeded, observedAt: time.Now()},
		{value: flowcraftOperationStatusReady, observedAt: time.Now().Add(time.Hour)},
	}
	for _, status := range statuses {
		if err := store.temporal.Append(context.Background(), []recall.TemporalFact{{
			ID:         flowcraftOperationMarkerID(operationID, status.value),
			Scope:      store.scope,
			Kind:       recall.FactEpisode,
			ObservedAt: status.observedAt,
			Origin:     recall.FactOrigin{RequestID: operationID, Kind: recall.OriginKindEpisode},
			Metadata:   map[string]any{flowcraftOperationStatusAttribute: status.value},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenFlowcraftStore(context.Background(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	result, err := reopened.Wait(context.Background(), operationID)
	if err != nil || result.Operation == nil || result.Operation.Status != OperationSucceeded || len(result.Facts) != 0 {
		t.Fatalf("Wait() = %+v, %v", result, err)
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
	config := FlowcraftConfig{
		Dir: t.TempDir(), RuntimeID: "app", UserID: "user", ExtractionModel: "extract", Async: FlowcraftAsyncConfig{Enabled: true},
	}
	loader := &testFlowcraftLoader{model: failingTestLLM{}}
	store, err := OpenFlowcraftStore(context.Background(), config, loader)
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
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenFlowcraftStore(context.Background(), config, loader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	again, err = reopened.Wait(context.Background(), observed.Operation.ID)
	if err != nil || again.Operation == nil || again.Operation.Status != OperationFailed {
		t.Fatalf("restarted Wait() = %+v, %v", again, err)
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
