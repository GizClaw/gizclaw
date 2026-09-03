package workspace

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
)

func flowcraftInputParameters(t *testing.T, value apitypes.FlowcraftWorkspaceParameters) *apitypes.WorkspaceParameters {
	t.Helper()
	parameters := &apitypes.WorkspaceParameters{}
	if err := parameters.FromFlowcraftWorkspaceParameters(value); err != nil {
		t.Fatalf("FromFlowcraftWorkspaceParameters() error = %v", err)
	}
	return parameters
}

func workspaceInputMode(t *testing.T, workspace apitypes.Workspace) *apitypes.WorkspaceInputMode {
	t.Helper()
	if workspace.Parameters == nil {
		return nil
	}
	value, err := workspace.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatalf("AsFlowcraftWorkspaceParameters() error = %v", err)
	}
	return value.Input
}

func TestPutPeerWorkspaceInputKeepsOtherParameters(t *testing.T) {
	srv := newTestServer(t)
	seedFlowcraftWorkflow(t, srv, "workflow-1", "model-1")
	seedModel(t, srv, "model-1", apitypes.ModelKindLlm)
	ctx := ownership.WithOwner(t.Context(), "peer-owner")

	pushToTalk := apitypes.WorkspaceInputModePushToTalk
	initiative := apitypes.ConversationParametersInitiativeAgent
	toolIDs := []string{"tool-a"}
	created, err := srv.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{
		Name: "workspace-1", WorkflowID: "workflow-1",
		Parameters: flowcraftInputParameters(t, apitypes.FlowcraftWorkspaceParameters{
			AgentType:    apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
			Conversation: &apitypes.ConversationParameters{Initiative: &initiative},
			Input:        &pushToTalk,
		}),
		Toolkit: &apitypes.ToolkitPolicy{ToolIds: &toolIDs},
	})
	if err != nil {
		t.Fatalf("CreatePeerWorkspace() error = %v", err)
	}

	updated, err := srv.PutPeerWorkspaceInput(ctx, PeerWorkspaceInputPutRequest{
		ID: created.Id, Input: apitypes.WorkspaceInputModeRealtime,
	})
	if err != nil {
		t.Fatalf("PutPeerWorkspaceInput(realtime) error = %v", err)
	}
	if mode := workspaceInputMode(t, updated); mode == nil || *mode != apitypes.WorkspaceInputModeRealtime {
		t.Fatalf("input = %+v, want realtime", mode)
	}
	parameters, err := updated.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatalf("AsFlowcraftWorkspaceParameters() error = %v", err)
	}
	if parameters.Conversation == nil || parameters.Conversation.Initiative == nil || *parameters.Conversation.Initiative != initiative {
		t.Fatalf("conversation = %+v, want initiative %q preserved", parameters.Conversation, initiative)
	}
	if created.Toolkit == nil || created.Toolkit.ToolIds == nil || len(*created.Toolkit.ToolIds) != 1 {
		t.Fatalf("created toolkit = %+v, want one tool id", created.Toolkit)
	}
	if !reflect.DeepEqual(updated.Toolkit, created.Toolkit) {
		t.Fatalf("toolkit = %+v, want %+v", updated.Toolkit, created.Toolkit)
	}

	back, err := srv.PutPeerWorkspaceInput(ctx, PeerWorkspaceInputPutRequest{
		ID: created.Id, Input: apitypes.WorkspaceInputModePushToTalk,
	})
	if err != nil {
		t.Fatalf("PutPeerWorkspaceInput(push-to-talk) error = %v", err)
	}
	if mode := workspaceInputMode(t, back); mode == nil || *mode != apitypes.WorkspaceInputModePushToTalk {
		t.Fatalf("input = %+v, want push-to-talk", mode)
	}
}

func TestPutPeerWorkspaceInputResolvesInheritedParameters(t *testing.T) {
	srv := newTestServer(t)
	seedFlowcraftWorkflow(t, srv, "workflow-1", "model-1")
	seedModel(t, srv, "model-1", apitypes.ModelKindLlm)
	ctx := ownership.WithOwner(t.Context(), "peer-owner")

	created, err := srv.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{Name: "workspace-1", WorkflowID: "workflow-1"})
	if err != nil {
		t.Fatalf("CreatePeerWorkspace() error = %v", err)
	}
	if created.Parameters != nil {
		t.Fatalf("created parameters = %+v, want inherited", created.Parameters)
	}

	updated, err := srv.PutPeerWorkspaceInput(ctx, PeerWorkspaceInputPutRequest{
		ID: created.Id, Input: apitypes.WorkspaceInputModeRealtime,
	})
	if err != nil {
		t.Fatalf("PutPeerWorkspaceInput() error = %v", err)
	}
	if mode := workspaceInputMode(t, updated); mode == nil || *mode != apitypes.WorkspaceInputModeRealtime {
		t.Fatalf("input = %+v, want realtime", mode)
	}
}

func TestPutPeerWorkspaceInputRejectsInvalidRequests(t *testing.T) {
	srv := newTestServer(t)
	seedFlowcraftWorkflow(t, srv, "workflow-1", "model-1")
	seedModel(t, srv, "model-1", apitypes.ModelKindLlm)
	ctx := ownership.WithOwner(t.Context(), "peer-owner")
	created, err := srv.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{Name: "workspace-1", WorkflowID: "workflow-1"})
	if err != nil {
		t.Fatalf("CreatePeerWorkspace() error = %v", err)
	}

	var inputErr *PeerWorkspaceInputPutError
	if _, err := srv.PutPeerWorkspaceInput(ctx, PeerWorkspaceInputPutRequest{ID: created.Id, Input: "spoken"}); !errors.As(err, &inputErr) || inputErr.Kind != PeerWorkspaceInputPutInvalid {
		t.Fatalf("PutPeerWorkspaceInput(unknown mode) error = %#v", err)
	}
	if _, err := srv.PutPeerWorkspaceInput(ctx, PeerWorkspaceInputPutRequest{ID: "missing", Input: apitypes.WorkspaceInputModeRealtime}); !errors.As(err, &inputErr) || inputErr.Kind != PeerWorkspaceInputPutNotFound {
		t.Fatalf("PutPeerWorkspaceInput(missing workspace) error = %#v", err)
	}
}

func TestWorkspaceParametersWithInputRejectsDriversWithoutInput(t *testing.T) {
	if _, err := workspaceParametersWithInput(nil, apitypes.WorkflowDriverDashscopeRealtime, apitypes.WorkspaceInputModeRealtime); err == nil {
		t.Fatal("workspaceParametersWithInput(dashscope-realtime) error = nil, want unsupported driver")
	}
	if _, err := workspaceParametersWithInput(nil, apitypes.WorkflowDriverDoubaoRealtimeDuplex, apitypes.WorkspaceInputModeRealtime); err == nil {
		t.Fatal("workspaceParametersWithInput(doubao-realtime-duplex) error = nil, want unsupported driver")
	}
}
