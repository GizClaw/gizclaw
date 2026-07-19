package dashscoperealtime

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/GizClaw/dashscope-realtime-go"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
)

func TestNewAcceptsOnlyQwen35FunctionCallModels(t *testing.T) {
	inherited, err := New(Config{Transformer: testTransformer{}, Pattern: "model/demo", Toolkit: commonagent.EmptyToolkit()})
	if err != nil {
		t.Fatalf("New(inherited model) error = %v", err)
	}
	if inherited.config.Model != "" {
		t.Fatalf("inherited model = %q, want transformer-resolved model", inherited.config.Model)
	}
	for _, model := range []string{
		dashscope.ModelQwen35OmniPlusRealtime,
		dashscope.ModelQwen35OmniPlusRealtime20260315,
		dashscope.ModelQwen35OmniFlashRealtime,
		dashscope.ModelQwen35OmniFlashRealtime20260315,
	} {
		if _, err := New(Config{Transformer: testTransformer{}, Pattern: "model/demo", Model: model, Toolkit: commonagent.EmptyToolkit()}); err != nil {
			t.Fatalf("New(%q) error = %v", model, err)
		}
	}
	if _, err := New(Config{Transformer: testTransformer{}, Pattern: "model/demo", Model: dashscope.ModelQwen3OmniFlashRealtime, Toolkit: commonagent.EmptyToolkit()}); err == nil {
		t.Fatal("New(qwen3 model) succeeded")
	}
}

func TestInvokePreservesOutputIndexOrder(t *testing.T) {
	var order []string
	toolkit := commonagent.ToolkitFunc{
		List: func() []commonagent.Tool { return []commonagent.Tool{{Name: "first"}, {Name: "second"}} },
		InvokeFunc: func(_ context.Context, call commonagent.ToolCall) (commonagent.ToolResult, error) {
			order = append(order, call.ID)
			return commonagent.ToolResult{ID: call.ID, Content: json.RawMessage(`{"ok":true}`)}, nil
		},
	}
	agent, err := New(Config{Transformer: testTransformer{}, Pattern: "model/demo", Toolkit: toolkit})
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := agent.invoke(t.Context(), []transformers.DashScopeRealtimeFunctionCall{
		{CallID: "call-2", Name: "second", Arguments: `{}`, OutputIndex: 0},
		{CallID: "call-1", Name: "first", Arguments: `{}`, OutputIndex: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(order, []string{"call-2", "call-1"}) || outputs[0].CallID != "call-2" || outputs[1].CallID != "call-1" {
		t.Fatalf("order=%v outputs=%#v", order, outputs)
	}
}

func TestTransformUsesConfiguredPattern(t *testing.T) {
	transformer := &recordingTransformer{}
	agent, err := New(Config{Transformer: transformer, Pattern: "model/qwen", Toolkit: commonagent.EmptyToolkit()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agent.Transform(t.Context(), "ignored", emptyStream{}); err != nil {
		t.Fatal(err)
	}
	if transformer.pattern != "model/qwen" {
		t.Fatalf("pattern = %q", transformer.pattern)
	}
}

func TestProviderToolsRejectUnsupportedSchema(t *testing.T) {
	toolkit := commonagent.ToolkitFunc{List: func() []commonagent.Tool {
		return []commonagent.Tool{{Name: "bad", InputSchema: nil}}
	}}
	agent, err := New(Config{Transformer: testTransformer{}, Pattern: "model/demo", Toolkit: toolkit})
	if err != nil || len(agent.tools) != 1 || !strings.EqualFold(agent.tools[0].Function.Name, "bad") {
		t.Fatalf("New() agent=%#v err=%v", agent, err)
	}
}

type testTransformer struct{}

func (testTransformer) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return emptyStream{}, nil
}

type recordingTransformer struct{ pattern string }

func (t *recordingTransformer) Transform(_ context.Context, pattern string, _ genx.Stream) (genx.Stream, error) {
	t.pattern = pattern
	return emptyStream{}, nil
}

type emptyStream struct{}

func (emptyStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }
func (emptyStream) Close() error                      { return nil }
func (emptyStream) CloseWithError(error) error        { return nil }
