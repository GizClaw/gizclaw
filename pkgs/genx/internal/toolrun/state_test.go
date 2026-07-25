package toolrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type recordingInvoker struct {
	name      string
	arguments json.RawMessage
	result    json.RawMessage
	err       error
}

func (*recordingInvoker) ResolveTools(context.Context) ([]genx.ToolDefinition, error) {
	return nil, nil
}

func (invoker *recordingInvoker) InvokeTool(
	_ context.Context,
	name string,
	arguments json.RawMessage,
) (json.RawMessage, error) {
	invoker.name = name
	invoker.arguments = append(invoker.arguments[:0], arguments...)
	return append(json.RawMessage(nil), invoker.result...), invoker.err
}

func TestStateKeepsProviderCallIDOutsideToolInvoker(t *testing.T) {
	invoker := &recordingInvoker{result: json.RawMessage(`{"ok":true}`)}
	state := New(invoker, 1)
	result, err := state.Invoke(t.Context(), genx.ToolCall{
		ID: " provider-call ",
		FuncCall: &genx.FuncCall{
			Name: " lookup ", Arguments: `{"key":"value"}`,
		},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.ID != "provider-call" || result.Result != `{"ok":true}` {
		t.Fatalf("result = %#v", result)
	}
	if invoker.name != "lookup" || string(invoker.arguments) != `{"key":"value"}` {
		t.Fatalf("invoker call = %q %q", invoker.name, invoker.arguments)
	}
}

func TestStateRejectsInvalidInvokerJSON(t *testing.T) {
	state := New(&recordingInvoker{result: json.RawMessage(`not-json`)}, 1)
	_, err := state.Invoke(t.Context(), genx.ToolCall{
		ID: "call", FuncCall: &genx.FuncCall{Name: "lookup", Arguments: `{}`},
	})
	if !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("Invoke() error = %v", err)
	}
}

func TestStateRejectsMalformedCallsAndPropagatesInvokerError(t *testing.T) {
	invokerErr := errors.New("invocation failed")
	tests := []struct {
		name    string
		state   *State
		call    genx.ToolCall
		wantErr error
	}{
		{name: "nil state", wantErr: ErrInvalidCall},
		{
			name: "blank ID", state: New(&recordingInvoker{}, 1),
			call:    genx.ToolCall{FuncCall: &genx.FuncCall{Name: "lookup"}},
			wantErr: ErrInvalidCall,
		},
		{
			name: "missing function", state: New(&recordingInvoker{}, 1),
			call: genx.ToolCall{ID: "call"}, wantErr: ErrInvalidCall,
		},
		{
			name: "blank function name", state: New(&recordingInvoker{}, 1),
			call:    genx.ToolCall{ID: "call", FuncCall: &genx.FuncCall{}},
			wantErr: ErrInvalidCall,
		},
		{
			name:  "invoker error",
			state: New(&recordingInvoker{err: invokerErr}, 1),
			call: genx.ToolCall{
				ID: "call", FuncCall: &genx.FuncCall{Name: "lookup", Arguments: `{}`},
			},
			wantErr: invokerErr,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.state.Invoke(t.Context(), test.call); !errors.Is(err, test.wantErr) {
				t.Fatalf("Invoke() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestStateRejectsCancellationAndLateResult(t *testing.T) {
	invoker := &recordingInvoker{result: json.RawMessage(`{"ok":true}`)}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := New(invoker, 1).Invoke(cancelled, genx.ToolCall{
		ID: "cancelled", FuncCall: &genx.FuncCall{Name: "lookup", Arguments: `{}`},
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Invoke() error = %v", err)
	}
	if invoker.name != "" {
		t.Fatalf("cancelled InvokeTool() name = %q", invoker.name)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	late := &blockingInvoker{
		started: started,
		release: release,
		result:  json.RawMessage(`{"too_late":true}`),
	}
	ctx, stop := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := New(late, 1).Invoke(ctx, genx.ToolCall{
			ID: "late", FuncCall: &genx.FuncCall{Name: "lookup", Arguments: `{}`},
		})
		result <- err
	}()
	<-started
	stop()
	close(release)
	if err := <-result; !errors.Is(err, context.Canceled) ||
		!strings.Contains(err.Error(), "discard late") {
		t.Fatalf("late Invoke() error = %v", err)
	}
}

type blockingInvoker struct {
	started chan struct{}
	release chan struct{}
	result  json.RawMessage
}

func (*blockingInvoker) ResolveTools(context.Context) ([]genx.ToolDefinition, error) {
	return nil, nil
}

func (invoker *blockingInvoker) InvokeTool(
	context.Context,
	string,
	json.RawMessage,
) (json.RawMessage, error) {
	close(invoker.started)
	<-invoker.release
	return invoker.result, nil
}

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
	if got := FromContext(nil); got != nil {
		t.Fatalf("FromContext(nil) = %p", got)
	}
	background := WithContext(nil, nil)
	if background == nil || FromContext(background) != nil {
		t.Fatalf("WithContext(nil, nil) = %#v", background)
	}
	if got := New(nil, 1); got != nil {
		t.Fatalf("New(nil, 1) = %p", got)
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
	}); !errors.Is(err, ErrInvalidCall) {
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
