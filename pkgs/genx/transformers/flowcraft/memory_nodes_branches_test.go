package flowcraft

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	flownode "github.com/GizClaw/flowcraft/sdk/graph/node"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestRegisterMemoryRecallNodeValidationAndIdentity(t *testing.T) {
	valid := map[string]any{
		"query":  map[string]any{"text_from": "input"},
		"output": "memory",
		"top_k":  1,
	}
	tests := []struct {
		name    string
		config  Config
		node    map[string]any
		wantErr string
	}{
		{name: "invalid config", config: Config{Memory: &memoryNodeStore{}}, node: map[string]any{"query": make(chan int)}, wantErr: "unsupported type"},
		{name: "missing memory", node: valid, wantErr: "requires Memory"},
		{name: "missing fields", config: Config{Memory: &memoryNodeStore{}}, node: map[string]any{"top_k": 0}, wantErr: "requires query.text_from"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory := flownode.NewFactory()
			registerMemoryNodes(factory, test.config)
			_, err := factory.Build(flowgraph.NodeDefinition{ID: "recall", Type: "memory_recall", Config: test.node})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Build() error = %v, want %q", err, test.wantErr)
			}
		})
	}

	factory := flownode.NewFactory()
	registerMemoryNodes(factory, Config{Memory: &memoryNodeStore{}, MemoryScope: memory.Scope{AppID: "app"}})
	node, err := factory.Build(flowgraph.NodeDefinition{ID: "recall", Type: "memory_recall", Config: valid})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if node.ID() != "recall" || node.Type() != "memory_recall" {
		t.Fatalf("node identity = (%q, %q)", node.ID(), node.Type())
	}
}

func TestRegisterMemoryObserveNodeValidationAndIdentity(t *testing.T) {
	valid := map[string]any{"observations": []any{map[string]any{"text_from": "input"}}}
	factObservation := map[string]any{"observations": []any{map[string]any{
		"facts": []any{map[string]any{"text_from": "fact"}},
	}}}
	tests := []struct {
		name    string
		config  Config
		node    map[string]any
		wantErr string
	}{
		{name: "invalid config", config: Config{Memory: &memoryNodeStore{}}, node: map[string]any{"observations": "bad"}, wantErr: "cannot unmarshal"},
		{name: "missing memory", node: valid, wantErr: "requires Memory"},
		{name: "missing observations", config: Config{Memory: &memoryNodeStore{}}, node: map[string]any{}, wantErr: "requires observations"},
		{name: "missing fact capability", config: Config{Memory: memoryOnlyStore{Store: &memoryNodeStore{}}}, node: factObservation, wantErr: "direct Fact observation"},
		{name: "missing waiter", config: Config{Memory: &memoryNodeStore{}}, node: map[string]any{"observations": []any{map[string]any{"text_from": "input"}}, "wait_for_completion": true}, wantErr: "memory.OperationWaiter"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory := flownode.NewFactory()
			registerMemoryNodes(factory, test.config)
			_, err := factory.Build(flowgraph.NodeDefinition{ID: "observe", Type: "memory_observe", Config: test.node})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Build() error = %v, want %q", err, test.wantErr)
			}
		})
	}

	factory := flownode.NewFactory()
	store := &memoryWaitNodeStore{memoryNodeStore: &memoryNodeStore{}}
	registerMemoryNodes(factory, Config{Memory: store})
	node, err := factory.Build(flowgraph.NodeDefinition{ID: "observe", Type: "memory_observe", Config: map[string]any{
		"observations":        []any{map[string]any{"text_from": "input"}},
		"wait_for_completion": true,
	}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if node.ID() != "observe" || node.Type() != "memory_observe" {
		t.Fatalf("node identity = (%q, %q)", node.ID(), node.Type())
	}
}

func TestMemoryRecallNodeBranches(t *testing.T) {
	wantErr := errors.New("recall failed")
	errorNode := &memoryRecallNode{
		id: "recall", store: &memoryNodeStore{recallErr: wantErr},
		config: memoryRecallNodeConfig{TopK: 1},
	}
	if err := errorNode.ExecuteBoard(flowgraph.ExecutionContext{Context: t.Context()}, flowgraph.NewBoard()); !errors.Is(err, wantErr) {
		t.Fatalf("ExecuteBoard() error = %v, want %v", err, wantErr)
	}

	store := &memoryNodeStore{recallResult: memory.RecallResult{Matches: []memory.Match{
		{Fact: memory.Fact{Text: "first"}},
		{Fact: memory.Fact{Text: "second"}},
	}}}
	board := flowgraph.NewBoard()
	board.SetVar("input", 42)
	node := &memoryRecallNode{
		id: "recall", store: store,
		config: memoryRecallNodeConfig{
			Query: struct {
				TextFrom string         `json:"text_from"`
				Kinds    []string       `json:"kinds"`
				Lanes    []string       `json:"lanes"`
				Filters  []memoryFilter `json:"filters"`
			}{
				TextFrom: "input",
				Filters: []memoryFilter{
					{Field: " kind ", Value: "note"},
					{Field: "score", Operator: "gt", Value: 1},
				},
			},
			Output: "result",
			Render: &struct {
				Header     string `json:"header"`
				ItemPrefix string `json:"item_prefix"`
				MaxItems   int    `json:"max_items"`
			}{Header: "", ItemPrefix: "* ", MaxItems: 1},
			TopK: 2,
		},
	}
	if err := node.ExecuteBoard(flowgraph.ExecutionContext{Context: t.Context()}, board); err != nil {
		t.Fatalf("ExecuteBoard() error = %v", err)
	}
	if store.recallQuery.Text != "42" || store.recallQuery.Limit != 2 || len(store.recallQuery.Filters) != 2 ||
		store.recallQuery.Filters[0].Field != "kind" || store.recallQuery.Filters[0].Operator != memory.FilterEqual ||
		store.recallQuery.Filters[1].Operator != memory.FilterGreaterThan {
		t.Fatalf("Recall query = %#v", store.recallQuery)
	}
	if rendered, _ := board.GetVar("result"); rendered != "* first" {
		t.Fatalf("rendered result = %#v", rendered)
	}
}

func TestMemoryObserveNodeTerminalAndAsyncBranches(t *testing.T) {
	wantErr := errors.New("observe failed")
	errorNode := &memoryObserveNode{id: "observe", store: &memoryNodeStore{observeErr: wantErr}}
	if err := errorNode.ExecuteBoard(flowgraph.ExecutionContext{Context: t.Context()}, flowgraph.NewBoard()); !errors.Is(err, wantErr) {
		t.Fatalf("Observe error = %v, want %v", err, wantErr)
	}

	waitErr := errors.New("wait failed")
	waitStore := &memoryWaitNodeStore{
		memoryNodeStore: &memoryNodeStore{observeResult: pendingMemoryResult("operation")},
		waitErr:         waitErr,
	}
	waitNode := &memoryObserveNode{id: "observe", store: waitStore, config: memoryObserveNodeConfig{WaitForCompletion: true}}
	if err := waitNode.ExecuteBoard(flowgraph.ExecutionContext{Context: t.Context()}, flowgraph.NewBoard()); !errors.Is(err, waitErr) {
		t.Fatalf("Wait error = %v, want %v", err, waitErr)
	}

	waitStore.waitErr = nil
	waitStore.waitResult = memory.ObserveResult{Operation: &memory.Operation{ID: "operation", Status: memory.OperationFailed, Error: "materialization failed"}}
	if err := waitNode.ExecuteBoard(flowgraph.ExecutionContext{Context: t.Context()}, flowgraph.NewBoard()); err == nil || !strings.Contains(err.Error(), "materialization failed") {
		t.Fatalf("failed operation error = %v", err)
	}

	asyncStore := &memoryAsyncNodeStore{
		memoryNodeStore: &memoryNodeStore{observeResult: pendingMemoryResult("async")},
		processed:       make(chan memory.OperationRequest, 1),
	}
	if err := (&memoryObserveNode{id: "observe", store: asyncStore}).ExecuteBoard(
		flowgraph.ExecutionContext{Context: t.Context()}, flowgraph.NewBoard(),
	); err == nil || !strings.Contains(err.Error(), "no generation task owner") {
		t.Fatalf("missing task owner error = %v", err)
	}

	owner := newTaskOwner()
	defer owner.Close()
	asyncNode := &memoryObserveNode{id: "observe", store: asyncStore, tasks: owner}
	if err := asyncNode.ExecuteBoard(flowgraph.ExecutionContext{Context: t.Context()}, flowgraph.NewBoard()); err != nil {
		t.Fatalf("async ExecuteBoard() error = %v", err)
	}
	select {
	case request := <-asyncStore.processed:
		if request.ID != "async" {
			t.Fatalf("async operation ID = %q", request.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("async operation was not processed")
	}

	plainPending := &memoryNodeStore{observeResult: pendingMemoryResult("external")}
	if err := (&memoryObserveNode{id: "observe", store: plainPending}).ExecuteBoard(
		flowgraph.ExecutionContext{Context: t.Context()}, flowgraph.NewBoard(),
	); err != nil {
		t.Fatalf("provider-owned pending operation error = %v", err)
	}
}

func TestMemoryObserveNodeBuildsTextTurnsAndFacts(t *testing.T) {
	store := &memoryNodeStore{}
	board := flowgraph.NewBoard()
	board.SetVar("first", " first ")
	board.SetVar("second", "second")
	board.SetVar("blank", "  ")
	board.SetVar("turns", []memory.Turn{{ID: "turn", Role: memory.RoleUser, Text: "hello"}})
	node := &memoryObserveNode{
		id: "observe", store: store,
		config: memoryObserveNodeConfig{Observations: []struct {
			TurnsFrom string `json:"turns_from"`
			TextFrom  string `json:"text_from"`
			Facts     []struct {
				TextFrom   string            `json:"text_from"`
				Attributes map[string]string `json:"attributes"`
			} `json:"facts"`
		}{
			{TextFrom: "first", TurnsFrom: "turns"},
			{TextFrom: "second", Facts: []struct {
				TextFrom   string            `json:"text_from"`
				Attributes map[string]string `json:"attributes"`
			}{{TextFrom: "blank"}, {TextFrom: "first", Attributes: map[string]string{"lane": "facts"}}}},
		}},
	}
	if err := node.ExecuteBoard(flowgraph.ExecutionContext{Context: t.Context(), RunID: "run"}, board); err != nil {
		t.Fatalf("ExecuteBoard() error = %v", err)
	}
	if store.observation.Text != "first\nsecond" || len(store.observation.Turns) != 1 || len(store.observation.Facts) != 1 {
		t.Fatalf("Observation = %#v", store.observation)
	}
}

func TestMemoryNodeHelperBranches(t *testing.T) {
	if got := memoryRecallGuidance([]string{" clues ", "missing", "empty"}, map[string]string{
		"clues": " remember ", "empty": " ",
	}); len(got) != 1 || got[0] != "remember" {
		t.Fatalf("memoryRecallGuidance() = %#v", got)
	}
	if memoryRecallCandidateLimit(150, true) != 1000 || memoryRecallCandidateLimit(2, true) != 100 || memoryRecallCandidateLimit(3, false) != 3 {
		t.Fatal("memoryRecallCandidateLimit() boundaries are incorrect")
	}

	attributes := map[string]any{"categories": []any{"note", 1}, "lane": []string{"Clues"}}
	if !memoryAttributeMatches(attributes, "kind", "categories", []string{" NOTE "}) ||
		!memoryAttributeMatches(attributes, "lane", "", []string{"clues"}) ||
		memoryAttributeMatches(attributes, "missing", "", []string{"x"}) {
		t.Fatalf("memoryAttributeMatches() did not normalize supported values")
	}
	if got := memoryAttributeStrings(42); got != nil {
		t.Fatalf("memoryAttributeStrings(number) = %#v", got)
	}

	matches := []memory.Match{{Fact: memory.Fact{Text: " "}}, {Fact: memory.Fact{Text: "one"}}, {Fact: memory.Fact{Text: "two"}}}
	if got := renderMemoryMatches(matches, nil, nil); got != "Relevant memory:\n- one\n- two" {
		t.Fatalf("renderMemoryMatches() = %q", got)
	}
	if got := renderMemoryMatches(matches, &struct {
		Header     string `json:"header"`
		ItemPrefix string `json:"item_prefix"`
		MaxItems   int    `json:"max_items"`
	}{MaxItems: 1}, []string{"policy"}); got != "Recall policy:\npolicy\none" {
		t.Fatalf("renderMemoryMatches(empty formatting) = %q", got)
	}

	if boardString(nil, "key") != "" || boardString(flowgraph.NewBoard(), "missing") != "" {
		t.Fatal("boardString() missing values were not empty")
	}
	board := flowgraph.NewBoard()
	board.SetVar("value", " text ")
	if boardString(board, " value ") != "text" {
		t.Fatalf("boardString() = %q", boardString(board, " value "))
	}
}

func TestBoardTurnsSources(t *testing.T) {
	if boardTurns(nil, "turns") != nil || boardTurns(flowgraph.NewBoard(), "missing") != nil {
		t.Fatal("boardTurns() missing values were not nil")
	}
	board := flowgraph.NewBoard()
	board.AppendChannelMessage("__main_channel", flowmodel.NewTextMessage(flowmodel.RoleUser, " hello "))
	board.AppendChannelMessage("__main_channel", flowmodel.NewTextMessage(flowmodel.RoleAssistant, " "))
	if turns := boardTurns(board, "conversation"); len(turns) != 1 || turns[0].ID != "conversation:0" || turns[0].Text != "hello" {
		t.Fatalf("conversation turns = %#v", turns)
	}

	original := []memory.Turn{{ID: "one", Role: memory.RoleUser, Text: "one"}}
	board.SetVar("memory", original)
	copyOfTurns := boardTurns(board, "memory")
	copyOfTurns[0].Text = "changed"
	if original[0].Text != "one" {
		t.Fatal("boardTurns() retained caller-owned []memory.Turn")
	}

	board.SetVar("messages", []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleUser, "one"),
		flowmodel.NewTextMessage(flowmodel.RoleAssistant, "two"),
	})
	if turns := boardTurns(board, "messages"); len(turns) != 2 || turns[1].ID != "messages:1" || turns[1].Role != memory.RoleAssistant {
		t.Fatalf("message turns = %#v", turns)
	}
	board.SetVar("scalar", 42)
	if turns := boardTurns(board, "scalar"); len(turns) != 1 || turns[0].Text != "42" || turns[0].Role != memory.RoleUser {
		t.Fatalf("scalar turns = %#v", turns)
	}
	board.SetVar("blank", " ")
	if turns := boardTurns(board, "blank"); turns != nil {
		t.Fatalf("blank turns = %#v", turns)
	}
}

func TestTaskOwnerAndAgentCloseLifecycle(t *testing.T) {
	var nilOwner *taskOwner
	if nilOwner.Run(func(context.Context) {}) || nilOwner.Close() != nil {
		t.Fatal("nil task owner accepted work or failed to close")
	}
	owner := newTaskOwner()
	if owner.Run(nil) {
		t.Fatal("task owner accepted a nil task")
	}
	started := make(chan struct{})
	stopped := make(chan struct{})
	if !owner.Run(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(stopped)
	}) {
		t.Fatal("task owner rejected work while open")
	}
	<-started
	if err := owner.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("Close() returned before the owned task stopped")
	}
	if !errors.Is(context.Cause(owner.ctx), io.EOF) {
		t.Fatalf("task owner context cause = %v, want EOF", context.Cause(owner.ctx))
	}
	if owner.Run(func(context.Context) {}) {
		t.Fatal("closed task owner accepted work")
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	var nilAgent *Agent
	if err := nilAgent.Close(); err != nil {
		t.Fatalf("nil Agent.Close() error = %v", err)
	}
	if err := (&Agent{}).Close(); err != nil {
		t.Fatalf("empty Agent.Close() error = %v", err)
	}
	agentOwner := newTaskOwner()
	if err := (&Agent{asyncTasks: agentOwner}).Close(); err != nil {
		t.Fatalf("Agent.Close() error = %v", err)
	}
}

func pendingMemoryResult(id string) memory.ObserveResult {
	return memory.ObserveResult{Operation: &memory.Operation{ID: id, Status: memory.OperationPending}}
}

type memoryWaitNodeStore struct {
	*memoryNodeStore
	waitResult memory.ObserveResult
	waitErr    error
}

func (store *memoryWaitNodeStore) Wait(context.Context, memory.OperationRequest) (memory.ObserveResult, error) {
	return store.waitResult, store.waitErr
}

type memoryAsyncNodeStore struct {
	*memoryNodeStore
	processed chan memory.OperationRequest
}

func (store *memoryAsyncNodeStore) ProcessAsync(_ context.Context, request memory.OperationRequest) (memory.ObserveResult, error) {
	store.processed <- request
	return memory.ObserveResult{Operation: &memory.Operation{ID: request.ID, Status: memory.OperationSucceeded}}, nil
}
