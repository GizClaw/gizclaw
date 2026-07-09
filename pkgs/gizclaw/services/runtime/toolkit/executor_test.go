package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestExecutorRegistryInvoke(t *testing.T) {
	registry := NewExecutorRegistry()
	if err := registry.Register("music.play", ExecutorFunc(func(_ context.Context, call Call) (Result, error) {
		if call.Tool.ID != "system.music.play" {
			t.Fatalf("call.Tool.ID = %q, want system.music.play", call.Tool.ID)
		}
		if string(call.Args) != `{"query":"song"}` {
			t.Fatalf("call.Args = %s", call.Args)
		}
		return Result{Data: json.RawMessage(`{"ok":true}`)}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	tool := testBuiltinTool("system.music.play")
	result, err := registry.Invoke(context.Background(), Call{
		Tool: tool,
		Args: json.RawMessage(`{"query":"song"}`),
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if string(result.Data) != `{"ok":true}` {
		t.Fatalf("result = %s, want ok", result.Data)
	}
	result.Data[0] = '['
	result2, err := registry.Invoke(context.Background(), Call{
		Tool: tool,
		Args: json.RawMessage(`{"query":"song"}`),
	})
	if err != nil {
		t.Fatalf("Invoke(second) error = %v", err)
	}
	if string(result2.Data) != `{"ok":true}` {
		t.Fatalf("second result was mutated: %s", result2.Data)
	}
}

func TestExecutorRegistryMissing(t *testing.T) {
	_, err := NewExecutorRegistry().Invoke(context.Background(), Call{Tool: testBuiltinTool("system.music.play")})
	if !errors.Is(err, ErrExecutorNotFound) {
		t.Fatalf("Invoke(missing) error = %v, want %v", err, ErrExecutorNotFound)
	}

	tool := testDeviceTool("peer.peer-a.music.play", "peer-a")
	_, err = NewExecutorRegistry().Invoke(context.Background(), Call{Tool: tool})
	if !errors.Is(err, ErrExecutorNotFound) {
		t.Fatalf("Invoke(device_rpc) error = %v, want %v", err, ErrExecutorNotFound)
	}
}

func TestExecutorRegistryRegisterValidation(t *testing.T) {
	if err := (*ExecutorRegistry)(nil).Register("x", ExecutorFunc(func(context.Context, Call) (Result, error) {
		return Result{}, nil
	})); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Register(nil registry) error = %v, want %v", err, ErrNotConfigured)
	}
	registry := NewExecutorRegistry()
	if err := registry.Register("", ExecutorFunc(func(context.Context, Call) (Result, error) {
		return Result{}, nil
	})); err == nil {
		t.Fatal("Register(empty name) error = nil")
	}
	if err := registry.Register("x", nil); err == nil {
		t.Fatal("Register(nil executor) error = nil")
	}
}
