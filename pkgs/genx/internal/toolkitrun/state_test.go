package toolkitrun

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestStateTracksInvocationLocalIdentityAndLimit(t *testing.T) {
	tool, err := genx.NewFuncTool[map[string]any](
		"echo",
		"echo",
		genx.InvokeFunc[map[string]any](func(_ context.Context, _ *genx.FuncCall, value map[string]any) (any, error) {
			return value, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewFuncTool() error = %v", err)
	}
	toolkit, err := genx.NewToolkit(tool)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}
	state := New(toolkit, 2)
	call := func(id string) genx.ToolCall {
		return genx.ToolCall{ID: id, FuncCall: &genx.FuncCall{Name: "echo", Arguments: `{}`}}
	}
	if _, err := state.Invoke(t.Context(), call("one")); err != nil {
		t.Fatalf("first Invoke() error = %v", err)
	}
	if _, err := state.Invoke(t.Context(), call("one")); !errors.Is(err, ErrDuplicateCallID) {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := state.Invoke(t.Context(), call("two")); err != nil {
		t.Fatalf("second Invoke() error = %v", err)
	}
	if _, err := state.Invoke(t.Context(), call("three")); !errors.Is(err, ErrCallLimit) {
		t.Fatalf("limit error = %v", err)
	}

	other := New(toolkit, 1)
	if _, err := other.Invoke(t.Context(), call("one")); err != nil {
		t.Fatalf("same ID in another invocation = %v", err)
	}
}

func TestWithContextPreservesRootState(t *testing.T) {
	first := &State{}
	second := &State{}
	ctx := WithContext(t.Context(), first)
	if got := FromContext(ctx); got != first {
		t.Fatalf("FromContext() = %p, want %p", got, first)
	}
	if got := FromContext(WithContext(ctx, second)); got != first {
		t.Fatalf("nested FromContext() = %p, want %p", got, first)
	}
}

func TestStateUsesDefaultLimitAndInvalidCallsDoNotConsumeIt(t *testing.T) {
	tool, err := genx.NewFuncTool[struct{}]("echo", "echo")
	if err != nil {
		t.Fatalf("NewFuncTool() error = %v", err)
	}
	toolkit, err := genx.NewToolkit(tool)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}
	state := New(toolkit, 0)
	if _, err := state.Invoke(t.Context(), genx.ToolCall{
		ID: " ", FuncCall: &genx.FuncCall{Name: "echo", Arguments: `{}`},
	}); !errors.Is(err, genx.ErrInvalidToolkit) {
		t.Fatalf("blank call error = %v", err)
	}
	for index := range genx.DefaultMaxToolCalls {
		if _, err := state.Invoke(t.Context(), genx.ToolCall{
			ID:       fmt.Sprintf("call-%d", index),
			FuncCall: &genx.FuncCall{Name: "echo", Arguments: `{}`},
		}); err != nil {
			t.Fatalf("Invoke(%d) error = %v", index, err)
		}
	}
	if _, err := state.Invoke(t.Context(), genx.ToolCall{
		ID: "overflow", FuncCall: &genx.FuncCall{Name: "echo", Arguments: `{}`},
	}); !errors.Is(err, ErrCallLimit) {
		t.Fatalf("overflow error = %v", err)
	}
}
