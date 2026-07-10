package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
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

	auth := &recordingAuthorizer{
		allowed: map[string]bool{
			"system.music.play": true,
		},
	}
	builder := &Builder{Tools: store, Authorizer: auth}
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
			Subject:        acl.PublicKeySubject("owner-peer"),
			AllowedToolIDs: []string{"system.music.play"},
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
	if !auth.saw("system.music.play", apitypes.ACLPermissionUse) {
		t.Fatalf("authorizer did not check allowed tool: %#v", auth.requests)
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
		Build: BuildRequest{AllowedToolIDs: []string{"system.mode.switch"}},
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
		Name: "system.music.play",
	})
	if !errors.Is(err, ErrExecutorNotFound) {
		t.Fatalf("Invoke(missing executor) error = %v, want %v", err, ErrExecutorNotFound)
	}
}

func TestBuilderInvokeValidatesArgsAgainstInputSchema(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	tool := testBuiltinTool("system.music.play")
	tool.InputSchema = json.RawMessage(`{"type":"object","required":["query"],"properties":{"query":{"type":"string"},"limit":{"type":"integer"}}}`)
	if _, err := store.PutTool(ctx, tool); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}
	builder := &Builder{Tools: store}
	executors := NewExecutorRegistry()
	if err := executors.Register("music.play", ExecutorFunc(func(context.Context, Call) (Result, error) {
		t.Fatal("executor should not be called for invalid args")
		return Result{}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	tests := []struct {
		name string
		args json.RawMessage
	}{
		{name: "malformed", args: json.RawMessage(`{`)},
		{name: "non-object", args: json.RawMessage(`[]`)},
		{name: "missing required", args: json.RawMessage(`{"limit":1}`)},
		{name: "wrong type", args: json.RawMessage(`{"query":1}`)},
		{name: "wrong integer", args: json.RawMessage(`{"query":"song","limit":1.5}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := builder.Invoke(ctx, executors, InvokeRequest{
				Name: "system.music.play",
				Args: tt.args,
			})
			if !errors.Is(err, ErrInvalidTool) {
				t.Fatalf("Invoke() error = %v, want %v", err, ErrInvalidTool)
			}
		})
	}
}
