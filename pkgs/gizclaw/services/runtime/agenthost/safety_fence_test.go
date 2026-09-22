package agenthost

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestResolverSafetyFenceUsesOwnerProfileAndFailsClosed(t *testing.T) {
	profile := apitypes.RuntimeProfile{Id: "owner-profile", Spec: apitypes.RuntimeProfileSpec{SafetyFences: &apitypes.RuntimeProfileSafetyFences{
		General: &apitypes.RuntimeProfileSafetyFence{Prompt: "general text"}, Child: &apitypes.RuntimeProfileSafetyFence{Prompt: "complete child text"},
	}}}
	ws := systemWorkspace("fenced-workspace", "workflow", nil)
	ws.OwnerPublicKey = new("owner")
	resolver := ServiceResolver{Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{"workflow": mustWorkflow(t, "workflow")}}, RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) { return profile, nil }}
	for _, level := range []apitypes.SafetyFenceLevel{apitypes.SafetyFenceLevelGeneral, apitypes.SafetyFenceLevelChild, apitypes.SafetyFenceLevelOff} {
		ws.Parameters = &apitypes.WorkspaceParameters{}
		if err := ws.Parameters.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft, SafetyFenceLevel: &level}); err != nil {
			t.Fatal(err)
		}
		spec, err := resolver.resolveWorkspace(withRuntimeProfile(t.Context(), apitypes.RuntimeProfile{Id: "wrong-profile"}), ws)
		if err != nil {
			t.Fatal(err)
		}
		want := map[apitypes.SafetyFenceLevel]string{apitypes.SafetyFenceLevelGeneral: "general text", apitypes.SafetyFenceLevelChild: "complete child text"}[level]
		if spec.SafetyFencePrompt != want {
			t.Fatalf("prompt = %q, want %q", spec.SafetyFencePrompt, want)
		}
	}
	if err := ws.Parameters.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft, SafetyFenceLevel: new(apitypes.SafetyFenceLevelChild)}); err != nil {
		t.Fatal(err)
	}
	profile.Spec.SafetyFences.Child = nil
	_, err := resolver.resolveWorkspace(t.Context(), ws)
	for _, text := range []string{ws.Name, "child", profile.Id} {
		if err == nil || !strings.Contains(err.Error(), text) {
			t.Fatalf("missing %q in error %v", text, err)
		}
	}
	profile.Spec.SafetyFences.Child = &apitypes.RuntimeProfileSafetyFence{Prompt: "child text"}
	// ast-translate has no system prompt: a valid level stays a no-op even
	// when the RuntimeProfile never defines that level.
	workflow := apitypes.Workflow{Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverAstTranslate}}
	profile.Spec.SafetyFences = nil
	prompt, err := resolveSafetyFence(withRuntimeProfile(t.Context(), profile), ws, workflow)
	if err != nil || prompt != "" {
		t.Fatalf("AST fence = %q, %v; want no prompt and no error", prompt, err)
	}
}
