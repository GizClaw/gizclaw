package doubaorealtime

import (
	"net/url"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

func TestSafetyFencePrecedesWorkspaceInstructions(t *testing.T) {
	for _, override := range []string{"device prompt", ""} {
		parameters := &apitypes.WorkspaceParameters{}
		if err := parameters.FromDoubaoRealtimeWorkspaceParameters(apitypes.DoubaoRealtimeWorkspaceParameters{AgentType: apitypes.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime, Instructions: &override}); err != nil {
			t.Fatal(err)
		}
		spec := agenthost.Spec{SafetyFencePrompt: "profile fence", Workspace: apitypes.Workspace{Id: "workspace", Parameters: parameters}, Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{DoubaoRealtime: &apitypes.DoubaoRealtimeWorkflowSpec{Model: "model", Instructions: new("workflow prompt")}}}}
		pattern, _, err := resolveRealtimeModelPattern(t.Context(), spec)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := url.Parse(pattern)
		if err != nil {
			t.Fatal(err)
		}
		effective := override
		if override == "" {
			effective = "workflow prompt"
		}
		// The fence travels as its own pattern parameter: peergenx puts it in
		// front of whatever instructions the provider receives, and the
		// Workflow or Workspace text stays readable on its own.
		if got := parsed.Query().Get("instructions"); got != effective {
			t.Fatalf("instructions = %q; want %q", got, effective)
		}
		if got := parsed.Query().Get(peergenx.SafetyFenceParam); got != "profile fence" {
			t.Fatalf("safety fence = %q; want %q", got, "profile fence")
		}
	}
}
