package flowcraft

import (
	"context"
	"strings"
	"testing"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestMemoryObserveNodeRejectsStoreWithoutDirectFactCapability(t *testing.T) {
	t.Parallel()
	config := testConfig(&echoGenerator{})
	config.Memory = memoryOnlyStore{Store: &memoryNodeStore{}}
	config.MemoryScope = memory.Scope{AppID: "workspace"}
	config.Graph.Entry = "observe"
	config.Graph.Nodes = append([]flowgraph.NodeDefinition{{
		ID: "observe", Type: "memory_observe",
		Config: map[string]any{
			"observations": []any{map[string]any{
				"facts": []any{map[string]any{
					"text_from":  "fact",
					"attributes": map[string]string{"lane": "facts"},
				}},
			}},
		},
	}}, config.Graph.Nodes...)
	config.Graph.Edges = []flowgraph.EdgeDefinition{
		{From: "observe", To: "chat"},
		{From: "chat", To: flowgraph.END},
	}
	_, err := New(config)
	if err == nil || !strings.Contains(err.Error(), "requires direct Fact observation support") {
		t.Fatalf("New() error = %v", err)
	}
}

func TestMemoryRecallNodeMapsBoardToStoreAndBack(t *testing.T) {
	t.Parallel()
	store := &memoryNodeStore{recallResult: memory.RecallResult{Matches: []memory.Match{
		{Fact: memory.Fact{Text: "wrong lane", Attributes: map[string]any{"kind": "note", "lane": "other"}}},
		{Fact: memory.Fact{Text: "first", Attributes: map[string]any{"kind": "note", "lane": "clues"}}},
		{Fact: memory.Fact{Text: "second", Attributes: map[string]any{"categories": []string{"note"}, "lane": "clues"}}},
		{Fact: memory.Fact{Text: "over top-k", Attributes: map[string]any{"kind": "note", "lane": "clues"}}},
	}}}
	board := flowgraph.NewBoard()
	board.SetVar("input", "where is the key?")
	node := &memoryRecallNode{
		id:    "recall",
		store: store,
		scope: memory.Scope{AppID: "workspace"},
		config: memoryRecallNodeConfig{
			Query: struct {
				TextFrom string         `json:"text_from"`
				Kinds    []string       `json:"kinds"`
				Lanes    []string       `json:"lanes"`
				Filters  []memoryFilter `json:"filters"`
			}{
				TextFrom: "input",
				Kinds:    []string{"note"},
				Lanes:    []string{"clues"},
			},
			Output: "memory_context",
			TopK:   2,
		},
		laneRecall: map[string]string{
			"clues": "Use confirmed clues to maintain continuity.",
		},
	}

	if err := node.ExecuteBoard(flowgraph.ExecutionContext{
		Context: t.Context(),
		RunID:   "run",
	}, board); err != nil {
		t.Fatalf("ExecuteBoard() error = %v", err)
	}
	if store.recallQuery.Scope.AppID != "workspace" ||
		store.recallQuery.Text != "where is the key?" ||
		store.recallQuery.Limit != 100 {
		t.Fatalf("Recall query = %#v", store.recallQuery)
	}
	if len(store.recallQuery.Filters) != 0 {
		t.Fatalf("Recall filters = %#v", store.recallQuery.Filters)
	}
	rendered, _ := board.GetVar("memory_context")
	if rendered != "Recall policy:\nUse confirmed clues to maintain continuity.\nRelevant memory:\n- first\n- second" {
		t.Fatalf("Board memory_context = %#v", rendered)
	}
}

func TestMemoryObserveNodeMapsBoardFactsToStore(t *testing.T) {
	t.Parallel()
	store := &memoryNodeStore{observeResult: memory.ObserveResult{
		Operation: &memory.Operation{ID: "done", Status: memory.OperationSucceeded},
	}}
	board := flowgraph.NewBoard()
	board.SetVar("clue", "the key is under the mat")
	node := &memoryObserveNode{
		id:    "observe",
		store: store,
		scope: memory.Scope{AppID: "workspace"},
		config: memoryObserveNodeConfig{
			Observations: []struct {
				TurnsFrom string `json:"turns_from"`
				TextFrom  string `json:"text_from"`
				Facts     []struct {
					TextFrom   string            `json:"text_from"`
					Attributes map[string]string `json:"attributes"`
				} `json:"facts"`
			}{{
				Facts: []struct {
					TextFrom   string            `json:"text_from"`
					Attributes map[string]string `json:"attributes"`
				}{{
					TextFrom:   "clue",
					Attributes: map[string]string{"lane": "clues"},
				}},
			}},
		},
	}

	if err := node.ExecuteBoard(flowgraph.ExecutionContext{
		Context: t.Context(),
		RunID:   "run",
	}, board); err != nil {
		t.Fatalf("ExecuteBoard() error = %v", err)
	}
	if store.observation.ID != "run" || store.observation.Scope.AppID != "workspace" {
		t.Fatalf("Observation identity = %#v", store.observation)
	}
	if len(store.observation.Facts) != 1 ||
		store.observation.Facts[0].Text != "the key is under the mat" ||
		store.observation.Facts[0].Attributes["lane"] != "clues" {
		t.Fatalf("Observation facts = %#v", store.observation.Facts)
	}
}

type memoryNodeStore struct {
	recallResult  memory.RecallResult
	observeResult memory.ObserveResult
	recallQuery   memory.Query
	observation   memory.Observation
}

func (*memoryNodeStore) SupportsDirectFactObservation() bool { return true }

func (store *memoryNodeStore) Observe(
	_ context.Context,
	observation memory.Observation,
) (memory.ObserveResult, error) {
	store.observation = observation
	return store.observeResult, nil
}

func (store *memoryNodeStore) Recall(
	_ context.Context,
	query memory.Query,
) (memory.RecallResult, error) {
	store.recallQuery = query
	return store.recallResult, nil
}

func (*memoryNodeStore) Update(
	context.Context,
	memory.UpdateRequest,
) (memory.Fact, error) {
	return memory.Fact{}, nil
}

func (*memoryNodeStore) Delete(context.Context, memory.DeleteRequest) error {
	return nil
}
