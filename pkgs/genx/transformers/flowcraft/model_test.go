package flowcraft

import (
	"context"
	"strings"
	"testing"

	flowllm "github.com/GizClaw/flowcraft/sdk/llm"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
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
