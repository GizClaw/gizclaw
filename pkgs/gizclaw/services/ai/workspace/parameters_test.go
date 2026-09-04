package workspace

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
)

func TestSetPeerWorkspaceParametersMergesSupportedFields(t *testing.T) {
	srv := newTestServer(t)
	seedFlowcraftWorkflow(t, srv, "workflow-1", "model-1")
	seedModel(t, srv, "model-1", apitypes.ModelKindLlm)
	ctx := ownership.WithOwner(t.Context(), "peer-owner")
	pushToTalk := apitypes.WorkspaceInputModePushToTalk
	initiative := apitypes.ConversationParametersInitiativePeer
	e2e := true
	toolIDs := []string{"tool-1"}
	created, err := srv.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{
		Name: "workspace-1", WorkflowID: "workflow-1", Labels: map[string]string{"purpose": "test"},
		Parameters: flowcraftInputParameters(t, apitypes.FlowcraftWorkspaceParameters{
			AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
			Input:     &pushToTalk,
			E2e:       &e2e,
			Conversation: &apitypes.ConversationParameters{
				Initiative: &initiative,
			},
		}),
		Toolkit: &apitypes.ToolkitPolicy{ToolIds: &toolIDs},
	})
	if err != nil {
		t.Fatalf("CreatePeerWorkspace() error = %v", err)
	}

	realtime := apitypes.WorkspaceInputModeRealtime
	agent := apitypes.ConversationParametersInitiativeAgent
	policy := apitypes.ConversationParametersAgentInitiativePolicyOnReload
	updated, err := srv.SetPeerWorkspaceParameters(ctx, PeerWorkspaceParametersSetRequest{
		ID:    created.Id,
		Input: &realtime,
		Conversation: &apitypes.ConversationParameters{
			Initiative:            &agent,
			AgentInitiativePolicy: &policy,
		},
	})
	if err != nil {
		t.Fatalf("SetPeerWorkspaceParameters() error = %v", err)
	}
	parameters, err := updated.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if parameters.AgentType != apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft {
		t.Fatalf("agent_type = %q", parameters.AgentType)
	}
	if parameters.Input == nil || *parameters.Input != realtime {
		t.Fatalf("input = %+v, want realtime", parameters.Input)
	}
	if parameters.E2e == nil || !*parameters.E2e {
		t.Fatalf("e2e = %+v, want preserved true", parameters.E2e)
	}
	if parameters.Conversation == nil || parameters.Conversation.Initiative == nil || *parameters.Conversation.Initiative != agent ||
		parameters.Conversation.AgentInitiativePolicy == nil || *parameters.Conversation.AgentInitiativePolicy != policy {
		t.Fatalf("conversation = %+v", parameters.Conversation)
	}
	if updated.Labels == nil || (*updated.Labels)["purpose"] != "test" {
		t.Fatalf("labels = %+v, want purpose preserved", updated.Labels)
	}
	if updated.Toolkit == nil || updated.Toolkit.ToolIds == nil || len(*updated.Toolkit.ToolIds) != 1 || (*updated.Toolkit.ToolIds)[0] != "tool-1" {
		t.Fatalf("toolkit = %+v, want tool-1 preserved", updated.Toolkit)
	}

	once := apitypes.ConversationParametersAgentInitiativePolicyOnceWhenEmpty
	updated, err = srv.SetPeerWorkspaceParameters(ctx, PeerWorkspaceParametersSetRequest{
		ID: created.Id,
		Conversation: &apitypes.ConversationParameters{
			AgentInitiativePolicy: &once,
		},
	})
	if err != nil {
		t.Fatalf("SetPeerWorkspaceParameters(policy only) error = %v", err)
	}
	parameters, err = updated.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if parameters.Input == nil || *parameters.Input != realtime || parameters.Conversation.Initiative == nil || *parameters.Conversation.Initiative != agent {
		t.Fatalf("partial update lost stored fields: %+v", parameters)
	}
	if parameters.Conversation.AgentInitiativePolicy == nil || *parameters.Conversation.AgentInitiativePolicy != once {
		t.Fatalf("policy = %+v, want once_when_empty", parameters.Conversation.AgentInitiativePolicy)
	}
}

func TestSetPeerWorkspaceParametersRejectsInvalidPatch(t *testing.T) {
	if err := validateWorkspaceParametersPatch(PeerWorkspaceParametersSetRequest{}); err == nil {
		t.Fatal("empty patch error = nil")
	}
	invalid := apitypes.ConversationParametersInitiative("sometimes")
	err := validateWorkspaceParametersPatch(PeerWorkspaceParametersSetRequest{
		Conversation: &apitypes.ConversationParameters{Initiative: &invalid},
	})
	if err == nil {
		t.Fatal("invalid initiative error = nil")
	}
	if _, err := workspaceParametersWithPatch(nil, apitypes.WorkflowDriverPet, nil, &apitypes.ConversationParameters{Initiative: new(apitypes.ConversationParametersInitiativeAgent)}); err == nil {
		t.Fatal("pet conversation patch error = nil")
	}
}

func TestWorkspaceParametersWithPatchDerivesEino(t *testing.T) {
	realtime := apitypes.WorkspaceInputModeRealtime
	agent := apitypes.ConversationParametersInitiativeAgent
	policy := apitypes.ConversationParametersAgentInitiativePolicyOnceWhenEmpty

	updated, err := workspaceParametersWithPatch(nil, apitypes.WorkflowDriverEino, &realtime, &apitypes.ConversationParameters{
		Initiative:            &agent,
		AgentInitiativePolicy: &policy,
	})
	if err != nil {
		t.Fatalf("workspaceParametersWithPatch() error = %v", err)
	}
	parameters, err := updated.AsEinoWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if parameters.AgentType != apitypes.EinoWorkspaceParametersAgentTypeEino || parameters.Input == nil || *parameters.Input != realtime {
		t.Fatalf("parameters = %+v", parameters)
	}
	if parameters.Conversation == nil || parameters.Conversation.Initiative == nil || *parameters.Conversation.Initiative != agent ||
		parameters.Conversation.AgentInitiativePolicy == nil || *parameters.Conversation.AgentInitiativePolicy != policy {
		t.Fatalf("conversation = %+v", parameters.Conversation)
	}
}
