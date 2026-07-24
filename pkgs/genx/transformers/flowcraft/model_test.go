package flowcraft

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	flowllm "github.com/GizClaw/flowcraft/sdk/llm"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/toolkitrun"
)

type structuredOutputGenerator struct {
	pattern string
	tool    *genx.FuncTool
}

func (*structuredOutputGenerator) GenerateStream(context.Context, string, genx.ModelContext) (genx.Stream, error) {
	panic("GenerateStream must not be used for JSON Schema output")
}

func (g *structuredOutputGenerator) Invoke(_ context.Context, pattern string, _ genx.ModelContext, tool *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	g.pattern = pattern
	g.tool = tool
	return genx.Usage{PromptTokenCount: 11, CachedContentTokenCount: 3, GeneratedTokenCount: 7}, tool.NewFuncCall(`{"facts":[]}`), nil
}

func TestGenXLLMUsesInvokeForFlowcraftJSONSchema(t *testing.T) {
	generator := &structuredOutputGenerator{}
	model := &genXLLM{generator: generator, pattern: "model/memory"}
	message, usage, err := model.Generate(context.Background(), []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleUser, "remember this"),
	}, flowllm.WithJSONSchema(flowllm.JSONSchemaParam{
		Name:        "extracted_facts",
		Description: "facts",
		Strict:      true,
		Schema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"facts"},
			"properties": map[string]any{
				"facts": map[string]any{"type": "array"},
			},
		},
	}), flowllm.WithJSONMode(true))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got := message.Content(); got != `{"facts":[]}` {
		t.Fatalf("message content = %q", got)
	}
	if generator.pattern != "model/memory" {
		t.Fatalf("Invoke pattern = %q", generator.pattern)
	}
	if generator.tool == nil || generator.tool.Name != "extracted_facts" || generator.tool.Description != "facts" {
		t.Fatalf("Invoke tool = %#v", generator.tool)
	}
	if generator.tool.Argument == nil || generator.tool.Argument.Type != "object" || generator.tool.Argument.Properties["facts"].Type != "array" {
		t.Fatalf("Invoke schema = %#v", generator.tool.Argument)
	}
	if usage.InputTokens != 11 || usage.CachedInputTokens != 3 || usage.OutputTokens != 7 || usage.TotalTokens != 18 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestBuildModelContextMakesEmptyUserInputProviderSafe(t *testing.T) {
	messages := []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleSystem, "system"),
		flowmodel.NewTextMessage(flowmodel.RoleUser, ""),
	}
	modelContext, err := buildModelContext(messages, nil)
	if err != nil {
		t.Fatalf("buildModelContext() error = %v", err)
	}
	var userText string
	for message := range modelContext.Messages() {
		if message.Role != genx.RoleUser {
			continue
		}
		for _, part := range message.Payload.(genx.Contents) {
			if text, ok := part.(genx.Text); ok {
				userText += string(text)
			}
		}
	}
	if userText != providerSafeEmptyUserText || strings.TrimSpace(userText) == "" {
		t.Fatalf("provider user text = %q", userText)
	}
	if messages[1].Content() != "" {
		t.Fatalf("source message was mutated: %q", messages[1].Content())
	}
}

func TestGenXStreamPreservesTextOnEOS(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		errorText string
	}{
		{name: "success"},
		{name: "error", errorText: "provider failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
			if err := builder.Add(&genx.MessageChunk{
				Role: genx.RoleModel, Part: genx.Text("final"),
				Ctrl: &genx.StreamCtrl{EndOfStream: true, Error: test.errorText},
			}); err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			_ = builder.Done(genx.Usage{})
			stream := &genXStream{stream: builder.Stream()}
			if !stream.Next() {
				t.Fatalf("Next() = false, error = %v", stream.Err())
			}
			if got := stream.Current(); got.Role != flowmodel.RoleAssistant || got.Content != "final" {
				t.Fatalf("Current() = %#v", got)
			}
			if stream.Next() {
				t.Fatal("second Next() = true")
			}
			if test.errorText == "" && stream.Err() != nil {
				t.Fatalf("Err() = %v", stream.Err())
			}
			if test.errorText != "" && stream.Err() == nil {
				t.Fatal("Err() = nil")
			}
			if test.errorText != "" && stream.Err().Error() != test.errorText {
				t.Fatalf("Err() = %v", stream.Err())
			}
		})
	}
}

type toolRoundGenerator struct {
	mu         sync.Mutex
	rounds     [][]*genx.MessageChunk
	contexts   []genx.ModelContext
	mutateTool bool
}

func (generator *toolRoundGenerator) GenerateStream(_ context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
	generator.mu.Lock()
	defer generator.mu.Unlock()
	generator.contexts = append(generator.contexts, modelContext)
	if generator.mutateTool && len(generator.contexts) == 1 {
		for declared := range modelContext.Tools() {
			if tool, ok := declared.(*genx.FuncTool); ok {
				tool.Name = "mutated"
				tool.Argument.Type = "boolean"
			}
		}
	}
	if len(generator.rounds) == 0 {
		return nil, errors.New("no model round")
	}
	chunks := generator.rounds[0]
	generator.rounds = generator.rounds[1:]
	builder := genx.NewGrowableStreamBuilder(modelContext, len(chunks)+1)
	for _, chunk := range chunks {
		if err := builder.Add(chunk); err != nil {
			return nil, err
		}
	}
	if err := builder.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return builder.Stream(), nil
}

func (*toolRoundGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("Invoke must not be used")
}

func TestGenXLLMExecutesToolkitAndContinuesModelTurn(t *testing.T) {
	var calls []string
	tool, err := genx.NewFuncTool[map[string]string](
		"lookup",
		"lookup a value",
		genx.InvokeFunc[map[string]string](func(_ context.Context, call *genx.FuncCall, arguments map[string]string) (any, error) {
			calls = append(calls, call.Name+":"+arguments["key"])
			return map[string]string{"answer": "found"}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewFuncTool() error = %v", err)
	}
	toolkit, err := genx.NewToolkit(tool)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}
	generator := &toolRoundGenerator{mutateTool: true, rounds: [][]*genx.MessageChunk{
		{
			{Role: genx.RoleModel, Part: genx.Text("before ")},
			{Role: genx.RoleModel, ToolCall: &genx.ToolCall{
				ID: "call-1", FuncCall: &genx.FuncCall{Name: "lookup", Arguments: `{"key":"first"}`},
			}},
			{Role: genx.RoleModel, ToolCall: &genx.ToolCall{
				ID: "call-2", FuncCall: &genx.FuncCall{Name: "lookup", Arguments: `{"key":"second"}`},
			}},
		},
		{{Role: genx.RoleModel, Part: genx.Text("after")}},
	}}
	model := &genXLLM{generator: generator, pattern: "model/chat", toolkit: toolkit}
	ctx := toolkitrun.WithContext(t.Context(), toolkitrun.New(toolkit, 2))
	message, _, err := model.Generate(ctx, []flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleUser, "question"),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got := message.Content(); got != "before after" {
		t.Fatalf("message.Content() = %q", got)
	}
	if len(calls) != 2 || calls[0] != "lookup:first" || calls[1] != "lookup:second" {
		t.Fatalf("calls = %v", calls)
	}
	if len(generator.contexts) != 2 {
		t.Fatalf("model rounds = %d", len(generator.contexts))
	}
	if got := countModelTools(generator.contexts[0]); got != 1 {
		t.Fatalf("first round tools = %d", got)
	}
	for declared := range generator.contexts[1].Tools() {
		tool, ok := declared.(*genx.FuncTool)
		if !ok || tool.Name != "lookup" || tool.Argument.Type != "object" {
			t.Fatalf("second round tool = %#v", declared)
		}
	}
	var toolCallIDs, toolResultIDs, toolResults []string
	for message := range generator.contexts[1].Messages() {
		switch payload := message.Payload.(type) {
		case *genx.ToolCall:
			toolCallIDs = append(toolCallIDs, payload.ID)
		case *genx.ToolResult:
			toolResultIDs = append(toolResultIDs, payload.ID)
			toolResults = append(toolResults, payload.Result)
		}
	}
	if !slices.Equal(toolCallIDs, []string{"call-1", "call-2"}) ||
		!slices.Equal(toolResultIDs, []string{"call-1", "call-2"}) ||
		!slices.Equal(toolResults, []string{`{"answer":"found"}`, `{"answer":"found"}`}) {
		t.Fatalf("continuation calls=%v result IDs=%v results=%v", toolCallIDs, toolResultIDs, toolResults)
	}
}

func TestGenXLLMRejectsInvocationLocalDuplicateAndLimit(t *testing.T) {
	tool, err := genx.NewFuncTool[struct{}](
		"echo",
		"echo",
		genx.InvokeFunc[struct{}](func(context.Context, *genx.FuncCall, struct{}) (any, error) {
			return map[string]bool{"ok": true}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewFuncTool() error = %v", err)
	}
	toolkit, err := genx.NewToolkit(tool)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}
	for _, test := range []struct {
		name     string
		secondID string
		maximum  int
		want     error
	}{
		{name: "duplicate", secondID: "same", maximum: 2, want: toolkitrun.ErrDuplicateCallID},
		{name: "limit", secondID: "second", maximum: 1, want: toolkitrun.ErrCallLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			generator := &toolRoundGenerator{rounds: [][]*genx.MessageChunk{
				{{ToolCall: &genx.ToolCall{ID: "same", FuncCall: &genx.FuncCall{Name: "echo", Arguments: `{}`}}}},
				{{ToolCall: &genx.ToolCall{ID: test.secondID, FuncCall: &genx.FuncCall{Name: "echo", Arguments: `{}`}}}},
			}}
			model := &genXLLM{generator: generator, pattern: "model/chat", toolkit: toolkit}
			ctx := toolkitrun.WithContext(t.Context(), toolkitrun.New(toolkit, test.maximum))
			stream, err := model.GenerateStream(ctx, []flowmodel.Message{
				flowmodel.NewTextMessage(flowmodel.RoleUser, "question"),
			})
			if err != nil {
				t.Fatalf("GenerateStream() error = %v", err)
			}
			defer stream.Close()
			for stream.Next() {
			}
			if !errors.Is(stream.Err(), test.want) {
				t.Fatalf("stream.Err() = %v, want %v", stream.Err(), test.want)
			}
		})
	}
}

func countModelTools(modelContext genx.ModelContext) int {
	count := 0
	for range modelContext.Tools() {
		count++
	}
	return count
}
