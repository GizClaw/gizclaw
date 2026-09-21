package doubaorealtimeduplex

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
		if err := parameters.FromDoubaoRealtimeDuplexWorkspaceParameters(apitypes.DoubaoRealtimeDuplexWorkspaceParameters{AgentType: apitypes.DoubaoRealtimeDuplexWorkspaceParametersAgentTypeDoubaoRealtimeDuplex, Instructions: &override}); err != nil {
			t.Fatal(err)
		}
		spec := agenthost.Spec{SafetyFencePrompt: "profile fence", Workspace: apitypes.Workspace{Id: "workspace", Parameters: parameters}, Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{DoubaoRealtimeDuplex: &apitypes.DoubaoRealtimeDuplexWorkflowSpec{Model: "model", Instructions: new("workflow prompt")}}}}
		pattern, err := resolvePattern(spec)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := url.Parse(pattern)
		if err != nil {
			t.Fatal(err)
		}
		// The fence travels as its own pattern parameter: peergenx puts it in
		// front of whatever instructions the provider receives, and the
		// Workspace override stays readable exactly as the device sent it.
		if got := parsed.Query().Get("instructions"); got != override {
			t.Fatalf("instructions = %q; want %q", got, override)
		}
		if got := parsed.Query().Get(peergenx.SafetyFenceParam); got != "profile fence" {
			t.Fatalf("safety fence = %q; want %q", got, "profile fence")
		}
	}
}
