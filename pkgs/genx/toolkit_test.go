package genx

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

var _ ToolInvoker = (*Toolkit)(nil)

func TestToolkitHandlesNilReceiverAndResolutionContext(t *testing.T) {
	var toolkit *Toolkit
	if _, err := toolkit.ResolveTools(t.Context()); !errors.Is(err, ErrInvalidToolkit) {
		t.Fatalf("nil ResolveTools() error = %v", err)
	}
	if _, err := toolkit.InvokeTool(
		t.Context(), "lookup", json.RawMessage(`{}`),
	); !errors.Is(err, ErrInvalidToolkit) {
		t.Fatalf("nil InvokeTool() error = %v", err)
	}
	empty, err := NewToolkit()
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}
	if definitions, err := empty.ResolveTools(nil); err != nil || len(definitions) != 0 {
		t.Fatalf("ResolveTools(nil) = %#v, %v", definitions, err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := empty.ResolveTools(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ResolveTools() error = %v", err)
	}
}

func TestToolkitSnapshotsDeclarationsAndPreservesOrder(t *testing.T) {
	first := executableTestTool(t, " first ", func(_ context.Context, _ *FuncCall, arguments map[string]any) (any, error) {
		return arguments, nil
	})
	second := executableTestTool(t, "second", func(context.Context, *FuncCall, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	toolkit, err := NewToolkit(first, second)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}

	first.Name = "mutated"
	first.Argument.Properties["value"].Type = "integer"
	first.Invoke = func(context.Context, *FuncCall, string) (any, error) {
		return nil, errors.New("mutated executor")
	}
	got, err := toolkit.ResolveTools(t.Context())
	if err != nil {
		t.Fatalf("ResolveTools() error = %v", err)
	}
	if len(got) != 2 || got[0].Name != "first" || got[1].Name != "second" {
		t.Fatalf("Tools() = %#v", got)
	}
	got[0].Name = "changed"
	got[0].Argument.Properties["value"].Type = "boolean"
	again, err := toolkit.ResolveTools(t.Context())
	if err != nil {
		t.Fatalf("second ResolveTools() error = %v", err)
	}
	if again[0].Name != "first" || again[0].Argument.Properties["value"].Type != "string" {
		t.Fatalf("Tools() leaked caller mutation: %#v", again[0])
	}
	if _, err := toolkit.InvokeTool(
		t.Context(),
		"first",
		json.RawMessage(`{"value":"still-owned"}`),
	); err != nil {
		t.Fatalf("Invoke() used a mutated executor: %v", err)
	}
}

func TestNewToolkitRejectsInvalidDeclarations(t *testing.T) {
	valid := executableTestTool(t, "tool", func(context.Context, *FuncCall, map[string]any) (any, error) {
		return nil, nil
	})
	tests := []struct {
		name  string
		tools []*FuncTool
	}{
		{name: "nil", tools: []*FuncTool{nil}},
		{name: "blank name", tools: []*FuncTool{{Argument: valid.Argument, Invoke: valid.Invoke}}},
		{name: "missing schema", tools: []*FuncTool{{Name: "tool", Invoke: valid.Invoke}}},
		{name: "missing executor", tools: []*FuncTool{{Name: "tool", Argument: valid.Argument}}},
		{name: "duplicate normalized name", tools: []*FuncTool{valid, executableTestTool(t, " tool ", func(context.Context, *FuncCall, map[string]any) (any, error) {
			return nil, nil
		})}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewToolkit(test.tools...); !errors.Is(err, ErrInvalidToolkit) {
				t.Fatalf("NewToolkit() error = %v, want %v", err, ErrInvalidToolkit)
			}
		})
	}
}

func TestToolkitInvokeValidatesAndSerializes(t *testing.T) {
	var invoked atomic.Int32
	tool := executableTestTool(t, "lookup", func(_ context.Context, call *FuncCall, arguments map[string]any) (any, error) {
		invoked.Add(1)
		if call.Name != "lookup" {
			t.Fatalf("call.Name = %q", call.Name)
		}
		return map[string]any{"value": arguments["value"], "ok": true}, nil
	})
	toolkit, err := NewToolkit(tool)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}
	result, err := toolkit.InvokeTool(
		t.Context(),
		" lookup ",
		json.RawMessage(`{"value":"x"}`),
	)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("result JSON = %q: %v", result, err)
	}
	if decoded["value"] != "x" || decoded["ok"] != true {
		t.Fatalf("result = %#v", decoded)
	}
	if invoked.Load() != 1 {
		t.Fatalf("invocations = %d", invoked.Load())
	}

	invalid := []struct {
		name      string
		arguments json.RawMessage
	}{
		{arguments: json.RawMessage(`{"value":"x"}`)},
		{name: "missing", arguments: json.RawMessage(`{}`)},
		{name: "lookup", arguments: json.RawMessage(`{`)},
		{name: "lookup", arguments: json.RawMessage(`{"value":1}`)},
		{name: "lookup", arguments: json.RawMessage(`{"value":"x"} {}`)},
	}
	for index, call := range invalid {
		if _, err := toolkit.InvokeTool(t.Context(), call.name, call.arguments); err == nil {
			t.Fatalf("invalid call %d succeeded", index)
		}
	}
	if invoked.Load() != 1 {
		t.Fatalf("invalid calls invoked executor: %d", invoked.Load())
	}
}

func TestToolkitInvokePropagatesExecutorAndSerializationErrors(t *testing.T) {
	executorErr := errors.New("executor failed")
	failing := executableTestTool(t, "failing", func(context.Context, *FuncCall, map[string]any) (any, error) {
		return nil, executorErr
	})
	unserializable := executableTestTool(t, "unserializable", func(context.Context, *FuncCall, map[string]any) (any, error) {
		return make(chan int), nil
	})
	toolkit, err := NewToolkit(failing, unserializable)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}
	if _, err := toolkit.InvokeTool(
		t.Context(),
		"failing",
		json.RawMessage(`{"value":"x"}`),
	); !errors.Is(err, executorErr) {
		t.Fatalf("executor error = %v", err)
	}
	if _, err := toolkit.InvokeTool(
		t.Context(),
		"unserializable",
		json.RawMessage(`{"value":"x"}`),
	); err == nil {
		t.Fatal("serialization error = nil")
	}
}

func TestToolkitInvokePreservesBusinessValuesAndContextErrors(t *testing.T) {
	type businessFailure struct {
		Code    string `json:"code"`
		Allowed bool   `json:"allowed"`
	}
	business := executableTestTool(t, "business", func(context.Context, *FuncCall, map[string]any) (any, error) {
		return businessFailure{Code: "denied", Allowed: false}, nil
	})
	blocked := executableTestTool(t, "blocked", func(ctx context.Context, _ *FuncCall, _ map[string]any) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	toolkit, err := NewToolkit(business, blocked)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}
	result, err := toolkit.InvokeTool(
		t.Context(),
		"business",
		json.RawMessage(`{"value":"x"}`),
	)
	if err != nil {
		t.Fatalf("business Invoke() error = %v", err)
	}
	if string(result) != `{"code":"denied","allowed":false}` {
		t.Fatalf("business result = %q", result)
	}

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := toolkit.InvokeTool(
		cancelled,
		"blocked",
		json.RawMessage(`{"value":"x"}`),
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Invoke() error = %v", err)
	}
	timed, stop := context.WithTimeout(t.Context(), time.Millisecond)
	defer stop()
	if _, err := toolkit.InvokeTool(
		timed,
		"blocked",
		json.RawMessage(`{"value":"x"}`),
	); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed Invoke() error = %v", err)
	}
}

func TestToolkitInvokeRejectsLateResultAfterCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	tool := executableTestTool(t, "late", func(_ context.Context, _ *FuncCall, _ map[string]any) (any, error) {
		close(started)
		<-release
		return map[string]bool{"too_late": true}, nil
	})
	toolkit, err := NewToolkit(tool)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, invokeErr := toolkit.InvokeTool(
			ctx,
			"late",
			json.RawMessage(`{"value":"x"}`),
		)
		result <- invokeErr
	}()
	<-started
	cancel()
	close(release)
	if err := <-result; !errors.Is(err, context.Canceled) ||
		!strings.Contains(err.Error(), "discard late") {
		t.Fatalf("late Invoke() error = %v", err)
	}
}

func TestToolkitAllowsConcurrentInvocations(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	release := make(chan struct{})
	tool := executableTestTool(t, "parallel", func(ctx context.Context, _ *FuncCall, arguments map[string]any) (any, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			seen := maximum.Load()
			if current <= seen || maximum.CompareAndSwap(seen, current) {
				break
			}
		}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return arguments, nil
	})
	toolkit, err := NewToolkit(tool)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, invokeErr := toolkit.InvokeTool(
				t.Context(),
				"parallel",
				json.RawMessage(`{"value":"x"}`),
			)
			errs <- invokeErr
		})
	}
	deadline := time.Now().Add(time.Second)
	for maximum.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Invoke() error = %v", err)
		}
	}
	if maximum.Load() != 2 {
		t.Fatalf("maximum concurrent invocations = %d, want 2", maximum.Load())
	}
}

func executableTestTool(
	t *testing.T,
	name string,
	invoke InvokeFunc[map[string]any],
) *FuncTool {
	t.Helper()
	required := "value"
	return &FuncTool{
		Name: name,
		Argument: &jsonschema.Schema{
			Type:                 "object",
			Required:             []string{required},
			Properties:           map[string]*jsonschema.Schema{required: {Type: "string"}},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Invoke: func(ctx context.Context, call *FuncCall, arguments string) (any, error) {
			var decoded map[string]any
			if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
				return nil, err
			}
			return invoke(ctx, call, decoded)
		},
	}
}
