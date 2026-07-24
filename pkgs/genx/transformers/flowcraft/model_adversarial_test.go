package flowcraft

import (
	"context"
	"errors"
	"strings"
	"testing"

	flowllm "github.com/GizClaw/flowcraft/sdk/llm"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestResolveLLMValidatesAliasesAndGenerator(t *testing.T) {
	t.Parallel()
	if _, err := ResolveLLM(nil, "chat"); err == nil || !strings.Contains(err.Error(), "Generator") {
		t.Fatalf("ResolveLLM(nil) error = %v", err)
	}
	for _, alias := range []string{"", " ", "provider/model"} {
		if _, err := ResolveLLM(&echoGenerator{}, alias); err == nil {
			t.Fatalf("ResolveLLM(%q) succeeded", alias)
		}
	}
	model, err := ResolveLLM(&echoGenerator{}, " chat ")
	if err != nil {
		t.Fatalf("ResolveLLM(valid) error = %v", err)
	}
	if model == nil {
		t.Fatal("ResolveLLM(valid) returned nil")
	}
	(&modelResolver{}).InvalidateCache()
}

func TestBuildModelContextCoversParametersAndRejectsUnsupportedSurface(t *testing.T) {
	t.Parallel()
	maxTokens := int64(123)
	temperature := 0.25
	topP := 0.8
	topK := int64(17)
	frequency := 0.2
	presence := 0.3
	thinking := true
	jsonMode := true
	options := &flowllm.GenerateOptions{
		MaxTokens: &maxTokens, Temperature: &temperature, TopP: &topP, TopK: &topK,
		FrequencyPenalty: &frequency, PresencePenalty: &presence, Thinking: &thinking,
		JSONMode: &jsonMode, Extra: map[string]any{"provider": "value"},
	}
	modelContext, err := buildModelContext([]flowmodel.Message{
		flowmodel.NewTextMessage(flowmodel.RoleSystem, "system"),
		flowmodel.NewTextMessage(flowmodel.RoleUser, "user"),
		flowmodel.NewTextMessage(flowmodel.RoleAssistant, "assistant"),
	}, options)
	if err != nil {
		t.Fatalf("buildModelContext(parameters) error = %v", err)
	}
	if modelContext == nil {
		t.Fatal("buildModelContext(parameters) returned nil")
	}
	options.Extra["provider"] = "mutated"
	if got := modelContext.Params().ExtraFields["provider"]; got != "value" {
		t.Fatalf("model context extra field = %#v", got)
	}

	tests := []struct {
		name     string
		messages []flowmodel.Message
		options  *flowllm.GenerateOptions
		wantErr  string
	}{
		{
			name: "tools", options: &flowllm.GenerateOptions{
				Tools: []flowllm.ToolDefinition{{Name: "tool"}},
			}, wantErr: "tool calls",
		},
		{
			name: "tool choice", options: &flowllm.GenerateOptions{
				ToolChoice: &flowllm.ToolChoice{Type: flowllm.ToolChoiceAuto},
			}, wantErr: "tool calls",
		},
		{
			name: "stop words", options: &flowllm.GenerateOptions{
				StopWords: []string{"stop"},
			}, wantErr: "stop words",
		},
		{
			name: "image generation", options: &flowllm.GenerateOptions{
				ImageGen: &flowllm.ImageGenOptions{},
			}, wantErr: "image generation",
		},
		{
			name: "non text part",
			messages: []flowmodel.Message{{
				Role:  flowmodel.RoleUser,
				Parts: []flowmodel.Part{{Type: flowmodel.PartImage}},
			}},
			wantErr: "unsupported model message part",
		},
		{
			name: "unknown role",
			messages: []flowmodel.Message{{
				Role:  "critic",
				Parts: []flowmodel.Part{{Type: flowmodel.PartText, Text: "no"}},
			}},
			wantErr: "unsupported model message role",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := buildModelContext(test.messages, test.options); err == nil ||
				!strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("buildModelContext() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestStructuredOutputRejectsMissingSchemaAndProviderFailures(t *testing.T) {
	t.Parallel()
	if _, err := structuredOutputTool(nil); err == nil {
		t.Fatal("structuredOutputTool(nil) succeeded")
	}
	if _, err := convertJSONSchema(make(chan int)); err == nil ||
		!strings.Contains(err.Error(), "encode") {
		t.Fatalf("convertJSONSchema(channel) error = %v", err)
	}
	schema := &jsonschema.Schema{Type: "object"}
	converted, err := convertJSONSchema(schema)
	if err != nil || converted != schema {
		t.Fatalf("convertJSONSchema(schema) = %#v, %v", converted, err)
	}
	tool, err := structuredOutputTool(&flowllm.JSONSchemaParam{Schema: map[string]any{"type": "object"}})
	if err != nil {
		t.Fatalf("structuredOutputTool(defaults) error = %v", err)
	}
	if tool.Name != "flowcraft_structured_output" || tool.Description == "" {
		t.Fatalf("structured output defaults = %#v", tool)
	}

	for _, test := range []struct {
		name      string
		generator *structuredFailureGenerator
		wantErr   string
	}{
		{name: "invoke failure", generator: &structuredFailureGenerator{err: errors.New("invoke failed")}, wantErr: "invoke failed"},
		{name: "empty call", generator: &structuredFailureGenerator{}, wantErr: "output is empty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := &genXLLM{generator: test.generator, pattern: "model/structured"}
			_, err := model.GenerateStream(t.Context(), nil, flowllm.WithJSONSchema(flowllm.JSONSchemaParam{
				Schema: map[string]any{"type": "object"},
			}))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("GenerateStream() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestGenXLLMAndStreamRejectProviderProtocolViolations(t *testing.T) {
	t.Parallel()
	model := &genXLLM{
		generator: &failingGraphGenerator{openErr: errors.New("open failed")},
		pattern:   "model/chat",
	}
	if _, _, err := model.Generate(t.Context(), nil); err == nil ||
		!strings.Contains(err.Error(), "open failed") {
		t.Fatalf("Generate(open failure) error = %v", err)
	}

	tests := []struct {
		name    string
		chunk   *genx.MessageChunk
		wantErr string
	}{
		{
			name: "tool call",
			chunk: &genx.MessageChunk{
				Role: genx.RoleModel, ToolCall: &genx.ToolCall{},
			},
			wantErr: "tool calls",
		},
		{
			name: "non text",
			chunk: &genx.MessageChunk{
				Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "application/octet-stream"},
			},
			wantErr: "non-text",
		},
		{
			name: "terminal error without text",
			chunk: &genx.MessageChunk{
				Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{EndOfStream: true, Error: "terminal failed"},
			},
			wantErr: "terminal failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
			if err := builder.Add(test.chunk); err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			_ = builder.Done(genx.Usage{})
			stream := &genXStream{stream: builder.Stream()}
			if stream.Next() || stream.Err() == nil || !strings.Contains(stream.Err().Error(), test.wantErr) {
				t.Fatalf("Next()=%v Err()=%v, want containing %q", stream.Current(), stream.Err(), test.wantErr)
			}
		})
	}
}

type structuredFailureGenerator struct {
	err error
}

func (*structuredFailureGenerator) GenerateStream(
	context.Context,
	string,
	genx.ModelContext,
) (genx.Stream, error) {
	return nil, errors.New("unexpected GenerateStream")
}

func (generator *structuredFailureGenerator) Invoke(
	context.Context,
	string,
	genx.ModelContext,
	*genx.FuncTool,
) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, generator.err
}
