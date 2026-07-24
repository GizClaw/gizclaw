package eino

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func TestBuilderAdversarialDefensiveBoundaries(t *testing.T) {
	t.Parallel()
	for _, format := range []PromptFormat{PromptFString, PromptGoTemplate, PromptJinja2} {
		template, err := buildPrompt(PromptNode{
			Format: format,
			Messages: []PromptMessage{
				{Role: PromptSystem, Template: "system"},
				{Placeholder: "history", Optional: true},
				{Role: PromptUser, Template: "user"},
				{Role: PromptAssistant, Template: "assistant"},
			},
		})
		if err != nil || template == nil {
			t.Fatalf("buildPrompt(%q) = %#v, %v", format, template, err)
		}
	}
	for _, config := range []PromptNode{
		{Format: "unsupported"},
		{
			Format: PromptFString,
			Messages: []PromptMessage{{
				Role: PromptUser, Template: "x", Placeholder: "history",
			}},
		},
		{
			Format:   PromptFString,
			Messages: []PromptMessage{{Role: "unsupported", Template: "x"}},
		},
	} {
		if _, err := buildPrompt(config); err == nil {
			t.Fatalf("buildPrompt(%#v) succeeded", config)
		}
	}

	if _, err := graphState(context.Background()); err == nil {
		t.Fatalf("graphState() error = %v", err)
	}
	if err := (&compiledGraph{}).execute(t.Context(), nil); err == nil {
		t.Fatal("compiledGraph.execute(nil) succeeded")
	}
	if node, err := compileNode(t.Context(), &normalizedConfig{}, NodeDefinition{
		ID: "unsupported",
	}, nil, "Graph"); err == nil || node != nil {
		t.Fatalf("compileNode(unsupported) = %#v, %v", node, err)
	}
	passthrough, err := compileNode(t.Context(), &normalizedConfig{}, NodeDefinition{
		ID: "passthrough", Passthrough: &PassthroughNode{},
	}, nil, "Graph")
	if err != nil {
		t.Fatalf("compileNode(passthrough) error = %v", err)
	}
	state, err := newRunState(nil, graphInput{}, nil, nil)
	if err != nil {
		t.Fatalf("newRunState() error = %v", err)
	}
	if _, _, err := passthrough.run(t.Context(), state); err == nil ||
		!strings.Contains(err.Error(), "no input") {
		t.Fatalf("passthrough.run() error = %v", err)
	}

	inputs := map[string]any{
		"text":     "text",
		"messages": []*schema.Message{schema.UserMessage("message")},
		"parts":    []any{"part"},
	}
	graphInput := graphInputFromNodeInputs(inputs)
	if graphInput.Text != "text" || len(graphInput.Messages) != 1 || len(graphInput.Parts) != 1 {
		t.Fatalf("graphInputFromNodeInputs() = %#v", graphInput)
	}
	emitter := &captureEmitter{}
	output := OutputDefinition{Name: "answer"}
	if err := emitter.Emit(output, "a"); err != nil {
		t.Fatalf("Emit(a) error = %v", err)
	}
	if err := emitter.Emit(output, "b"); err != nil {
		t.Fatalf("Emit(b) error = %v", err)
	}
	if emitter.values["answer"] != "ab" {
		t.Fatalf("capture = %#v", emitter.values)
	}

	component := &fakeChatModel{chunks: []*schema.Message{schema.AssistantMessage("stream", nil)}}
	adapter := &streamingChatModel{component: component}
	reader, err := adapter.Stream(t.Context(), []*schema.Message{schema.UserMessage("input")})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer reader.Close()
	if message, err := reader.Recv(); err != nil || message.Content != "stream" {
		t.Fatalf("Recv() = %#v, %v", message, err)
	}
}

func TestResolvedLambdaValidationAdversarialMatrix(t *testing.T) {
	t.Parallel()
	node := NodeDefinition{
		ID: "lambda", Inputs: map[string]Binding{"input": {From: "input.text"}},
		Outputs: map[string]string{"output": "answer"}, Lambda: &LambdaRefNode{Lambda: "named"},
	}
	fields := map[string]StateField{
		"answer": {Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	lambda := compose.InvokableLambda(
		func(context.Context, map[string]any) (map[string]any, error) {
			return map[string]any{"output": "ok"}, nil
		},
	)
	tests := []ResolvedLambda{
		{},
		{Lambda: lambda},
		{
			Lambda:  lambda,
			Inputs:  map[string]StateType{"input": StateString, "extra": StateString},
			Outputs: map[string]StateType{"output": StateString},
		},
		{
			Lambda:  lambda,
			Inputs:  map[string]StateType{"missing": StateString},
			Outputs: map[string]StateType{"output": StateString},
		},
		{
			Lambda:  lambda,
			Inputs:  map[string]StateType{"input": "unsupported"},
			Outputs: map[string]StateType{"output": StateString},
		},
		{
			Lambda:  lambda,
			Inputs:  map[string]StateType{"input": StateBoolean},
			Outputs: map[string]StateType{"output": StateString},
		},
		{
			Lambda:  lambda,
			Inputs:  map[string]StateType{"input": StateString},
			Outputs: map[string]StateType{"missing": StateString},
		},
		{
			Lambda:  lambda,
			Inputs:  map[string]StateType{"input": StateString},
			Outputs: map[string]StateType{"output": "unsupported"},
		},
		{
			Lambda:  lambda,
			Inputs:  map[string]StateType{"input": StateString},
			Outputs: map[string]StateType{"output": StateBoolean},
		},
	}
	for index, resolved := range tests {
		if err := validateResolvedLambda(node, resolved, fields); err == nil {
			t.Fatalf("validateResolvedLambda(case %d) succeeded", index)
		}
	}
	if err := validateResolvedLambda(node, ResolvedLambda{
		Lambda:  lambda,
		Inputs:  map[string]StateType{"input": StateString},
		Outputs: map[string]StateType{"output": StateString},
	}, fields); err != nil {
		t.Fatalf("validateResolvedLambda(valid) error = %v", err)
	}
}

func TestNativeComponentResolutionFailures(t *testing.T) {
	t.Parallel()
	resolverErr := errors.New("resolver failed")
	config := &normalizedConfig{Config: Config{
		Components: adversarialComponentResolver{err: resolverErr},
	}}
	for _, node := range []NodeDefinition{
		{ID: "chat", ChatModel: &ChatModelNode{Model: "chat"}},
		{ID: "retriever", Retriever: &RetrieverNode{Retriever: "store"}},
	} {
		if _, err := addNativeComponentNode(t.Context(), config, node, nil); err == nil ||
			!strings.Contains(err.Error(), resolverErr.Error()) {
			t.Fatalf("addNativeComponentNode(%q) error = %v", node.ID, err)
		}
	}
	config.Components = adversarialComponentResolver{}
	for _, node := range []NodeDefinition{
		{ID: "chat", ChatModel: &ChatModelNode{Model: "chat"}},
		{ID: "retriever", Retriever: &RetrieverNode{Retriever: "store"}},
	} {
		if _, err := addNativeComponentNode(t.Context(), config, node, nil); err == nil ||
			!strings.Contains(err.Error(), "resolved nil") {
			t.Fatalf("addNativeComponentNode(%q nil) error = %v", node.ID, err)
		}
	}
	if graph, err := addNativeComponentNode(t.Context(), config, NodeDefinition{
		ID: "plain", Transform: &TransformNode{Operation: TransformSelect},
	}, nil); err != nil || graph != nil {
		t.Fatalf("addNativeComponentNode(plain) = %#v, %v", graph, err)
	}
}

func TestGraphExecutionPromptFormatsAndChatOptions(t *testing.T) {
	t.Parallel()
	temperature := float32(0.25)
	maxTokens := 16
	for _, test := range []struct {
		name     string
		format   PromptFormat
		template string
	}{
		{name: "f-string", format: PromptFString, template: "{text}"},
		{name: "go-template", format: PromptGoTemplate, template: "{{.text}}"},
		{name: "jinja2", format: PromptJinja2, template: "{{text}}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			chat := &fakeChatModel{chunks: []*schema.Message{
				schema.AssistantMessage("answer", nil),
			}}
			config := chatConfig(&componentMapResolver{chat: chat})
			config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{
				Name: "model_message", Type: StateMessages, Merge: MergeReplace,
			})
			config.Graph.Nodes[0].Prompt.Format = test.format
			config.Graph.Nodes[0].Prompt.Messages = []PromptMessage{
				{Role: PromptSystem, Template: "system"},
				{Placeholder: "history", Optional: true},
				{Role: PromptUser, Template: test.template},
			}
			config.Graph.Nodes[0].Inputs["history"] = Binding{From: "history.messages"}
			config.Graph.Nodes[1].Outputs["messages"] = "model_message"
			config.Graph.Nodes[1].ChatModel.Temperature = &temperature
			config.Graph.Nodes[1].ChatModel.MaxTokens = &maxTokens
			transformer, err := New(t.Context(), config)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			output, err := transformer.Transform(t.Context(), textInput("question"))
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			if got := joinedText(drain(t, output)); got != "answer" {
				t.Fatalf("output = %q", got)
			}
		})
	}
}

type adversarialComponentResolver struct {
	err error
}

func (resolver adversarialComponentResolver) ResolveChatModel(
	context.Context,
	string,
) (model.BaseChatModel, error) {
	return nil, resolver.err
}

func (resolver adversarialComponentResolver) ResolveRetriever(
	context.Context,
	string,
) (retriever.Retriever, error) {
	return nil, resolver.err
}
