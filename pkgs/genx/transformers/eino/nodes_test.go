package eino

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestChatModelUsesNativeComponentAndStreams(t *testing.T) {
	t.Parallel()
	chat := &fakeChatModel{chunks: []*schema.Message{
		{Role: schema.Assistant, Content: "hel"},
		{Role: schema.Assistant, Content: "lo"},
	}}
	config := chatConfig(&componentMapResolver{chat: chat})
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("world"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drain(t, output)
	if got := joinedText(chunks); got != "hello" {
		t.Fatalf("output = %q", got)
	}
	chat.mu.Lock()
	defer chat.mu.Unlock()
	if len(chat.inputs) != 1 || len(chat.inputs[0]) != 2 {
		t.Fatalf("model inputs = %#v", chat.inputs)
	}
	if chat.inputs[0][0].Role != schema.System || chat.inputs[0][1].Content != "world" {
		t.Fatalf("model messages = %#v", chat.inputs[0])
	}
}

func TestRetrieverUsesNativeComponent(t *testing.T) {
	t.Parallel()
	store := &fakeRetriever{documents: []*schema.Document{{ID: "one", Content: "knowledge"}}}
	resolver := &componentMapResolver{retriever: store}
	config := textConfig()
	config.Components = resolver
	config.Graph.State.Fields = []StateField{
		{Name: "documents", Type: StateDocuments, Merge: MergeReplace},
		{Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{
		{
			ID: "retrieve", Outputs: map[string]string{"documents": "documents"},
			Retriever: &RetrieverNode{
				Retriever: "knowledge", Query: Binding{From: "input.text"}, TopK: 3,
			},
		},
		{
			ID: "answer", Inputs: map[string]Binding{"value": {From: "input.text"}},
			Outputs: map[string]string{"value": "answer"}, Transform: &TransformNode{Operation: TransformSelect},
		},
	}
	config.Graph.Edges = []EdgeDefinition{
		{From: "start", To: "retrieve"}, {From: "retrieve", To: "answer"}, {From: "answer", To: "end"},
	}
	config.Graph.Outputs[0].Node = "answer"
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("query"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "query" {
		t.Fatalf("output = %q", got)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.queries) != 1 || store.queries[0] != "query" || store.topK[0] != 3 {
		t.Fatalf("retriever calls = %v topK=%v", store.queries, store.topK)
	}
}

func TestRaceSelectsSuccessfulNestedGraph(t *testing.T) {
	t.Parallel()
	good := childTextGraph("good", &TransformNode{
		Operation: TransformConcatText, Order: []string{"input"},
	})
	bad := childTextGraph("bad", nil)
	bad.Nodes[0].Transform = nil
	bad.Nodes[0].Script = &ScriptNode{
		Language: ScriptStarlark, Source: "def run(input):\n  return 1 / 0\n",
		Limits: ScriptLimits{
			MaxExecutionSteps: 1000, Timeout: time.Second, MaxInputBytes: 1024, MaxOutputBytes: 1024,
		},
	}
	config := textConfig()
	config.Graph.Nodes[0] = NodeDefinition{
		ID: "answer", Inputs: map[string]Binding{"text": {From: "input.text"}},
		Outputs: map[string]string{"answer": "answer"},
		Race: &RaceNode{
			Branches: []RaceBranch{{ID: "bad", Graph: bad}, {ID: "good", Graph: good}},
			Winner:   RaceWinnerDefinition{Mode: RaceFirstSuccess}, MaxConcurrency: 2,
		},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("winner"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "winner" {
		t.Fatalf("output = %q", got)
	}
}

func TestBatchPreservesInputOrder(t *testing.T) {
	t.Parallel()
	child := GraphDefinition{
		Name: "item",
		State: StateDefinition{Fields: []StateField{
			{Name: "item", Type: StateString, Merge: MergeReplace},
			{Name: "answer", Type: StateString, Merge: MergeReplace},
		}},
		Nodes: []NodeDefinition{{
			ID: "answer", Inputs: map[string]Binding{"value": {From: "item"}},
			Outputs: map[string]string{"value": "answer"}, Transform: &TransformNode{Operation: TransformSelect},
		}},
		Edges: []EdgeDefinition{{From: "start", To: "answer"}, {From: "answer", To: "end"}},
		Outputs: []OutputDefinition{{
			Node: "answer", Field: "answer", Name: "answer", MIMEType: "text/plain", Primary: true,
		}},
	}
	config := textConfig()
	config.Graph.State.Fields = []StateField{
		{Name: "items", Type: StateList, Merge: MergeReplace},
		{Name: "results", Type: StateList, Merge: MergeReplace},
		{Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{
		{
			ID: "items", Outputs: map[string]string{"items": "items"},
			Script: &ScriptNode{
				Language: ScriptStarlark, Source: "def run(input):\n  return {\"items\": [\"a\", \"b\", \"c\"]}\n",
				Limits: ScriptLimits{
					MaxExecutionSteps: 1000, Timeout: time.Second, MaxInputBytes: 1024, MaxOutputBytes: 1024,
				},
			},
		},
		{
			ID: "batch", Outputs: map[string]string{"items": "results"},
			Batch: &BatchNode{Items: Binding{From: "items"}, Graph: child, MaxConcurrency: 2},
		},
		{
			ID: "answer", Inputs: map[string]Binding{"items": {From: "results"}},
			Outputs: map[string]string{"text": "answer"},
			Script: &ScriptNode{
				Language: ScriptStarlark,
				Source:   "def run(input):\n  return {\"text\": \"|\".join(input[\"items\"])}\n",
				Limits: ScriptLimits{
					MaxExecutionSteps: 1000, Timeout: time.Second, MaxInputBytes: 1024, MaxOutputBytes: 1024,
				},
			},
		},
	}
	config.Graph.Edges = []EdgeDefinition{
		{From: "start", To: "items"}, {From: "items", To: "batch"},
		{From: "batch", To: "answer"}, {From: "answer", To: "end"},
	}
	config.Graph.Outputs[0].Node = "answer"
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("ignored"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "a|b|c" {
		t.Fatalf("output = %q", got)
	}
}

func TestSubgraphUsesDeclaredInputsAndOutputs(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Graph.Nodes[0] = NodeDefinition{
		ID:      "answer",
		Inputs:  map[string]Binding{"text": {From: "input.text"}},
		Outputs: map[string]string{"answer": "answer"},
		Subgraph: &SubgraphNode{
			Graph: childTextGraph("child", &TransformNode{
				Operation: TransformConcatText, Order: []string{"input"},
			}),
		},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("nested"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "nested" {
		t.Fatalf("output = %q", got)
	}
}

func TestRaceSelectsPredicateWinner(t *testing.T) {
	t.Parallel()
	branch := func(name, value string) RaceBranch {
		graph := childTextGraph(name, nil)
		graph.Nodes[0].Script = constantTextScript(value)
		return RaceBranch{ID: name, Graph: graph}
	}
	config := textConfig()
	config.Graph.Nodes[0] = NodeDefinition{
		ID:      "answer",
		Inputs:  map[string]Binding{"text": {From: "input.text"}},
		Outputs: map[string]string{"answer": "answer"},
		Race: &RaceNode{
			Branches: []RaceBranch{branch("red", "red"), branch("blue", "blue")},
			Winner: RaceWinnerDefinition{
				Mode: RacePredicate,
				When: &Predicate{Field: "answer", Op: PredicateEqual, Value: "blue"},
			},
			MaxConcurrency: 2,
		},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("ignored"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "blue" {
		t.Fatalf("output = %q", got)
	}
}

func TestChatModelRejectsToolCallsWithoutToolkit(t *testing.T) {
	t.Parallel()
	chat := &fakeChatModel{chunks: []*schema.Message{{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID: "call-1", Function: schema.FunctionCall{Name: "lookup", Arguments: `{}`},
		}},
	}}}
	transformer, err := New(t.Context(), chatConfig(&componentMapResolver{chat: chat}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("tool"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	var terminalError string
	for _, chunk := range drain(t, output) {
		if chunk.IsEndOfStream() && chunk.Ctrl != nil {
			terminalError = chunk.Ctrl.Error
		}
	}
	if !strings.Contains(terminalError, "ToolCalls but Toolkit is not configured") {
		t.Fatalf("terminal error = %q", terminalError)
	}
}

func TestChatModelExecutesToolsAndContinues(t *testing.T) {
	t.Parallel()
	var invoked atomic.Int32
	toolkit := einoTestToolkit(t, func(value string) (any, error) {
		invoked.Add(1)
		return map[string]string{"found": value}, nil
	})
	chat := &scriptedChatModel{mutateTool: true, rounds: [][]*schema.Message{
		{{
			Role: schema.Assistant, Content: "checking ",
			ToolCalls: []schema.ToolCall{
				{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"alpha"}`}},
				{ID: "call-2", Type: "function", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"beta"}`}},
			},
		}},
		{{Role: schema.Assistant, Content: "done"}},
	}}
	config := chatConfig(&componentMapResolver{chat: chat})
	config.Toolkit = toolkit
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("tool"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "checking done" {
		t.Fatalf("output = %q", got)
	}
	if invoked.Load() != 2 {
		t.Fatalf("tool invocations = %d", invoked.Load())
	}

	chat.mu.Lock()
	defer chat.mu.Unlock()
	if len(chat.inputs) != 2 {
		t.Fatalf("model rounds = %d", len(chat.inputs))
	}
	continued := chat.inputs[1]
	if len(continued) != 5 {
		t.Fatalf("continuation messages = %#v", continued)
	}
	if continued[2].Role != schema.Assistant || len(continued[2].ToolCalls) != 2 {
		t.Fatalf("assistant ToolCalls = %#v", continued[2])
	}
	if continued[3].Role != schema.Tool || continued[3].ToolCallID != "call-1" ||
		continued[3].ToolName != "lookup" || continued[3].Content != `{"found":"alpha"}` {
		t.Fatalf("first ToolResult = %#v", continued[3])
	}
	if continued[4].Role != schema.Tool || continued[4].ToolCallID != "call-2" ||
		continued[4].Content != `{"found":"beta"}` {
		t.Fatalf("second ToolResult = %#v", continued[4])
	}
	for round, tools := range chat.toolDetails {
		if !slices.Equal(tools, []string{"lookup|looks up a value|schema"}) {
			t.Fatalf("round %d tools = %#v", round, tools)
		}
	}
}

func TestChatModelToolCallGuards(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		maximum   int
		calls     []schema.ToolCall
		wantError string
	}{
		{
			name: "duplicate ID",
			calls: []schema.ToolCall{
				{ID: "same", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"one"}`}},
				{ID: "same", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"two"}`}},
			},
			wantError: "duplicate ToolCall ID",
		},
		{
			name: "invocation limit", maximum: 1,
			calls: []schema.ToolCall{
				{ID: "one", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"one"}`}},
				{ID: "two", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"two"}`}},
			},
			wantError: "ToolCall limit exceeded",
		},
		{
			name: "blank ID",
			calls: []schema.ToolCall{
				{ID: " ", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"one"}`}},
			},
			wantError: "call ID is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			toolkit := einoTestToolkit(t, func(value string) (any, error) {
				return value, nil
			})
			chat := &scriptedChatModel{rounds: [][]*schema.Message{{{
				Role: schema.Assistant, ToolCalls: test.calls,
			}}}}
			config := chatConfig(&componentMapResolver{chat: chat})
			config.Toolkit = toolkit
			config.MaxToolCalls = test.maximum
			transformer, err := New(t.Context(), config)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			output, err := transformer.Transform(t.Context(), textInput("tool"))
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			var terminalError string
			for _, chunk := range drain(t, output) {
				if chunk.IsEndOfStream() && chunk.Ctrl != nil {
					terminalError = chunk.Ctrl.Error
				}
			}
			if !strings.Contains(terminalError, test.wantError) {
				t.Fatalf("terminal error = %q, want %q", terminalError, test.wantError)
			}
		})
	}
}

func TestChatModelConcurrentToolkitIsolation(t *testing.T) {
	t.Parallel()
	var active atomic.Int32
	var maximum atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	toolkit := einoTestToolkit(t, func(value string) (any, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			seen := maximum.Load()
			if current <= seen || maximum.CompareAndSwap(seen, current) {
				break
			}
		}
		if current >= 2 {
			releaseOnce.Do(func() { close(release) })
		}
		select {
		case <-release:
		case <-t.Context().Done():
			return nil, t.Context().Err()
		}
		return map[string]string{"found": value}, nil
	})
	chat := &concurrentToolChatModel{}
	config := chatConfig(&componentMapResolver{chat: chat})
	config.Toolkit = toolkit
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	const count = 8
	var wait sync.WaitGroup
	failures := make(chan error, count)
	for index := range count {
		wait.Go(func() {
			input := fmt.Sprintf("tool-run-%d", index)
			output, transformErr := transformer.Transform(t.Context(), textInput(input))
			if transformErr != nil {
				failures <- transformErr
				return
			}
			if got := joinedText(drain(t, output)); got != input {
				failures <- fmt.Errorf("output %q does not belong to %q", got, input)
			}
		})
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
	if maximum.Load() < 2 {
		t.Fatalf("maximum concurrent tool calls = %d", maximum.Load())
	}
	if chat.missingTools.Load() != 0 {
		t.Fatalf("model rounds without Toolkit = %d", chat.missingTools.Load())
	}
}

func chatConfig(resolver ComponentResolver) Config {
	config := textConfig()
	config.Components = resolver
	config.Graph.State.Fields = []StateField{
		{Name: "messages", Type: StateMessages, Merge: MergeReplace},
		{Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{
		{
			ID: "prompt", Inputs: map[string]Binding{"text": {From: "input.text"}},
			Outputs: map[string]string{"messages": "messages"},
			Prompt: &PromptNode{
				Format: PromptFString,
				Messages: []PromptMessage{
					{Role: PromptSystem, Template: "system"},
					{Role: PromptUser, Template: "{text}"},
				},
			},
		},
		{
			ID: "model", Inputs: map[string]Binding{"messages": {From: "messages"}},
			Outputs:   map[string]string{"text": "answer"},
			ChatModel: &ChatModelNode{Model: "chat"},
		},
	}
	config.Graph.Edges = []EdgeDefinition{
		{From: "start", To: "prompt"}, {From: "prompt", To: "model"}, {From: "model", To: "end"},
	}
	config.Graph.Outputs[0].Node = "model"
	return config
}

func childTextGraph(name string, transform *TransformNode) GraphDefinition {
	return GraphDefinition{
		Name:  name,
		State: StateDefinition{Fields: []StateField{{Name: "answer", Type: StateString, Merge: MergeReplace}}},
		Nodes: []NodeDefinition{{
			ID: "answer", Inputs: map[string]Binding{"input": {From: "input.text"}},
			Outputs: map[string]string{"text": "answer"}, Transform: transform,
		}},
		Edges: []EdgeDefinition{{From: "start", To: "answer"}, {From: "answer", To: "end"}},
		Outputs: []OutputDefinition{{
			Node: "answer", Field: "answer", Name: "answer", MIMEType: "text/plain", Primary: true,
		}},
	}
}

type componentMapResolver struct {
	chat      model.BaseChatModel
	retriever retriever.Retriever
}

func (resolver *componentMapResolver) ResolveChatModel(context.Context, string) (model.BaseChatModel, error) {
	return resolver.chat, nil
}

func (resolver *componentMapResolver) ResolveRetriever(context.Context, string) (retriever.Retriever, error) {
	return resolver.retriever, nil
}

type fakeChatModel struct {
	mu     sync.Mutex
	chunks []*schema.Message
	inputs [][]*schema.Message
}

type scriptedChatModel struct {
	mu          sync.Mutex
	rounds      [][]*schema.Message
	inputs      [][]*schema.Message
	toolDetails [][]string
	mutateTool  bool
}

type concurrentToolChatModel struct {
	missingTools atomic.Int32
}

func (chat *concurrentToolChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	reader, err := chat.Stream(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return reader.Recv()
}

func (chat *concurrentToolChatModel) Stream(
	_ context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	if len(model.GetCommonOptions(nil, options...).Tools) != 1 {
		chat.missingTools.Add(1)
	}
	var userText string
	var toolResult string
	for _, message := range input {
		if message == nil {
			continue
		}
		switch message.Role {
		case schema.User:
			userText = message.Content
		case schema.Tool:
			toolResult = message.Content
		}
	}
	if toolResult == "" {
		arguments, err := json.Marshal(map[string]string{"value": userText})
		if err != nil {
			return nil, err
		}
		return schema.StreamReaderFromArray([]*schema.Message{{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID: "same-provider-id", Type: "function",
				Function: schema.FunctionCall{Name: "lookup", Arguments: string(arguments)},
			}},
		}}), nil
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(toolResult), &result); err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{
		schema.AssistantMessage(result["found"], nil),
	}), nil
}

func (chat *scriptedChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	reader, err := chat.Stream(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var chunks []*schema.Message
	for {
		chunk, recvErr := reader.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return nil, recvErr
		}
		chunks = append(chunks, chunk)
	}
	return schema.ConcatMessages(chunks)
}

func (chat *scriptedChatModel) Stream(
	_ context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	chat.mu.Lock()
	defer chat.mu.Unlock()
	index := len(chat.inputs)
	if index >= len(chat.rounds) {
		return nil, fmt.Errorf("unexpected model round %d", index)
	}
	config := model.GetCommonOptions(nil, options...)
	chat.inputs = append(chat.inputs, cloneMessages(input))
	details := make([]string, 0, len(config.Tools))
	for _, tool := range config.Tools {
		suffix := "no-schema"
		if tool != nil && tool.ParamsOneOf != nil {
			suffix = "schema"
		}
		if tool == nil {
			details = append(details, "<nil>")
			continue
		}
		details = append(details, tool.Name+"|"+tool.Desc+"|"+suffix)
	}
	chat.toolDetails = append(chat.toolDetails, details)
	if chat.mutateTool && index == 0 && len(config.Tools) > 0 {
		config.Tools[0].Name = "mutated"
	}
	return schema.StreamReaderFromArray(cloneMessages(chat.rounds[index])), nil
}

func einoTestToolkit(
	t *testing.T,
	invoke func(string) (any, error),
) *genx.Toolkit {
	t.Helper()
	tool := &genx.FuncTool{
		Name: "lookup", Description: "looks up a value",
		Argument: &jsonschema.Schema{
			Type:                 "object",
			Required:             []string{"value"},
			Properties:           map[string]*jsonschema.Schema{"value": {Type: "string"}},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Invoke: func(_ context.Context, _ *genx.FuncCall, arguments string) (any, error) {
			var input struct {
				Value string `json:"value"`
			}
			if err := json.Unmarshal([]byte(arguments), &input); err != nil {
				return nil, err
			}
			return invoke(input.Value)
		},
	}
	toolkit, err := genx.NewToolkit(tool)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}
	return toolkit
}

func (chat *fakeChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	reader, err := chat.Stream(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var chunks []*schema.Message
	for {
		chunk, recvErr := reader.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return nil, recvErr
		}
		chunks = append(chunks, chunk)
	}
	return schema.ConcatMessages(chunks)
}

func (chat *fakeChatModel) Stream(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	chat.mu.Lock()
	chat.inputs = append(chat.inputs, cloneMessages(input))
	chunks := cloneMessages(chat.chunks)
	chat.mu.Unlock()
	return schema.StreamReaderFromArray(chunks), nil
}

type fakeRetriever struct {
	mu        sync.Mutex
	documents []*schema.Document
	queries   []string
	topK      []int
}

func (store *fakeRetriever) Retrieve(_ context.Context, query string, options ...retriever.Option) ([]*schema.Document, error) {
	config := retriever.GetCommonOptions(nil, options...)
	topK := 0
	if config.TopK != nil {
		topK = *config.TopK
	}
	store.mu.Lock()
	store.queries = append(store.queries, query)
	store.topK = append(store.topK, topK)
	store.mu.Unlock()
	return store.documents, nil
}
