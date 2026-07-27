package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestResolveInvokeReauthorizesAndValidatesArguments(t *testing.T) {
	t.Parallel()
	server := &Server{Store: kv.NewMemory(nil)}
	tool := testClientTool("volume_set")
	tool.InputSchema = jsonschema.Schema{
		Type:       "object",
		Required:   []string{"level"},
		Properties: map[string]*jsonschema.Schema{"level": {Type: "integer"}},
		AdditionalProperties: &jsonschema.Schema{
			Not: &jsonschema.Schema{},
		},
	}
	if _, err := server.PutTool(context.Background(), tool); err != nil {
		t.Fatalf("PutTool(): %v", err)
	}
	builder := &Builder{Tools: server}
	resolved, args, err := builder.ResolveInvoke(context.Background(), InvokeRequest{
		Build: BuildRequest{
			ProfileTools:  []string{"volume_set"},
			AllowedTools:  []string{"volume_set"},
			RestrictTools: true,
		},
		Name: "volume_set",
		Args: json.RawMessage(`{"level":7}`),
	})
	if err != nil {
		t.Fatalf("ResolveInvoke(): %v", err)
	}
	if resolved.Name != "volume_set" || string(args) != `{"level":7}` {
		t.Fatalf("ResolveInvoke() = %#v %s", resolved, args)
	}

	for _, bad := range []json.RawMessage{json.RawMessage(`[]`), json.RawMessage(`{"level":"loud"}`), json.RawMessage(`{"level":7,"extra":true}`)} {
		if _, _, err := builder.ResolveInvoke(context.Background(), InvokeRequest{
			Build: BuildRequest{ProfileTools: []string{"volume_set"}},
			Name:  "volume_set",
			Args:  bad,
		}); !errors.Is(err, ErrInvalidTool) {
			t.Fatalf("ResolveInvoke(%s) error = %v", bad, err)
		}
	}
}

func TestResolveInvokeRejectsUnboundAliasAndSeesResourceUpdate(t *testing.T) {
	t.Parallel()
	server := &Server{Store: kv.NewMemory(nil)}
	if _, err := server.PutTool(context.Background(), testClientTool("volume_set")); err != nil {
		t.Fatalf("PutTool(): %v", err)
	}
	builder := &Builder{Tools: server}
	request := BuildRequest{ProfileTools: []string{"volume_set"}}
	if _, _, err := builder.ResolveInvoke(context.Background(), InvokeRequest{
		Build: request,
		Name:  "device-volume",
	}); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("alias invocation error = %v", err)
	}
	disabled := testClientTool("volume_set")
	disabled.Enabled = false
	if _, err := server.PutTool(context.Background(), disabled); err != nil {
		t.Fatalf("disable PutTool(): %v", err)
	}
	if _, _, err := builder.ResolveInvoke(context.Background(), InvokeRequest{
		Build: request,
		Name:  "volume_set",
	}); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("disabled invocation error = %v", err)
	}
}
