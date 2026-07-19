package agent

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestInvokeToolCallsPreservesStrictOrder(t *testing.T) {
	var invoked []string
	toolkit := ToolkitFunc{
		InvokeFunc: func(_ context.Context, call ToolCall) (ToolResult, error) {
			invoked = append(invoked, call.ID)
			return ToolResult{ID: call.ID, Content: json.RawMessage(`{"ok":true}`)}, nil
		},
	}
	calls := []ToolCall{
		{ID: "call-2", Name: "second", Arguments: json.RawMessage(`{"n":2}`)},
		{ID: "call-1", Name: "first", Arguments: json.RawMessage(`{"n":1}`)},
	}

	results, err := InvokeToolCalls(t.Context(), toolkit, calls, ToolLoopConfig{MaxCalls: 2})
	if err != nil {
		t.Fatalf("InvokeToolCalls() error = %v", err)
	}
	if !slices.Equal(invoked, []string{"call-2", "call-1"}) {
		t.Fatalf("invoke order = %v", invoked)
	}
	if got := []string{results[0].ID, results[1].ID}; !slices.Equal(got, invoked) {
		t.Fatalf("result order = %v, want %v", got, invoked)
	}
}

func TestInvokeToolCallsStopsOnProtocolFailure(t *testing.T) {
	invocations := 0
	toolkit := ToolkitFunc{
		InvokeFunc: func(_ context.Context, call ToolCall) (ToolResult, error) {
			invocations++
			return ToolResult{ID: "wrong-call", Content: json.RawMessage(`null`)}, nil
		},
	}
	calls := []ToolCall{
		{ID: "call-1", Name: "first", Arguments: json.RawMessage(`{}`)},
		{ID: "call-2", Name: "second", Arguments: json.RawMessage(`{}`)},
	}

	_, err := InvokeToolCalls(t.Context(), toolkit, calls, ToolLoopConfig{})
	if !errors.Is(err, ErrInvalidToolCall) {
		t.Fatalf("InvokeToolCalls() error = %v, want ErrInvalidToolCall", err)
	}
	if invocations != 1 {
		t.Fatalf("invocations = %d, want 1", invocations)
	}
}

func TestInvokeToolCallsEnforcesLimitBeforeInvocation(t *testing.T) {
	invoked := false
	toolkit := ToolkitFunc{InvokeFunc: func(context.Context, ToolCall) (ToolResult, error) {
		invoked = true
		return ToolResult{}, nil
	}}
	calls := []ToolCall{
		{ID: "call-1", Name: "first", Arguments: json.RawMessage(`{}`)},
		{ID: "call-2", Name: "second", Arguments: json.RawMessage(`{}`)},
	}

	_, err := InvokeToolCalls(t.Context(), toolkit, calls, ToolLoopConfig{MaxCalls: 1})
	if !errors.Is(err, ErrToolCallLimit) {
		t.Fatalf("InvokeToolCalls() error = %v, want ErrToolCallLimit", err)
	}
	if invoked {
		t.Fatal("Toolkit was invoked after call-limit failure")
	}
}

func TestInvokeToolCallsRejectsDuplicateIDsBeforeInvocation(t *testing.T) {
	invoked := false
	toolkit := ToolkitFunc{InvokeFunc: func(context.Context, ToolCall) (ToolResult, error) {
		invoked = true
		return ToolResult{}, nil
	}}
	calls := []ToolCall{
		{ID: "duplicate", Name: "first", Arguments: json.RawMessage(`{}`)},
		{ID: "duplicate", Name: "second", Arguments: json.RawMessage(`{}`)},
	}

	_, err := InvokeToolCalls(t.Context(), toolkit, calls, ToolLoopConfig{})
	if !errors.Is(err, ErrInvalidToolCall) {
		t.Fatalf("InvokeToolCalls() error = %v, want ErrInvalidToolCall", err)
	}
	if invoked {
		t.Fatal("Toolkit was invoked before duplicate call identity was rejected")
	}
}

func TestInvokeToolCallsAppliesPerCallTimeout(t *testing.T) {
	toolkit := ToolkitFunc{InvokeFunc: func(ctx context.Context, call ToolCall) (ToolResult, error) {
		<-ctx.Done()
		return ToolResult{}, ctx.Err()
	}}
	calls := []ToolCall{{ID: "call-1", Name: "slow", Arguments: json.RawMessage(`{}`)}}

	_, err := InvokeToolCalls(t.Context(), toolkit, calls, ToolLoopConfig{Timeout: time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("InvokeToolCalls() error = %v, want deadline exceeded", err)
	}
}

func TestInvokeToolCallsStopsBeforeLaterCallAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var invoked []string
	toolkit := ToolkitFunc{InvokeFunc: func(_ context.Context, call ToolCall) (ToolResult, error) {
		invoked = append(invoked, call.ID)
		cancel()
		return ToolResult{ID: call.ID, Content: json.RawMessage(`null`)}, nil
	}}
	calls := []ToolCall{
		{ID: "call-1", Name: "first", Arguments: json.RawMessage(`{}`)},
		{ID: "call-2", Name: "second", Arguments: json.RawMessage(`{}`)},
	}

	_, err := InvokeToolCalls(ctx, toolkit, calls, ToolLoopConfig{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("InvokeToolCalls() error = %v, want context canceled", err)
	}
	if !slices.Equal(invoked, []string{"call-1"}) {
		t.Fatalf("invoked calls = %v, want first call only", invoked)
	}
}

func TestInvokeToolCallsContinuesAfterStructuredBusinessError(t *testing.T) {
	var invoked []string
	toolkit := ToolkitFunc{InvokeFunc: func(_ context.Context, call ToolCall) (ToolResult, error) {
		invoked = append(invoked, call.ID)
		if call.ID == "call-1" {
			return ErrorToolResult(call.ID, "offline", "device is offline"), nil
		}
		return ToolResult{ID: call.ID, Content: json.RawMessage(`{"ok":true}`)}, nil
	}}
	calls := []ToolCall{
		{ID: "call-1", Name: "first", Arguments: json.RawMessage(`{}`)},
		{ID: "call-2", Name: "second", Arguments: json.RawMessage(`{}`)},
	}

	results, err := InvokeToolCalls(t.Context(), toolkit, calls, ToolLoopConfig{})
	if err != nil {
		t.Fatalf("InvokeToolCalls() error = %v", err)
	}
	if !slices.Equal(invoked, []string{"call-1", "call-2"}) || len(results) != 2 || !results[0].IsError || results[1].IsError {
		t.Fatalf("invoked = %v, results = %#v", invoked, results)
	}
}

func TestInvokeToolCallsRejectsInvalidResultJSON(t *testing.T) {
	toolkit := ToolkitFunc{InvokeFunc: func(_ context.Context, call ToolCall) (ToolResult, error) {
		return ToolResult{ID: call.ID, Content: json.RawMessage(`{`)}, nil
	}}

	_, err := InvokeToolCalls(t.Context(), toolkit, []ToolCall{{ID: "call-1", Name: "bad", Arguments: json.RawMessage(`{}`)}}, ToolLoopConfig{})
	if !errors.Is(err, ErrInvalidToolCall) {
		t.Fatalf("InvokeToolCalls() error = %v, want ErrInvalidToolCall", err)
	}
}

func TestToolkitFuncDefensivelyClonesDeclarationsAndCalls(t *testing.T) {
	schema := &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{"value": {Type: "string"}}}
	tools := []Tool{{ID: "tool-1", Name: "demo", InputSchema: schema}}
	var got ToolCall
	toolkit := ToolkitFunc{
		List: func() []Tool { return tools },
		InvokeFunc: func(_ context.Context, call ToolCall) (ToolResult, error) {
			got = call
			call.Arguments[0] = '['
			return ToolResult{ID: call.ID, Content: json.RawMessage(`null`)}, nil
		},
	}

	listed := toolkit.Tools()
	listed[0].InputSchema.Type = "string"
	if schema.Type != "object" {
		t.Fatal("Tools() returned aliased schema")
	}
	call := ToolCall{ID: "call-1", Name: "demo", Arguments: json.RawMessage(`{}`)}
	if _, err := toolkit.Invoke(t.Context(), call); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if string(call.Arguments) != `{}` || got.ID != call.ID {
		t.Fatalf("Invoke() mutated caller arguments or call identity: call=%s got=%+v", call.Arguments, got)
	}
}

func TestErrorToolResultIsStructuredJSON(t *testing.T) {
	result := ErrorToolResult("call-1", "not_found", "device is offline")
	if result.ID != "call-1" || !result.IsError || !json.Valid(result.Content) {
		t.Fatalf("ErrorToolResult() = %+v", result)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(result.Content, &body); err != nil || body.Error.Code != "not_found" {
		t.Fatalf("error content = %s, err = %v", result.Content, err)
	}
}

func TestEmptyToolkitAndInvalidCalls(t *testing.T) {
	empty := EmptyToolkit()
	if tools := empty.Tools(); len(tools) != 0 {
		t.Fatalf("EmptyToolkit().Tools() = %#v", tools)
	}
	if _, err := empty.Invoke(t.Context(), ToolCall{}); err == nil {
		t.Fatal("EmptyToolkit().Invoke() succeeded")
	}
	for _, call := range []ToolCall{
		{Name: "tool", Arguments: json.RawMessage(`{}`)},
		{ID: "call", Arguments: json.RawMessage(`{}`)},
		{ID: "call", Name: "tool", Arguments: json.RawMessage(`{`)},
	} {
		_, err := InvokeToolCalls(t.Context(), ToolkitFunc{InvokeFunc: func(context.Context, ToolCall) (ToolResult, error) {
			t.Fatal("invalid call reached Toolkit")
			return ToolResult{}, nil
		}}, []ToolCall{call}, ToolLoopConfig{})
		if !errors.Is(err, ErrInvalidToolCall) {
			t.Fatalf("InvokeToolCalls(%+v) error = %v", call, err)
		}
	}
	if _, err := InvokeToolCalls(t.Context(), nil, nil, ToolLoopConfig{}); err == nil {
		t.Fatal("InvokeToolCalls(nil Toolkit) succeeded")
	}
	if _, err := (ToolkitFunc{}).Invoke(t.Context(), ToolCall{}); err == nil {
		t.Fatal("ToolkitFunc{}.Invoke() succeeded")
	}
}
