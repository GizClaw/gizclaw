package agenthost

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

type passthroughFactory struct{}

func (passthroughFactory) NewAgent(context.Context, Spec) (Agent, error) {
	return NewTransformerAgent(passthroughTransformer{}), nil
}

func TestNewWorkspaceAgentSkipsHistoryWrapperForSFU(t *testing.T) {
	host := New(nil)
	for _, agentType := range []string{"sfu", "flowcraft"} {
		if err := host.Register(agentType, passthroughFactory{}); err != nil {
			t.Fatalf("Register(%q) error = %v", agentType, err)
		}
	}
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))

	sfuSpec := Spec{
		AgentType: "sfu",
		Workflow:  apitypes.Workflow{Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverSfu}},
		Runtime:   workspace.Runtime{History: history},
	}
	agent, release, err := host.newWorkspaceAgent(t.Context(), sfuSpec)
	if err != nil {
		t.Fatalf("newWorkspaceAgent(sfu) error = %v", err)
	}
	defer release()
	if _, wrapped := agent.(*historyAgent); wrapped {
		t.Fatal("sfu Workspace Agent was wrapped with the history recorder")
	}
	if _, ok := agent.(noHistoryAgent); !ok {
		t.Fatalf("sfu Workspace Agent type = %T, want noHistoryAgent", agent)
	}
	list, err := agent.ListHistory(t.Context(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if list.Available || len(list.Items) != 0 || list.HasNext || list.Message != nil {
		t.Fatalf("ListHistory() = %+v, want empty list", list)
	}
	play, err := agent.PlayHistory(t.Context(), apitypes.PeerRunHistoryPlayRequest{HistoryName: "entry-1"})
	if err != nil {
		t.Fatalf("PlayHistory() error = %v", err)
	}
	if play.Accepted || play.State != "not_found" || play.HistoryName != "entry-1" {
		t.Fatalf("PlayHistory() = %+v, want not_found", play)
	}

	workflowSpec := Spec{
		AgentType: "flowcraft",
		Workflow:  apitypes.Workflow{Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft}},
		Runtime:   workspace.Runtime{History: history},
	}
	agent, release, err = host.newWorkspaceAgent(t.Context(), workflowSpec)
	if err != nil {
		t.Fatalf("newWorkspaceAgent(flowcraft) error = %v", err)
	}
	defer release()
	if _, wrapped := agent.(*historyAgent); !wrapped {
		t.Fatalf("Workflow Workspace Agent type = %T, want *historyAgent", agent)
	}
}

func TestIsSFUSpecUsesWorkflowDriverFallback(t *testing.T) {
	if !isSFUSpec(Spec{Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverSfu}}}) {
		t.Fatal("sfu Workflow driver was not recognised")
	}
	if isSFUSpec(Spec{AgentType: "flowcraft"}) {
		t.Fatal("flowcraft Spec was classified as sfu")
	}
}
