package eino

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/jsonschema-go/jsonschema"
)

func intentOutput(t testing.TB) *StructuredOutput {
	t.Helper()
	var definition jsonschema.Schema
	if err := json.Unmarshal([]byte(`{"type":"object","additionalProperties":false,"required":["mode"],"properties":{"mode":{"type":"string","enum":["plot","research"]}}}`), &definition); err != nil {
		t.Fatal(err)
	}
	return &StructuredOutput{Name: "turn_intent", Schema: &definition}
}

func TestStructuredControlPreservesNativeAudioHistoryAndHiddenPublication(t *testing.T) {
	control := &structuredTestModel{reply: schema.AssistantMessage(`{"mode":"research"}`, nil)}
	reply := &fakeChatModel{chunks: []*schema.Message{
		schema.AssistantMessage("hello ", nil), schema.AssistantMessage("world", nil),
	}}
	config := audioInputConfig(&namedComponentResolver{chat: map[string]model.BaseChatModel{
		"control": control, "reply": reply,
	}})
	config.Graph.Nodes[1].ChatModel.Model = "reply"
	definition := intentOutput(t)
	config.Graph.Nodes = append(config.Graph.Nodes, NodeDefinition{
		ID: "control", Inputs: map[string]Binding{"messages": {From: "messages"}},
		Outputs:   map[string]string{"text": "answer"},
		ChatModel: &ChatModelNode{Model: "control", StructuredOutput: definition},
	}, NodeDefinition{
		ID: "reply-prompt", Inputs: map[string]Binding{"control": {From: "answer"}, "conversation": {From: "input.messages"}},
		Outputs: map[string]string{"messages": "messages"},
		Prompt: &PromptNode{Format: PromptFString, Messages: []PromptMessage{
			{Role: PromptSystem, Template: "Role selection: {control}"}, {Placeholder: "conversation"},
		}},
	})
	config.Graph.Edges = []EdgeDefinition{{From: "start", To: "prompt"}, {From: "prompt", To: "control"},
		{From: "control", To: "reply-prompt"}, {From: "reply-prompt", To: "model"}, {From: "model", To: "end"}}
	if len(config.Graph.State.Fields) != 2 {
		t.Fatal("control must reuse the existing message/answer fields")
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	// Mutation after construction must not alter the immutable Graph schema.
	definition.Schema.Properties["mode"].Enum = []any{"invalid"}
	previous, err := transformer.Transform(t.Context(), textInput("earlier context"))
	if err != nil {
		t.Fatal(err)
	}
	if got := joinedText(drain(t, previous)); got != "hello world" {
		t.Fatalf("hidden control leaked into output: %q", got)
	}
	audio := bytes.Repeat([]byte{1, 2, 3, 4}, 100000)
	output, err := transformer.Transform(t.Context(), audioInput("native-turn", [][]byte{audio}))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	if got := joinedText(drain(t, output)); got != "hello world" {
		t.Fatalf("hidden control leaked into output: %q", got)
	}
	if len(control.inputs) != 2 {
		t.Fatalf("structured calls = %d", len(control.inputs))
	}
	for _, input := range [][]*schema.Message{control.inputs[1], reply.inputs[1]} {
		if len(input) != 4 || input[1].Content != "earlier context" || input[2].Content != "hello world" {
			t.Fatalf("History lost, reordered or polluted by control metadata: %#v", input)
		}
		last := input[len(input)-1]
		if last.Role != schema.User || len(last.UserInputMultiContent) != 1 {
			t.Fatal("current native audio was not preserved as the final user message")
		}
		data, err := base64.StdEncoding.DecodeString(*last.UserInputMultiContent[0].Audio.Base64Data)
		if err != nil || !slices.Equal(data, audio) {
			t.Fatalf("original audio changed: decode=%v bytes=%d", err, len(data))
		}
	}
}

func TestStructuredOutputRejectsInvalidRepliesBeforePublication(t *testing.T) {
	for _, test := range []struct{ name, reply string }{
		{"single quotes", `{'mode':'plot'}`},
		{"wrong enum", `{"mode":"invented"}`},
		{"extra property", `{"mode":"plot","story":"leak"}`},
		{"duplicate key", `{"mode":"research","mode":"plot"}`},
		{"trailing prose", `{"mode":"plot"} and a story`},
		{"truncated", `{"mode":`},
		{"wrong type", `{"mode":5}`},
		{"oversized", `{"mode":"` + strings.Repeat("x", 300) + `"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			chat := &structuredTestModel{reply: schema.AssistantMessage(test.reply, nil)}
			config := chatConfig(&componentMapResolver{chat: chat})
			config.Graph.Nodes[1].ChatModel.StructuredOutput = intentOutput(t)
			config.Limits.MaxOutputBytes = 256
			transformer, err := New(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer transformer.Close()
			output, err := transformer.Transform(t.Context(), textInput("request"))
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			text, err := structuredResult(output)
			if err == nil || text != "" || len(chat.inputs) != 1 {
				t.Fatalf("invalid response was published/repaired/retried: text=%q error=%v calls=%d", text, err, len(chat.inputs))
			}
		})
	}
}

func TestStructuredOutputRequiresCapabilityAndValidSchema(t *testing.T) {
	config := chatConfig(&componentMapResolver{chat: &fakeChatModel{}})
	config.Graph.Nodes[1].ChatModel.StructuredOutput = intentOutput(t)
	if _, err := New(t.Context(), config); err == nil || !strings.Contains(err.Error(), "does not support structured output") {
		t.Fatalf("unsupported component: %v", err)
	}
	config.Components = &componentMapResolver{chat: &structuredTestModel{}}
	for _, invalid := range []*StructuredOutput{
		{Name: "bad name", Schema: intentOutput(t).Schema},
		{Name: "x"},
		{Name: "x", Schema: &jsonschema.Schema{Type: "string"}},
		{Name: "x", Schema: &jsonschema.Schema{Type: "object", Ref: "#/missing"}},
	} {
		config.Graph.Nodes[1].ChatModel.StructuredOutput = invalid
		if _, err := New(t.Context(), config); err == nil {
			t.Fatal("invalid structured schema was accepted")
		}
	}
}

func TestStructuredOutputDoesNotResolveRuntimeTools(t *testing.T) {
	chat := &structuredTestModel{reply: schema.AssistantMessage(`{"mode":"plot"}`, nil)}
	config := chatConfig(&componentMapResolver{chat: chat})
	config.Graph.Nodes[1].ChatModel.StructuredOutput = intentOutput(t)
	config.ToolInvoker = &structuredForbiddenTools{}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	output, err := transformer.Transform(t.Context(), textInput("request"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	if got := joinedText(drain(t, output)); got != `{"mode":"plot"}` {
		t.Fatalf("structured response = %q", got)
	}
}

func TestStructuredOutputRejectsToolVerificationInsteadOfBypassingIt(t *testing.T) {
	config := chatConfig(&componentMapResolver{chat: &structuredTestModel{}})
	config.Graph.Nodes[1].ChatModel.StructuredOutput = intentOutput(t)
	config.ToolInvoker = &structuredForbiddenTools{}
	config.VerifyToolResponse = func(context.Context, genx.ToolConversation, string) (string, error) {
		return "reject", nil
	}
	if _, err := New(t.Context(), config); err == nil || !strings.Contains(err.Error(), "does not support VerifyToolResponse") {
		t.Fatalf("structured output silently bypassed Tool verification: %v", err)
	}
}

func TestStructuredOutputPropagatesCancellationWithoutPublication(t *testing.T) {
	chat := &structuredTestModel{entered: make(chan struct{})}
	config := chatConfig(&componentMapResolver{chat: chat})
	config.Graph.Nodes[1].ChatModel.StructuredOutput = intentOutput(t)
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, err := transformer.Transform(ctx, textInput("request"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	done := make(chan error, 1)
	go func() { _, err := structuredResult(output); done <- err }()
	select {
	case <-chat.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("structured model was not invoked")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("structured invocation ignored cancellation")
	}
}

type structuredTestModel struct {
	fakeChatModel
	reply   *schema.Message
	inputs  [][]*schema.Message
	entered chan struct{}
}

func (chat *structuredTestModel) GenerateStructured(ctx context.Context, input []*schema.Message, _ StructuredOutput, _ ...model.Option) (*schema.Message, error) {
	chat.inputs = append(chat.inputs, cloneMessages(input))
	if chat.entered != nil {
		close(chat.entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return chat.reply, nil
}

type structuredForbiddenTools struct{}

func (*structuredForbiddenTools) ResolveTools(context.Context) ([]genx.ToolDefinition, error) {
	return nil, errors.New("structured control must not resolve runtime Tools")
}

func (*structuredForbiddenTools) InvokeTool(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("structured control must not invoke runtime Tools")
}

func structuredResult(stream genx.Stream) (string, error) {
	var text strings.Builder
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return text.String(), nil
			}
			return text.String(), err
		}
		if chunk == nil {
			continue
		}
		if err := genx.StreamError(chunk.Ctrl); err != nil {
			return text.String(), err
		}
		if value, ok := chunk.Part.(genx.Text); ok {
			text.WriteString(string(value))
		}
	}
}
