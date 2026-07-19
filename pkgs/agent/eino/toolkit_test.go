package eino

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/cloudwego/eino/schema"
)

func TestNativeToolBalancesHistoryWhenInvocationFails(t *testing.T) {
	wantErr := errors.New("device unavailable")
	history := &conversationHistory{}
	tool := &nativeTool{
		declaration: commonagent.Tool{Name: "device_call"},
		toolkit: commonagent.ToolkitFunc{InvokeFunc: func(context.Context, commonagent.ToolCall) (commonagent.ToolResult, error) {
			return commonagent.ToolResult{}, wantErr
		}},
		history: history,
	}
	if _, err := tool.run(t.Context(), "call-1", `{"value":1}`); !errors.Is(err, wantErr) {
		t.Fatalf("run() error = %v, want %v", err, wantErr)
	}
	messages, err := history.recent(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != schema.Assistant || messages[1].Role != schema.Tool {
		t.Fatalf("history = %#v, want assistant/tool pair", messages)
	}
	if len(messages[0].ToolCalls) != 1 || messages[0].ToolCalls[0].ID != "call-1" || messages[1].ToolCallID != "call-1" {
		t.Fatalf("tool call pairing = %#v / %#v", messages[0], messages[1])
	}
	if !json.Valid([]byte(messages[1].Content)) || !strings.Contains(messages[1].Content, "device unavailable") {
		t.Fatalf("error tool content = %q", messages[1].Content)
	}
}

func TestNativeToolBalancesHistoryWhenArgumentsAreInvalid(t *testing.T) {
	history := &conversationHistory{}
	tool := &nativeTool{
		declaration: commonagent.Tool{Name: "device_call"},
		toolkit:     commonagent.EmptyToolkit(),
		history:     history,
	}
	if _, err := tool.run(t.Context(), "call-1", `{invalid`); err == nil {
		t.Fatal("run() error = nil")
	}
	messages, err := history.recent(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].Role != schema.Tool || messages[1].ToolCallID != "call-1" {
		t.Fatalf("history = %#v, want balanced tool failure", messages)
	}
}

func TestNativeToolDropsHistoryAfterContextCancellation(t *testing.T) {
	history := &conversationHistory{}
	tool := &nativeTool{
		declaration: commonagent.Tool{Name: "device_call"},
		toolkit: commonagent.ToolkitFunc{InvokeFunc: func(ctx context.Context, call commonagent.ToolCall) (commonagent.ToolResult, error) {
			return commonagent.ToolResult{}, ctx.Err()
		}},
		history: history,
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := tool.run(ctx, "call-1", `{"value":1}`); !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want context.Canceled", err)
	}
	messages, err := history.recent(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("history after pre-invoke cancellation = %#v, want empty", messages)
	}

	ctx, cancel = context.WithCancel(t.Context())
	tool.toolkit = commonagent.ToolkitFunc{InvokeFunc: func(context.Context, commonagent.ToolCall) (commonagent.ToolResult, error) {
		cancel()
		return commonagent.ToolResult{}, context.Canceled
	}}
	if _, err := tool.run(ctx, "call-2", `{"value":2}`); !errors.Is(err, context.Canceled) {
		t.Fatalf("run(after invoke cancellation) error = %v, want context.Canceled", err)
	}
	messages, err = history.recent(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("history after invoke cancellation = %#v, want empty", messages)
	}
}
