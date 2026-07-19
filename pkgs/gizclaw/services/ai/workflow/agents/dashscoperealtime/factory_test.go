package dashscoperealtime

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

func TestFactoryValidatesAndBuildsAgent(t *testing.T) {
	valid := agenthost.Spec{Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverDashscopeRealtime,
		DashscopeRealtime: &apitypes.DashScopeRealtimeWorkflowSpec{
			Model: " chat ",
		},
	}}}
	if _, err := (Factory{}).NewAgent(t.Context(), valid); err == nil || !strings.Contains(err.Error(), "transformer is required") {
		t.Fatalf("nil transformer error = %v", err)
	}
	if _, err := (Factory{Transformer: noopTransformer{}}).NewAgent(t.Context(), agenthost.Spec{}); err == nil || !strings.Contains(err.Error(), "spec is required") {
		t.Fatalf("missing spec error = %v", err)
	}
	missingModel := valid
	missingModel.Workflow.Spec.DashscopeRealtime = &apitypes.DashScopeRealtimeWorkflowSpec{}
	if _, err := (Factory{Transformer: noopTransformer{}}).NewAgent(t.Context(), missingModel); err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("missing model error = %v", err)
	}
	providerModel := "unsupported"
	unsupported := valid
	unsupported.Workflow.Spec.DashscopeRealtime = &apitypes.DashScopeRealtimeWorkflowSpec{Model: "chat", ProviderModel: &providerModel}
	if _, err := (Factory{Transformer: noopTransformer{}}).NewAgent(t.Context(), unsupported); err == nil || !strings.Contains(err.Error(), "not a supported") {
		t.Fatalf("unsupported provider model error = %v", err)
	}
	agent, err := (Factory{Transformer: noopTransformer{}}).NewAgent(t.Context(), valid)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if agent == nil {
		t.Fatal("NewAgent() returned nil")
	}
}

type noopTransformer struct{}

func (noopTransformer) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return nil, nil
}
