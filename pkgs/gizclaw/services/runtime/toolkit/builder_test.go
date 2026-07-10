package toolkit

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestBuilderFiltersByEnabledPolicyACLAndAvailability(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	tools := []Tool{
		testBuiltinTool("system.music.play"),
		testBuiltinTool("system.mode.switch"),
		testBuiltinTool("system.disabled"),
		testBuiltinTool("system.offline"),
	}
	tools[2].Enabled = false
	for _, tool := range tools {
		if _, err := store.PutTool(ctx, tool); err != nil {
			t.Fatalf("PutTool(%s) error = %v", tool.ID, err)
		}
	}

	auth := &recordingAuthorizer{
		allowed: map[string]bool{
			"system.music.play": true,
			"system.offline":    true,
		},
	}
	builder := &Builder{
		Tools:      store,
		Authorizer: auth,
		Availability: availabilityFunc(func(_ context.Context, tool Tool) (bool, error) {
			return tool.ID != "system.offline", nil
		}),
	}
	kit, err := builder.Build(ctx, BuildRequest{
		Subject:        acl.PublicKeySubject("peer-a"),
		AllowedToolIDs: []string{"system.music.play", "system.disabled", "system.offline"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(kit.Tools) != 1 || kit.Tools[0].ID != "system.music.play" {
		t.Fatalf("ToolKit tools = %#v, want only system.music.play", toolIDs(kit.Tools))
	}
	if _, ok := kit.Find("system.music.play"); !ok {
		t.Fatal("Find(system.music.play) ok = false")
	}
	if _, ok := kit.Find("system.mode.switch"); ok {
		t.Fatal("Find(system.mode.switch) ok = true")
	}
	if !auth.saw("system.music.play", apitypes.ACLPermissionUse) {
		t.Fatalf("authorizer did not see use check for system.music.play: %#v", auth.requests)
	}
	if auth.saw("system.disabled", apitypes.ACLPermissionUse) {
		t.Fatalf("authorizer checked disabled tool: %#v", auth.requests)
	}
}

func TestBuilderReturnsUnexpectedAuthorizeError(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	if _, err := store.PutTool(ctx, testBuiltinTool("system.music.play")); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}
	want := errors.New("boom")
	_, err := (&Builder{
		Tools:      store,
		Authorizer: authorizerFunc(func(context.Context, acl.AuthorizeRequest) error { return want }),
	}).Build(ctx, BuildRequest{Subject: acl.PublicKeySubject("peer-a")})
	if !errors.Is(err, want) {
		t.Fatalf("Build() error = %v, want %v", err, want)
	}
}

func TestBuilderConfigAndAvailabilityErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := (*Builder)(nil).Build(ctx, BuildRequest{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Build(nil) error = %v, want %v", err, ErrNotConfigured)
	}
	store := &Server{Store: kv.NewMemory(nil)}
	if _, err := store.PutTool(ctx, testBuiltinTool("system.music.play")); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}
	_, err := (&Builder{
		Tools:      store,
		Authorizer: authorizerFunc(func(context.Context, acl.AuthorizeRequest) error { return nil }),
	}).Build(ctx, BuildRequest{})
	if err == nil {
		t.Fatal("Build(authorizer without subject) error = nil")
	}
	want := errors.New("availability failed")
	_, err = (&Builder{
		Tools: store,
		Availability: availabilityFunc(func(context.Context, Tool) (bool, error) {
			return false, want
		}),
	}).Build(ctx, BuildRequest{})
	if !errors.Is(err, want) {
		t.Fatalf("Build(availability error) error = %v, want %v", err, want)
	}
}

type recordingAuthorizer struct {
	allowed  map[string]bool
	requests []acl.AuthorizeRequest
}

func (a *recordingAuthorizer) Authorize(_ context.Context, request acl.AuthorizeRequest) error {
	a.requests = append(a.requests, request)
	if a.allowed[request.Resource.Id] {
		return nil
	}
	return acl.ErrDenied
}

func (a *recordingAuthorizer) saw(id string, permission apitypes.ACLPermission) bool {
	for _, request := range a.requests {
		if request.Resource.Id == id && request.Permission == permission {
			return true
		}
	}
	return false
}

type authorizerFunc func(context.Context, acl.AuthorizeRequest) error

func (f authorizerFunc) Authorize(ctx context.Context, request acl.AuthorizeRequest) error {
	return f(ctx, request)
}

type availabilityFunc func(context.Context, Tool) (bool, error)

func (f availabilityFunc) ToolAvailable(ctx context.Context, tool Tool) (bool, error) {
	return f(ctx, tool)
}

func toolIDs(tools []Tool) []string {
	out := make([]string, len(tools))
	for i, tool := range tools {
		out[i] = tool.ID
	}
	return out
}
