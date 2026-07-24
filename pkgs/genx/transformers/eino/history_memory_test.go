package eino

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestHistoryAndMemoryUseStableScopesAndDeliveryOrder(t *testing.T) {
	t.Parallel()
	events := &eventRecorder{}
	history := &recordingHistoryStore{events: events}
	memories := &recordingMemoryStore{events: events}
	config := textConfig()
	config.Agent.ContextID = "conversation-7"
	config.History = &HistoryConfig{Store: history, Scope: "history-scope", Limit: 20}
	config.Memory = &MemoryConfig{
		Store: memories, Scope: "memory-scope",
		Recall: []RecallDefinition{{
			QueryFrom: "input.text", Output: "recalled", TopK: 2,
		}},
		Observe: ObservePolicy{
			Enabled: true,
			Facts: []ObserveDefinition{{
				TextFrom: "fact", Attributes: map[string]string{"category": "category"},
			}},
		},
	}
	config.Graph.State.Fields = []StateField{
		{Name: "recalled", Type: StateString, Merge: MergeReplace},
		{Name: "fact", Type: StateString, Merge: MergeReplace},
		{Name: "category", Type: StateString, Merge: MergeReplace},
		{Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{{
		ID: "answer",
		Inputs: map[string]Binding{
			"user": {From: "input.text"}, "memory": {From: "recalled"},
		},
		Outputs: map[string]string{
			"text": "answer", "fact": "fact", "category": "category",
		},
		Script: &ScriptNode{
			Language: ScriptStarlark,
			Source: "def run(input):\n" +
				"  return {\"text\": input[\"user\"] + \"|\" + input[\"memory\"], " +
				"\"fact\": \"likes tea\", \"category\": \"preference\"}\n",
			Limits: ScriptLimits{
				MaxExecutionSteps: 1_000, Timeout: time.Second,
				MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
			},
		},
	}}
	config.Graph.Edges = []EdgeDefinition{{From: "start", To: "answer"}, {From: "answer", To: "end"}}
	config.Graph.Outputs[0].Node = "answer"

	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("hello"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "hello|- remembered" {
		t.Fatalf("output = %q", got)
	}
	if got := events.snapshot(); !slices.Equal(got, []string{
		"history.query", "memory.recall", "history.append", "memory.observe",
	}) {
		t.Fatalf("event order = %v", got)
	}

	history.mu.Lock()
	if len(history.records) != 2 {
		t.Fatalf("History records = %d, want user and assistant", len(history.records))
	}
	for _, record := range history.records {
		if record.Attributes["agent_id"] != "assistant" ||
			record.Attributes["context_id"] != "conversation-7" ||
			record.Attributes["scope"] != "history-scope" {
			t.Fatalf("History attributes = %#v", record.Attributes)
		}
	}
	history.mu.Unlock()

	memories.mu.Lock()
	defer memories.mu.Unlock()
	if len(memories.queries) != 1 {
		t.Fatalf("Memory queries = %#v", memories.queries)
	}
	query := memories.queries[0]
	if query.Scope != "memory-scope" || query.Text != "hello" || query.Limit != 2 {
		t.Fatalf("Memory query = %#v", query)
	}
	if len(memories.observations) != 1 {
		t.Fatalf("Memory observations = %#v", memories.observations)
	}
	observation := memories.observations[0]
	if observation.Scope != "memory-scope" || observation.ID == "" || len(observation.Turns) != 2 {
		t.Fatalf("Memory observation = %#v", observation)
	}
	if observation.Turns[0].Text != "hello" || observation.Turns[1].Text != "hello|- remembered" {
		t.Fatalf("Memory turns = %#v", observation.Turns)
	}
	if len(observation.Facts) != 1 ||
		observation.Facts[0].Text != "likes tea" ||
		observation.Facts[0].Attributes["category"] != "preference" {
		t.Fatalf("Memory facts = %#v", observation.Facts)
	}
}

func TestMemoryWaitRequiresTerminalSuccess(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		result  memory.ObserveResult
		wantErr string
	}{
		{name: "success", result: memory.ObserveResult{
			Operation: &memory.Operation{ID: "operation-1", Status: memory.OperationSucceeded},
		}},
		{name: "missing operation", wantErr: "returned no operation"},
		{name: "pending", result: memory.ObserveResult{
			Operation: &memory.Operation{ID: "operation-1", Status: memory.OperationPending},
		}, wantErr: "remained pending"},
		{name: "failed", result: memory.ObserveResult{
			Operation: &memory.Operation{ID: "operation-1", Status: memory.OperationFailed, Error: "failed"},
		}, wantErr: "failed"},
		{name: "unsupported", result: memory.ObserveResult{
			Operation: &memory.Operation{ID: "operation-1", Status: "unknown"},
		}, wantErr: "unsupported status"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &waitingMemoryStore{waitResult: test.result}
			state, err := newRunState(nil, graphInput{}, nil, nil)
			if err != nil {
				t.Fatalf("newRunState() error = %v", err)
			}
			err = observeMemory(t.Context(), &MemoryConfig{
				Store: store, Scope: "scope",
				Observe: ObservePolicy{Enabled: true, WaitForCompletion: true},
			}, state, "stream-1", "user", "assistant", false)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("observeMemory() error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("observeMemory() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

type eventRecorder struct {
	mu     sync.Mutex
	events []string
}

func (recorder *eventRecorder) add(event string) {
	recorder.mu.Lock()
	recorder.events = append(recorder.events, event)
	recorder.mu.Unlock()
}

func (recorder *eventRecorder) snapshot() []string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return slices.Clone(recorder.events)
}

type recordingHistoryStore struct {
	mu      sync.Mutex
	events  *eventRecorder
	records []logstore.Record
	query   logstore.Query
}

func (store *recordingHistoryStore) Append(
	_ context.Context,
	records []logstore.Record,
) ([]logstore.RecordKey, error) {
	store.events.add("history.append")
	store.mu.Lock()
	defer store.mu.Unlock()
	store.records = append(store.records, records...)
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		keys[index] = record.Key()
	}
	return keys, nil
}

func (store *recordingHistoryStore) Query(
	_ context.Context,
	query logstore.Query,
) (logstore.Page, error) {
	store.events.add("history.query")
	store.mu.Lock()
	store.query = query
	store.mu.Unlock()
	return logstore.Page{}, nil
}

func (*recordingHistoryStore) Replace(context.Context, logstore.Record) error { return nil }
func (*recordingHistoryStore) Delete(context.Context, logstore.RecordKey) error {
	return nil
}
func (*recordingHistoryStore) Close() error { return nil }

type recordingMemoryStore struct {
	mu           sync.Mutex
	events       *eventRecorder
	queries      []memory.Query
	observations []memory.Observation
}

type waitingMemoryStore struct {
	recordingMemoryStore
	waitResult memory.ObserveResult
}

func (*waitingMemoryStore) Observe(
	context.Context,
	memory.Observation,
) (memory.ObserveResult, error) {
	return memory.ObserveResult{
		Operation: &memory.Operation{ID: "operation-1", Status: memory.OperationPending},
	}, nil
}

func (store *waitingMemoryStore) Wait(
	context.Context,
	string,
) (memory.ObserveResult, error) {
	return store.waitResult, nil
}

func (store *recordingMemoryStore) Recall(
	_ context.Context,
	query memory.Query,
) (memory.RecallResult, error) {
	store.events.add("memory.recall")
	store.mu.Lock()
	store.queries = append(store.queries, query)
	store.mu.Unlock()
	return memory.RecallResult{Matches: []memory.Match{{
		Fact: memory.Fact{ID: "fact-1", Text: "remembered"}, Score: 1,
	}}}, nil
}

func (store *recordingMemoryStore) Observe(
	_ context.Context,
	observation memory.Observation,
) (memory.ObserveResult, error) {
	store.events.add("memory.observe")
	store.mu.Lock()
	store.observations = append(store.observations, observation)
	store.mu.Unlock()
	return memory.ObserveResult{
		Operation: &memory.Operation{ID: fmt.Sprintf("operation-%s", observation.ID), Status: memory.OperationSucceeded},
	}, nil
}

func (*recordingMemoryStore) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, errors.New("not supported")
}

func (*recordingMemoryStore) Delete(context.Context, memory.DeleteRequest) error {
	return errors.New("not supported")
}
