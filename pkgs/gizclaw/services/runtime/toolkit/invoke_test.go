package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestBuilderInvokeUsesAllowedToolsACLAndExecutor(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	tool := testBuiltinTool("system.music.play")
	tool.Name = stringPtr("play_music")
	if _, err := store.PutTool(ctx, tool); err != nil {
		t.Fatalf("PutTool(play) error = %v", err)
	}
	if _, err := store.PutTool(ctx, testBuiltinTool("system.mode.switch")); err != nil {
		t.Fatalf("PutTool(mode) error = %v", err)
	}

	builder := &Builder{Tools: store}
	executors := NewExecutorRegistry()
	if err := executors.Register("music.play", ExecutorFunc(func(_ context.Context, call Call) (Result, error) {
		if call.ID != "call-1" {
			t.Fatalf("call.ID = %q, want call-1", call.ID)
		}
		if call.SubjectID != "owner-peer" {
			t.Fatalf("call.SubjectID = %q, want owner-peer", call.SubjectID)
		}
		if call.Tool.ID != "system.music.play" {
			t.Fatalf("call.Tool.ID = %q, want system.music.play", call.Tool.ID)
		}
		if string(call.Args) != `{"query":"song"}` {
			t.Fatalf("call.Args = %s", call.Args)
		}
		return Result{Data: json.RawMessage(`{"queued":true}`)}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	result, err := builder.Invoke(ctx, executors, InvokeRequest{
		Build: BuildRequest{
			CallerPublicKey: "owner-peer",
			ProfileToolIDs:  []string{"system.music.play"},
			AllowedToolIDs:  []string{"system.music.play"},
		},
		CallID: "call-1",
		Name:   "play_music",
		Args:   json.RawMessage(`{"query":"song"}`),
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if string(result.Data) != `{"queued":true}` {
		t.Fatalf("Invoke() result = %s", result.Data)
	}
}

func TestBuilderInvokeRejectsAdvertisedNameAndIDCollision(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	advertised := testBuiltinTool("system.music.play")
	advertised.Name = stringPtr("bar")
	if _, err := store.PutTool(ctx, advertised); err != nil {
		t.Fatalf("PutTool(advertised) error = %v", err)
	}
	idCollision := testBuiltinTool("bar")
	idCollision.Name = nil
	idCollision.Executor.Name = stringPtr("mode.switch")
	if _, err := store.PutTool(ctx, idCollision); err != nil {
		t.Fatalf("PutTool(collision) error = %v", err)
	}

	executors := NewExecutorRegistry()
	if err := executors.Register("music.play", ExecutorFunc(func(_ context.Context, call Call) (Result, error) {
		if call.Tool.ID != "system.music.play" {
			t.Fatalf("call.Tool.ID = %q, want advertised tool", call.Tool.ID)
		}
		return Result{Data: json.RawMessage(`{"hit":"advertised"}`)}, nil
	})); err != nil {
		t.Fatalf("Register(music.play) error = %v", err)
	}
	if err := executors.Register("mode.switch", ExecutorFunc(func(context.Context, Call) (Result, error) {
		t.Fatal("ID collision executor should not be called")
		return Result{}, nil
	})); err != nil {
		t.Fatalf("Register(mode.switch) error = %v", err)
	}

	_, err := (&Builder{Tools: store}).Invoke(ctx, executors, InvokeRequest{
		Build: BuildRequest{ProfileToolIDs: []string{"system.music.play", "bar"}},
		Name:  "bar", Args: json.RawMessage(`{}`),
	})
	if !errors.Is(err, ErrDuplicateToolName) || !strings.Contains(err.Error(), `"bar"`) || !strings.Contains(err.Error(), `"system.music.play"`) {
		t.Fatalf("Invoke() error = %v, want duplicate name with both IDs", err)
	}
}

func TestBuilderInvokeNormalizesEmptyArgs(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	if _, err := store.PutTool(ctx, testBuiltinTool("system.music.play")); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}
	executors := NewExecutorRegistry()
	if err := executors.Register("music.play", ExecutorFunc(func(_ context.Context, call Call) (Result, error) {
		if string(call.Args) != `{}` {
			t.Fatalf("call.Args = %q, want {}", call.Args)
		}
		return Result{Data: json.RawMessage(`{"ok":true}`)}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if _, err := (&Builder{Tools: store}).Invoke(ctx, executors, InvokeRequest{Build: BuildRequest{ProfileToolIDs: []string{"system.music.play"}}, Name: "system.music.play"}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
}

func TestBuilderInvokeRejectsToolOutsideWorkspaceAllowlist(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	tool := testBuiltinTool("system.music.play")
	tool.Name = stringPtr("play_music")
	if _, err := store.PutTool(ctx, tool); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}

	_, err := (&Builder{Tools: store}).Invoke(ctx, NewExecutorRegistry(), InvokeRequest{
		Build: BuildRequest{ProfileToolIDs: []string{"system.music.play"}, AllowedToolIDs: []string{"system.mode.switch"}},
		Name:  "play_music",
	})
	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("Invoke(disallowed) error = %v, want %v", err, ErrToolNotFound)
	}
}

func TestBuilderInvokeReturnsExecutorErrors(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	if _, err := store.PutTool(ctx, testBuiltinTool("system.music.play")); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}

	_, err := (&Builder{Tools: store}).Invoke(ctx, NewExecutorRegistry(), InvokeRequest{
		Build: BuildRequest{ProfileToolIDs: []string{"system.music.play"}}, Name: "system.music.play",
	})
	if !errors.Is(err, ErrExecutorNotFound) {
		t.Fatalf("Invoke(missing executor) error = %v, want %v", err, ErrExecutorNotFound)
	}
}

func TestBuilderInvokeRequiresJSONObjectButLeavesSchemaCompatibilityToAdapters(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	tool := testBuiltinTool("system.music.play")
	tool.InputSchema = jsonschema.Schema{Type: "object", Required: []string{"query"}, Properties: map[string]*jsonschema.Schema{"query": {Type: "string"}}}
	if _, err := store.PutTool(ctx, tool); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}
	builder := &Builder{Tools: store}
	executors := NewExecutorRegistry()
	called := 0
	if err := executors.Register("music.play", ExecutorFunc(func(context.Context, Call) (Result, error) {
		called++
		return Result{Data: json.RawMessage(`{"ok":true}`)}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	tests := []struct {
		name string
		args json.RawMessage
	}{
		{name: "malformed", args: json.RawMessage(`{`)},
		{name: "non-object", args: json.RawMessage(`[]`)},
	}
	for _, args := range []json.RawMessage{json.RawMessage(`{"limit":1}`), json.RawMessage(`{"query":1}`)} {
		if _, err := builder.Invoke(ctx, executors, InvokeRequest{Build: BuildRequest{ProfileToolIDs: []string{"system.music.play"}}, Name: "system.music.play", Args: args}); err != nil {
			t.Fatalf("Invoke(provider-specific schema args) error = %v", err)
		}
	}
	if called != 2 {
		t.Fatalf("executor calls = %d, want 2", called)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := builder.Invoke(ctx, executors, InvokeRequest{
				Build: BuildRequest{ProfileToolIDs: []string{"system.music.play"}}, Name: "system.music.play",
				Args: tt.args,
			})
			if !errors.Is(err, ErrInvalidTool) {
				t.Fatalf("Invoke() error = %v, want %v", err, ErrInvalidTool)
			}
		})
	}
}

func TestBuilderInvokeAcceptsNullableUnionArgs(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	tool := testBuiltinTool("system.music.play")
	tool.InputSchema = jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{"query": {Types: []string{"string", "null"}}}}
	if _, err := store.PutTool(ctx, tool); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}
	executors := NewExecutorRegistry()
	if err := executors.Register("music.play", ExecutorFunc(func(_ context.Context, call Call) (Result, error) {
		if string(call.Args) != `{"query":null}` {
			t.Fatalf("call.Args = %s, want nullable arg", call.Args)
		}
		return Result{Data: json.RawMessage(`{"ok":true}`)}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if _, err := (&Builder{Tools: store}).Invoke(ctx, executors, InvokeRequest{
		Build: BuildRequest{ProfileToolIDs: []string{"system.music.play"}}, Name: "system.music.play",
		Args: json.RawMessage(`{"query":null}`),
	}); err != nil {
		t.Fatalf("Invoke(nullable) error = %v", err)
	}
}
