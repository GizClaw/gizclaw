package doubaorealtimeduplex

import (
	"net/url"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

func TestResolvePatternMergesWorkspaceOverrides(t *testing.T) {
	workflowVoice := "workflow-voice"
	workspaceVoice := "workspace-voice"
	sampleRate := apitypes.DoubaoRealtimeDuplexWorkspaceParametersSampleRate(24000)
	params := &apitypes.WorkspaceParameters{}
	if err := params.FromDoubaoRealtimeDuplexWorkspaceParameters(apitypes.DoubaoRealtimeDuplexWorkspaceParameters{
		AgentType:  apitypes.DoubaoRealtimeDuplexWorkspaceParametersAgentTypeDoubaoRealtimeDuplex,
		Model:      new("workspace-model"),
		Voice:      &workspaceVoice,
		SampleRate: &sampleRate,
	}); err != nil {
		t.Fatal(err)
	}
	pattern, err := resolvePattern(agenthost.Spec{
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverDoubaoRealtimeDuplex,
			DoubaoRealtimeDuplex: &apitypes.DoubaoRealtimeDuplexWorkflowSpec{
				Model: "workflow-model",
				Voice: &workflowVoice,
			},
		}},
		Workspace: apitypes.Workspace{Parameters: params},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "model/workspace-model" {
		t.Fatalf("path = %q", parsed.Path)
	}
	if got := parsed.Query().Get("output_voice"); got != workspaceVoice {
		t.Fatalf("output_voice = %q", got)
	}
	if got := parsed.Query().Get("sample_rate"); got != "24000" {
		t.Fatalf("sample_rate = %q", got)
	}
}

func TestResolvePatternRequiresModel(t *testing.T) {
	_, err := resolvePattern(agenthost.Spec{Workflow: apitypes.Workflow{
		Spec: apitypes.WorkflowSpec{
			Driver:               apitypes.WorkflowDriverDoubaoRealtimeDuplex,
			DoubaoRealtimeDuplex: &apitypes.DoubaoRealtimeDuplexWorkflowSpec{},
		},
	}})
	if err == nil {
		t.Fatal("resolvePattern() error = nil")
	}
}
