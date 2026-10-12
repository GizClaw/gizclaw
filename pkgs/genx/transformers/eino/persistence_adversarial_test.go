package eino

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/schema"
)

func TestPersistentStateAdversarialLoadAndCommitFailures(t *testing.T) {
	t.Parallel()
	fields := map[string]StateField{
		"answer": {Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	storeErr := errors.New("store unavailable")
	if _, _, err := loadPersistentState(t.Context(), &StatePersistenceConfig{
		Store: adversarialStateStore{loadErr: storeErr}, Scope: "scope", Fields: []string{"answer"},
	}, fields); err == nil || !strings.Contains(err.Error(), storeErr.Error()) {
		t.Fatalf("loadPersistentState() error = %v", err)
	}
	if _, _, err := loadPersistentState(t.Context(), &StatePersistenceConfig{
		Store: adversarialStateStore{snapshot: StateSnapshot{
			Version: "v1", Fields: map[string]any{"answer": 42, "ignored": "value"},
		}},
		Scope: "scope", Fields: []string{"answer"},
	}, fields); err == nil || !strings.Contains(err.Error(), "field \"answer\"") {
		t.Fatalf("loadPersistentState(invalid field) error = %v", err)
	}
	values, version, err := loadPersistentState(t.Context(), &StatePersistenceConfig{
		Store: adversarialStateStore{snapshot: StateSnapshot{
			Version: "v1", Fields: map[string]any{"answer": "saved", "ignored": "value"},
		}},
		Scope: "scope", Fields: []string{"answer"},
	}, fields)
	if err != nil || version != "v1" || values["answer"] != "saved" || len(values) != 1 {
		t.Fatalf("loadPersistentState() = %#v, %q, %v", values, version, err)
	}

	state, err := newRunState(fields, graphInput{}, nil, nil)
	if err != nil {
		t.Fatalf("newRunState() error = %v", err)
	}
	if err := commitPersistentState(t.Context(), &StatePersistenceConfig{
		Store: adversarialStateStore{compareErr: storeErr}, Scope: "scope", Fields: []string{"answer"},
	}, state, "v1"); err == nil || !strings.Contains(err.Error(), storeErr.Error()) {
		t.Fatalf("commitPersistentState() error = %v", err)
	}
	if err := commitPersistentState(t.Context(), nil, state, ""); err != nil {
		t.Fatalf("commitPersistentState(nil) error = %v", err)
	}
}

func TestHistoryAdversarialStoreRecordsAndFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("history unavailable")
	reader := &adversarialHistoryStore{queryErr: failure}
	history := &conversationHistory{config: &HistoryConfig{Store: reader, Scope: "scope", Limit: 10}}
	if _, err := history.load(t.Context()); !errors.Is(err, failure) {
		t.Fatalf("load=%v", err)
	}
	reader.queryErr = nil
	reader.page = memory.HistoryPage{Entries: []memory.HistoryEntry{{ConversationID: "other", Role: memory.RoleUser, Text: "private"}}}
	if _, err := history.load(t.Context()); err == nil {
		t.Fatal("accepted another conversation")
	}
	reader.page = memory.HistoryPage{Entries: []memory.HistoryEntry{{ConversationID: "scope", Role: memory.RoleTool, Text: "tool"}}}
	if _, err := history.load(t.Context()); err == nil {
		t.Fatal("accepted unsupported role")
	}
	reader.page = memory.HistoryPage{Entries: []memory.HistoryEntry{{ConversationID: "scope", Role: memory.RoleAssistant, Text: "second"}, {ConversationID: "scope", Role: memory.RoleUser, Text: "first"}}}
	messages, err := history.load(t.Context())
	if err != nil || len(messages) != 2 || messages[0].Content != "first" {
		t.Fatalf("messages=%+v err=%v", messages, err)
	}
	if err := history.append(t.Context(), []*schema.Message{schema.UserMessage("new")}, false); err != nil {
		t.Fatal(err)
	}
	if len(reader.page.Entries) != 2 || len(history.live) != 0 {
		t.Fatal("external History reader was written or shadowed")
	}
	local := &conversationHistory{config: &HistoryConfig{Limit: 2}}
	if err := local.append(t.Context(), []*schema.Message{schema.UserMessage("one"), schema.AssistantMessage("two", nil), schema.UserMessage("three")}, false); err != nil {
		t.Fatal(err)
	}
	messages, err = local.load(t.Context())
	if err != nil || len(messages) != 2 || messages[0].Content != "two" {
		t.Fatalf("local=%+v err=%v", messages, err)
	}
}

func TestMemoryAdversarialRecallAndObserveFailures(t *testing.T) {
	t.Parallel()
	storeErr := errors.New("memory unavailable")
	fields := map[string]StateField{
		"query":    {Name: "query", Type: StateString, Merge: MergeReplace},
		"recalled": {Name: "recalled", Type: StateString, Merge: MergeReplace},
		"fact":     {Name: "fact", Type: StateString, Merge: MergeReplace},
	}
	state, err := newRunState(fields, graphInput{}, map[string]any{
		"query": "question", "fact": "remember me",
	}, nil)
	if err != nil {
		t.Fatalf("newRunState() error = %v", err)
	}
	if err := recallMemory(t.Context(), &MemoryConfig{
		Store:  &adversarialMemoryStore{recallErr: storeErr},
		Recall: []RecallDefinition{{QueryFrom: "query", Output: "recalled", TopK: 1}},
	}, state); err == nil || !strings.Contains(err.Error(), storeErr.Error()) {
		t.Fatalf("recallMemory() error = %v", err)
	}
	if err := state.set("query", " "); err != nil {
		t.Fatalf("set(query) error = %v", err)
	}
	emptyStore := &adversarialMemoryStore{recallErr: storeErr}
	if err := recallMemory(t.Context(), &MemoryConfig{
		Store:  emptyStore,
		Recall: []RecallDefinition{{QueryFrom: "query", Output: "recalled", TopK: 1}},
	}, state); err != nil {
		t.Fatalf("recallMemory(empty) error = %v, want skipped recall", err)
	}
	if got, _ := state.value("recalled"); got != "" {
		t.Fatalf("recalled after empty query = %#v, want empty", got)
	}
	if emptyStore.recallCalls != 0 {
		t.Fatalf("empty query reached the store %d times", emptyStore.recallCalls)
	}
	if err := state.set("query", "question"); err != nil {
		t.Fatalf("set(query) error = %v", err)
	}
	if err := recallMemory(t.Context(), &MemoryConfig{
		Store: &adversarialMemoryStore{recallResult: memory.RecallResult{Matches: []memory.Match{
			{Fact: memory.Fact{Text: " "}},
			{Fact: memory.Fact{Text: "first"}},
			{Fact: memory.Fact{Text: "second"}},
		}}},
		Recall: []RecallDefinition{{QueryFrom: "query", Output: "recalled", TopK: 3}},
	}, state); err != nil {
		t.Fatalf("recallMemory() error = %v", err)
	}
	if got, _ := state.value("recalled"); got != "- first\n- second" {
		t.Fatalf("recalled = %#v", got)
	}

	observeConfig := func(store memory.Store) *MemoryConfig {
		return &MemoryConfig{
			Store: store, Scope: memory.Scope{AppID: "app"},
			Observe: ObservePolicy{
				Enabled: true,
				Facts: []ObserveDefinition{{
					TextFrom: "fact", Attributes: map[string]string{"missing": "missing"},
				}},
			},
		}
	}
	if err := observeMemory(t.Context(), observeConfig(&adversarialMemoryStore{}), state, "id", "user", "answer", false); err == nil ||
		!strings.Contains(err.Error(), "attribute") {
		t.Fatalf("observeMemory(attribute) error = %v", err)
	}
	config := observeConfig(&adversarialMemoryStore{observeErr: storeErr})
	config.Observe.Facts = nil
	if err := observeMemory(t.Context(), config, state, "id", "user", "answer", false); err == nil ||
		!strings.Contains(err.Error(), storeErr.Error()) {
		t.Fatalf("observeMemory(store) error = %v", err)
	}
	for _, operation := range []*memory.Operation{
		nil,
		{ID: "failed", Status: memory.OperationFailed, Error: "failed"},
		{Status: memory.OperationPending},
		{ID: "unknown", Status: "unknown"},
	} {
		store := &adversarialMemoryStore{observeResult: memory.ObserveResult{Operation: operation}}
		err := observeMemory(t.Context(), &MemoryConfig{
			Store: store, Scope: memory.Scope{AppID: "app"}, Observe: ObservePolicy{Enabled: true},
		}, state, "id", "user", "answer", false)
		if operation == nil {
			if err != nil {
				t.Fatalf("observeMemory(nil operation) error = %v", err)
			}
		} else if err == nil {
			t.Fatalf("observeMemory(%#v) succeeded", operation)
		}
	}
	if err := observeMemory(t.Context(), nil, state, "", "", "", false); err != nil {
		t.Fatalf("observeMemory(nil) error = %v", err)
	}
	if err := observeMemory(t.Context(), &MemoryConfig{
		Store: &adversarialMemoryStore{}, Scope: memory.Scope{AppID: "app"}, Observe: ObservePolicy{Enabled: false},
	}, state, "", "", "", false); err != nil {
		t.Fatalf("observeMemory(disabled) error = %v", err)
	}
	if err := observeMemory(t.Context(), &MemoryConfig{
		Store: &adversarialMemoryStore{}, Scope: memory.Scope{AppID: "app"}, Observe: ObservePolicy{Enabled: true},
	}, state, "", "", "", true); err != nil {
		t.Fatalf("observeMemory(empty interrupted) error = %v", err)
	}
}

type adversarialStateStore struct {
	snapshot   StateSnapshot
	loadErr    error
	compareErr error
}

func (store adversarialStateStore) Load(context.Context, string) (StateSnapshot, error) {
	return store.snapshot, store.loadErr
}

func (store adversarialStateStore) CompareAndSwap(
	context.Context,
	string,
	string,
	map[string]any,
) (StateSnapshot, error) {
	return StateSnapshot{}, store.compareErr
}

type adversarialHistoryStore struct {
	page     memory.HistoryPage
	queryErr error
}

func (store *adversarialHistoryStore) Query(context.Context, memory.HistoryQuery) (memory.HistoryPage, error) {
	return store.page, store.queryErr
}

type adversarialMemoryStore struct {
	mu            sync.Mutex
	recallResult  memory.RecallResult
	recallErr     error
	observeResult memory.ObserveResult
	observeErr    error
	recallCalls   int
}

func (*adversarialMemoryStore) SupportsDirectFactObservation() bool { return true }

func (store *adversarialMemoryStore) Observe(
	context.Context,
	memory.Observation,
) (memory.ObserveResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.observeResult, store.observeErr
}

func (store *adversarialMemoryStore) Recall(
	context.Context,
	memory.Query,
) (memory.RecallResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.recallCalls++
	return store.recallResult, store.recallErr
}

func (*adversarialMemoryStore) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, nil
}

func (*adversarialMemoryStore) Delete(context.Context, memory.DeleteRequest) error {
	return nil
}
