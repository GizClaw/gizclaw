package agenthost

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestResolverSafetyFenceUsesOwnerProfileAndFailsClosed(t *testing.T) {
	fences := apitypes.RuntimeProfileSafetyFences{"general": {Prompt: "general text"}, "child": {Prompt: "complete child text"}}
	profile := apitypes.RuntimeProfile{Id: "owner-profile", Spec: apitypes.RuntimeProfileSpec{SafetyFences: &fences}}
	ws := systemWorkspace("fenced-workspace", "workflow", nil)
	ws.OwnerPublicKey = new("owner")
	resolver := ServiceResolver{Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{"workflow": mustWorkflow(t, "workflow")}}, RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) { return profile, nil }}
	for _, level := range []apitypes.SafetyFenceLevel{"general", "child"} {
		ws.Parameters = &apitypes.WorkspaceParameters{}
		if err := ws.Parameters.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft, SafetyFenceLevel: &level}); err != nil {
			t.Fatal(err)
		}
		spec, err := resolver.resolveWorkspace(withRuntimeProfile(t.Context(), apitypes.RuntimeProfile{Id: "wrong-profile"}), ws)
		if err != nil {
			t.Fatal(err)
		}
		want := map[apitypes.SafetyFenceLevel]string{apitypes.SafetyFenceLevel("general"): "general text", apitypes.SafetyFenceLevel("child"): "complete child text"}[level]
		if spec.SafetyFencePrompt != want {
			t.Fatalf("prompt = %q, want %q", spec.SafetyFencePrompt, want)
		}
	}
	if err := ws.Parameters.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft, SafetyFenceLevel: new(apitypes.SafetyFenceLevel("child"))}); err != nil {
		t.Fatal(err)
	}
	(*profile.Spec.SafetyFences)["child"] = apitypes.RuntimeProfileSafetyFence{Prompt: "updated child text"}
	updated, err := resolver.resolveWorkspace(t.Context(), ws)
	if err != nil || updated.SafetyFencePrompt != "updated child text" {
		t.Fatalf("updated prompt = %q, error = %v", updated.SafetyFencePrompt, err)
	}
	delete(*profile.Spec.SafetyFences, "child")
	_, err = resolver.resolveWorkspace(t.Context(), ws)
	for _, text := range []string{ws.Name, "child", profile.Id} {
		if err == nil || !strings.Contains(err.Error(), text) {
			t.Fatalf("missing %q in error %v", text, err)
		}
	}
	(*profile.Spec.SafetyFences)["child"] = apitypes.RuntimeProfileSafetyFence{Prompt: "child text"}
	// ast-translate has no system prompt: a valid level stays a no-op even
	// when the RuntimeProfile never defines that level.
	workflow := apitypes.Workflow{Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverAstTranslate}}
	profile.Spec.SafetyFences = nil
	prompt, err := resolveSafetyFence(withRuntimeProfile(t.Context(), profile), ws, workflow)
	if err != nil || prompt != "" {
		t.Fatalf("AST fence = %q, %v; want no prompt and no error", prompt, err)
	}
}

func TestResolveSafetyFenceUsesEachProfilesOwnIdentifiers(t *testing.T) {
	workflow := apitypes.Workflow{Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft}}
	ws := systemWorkspace("custom-fence", "workflow", nil)
	for _, tc := range []struct {
		profile string
		level   string
		prompt  string
	}{
		{"profile-a", "alpha", "alpha prompt"},
		{"profile-b", "bravo", "bravo prompt"},
	} {
		fences := apitypes.RuntimeProfileSafetyFences{tc.level: {Prompt: tc.prompt}}
		profile := apitypes.RuntimeProfile{Id: tc.profile, Spec: apitypes.RuntimeProfileSpec{SafetyFences: &fences}}
		level := apitypes.SafetyFenceLevel(tc.level)
		ws.Parameters = &apitypes.WorkspaceParameters{}
		if err := ws.Parameters.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft, SafetyFenceLevel: &level}); err != nil {
			t.Fatal(err)
		}
		got, err := resolveSafetyFence(withRuntimeProfile(t.Context(), profile), ws, workflow)
		if err != nil || got != tc.prompt {
			t.Fatalf("profile %q: prompt = %q, error = %v", tc.profile, got, err)
		}
		level = "unknown"
		if err := ws.Parameters.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft, SafetyFenceLevel: &level}); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveSafetyFence(withRuntimeProfile(t.Context(), profile), ws, workflow); err == nil || !strings.Contains(err.Error(), "unknown") {
			t.Fatalf("unknown level error = %v", err)
		}
		ws.Parameters = nil
		if _, err := resolveSafetyFence(withRuntimeProfile(t.Context(), profile), ws, workflow); err == nil || !strings.Contains(err.Error(), "no safety_fence_level") {
			t.Fatalf("missing selection error = %v", err)
		}
	}
}
