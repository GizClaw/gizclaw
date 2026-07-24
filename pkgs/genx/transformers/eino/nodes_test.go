package eino

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
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

func TestChatModelRejectsToolCalls(t *testing.T) {
	t.Parallel()
	chat := &fakeChatModel{chunks: []*schema.Message{{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID: "call-1",
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
	if !strings.Contains(terminalError, "ToolCalls are not supported") {
		t.Fatalf("terminal error = %q", terminalError)
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
