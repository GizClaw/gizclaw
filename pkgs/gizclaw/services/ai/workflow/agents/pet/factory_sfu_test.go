package pet

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

func TestFactoryRejectsNestedSFUDriver(t *testing.T) {
	registry := agenthost.NewRegistry()
	nested := &captureFactory{}
	if err := registry.Register("sfu", nested); err != nil {
		t.Fatal(err)
	}
	_, err := (Factory{
		Pets: staticPetContext{
			pet:    apitypes.Pet{DisplayName: "Dewey"},
			petDef: apitypes.PetDef{Spec: apitypes.PetDefSpec{Character: apitypes.PetDefCharacterSpec{Prompt: "sfu"}}},
		},
		Factories: registry,
	}).NewAgent(t.Context(), agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-id-a", Name: "pet-demo"},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet:    &apitypes.PetWorkflowSpec{Driver: apitypes.ReusableWorkflowDriver("sfu")},
		}},
		AgentType: Type,
	})
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("NewAgent() error = %v, want nested sfu rejection", err)
	}
	if nested.spec.AgentType != "" {
		t.Fatalf("nested sfu factory was invoked: %#v", nested.spec)
	}
}
