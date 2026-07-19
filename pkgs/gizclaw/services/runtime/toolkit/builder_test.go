package toolkit

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestBuilderUsesProfileAndOwnerUnion(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	profileTool := testBuiltinTool("profile")
	ownedTool := testBuiltinTool("owned")
	owner := "peer-a"
	ownedTool.OwnerPublicKey = &owner
	otherTool := testBuiltinTool("other")
	other := "peer-b"
	otherTool.OwnerPublicKey = &other
	disabled := testBuiltinTool("disabled")
	disabled.Enabled = false
	for _, tool := range []Tool{profileTool, ownedTool, otherTool, disabled} {
		if _, err := store.PutTool(ctx, tool); err != nil {
			t.Fatal(err)
		}
	}

	kit, err := (&Builder{Tools: store}).Build(ctx, BuildRequest{
		OwnerPublicKey: owner,
		ProfileToolIDs: []string{"profile", "missing", "profile"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := toolIDs(kit.Tools); len(got) != 2 || got[0] != "profile" || got[1] != "owned" {
		t.Fatalf("tools = %#v, want profile resources before owner resources", got)
	}
}

func TestBuilderPolicyAndAvailability(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	for _, id := range []string{"available", "offline"} {
		if _, err := store.PutTool(ctx, testBuiltinTool(id)); err != nil {
			t.Fatal(err)
		}
	}
	kit, err := (&Builder{
		Tools: store,
		Availability: availabilityFunc(func(_ context.Context, tool Tool) (bool, error) {
			return tool.ID != "offline", nil
		}),
	}).Build(ctx, BuildRequest{ProfileToolIDs: []string{"available", "offline"}, AllowedToolIDs: []string{"available", "offline"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := toolIDs(kit.Tools); len(got) != 1 || got[0] != "available" {
		t.Fatalf("tools = %#v", got)
	}

	want := errors.New("availability failed")
	_, err = (&Builder{Tools: store, Availability: availabilityFunc(func(context.Context, Tool) (bool, error) { return false, want })}).Build(ctx, BuildRequest{ProfileToolIDs: []string{"available"}})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestBuilderExplicitEmptyPolicyExposesNoTools(t *testing.T) {
	ctx := context.Background()
	store := &Server{Store: kv.NewMemory(nil)}
	if _, err := store.PutTool(ctx, testBuiltinTool("profile")); err != nil {
		t.Fatal(err)
	}
	kit, err := (&Builder{Tools: store}).Build(ctx, BuildRequest{ProfileToolIDs: []string{"profile"}, RestrictToolIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(kit.Tools) != 0 {
		t.Fatalf("tools = %#v, want none", toolIDs(kit.Tools))
	}
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
