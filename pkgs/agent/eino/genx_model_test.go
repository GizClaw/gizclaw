package eino

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/schema"
)

func TestGenXChatModelPreservesToolsMessagesAndStreamingCalls(t *testing.T) {
	generator := &capturingGenerator{}
	base, err := NewGenXChatModel(generator, "model/chat")
	if err != nil {
		t.Fatal(err)
	}
	toolInfo := &schema.ToolInfo{
		Name: "weather", Desc: "Look up weather",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"city": {Type: schema.String, Required: true},
		}),
	}
	boundModel, err := base.WithTools([]*schema.ToolInfo{toolInfo})
	if err != nil {
		t.Fatal(err)
	}
	bound := boundModel.(*GenXChatModel)
	if len(base.tools) != 0 || len(bound.tools) != 1 {
		t.Fatalf("WithTools mutated base: base=%d bound=%d", len(base.tools), len(bound.tools))
	}
	toolInfo.Name = "mutated"
	if bound.tools[0].Name != "weather" {
		t.Fatalf("WithTools retained caller-owned ToolInfo: %#v", bound.tools[0])
	}

	stream, err := bound.Stream(t.Context(), []*schema.Message{
		schema.SystemMessage("be concise"),
		schema.UserMessage("weather?"),
		schema.AssistantMessage("", []schema.ToolCall{{ID: "old-call", Type: "function", Function: schema.FunctionCall{Name: "weather", Arguments: `{"city":"Paris"}`}}}),
		schema.ToolMessage(`{"temperature":20}`, "old-call", schema.WithToolName("weather")),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var messages []*schema.Message
	for {
		message, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message)
	}
	if len(messages) != 2 || messages[0].Content != "partial" || len(messages[1].ToolCalls) != 1 || messages[1].ToolCalls[0].ID != "new-call" {
		t.Fatalf("stream messages = %#v", messages)
	}

	pattern, modelContext := generator.snapshot()
	if pattern != "model/chat" {
		t.Fatalf("pattern = %q", pattern)
	}
	var prompt string
	for item := range modelContext.Prompts() {
		prompt += item.Text
	}
	if prompt != "be concise" {
		t.Fatalf("prompt = %q", prompt)
	}
	var gotTool *genx.FuncTool
	for tool := range modelContext.Tools() {
		gotTool, _ = tool.(*genx.FuncTool)
	}
	if gotTool == nil || gotTool.Name != "weather" || gotTool.Argument == nil || gotTool.Argument.Properties["city"] == nil || len(gotTool.Argument.Required) != 1 || gotTool.Argument.Required[0] != "city" {
		t.Fatalf("GenX tool = %#v", gotTool)
	}
	var sawOldCall, sawOldResult bool
	for message := range modelContext.Messages() {
		switch payload := message.Payload.(type) {
		case *genx.ToolCall:
			sawOldCall = payload.ID == "old-call" && payload.FuncCall.Name == "weather"
		case *genx.ToolResult:
			sawOldResult = payload.ID == "old-call" && payload.Result == `{"temperature":20}`
		}
	}
	if !sawOldCall || !sawOldResult {
		t.Fatalf("converted model context lost call/result: call=%v result=%v", sawOldCall, sawOldResult)
	}
}

type capturingGenerator struct {
	pattern string
	context genx.ModelContext
}

func (g *capturingGenerator) GenerateStream(_ context.Context, pattern string, modelContext genx.ModelContext) (genx.Stream, error) {
	g.pattern = pattern
	g.context = modelContext
	builder := genx.NewGrowableStreamBuilder(modelContext, 2)
	if err := builder.Add(
		&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("partial")},
		&genx.MessageChunk{Role: genx.RoleModel, ToolCall: &genx.ToolCall{ID: "new-call", FuncCall: &genx.FuncCall{Name: "weather", Arguments: `{"city":"Rome"}`}}},
	); err != nil {
		return nil, err
	}
	if err := builder.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return builder.Stream(), nil
}

func (*capturingGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("unexpected Invoke")
}

func (g *capturingGenerator) snapshot() (string, genx.ModelContext) { return g.pattern, g.context }
