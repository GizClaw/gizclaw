package agenthost

import (
	"testing"

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
