package agenthost

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestRuntimeKeyIncludesOwnerAndResolvedWorkflow(t *testing.T) {
	base := Spec{
		Workspace:      apitypes.Workspace{Name: "assistant"},
		Workflow:       apitypes.Workflow{Name: "flow-a"},
		AgentType:      "eino",
		OwnerPublicKey: "peer-a",
	}
	baseKey := runtimeKey("assistant", base)
	for name, mutate := range map[string]func(*Spec){
		"owner":    func(spec *Spec) { spec.OwnerPublicKey = "peer-b" },
		"workflow": func(spec *Spec) { spec.Workflow.Name = "flow-b" },
		"agent":    func(spec *Spec) { spec.AgentType = "flowcraft" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			if got := runtimeKey("assistant", changed); got == baseKey {
				t.Fatalf("runtimeKey() = %q for distinct %s", got, name)
			}
		})
	}
}

func TestRuntimeRegistryLeasesDistinctScopesIndependently(t *testing.T) {
	host := New(nil)
	calls := 0
	if err := host.Register("eino", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		calls++
		return passthroughTransformer{}, nil
	})); err != nil {
		t.Fatal(err)
	}
	base := Spec{
		Workspace:      apitypes.Workspace{Name: "assistant"},
		Workflow:       apitypes.Workflow{Name: "flow-a"},
		AgentType:      "eino",
		OwnerPublicKey: "peer-a",
	}
	first, releaseFirst, err := host.runtimeRegistry().Acquire(t.Context(), host, "assistant", base)
	if err != nil || first == nil {
		t.Fatalf("first Acquire() agent=%T error=%v", first, err)
	}
	secondSpec := base
	secondSpec.OwnerPublicKey = "peer-b"
	second, releaseSecond, err := host.runtimeRegistry().Acquire(t.Context(), host, "assistant", secondSpec)
	if err != nil || second == nil {
		t.Fatalf("second Acquire() agent=%T error=%v", second, err)
	}
	if calls != 2 {
		t.Fatalf("factory calls = %d, want 2 scoped runtimes", calls)
	}
	releaseFirst()
	releaseSecond()
}
