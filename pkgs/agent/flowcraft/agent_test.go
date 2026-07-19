package flowcraft

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/flowcraft/sdk/engine"
	"github.com/GizClaw/flowcraft/sdk/event"
	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestGenXStreamMessageSurfacesTerminalChunkError(t *testing.T) {
	builder := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	if err := builder.Add(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{EndOfStream: true, Error: "provider failed"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := builder.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	stream := &genXStreamMessage{stream: builder.Stream()}
	if stream.Next() {
		t.Fatal("Next() = true for terminal error chunk")
	}
	if err := stream.Err(); err == nil || !strings.Contains(err.Error(), "provider failed") {
		t.Fatalf("Err() = %v", err)
	}
}

func TestAgentRunsFlowcraftToolsStrictlyInProviderOrder(t *testing.T) {
	var mu sync.Mutex
	var calls []commonagent.ToolCall
	toolkit := commonagent.ToolkitFunc{
		List: func() []commonagent.Tool {
			return []commonagent.Tool{{Name: "first"}, {Name: "second"}}
		},
		InvokeFunc: func(_ context.Context, call commonagent.ToolCall) (commonagent.ToolResult, error) {
			mu.Lock()
			calls = append(calls, call)
			mu.Unlock()
			return commonagent.ToolResult{ID: call.ID, Content: json.RawMessage(`{"ok":true}`)}, nil
		},
	}
	generator := &scriptedGenerator{}
	resolver, err := NewGenXResolver(map[string]GenXModel{"chat": {Generator: generator, Pattern: "test/chat"}})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := New(Config{
		ID:           "test-agent",
		Graph:        toolLoopGraph(),
		Resolver:     resolver,
		Toolkit:      toolkit,
		MaxToolCalls: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	input := buffer.N[*genx.MessageChunk](3)
	addTextTurn(t, input, "user-1", "run tools")
	_ = input.CloseWrite()
	output, err := agent.Transform(t.Context(), "", input)
	if err != nil {
		t.Fatal(err)
	}
	chunks := readAll(t, output)

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 || calls[0].ID != "call-2" || calls[1].ID != "call-1" {
		t.Fatalf("tool calls = %#v, want call-2 then call-1", calls)
	}
	if got := visibleText(chunks); got != "done" {
		t.Fatalf("visible text = %q, want done; chunks=%#v", got, chunks)
	}
	for _, chunk := range chunks {
		if chunk.ToolCall != nil || chunk.Role == genx.RoleTool {
			t.Fatalf("internal tool traffic leaked: %#v", chunk)
		}
	}
}

func TestAgentPreservesTextCarriedByEOS(t *testing.T) {
	seen := make(chan genx.ModelContext, 1)
	generator := generatorFunc(func(_ context.Context, modelContext genx.ModelContext) (genx.Stream, error) {
		seen <- modelContext
		builder := genx.NewGrowableStreamBuilder(modelContext, 1)
		if err := builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("answer")}); err != nil {
			return nil, err
		}
		if err := builder.Done(genx.Usage{}); err != nil {
			return nil, err
		}
		return builder.Stream(), nil
	})
	resolver, err := NewGenXResolver(map[string]GenXModel{"chat": {Generator: generator, Pattern: "test/chat"}})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(Config{ID: "eos-agent", Graph: textGraph(), Resolver: resolver, Toolkit: commonagent.EmptyToolkit()})
	if err != nil {
		t.Fatal(err)
	}
	input := buffer.N[*genx.MessageChunk](2)
	for _, chunk := range []*genx.MessageChunk{
		genx.NewBeginOfStream("text-1"),
		{Role: genx.RoleUser, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "text-1", EndOfStream: true}},
	} {
		if err := input.Add(chunk); err != nil {
			t.Fatal(err)
		}
	}
	_ = input.CloseWrite()
	output, err := runtime.Transform(t.Context(), "", input)
	if err != nil {
		t.Fatal(err)
	}
	if got := visibleText(readAll(t, output)); got != "answer" {
		t.Fatalf("visible text = %q, want answer", got)
	}
	if got := lastText(<-seen, genx.RoleUser); got != "hello" {
		t.Fatalf("model user input = %q, want EOS text", got)
	}
}

func TestAgentReplacementInputInterruptsBufferedResponseAndKeepsPulledHistory(t *testing.T) {
	secondContext := make(chan genx.ModelContext, 1)
	generator := generatorFunc(func(_ context.Context, modelContext genx.ModelContext) (genx.Stream, error) {
		lastUser := lastText(modelContext, genx.RoleUser)
		builder := genx.NewGrowableStreamBuilder(modelContext, 1)
		if lastUser == "first" {
			if err := builder.Add(
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("visible")},
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("discarded")},
			); err != nil {
				return nil, err
			}
		} else {
			secondContext <- modelContext
			if err := builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("fresh")}); err != nil {
				return nil, err
			}
		}
		if err := builder.Done(genx.Usage{}); err != nil {
			return nil, err
		}
		return builder.Stream(), nil
	})
	resolver, err := NewGenXResolver(map[string]GenXModel{"chat": {Generator: generator, Pattern: "test/chat"}})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := New(Config{ID: "interrupt-agent", Graph: textGraph(), Resolver: resolver, Toolkit: commonagent.EmptyToolkit()})
	if err != nil {
		t.Fatal(err)
	}
	input := buffer.N[*genx.MessageChunk](8)
	addTextTurn(t, input, "user-1", "first")
	output, err := agent.Transform(t.Context(), "", input)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := output.Next()
	if err != nil || empty.Part != genx.Text("") {
		t.Fatalf("first output = %#v, %v", empty, err)
	}
	visible, err := output.Next()
	if err != nil || visible.Part != genx.Text("visible") {
		t.Fatalf("visible output = %#v, %v", visible, err)
	}
	firstStreamID := visible.Ctrl.StreamID
	addTextTurn(t, input, "user-2", "second")
	_ = input.CloseWrite()

	select {
	case modelContext := <-secondContext:
		if got := allText(modelContext, genx.RoleModel); got != "visible" {
			t.Fatalf("second model assistant history = %q, want pulled partial visible", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for second model input")
	}

	chunks := readAll(t, output)
	if slicesContainsText(chunks, "discarded") {
		t.Fatalf("unpulled output was not discarded: %#v", chunks)
	}
	if !hasInterruptedEOS(chunks, firstStreamID) {
		t.Fatalf("missing interrupted EOS for %q: %#v", firstStreamID, chunks)
	}
	if got := visibleText(chunks); got != "fresh" {
		t.Fatalf("remaining text = %q, want fresh", got)
	}
	history, err := agent.history.recent(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if !hasInterruptedAssistant(history, "visible") {
		t.Fatalf("history does not mark pulled partial interrupted: %#v", history)
	}
}

func TestAgentRecallsAndObservesInjectedMemory(t *testing.T) {
	store := &recordingMemoryStore{recall: memory.RecallResult{Matches: []memory.Match{{Fact: memory.Fact{ID: "fact-1", Text: "likes tea"}, Score: 0.9}}}}
	seen := make(chan genx.ModelContext, 1)
	generator := generatorFunc(func(_ context.Context, modelContext genx.ModelContext) (genx.Stream, error) {
		seen <- modelContext
		builder := genx.NewGrowableStreamBuilder(modelContext, 1)
		if err := builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("answer")}); err != nil {
			return nil, err
		}
		if err := builder.Done(genx.Usage{}); err != nil {
			return nil, err
		}
		return builder.Stream(), nil
	})
	resolver, err := NewGenXResolver(map[string]GenXModel{"chat": {Generator: generator, Pattern: "test/chat"}})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(Config{ID: "memory-agent", Graph: textGraph(), Resolver: resolver, Toolkit: commonagent.EmptyToolkit(), Memory: store, MemoryLimit: 3})
	if err != nil {
		t.Fatal(err)
	}
	input := buffer.N[*genx.MessageChunk](3)
	addTextTurn(t, input, "user-1", "what do I like?")
	_ = input.CloseWrite()
	output, err := runtime.Transform(t.Context(), "", input)
	if err != nil {
		t.Fatal(err)
	}
	if got := visibleText(readAll(t, output)); got != "answer" {
		t.Fatalf("visible text = %q, want answer", got)
	}
	modelContext := <-seen
	if got := allPrompts(modelContext); !strings.Contains(got, "likes tea") {
		t.Fatalf("system memory context = %q, want recalled fact", got)
	}
	query, observations := store.snapshot()
	if query.Text != "what do I like?" || query.Limit != 3 {
		t.Fatalf("memory query = %+v", query)
	}
	if len(observations) != 1 || len(observations[0].Turns) != 2 || observations[0].Turns[0].Text != "what do I like?" || observations[0].Turns[1].Text != "answer" {
		t.Fatalf("memory observations = %+v", observations)
	}
}

func TestAgentDefersHistoryUntilExternalOutputIsObserved(t *testing.T) {
	generator := generatorFunc(func(_ context.Context, modelContext genx.ModelContext) (genx.Stream, error) {
		builder := genx.NewGrowableStreamBuilder(modelContext, 1)
		if err := builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("hidden answer")}); err != nil {
			return nil, err
		}
		if err := builder.Done(genx.Usage{}); err != nil {
			return nil, err
		}
		return builder.Stream(), nil
	})
	resolver, err := NewGenXResolver(map[string]GenXModel{"chat": {Generator: generator, Pattern: "test/chat"}})
	if err != nil {
		t.Fatal(err)
	}
	memoryStore := &recordingMemoryStore{}
	runtime, err := New(Config{
		ID: "claw", Conversation: "conversation", HistoryWorkspace: "workspace",
		Graph: textGraph(), Resolver: resolver, Toolkit: commonagent.EmptyToolkit(), Memory: memoryStore,
		ExternalOutputObservation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.history.store != nil {
		t.Fatal("nil History store unexpectedly created persistent history")
	}
	input := buffer.N[*genx.MessageChunk](3)
	addTextTurn(t, input, "user-1", "question")
	_ = input.CloseWrite()
	output, err := runtime.Transform(t.Context(), "", input)
	if err != nil {
		t.Fatal(err)
	}
	if got := visibleText(readAll(t, output)); got != "hidden answer" {
		t.Fatalf("inner visible text = %q", got)
	}
	history, err := runtime.History(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Role != flowmodel.RoleUser {
		t.Fatalf("history before external pull = %#v, want only user", history)
	}

	runtime.BeginOutput("device-stream", "question")
	runtime.ObserveOutput(&genx.MessageChunk{
		Role: genx.RoleModel, Part: genx.Text("visible partial"),
		Ctrl: &genx.StreamCtrl{StreamID: "device-stream", Label: assistantLabel},
	})
	runtime.InterruptOutput("device-stream")
	history, err = runtime.History(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if !hasInterruptedAssistant(history, "visible partial") || slicesContainsMessage(history, "hidden answer") {
		t.Fatalf("history after external interruption = %#v", history)
	}
	_, observations := memoryStore.snapshot()
	if len(observations) != 0 {
		t.Fatalf("interrupted external output observed in Memory: %+v", observations)
	}
}

func TestAgentUsesIndependentHistoryWorkspace(t *testing.T) {
	store := &recordingLogStore{}
	resolver, err := NewGenXResolver(map[string]GenXModel{"chat": {Generator: &scriptedGenerator{}, Pattern: "test/chat"}})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(Config{
		ID: "claw", Conversation: "conversation", HistoryWorkspace: "workspace",
		Graph: textGraph(), Resolver: resolver, Toolkit: commonagent.EmptyToolkit(), History: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.history.store == nil || runtime.history.store.workspace != "workspace" {
		t.Fatalf("history workspace = %#v, want workspace", runtime.history.store)
	}
}

func TestPulledHistoryAcceptsPendingMemoryAndReportsFailedOperation(t *testing.T) {
	store := &recordingMemoryStore{observeResult: memory.ObserveResult{Operation: &memory.Operation{ID: "pending-1", Status: memory.OperationPending}}}
	var reported []error
	pulled := newPulledHistory(&conversationHistory{}, store, func(err error) { reported = append(reported, err) })
	pulled.track("response-1", "hello")
	pulled.observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: "response-1"}})
	pulled.observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "response-1", EndOfStream: true}})
	pulled.mu.Lock()
	if len(pulled.states) != 0 || len(pulled.users) != 0 {
		pulled.mu.Unlock()
		t.Fatalf("completed pulled state retained: states=%d users=%d", len(pulled.states), len(pulled.users))
	}
	pulled.mu.Unlock()
	if len(reported) != 0 {
		t.Fatalf("pending memory operation reported errors: %v", reported)
	}

	store.setObserveResult(memory.ObserveResult{Operation: &memory.Operation{ID: "failed-1", Status: memory.OperationFailed, Error: "extractor unavailable"}}, nil)
	pulled.track("response-2", "again")
	pulled.observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("second"), Ctrl: &genx.StreamCtrl{StreamID: "response-2"}})
	pulled.observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "response-2", EndOfStream: true}})
	if len(reported) != 1 || !strings.Contains(reported[0].Error(), "failed-1") {
		t.Fatalf("reported errors = %v", reported)
	}
	pulled.track("response-3", "interrupt")
	pulled.observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("partial"), Ctrl: &genx.StreamCtrl{StreamID: "response-3"}})
	pulled.commitInterrupted("response-3")
	pulled.mu.Lock()
	defer pulled.mu.Unlock()
	if len(pulled.states) != 0 || len(pulled.users) != 0 {
		t.Fatalf("interrupted pulled state retained: states=%d users=%d", len(pulled.states), len(pulled.users))
	}
}

func TestHistoryStoreReopensSchemaV1InAppendOrder(t *testing.T) {
	store := &recordingLogStore{}
	first, err := newHistoryStore(store, "workspace", "conversation", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.append(t.Context(), []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleUser, "first"),
		flowmodel.NewTextMessage(flowmodel.RoleAssistant, "second"),
	}, false); err != nil {
		t.Fatal(err)
	}
	reopened, err := newHistoryStore(store, "workspace", "conversation", "", "")
	if err != nil {
		t.Fatal(err)
	}
	messages, err := reopened.recent(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Content() != "first" || messages[1].Content() != "second" {
		t.Fatalf("reopened history = %#v", messages)
	}
}

func TestHistoryStoreReadsLegacyScopeAndWritesCurrentScope(t *testing.T) {
	store := &recordingLogStore{}
	legacy, err := newHistoryStore(store, "workspace", "peer", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.append(t.Context(), []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleUser, "legacy user"),
		flowmodel.NewTextMessage(flowmodel.RoleAssistant, "legacy assistant"),
	}, false); err != nil {
		t.Fatal(err)
	}
	scoped, err := newHistoryStore(store, "runtime-scope", "runtime-scope", "workspace", "peer")
	if err != nil {
		t.Fatal(err)
	}
	if err := scoped.append(t.Context(), []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleUser, "scoped user"),
	}, false); err != nil {
		t.Fatal(err)
	}
	messages, err := scoped.recent(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(messages))
	for i := range messages {
		got[i] = messages[i].Content()
	}
	if want := []string{"legacy user", "legacy assistant", "scoped user"}; !slices.Equal(got, want) {
		t.Fatalf("recent() = %v, want %v", got, want)
	}
	store.mu.Lock()
	last := store.records[len(store.records)-1]
	store.mu.Unlock()
	if last.Attributes["workspace_name"] != "runtime-scope" || last.Attributes["conversation_id"] != "runtime-scope" {
		t.Fatalf("scoped record attributes = %#v", last.Attributes)
	}
}

func TestToolSequencerRejectsDuplicateCallIdentity(t *testing.T) {
	sequence := newToolSequencer()
	if err := sequence.record("call-1"); err != nil {
		t.Fatal(err)
	}
	if err := sequence.record("call-1"); !errors.Is(err, commonagent.ErrInvalidToolCall) {
		t.Fatalf("record(duplicate) error = %v, want ErrInvalidToolCall", err)
	}
}

func TestRunHostDoesNotSequenceCanceledSpeculativeToolCall(t *testing.T) {
	host := &runHost{sequence: newToolSequencer(), buffers: make(map[string][]bufferedDelta)}
	publish := func(delta engine.StreamDeltaPayload) {
		t.Helper()
		envelope, err := event.NewEnvelope(t.Context(), engine.SubjectStreamDelta("run", "node"), delta)
		if err != nil {
			t.Fatal(err)
		}
		if err := host.Publish(t.Context(), envelope); err != nil {
			t.Fatal(err)
		}
	}
	publish(engine.StreamDeltaPayload{Type: engine.StreamDeltaToolCall, ID: "canceled", Name: "tool", Speculative: true, ForkID: "fork", BranchID: "branch"})
	publish(engine.StreamDeltaPayload{Type: engine.StreamDeltaParallelBranchCancel, ForkID: "fork", BranchID: "branch"})
	publish(engine.StreamDeltaPayload{Type: engine.StreamDeltaToolCall, ID: "accepted", Name: "tool"})
	publish(engine.StreamDeltaPayload{Type: engine.StreamDeltaToolCall, ID: "speculative-accepted", Name: "tool", Speculative: true, ForkID: "fork-2", BranchID: "branch-2"})
	publish(engine.StreamDeltaPayload{Type: engine.StreamDeltaParallelBranchAccept, ForkID: "fork-2", BranchID: "branch-2"})

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	release, err := host.sequence.acquire(ctx, "accepted")
	if err != nil {
		t.Fatalf("acquire accepted call: %v", err)
	}
	release()
	release, err = host.sequence.acquire(ctx, "speculative-accepted")
	if err != nil {
		t.Fatalf("acquire accepted speculative call: %v", err)
	}
	release()
}

func toolLoopGraph() flowgraph.GraphDefinition {
	return flowgraph.GraphDefinition{
		Name:  "tool-loop",
		Entry: "answer",
		Nodes: []flowgraph.NodeDefinition{{
			ID: "answer", Type: "llm", Config: map[string]any{"model": "chat"},
		}},
		Edges: []flowgraph.EdgeDefinition{
			{From: "answer", To: "answer", Condition: "tool_pending == true"},
			{From: "answer", To: flowgraph.END, Condition: "tool_pending == false"},
		},
	}
}

func textGraph() flowgraph.GraphDefinition {
	return flowgraph.GraphDefinition{
		Name: "text", Entry: "answer",
		Nodes: []flowgraph.NodeDefinition{{ID: "answer", Type: "llm", Config: map[string]any{"model": "chat"}}},
		Edges: []flowgraph.EdgeDefinition{{From: "answer", To: flowgraph.END}},
	}
}

type scriptedGenerator struct {
	mu     sync.Mutex
	inputs []genx.ModelContext
}

func (g *scriptedGenerator) GenerateStream(_ context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
	g.mu.Lock()
	g.inputs = append(g.inputs, modelContext)
	g.mu.Unlock()
	builder := genx.NewGrowableStreamBuilder(modelContext, 1)
	hasResult := false
	for message := range modelContext.Messages() {
		if message.Role == genx.RoleTool {
			hasResult = true
		}
	}
	if hasResult {
		if err := builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("done")}); err != nil {
			return nil, err
		}
	} else {
		if err := builder.Add(
			&genx.MessageChunk{Role: genx.RoleModel, ToolCall: &genx.ToolCall{ID: "call-2", FuncCall: &genx.FuncCall{Name: "second", Arguments: `{"n":2}`}}},
			&genx.MessageChunk{Role: genx.RoleModel, ToolCall: &genx.ToolCall{ID: "call-1", FuncCall: &genx.FuncCall{Name: "first", Arguments: `{"n":1}`}}},
		); err != nil {
			return nil, err
		}
	}
	if err := builder.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return builder.Stream(), nil
}

func (*scriptedGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("unexpected Invoke")
}

type generatorFunc func(context.Context, genx.ModelContext) (genx.Stream, error)

func (f generatorFunc) GenerateStream(ctx context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
	return f(ctx, modelContext)
}

func (generatorFunc) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("unexpected Invoke")
}

type recordingMemoryStore struct {
	mu            sync.Mutex
	recall        memory.RecallResult
	query         memory.Query
	observations  []memory.Observation
	observeResult memory.ObserveResult
	observeErr    error
}

func (s *recordingMemoryStore) Observe(_ context.Context, observation memory.Observation) (memory.ObserveResult, error) {
	s.mu.Lock()
	s.observations = append(s.observations, observation)
	result, err := s.observeResult, s.observeErr
	s.mu.Unlock()
	return result, err
}

func (s *recordingMemoryStore) setObserveResult(result memory.ObserveResult, err error) {
	s.mu.Lock()
	s.observeResult = result
	s.observeErr = err
	s.mu.Unlock()
}

func (s *recordingMemoryStore) Recall(_ context.Context, query memory.Query) (memory.RecallResult, error) {
	s.mu.Lock()
	s.query = query
	s.mu.Unlock()
	return s.recall, nil
}

func (*recordingMemoryStore) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, nil
}

func (*recordingMemoryStore) Delete(context.Context, memory.DeleteRequest) error { return nil }

func (s *recordingMemoryStore) snapshot() (memory.Query, []memory.Observation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.query, slices.Clone(s.observations)
}

type recordingLogStore struct {
	mu      sync.Mutex
	records []logstore.Record
}

func (s *recordingLogStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		s.records = append(s.records, record)
		keys[index] = record.Key()
	}
	return keys, nil
}

func (s *recordingLogStore) Query(_ context.Context, query logstore.Query) (logstore.Page, error) {
	s.mu.Lock()
	records := slices.Clone(s.records)
	s.mu.Unlock()
	records = slices.DeleteFunc(records, func(record logstore.Record) bool {
		if len(query.Streams) > 0 && !slices.Contains(query.Streams, record.Stream) {
			return true
		}
		if len(query.Kinds) > 0 && !slices.Contains(query.Kinds, record.Kind) {
			return true
		}
		for _, matcher := range query.Matchers {
			if matcher.Op == logstore.MatchEqual && record.Attributes[matcher.Name] != matcher.Value {
				return true
			}
		}
		return false
	})
	slices.SortFunc(records, func(left, right logstore.Record) int { return left.Time.Compare(right.Time) })
	if query.Order == logstore.OrderDesc {
		slices.Reverse(records)
	}
	if query.Limit > 0 && len(records) > query.Limit {
		records = records[:query.Limit]
	}
	return logstore.Page{Records: records}, nil
}

func (*recordingLogStore) Replace(context.Context, logstore.Record) error   { return nil }
func (*recordingLogStore) Delete(context.Context, logstore.RecordKey) error { return nil }
func (*recordingLogStore) Close() error                                     { return nil }

func addTextTurn(t *testing.T, input *buffer.Buffer[*genx.MessageChunk], streamID, text string) {
	t.Helper()
	for _, chunk := range []*genx.MessageChunk{
		genx.NewBeginOfStream(streamID),
		{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: streamID}},
		{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
	} {
		if err := input.Add(chunk); err != nil {
			t.Fatal(err)
		}
	}
}

func readAll(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) {
			return chunks
		}
		if err != nil {
			t.Fatalf("Next() error = %v; chunks=%#v", err, chunks)
		}
		chunks = append(chunks, chunk)
	}
}

func visibleText(chunks []*genx.MessageChunk) string {
	var parts []string
	for _, chunk := range chunks {
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok && text != "" {
			parts = append(parts, string(text))
		}
	}
	return strings.Join(parts, "")
}

func lastText(modelContext genx.ModelContext, role genx.Role) string {
	last := ""
	for message := range modelContext.Messages() {
		if message.Role != role {
			continue
		}
		if contents, ok := message.Payload.(genx.Contents); ok {
			for _, part := range contents {
				if text, ok := part.(genx.Text); ok {
					last = string(text)
				}
			}
		}
	}
	return last
}

func allText(modelContext genx.ModelContext, role genx.Role) string {
	var content strings.Builder
	for message := range modelContext.Messages() {
		if message.Role != role {
			continue
		}
		if contents, ok := message.Payload.(genx.Contents); ok {
			for _, part := range contents {
				if text, ok := part.(genx.Text); ok {
					content.WriteString(string(text))
				}
			}
		}
	}
	return content.String()
}

func allPrompts(modelContext genx.ModelContext) string {
	var content strings.Builder
	for prompt := range modelContext.Prompts() {
		content.WriteString(prompt.Text)
	}
	return content.String()
}

func slicesContainsText(chunks []*genx.MessageChunk, want string) bool {
	for _, chunk := range chunks {
		if text, ok := chunk.Part.(genx.Text); ok && string(text) == want {
			return true
		}
	}
	return false
}

func slicesContainsMessage(messages []flowmodel.Message, want string) bool {
	return slices.ContainsFunc(messages, func(message flowmodel.Message) bool {
		return message.Content() == want
	})
}

func hasInterruptedEOS(chunks []*genx.MessageChunk, streamID string) bool {
	for _, chunk := range chunks {
		if chunk.IsEndOfStream() && chunk.Ctrl.StreamID == streamID && chunk.Ctrl.Error == commonagent.Interrupted {
			return true
		}
	}
	return false
}

func hasInterruptedAssistant(messages []flowmodel.Message, content string) bool {
	for _, message := range messages {
		if message.Role != flowmodel.RoleAssistant || message.Content() != content {
			continue
		}
		for _, part := range message.Parts {
			if part.Type == flowmodel.PartData && part.Data != nil && part.Data.MimeType == "application/vnd.gizclaw.interruption+json" {
				return part.Data.Value["interrupted"] == true
			}
		}
	}
	return false
}
