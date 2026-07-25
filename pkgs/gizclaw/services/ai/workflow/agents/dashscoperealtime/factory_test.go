package dashscoperealtime

import (
	"net/url"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

func TestResolvePatternMergesWorkspaceOverrides(t *testing.T) {
	workflowVoice := "workflow-voice"
	workspaceVoice := "workspace-voice"
	temperature := float32(0.7)
	params := &apitypes.WorkspaceParameters{}
	if err := params.FromDashScopeRealtimeWorkspaceParameters(apitypes.DashScopeRealtimeWorkspaceParameters{
		AgentType:   apitypes.DashScopeRealtimeWorkspaceParametersAgentTypeDashscopeRealtime,
		Model:       new("workspace-model"),
		Voice:       &workspaceVoice,
		Temperature: &temperature,
	}); err != nil {
		t.Fatal(err)
	}
	pattern, err := resolvePattern(agenthost.Spec{
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverDashscopeRealtime,
			DashscopeRealtime: &apitypes.DashScopeRealtimeWorkflowSpec{
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
	if got := parsed.Query().Get("temperature"); got != "0.7" {
		t.Fatalf("temperature = %q", got)
	}
}

func TestResolvePatternRequiresModel(t *testing.T) {
	_, err := resolvePattern(agenthost.Spec{Workflow: apitypes.Workflow{
		Spec: apitypes.WorkflowSpec{
			Driver:            apitypes.WorkflowDriverDashscopeRealtime,
			DashscopeRealtime: &apitypes.DashScopeRealtimeWorkflowSpec{},
		},
	}})
	if err == nil {
		t.Fatal("resolvePattern() error = nil")
	}
}
