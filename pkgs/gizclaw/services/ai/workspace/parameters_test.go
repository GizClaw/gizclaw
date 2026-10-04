package workspace

import (
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
)

func TestSetPeerWorkspaceParametersMergesSupportedFields(t *testing.T) {
	srv := newTestServer(t)
	seedEinoPortedWorkflow(t, srv, "workflow-1", "model-1")
	seedModel(t, srv, "model-1", apitypes.ModelKindLlm)
	ctx := ownership.WithOwner(t.Context(), "peer-owner")
	pushToTalk := apitypes.WorkspaceInputModePushToTalk
	initiative := apitypes.ConversationParametersInitiativePeer
	e2e := true
	toolIDs := []string{"tool-1"}
	created, err := srv.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{
		Name: "workspace-1", WorkflowID: "workflow-1", Labels: map[string]string{"purpose": "test"},
		Parameters: einoInputParameters(t, apitypes.EinoWorkspaceParameters{
			AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino,
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
	parameters, err := updated.Parameters.AsEinoWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if parameters.AgentType != apitypes.EinoWorkspaceParametersAgentTypeEino {
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
	parameters, err = updated.Parameters.AsEinoWorkspaceParameters()
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
}

func TestWorkspaceParametersWithPatchDerivesEino(t *testing.T) {
	realtime := apitypes.WorkspaceInputModeRealtime
	agent := apitypes.ConversationParametersInitiativeAgent
	policy := apitypes.ConversationParametersAgentInitiativePolicyOnceWhenEmpty

	updated, err := workspaceParametersWithPatch(nil, apitypes.WorkflowDriverEino, &realtime, &apitypes.ConversationParameters{
		Initiative:            &agent,
		AgentInitiativePolicy: &policy,
	}, nil, nil, nil)
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

func TestWorkspaceParametersWithPatchDerivesDoubaoRealtime(t *testing.T) {
	pushToTalk := apitypes.WorkspaceInputModePushToTalk
	agent := apitypes.ConversationParametersInitiativeAgent
	policy := apitypes.ConversationParametersAgentInitiativePolicyOnReload

	updated, err := workspaceParametersWithPatch(nil, apitypes.WorkflowDriverDoubaoRealtime, &pushToTalk, &apitypes.ConversationParameters{
		Initiative:            &agent,
		AgentInitiativePolicy: &policy,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("workspaceParametersWithPatch() error = %v", err)
	}
	parameters, err := updated.AsDoubaoRealtimeWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if parameters.AgentType != apitypes.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime || parameters.Input == nil || *parameters.Input != pushToTalk {
		t.Fatalf("parameters = %+v", parameters)
	}
	if parameters.Conversation == nil || parameters.Conversation.Initiative == nil || *parameters.Conversation.Initiative != agent ||
		parameters.Conversation.AgentInitiativePolicy == nil || *parameters.Conversation.AgentInitiativePolicy != policy {
		t.Fatalf("conversation = %+v", parameters.Conversation)
	}

	model := "workspace-dialog"
	existing := &apitypes.WorkspaceParameters{}
	if err := existing.FromDoubaoRealtimeWorkspaceParameters(apitypes.DoubaoRealtimeWorkspaceParameters{
		AgentType: apitypes.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime,
		Model:     &model,
	}); err != nil {
		t.Fatal(err)
	}
	peer := apitypes.ConversationParametersInitiativePeer
	updated, err = workspaceParametersWithPatch(existing, apitypes.WorkflowDriverDoubaoRealtime, nil, &apitypes.ConversationParameters{Initiative: &peer}, nil, nil, nil)
	if err != nil {
		t.Fatalf("workspaceParametersWithPatch(existing) error = %v", err)
	}
	parameters, err = updated.AsDoubaoRealtimeWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if parameters.Model == nil || *parameters.Model != model || parameters.Conversation == nil ||
		parameters.Conversation.Initiative == nil || *parameters.Conversation.Initiative != peer {
		t.Fatalf("patched parameters = %+v", parameters)
	}
}

func TestWorkspaceParametersPatchSupportsEveryDriver(t *testing.T) {
	for _, driver := range []apitypes.WorkflowDriver{
		apitypes.WorkflowDriverAstTranslate, apitypes.WorkflowDriverDoubaoRealtime,
		apitypes.WorkflowDriverEino, apitypes.WorkflowDriverEino,
		apitypes.WorkflowDriverDashscopeRealtime, apitypes.WorkflowDriverDoubaoRealtimeDuplex,
		apitypes.WorkflowDriverSfu,
	} {
		t.Run(string(driver), func(t *testing.T) {
			realtime := apitypes.WorkspaceInputModeRealtime
			conversation := &apitypes.ConversationParameters{Initiative: new(apitypes.ConversationParametersInitiativeAgent)}
			updated, err := workspaceParametersWithPatch(nil, driver, &realtime, conversation, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			switch driver {
			case apitypes.WorkflowDriverSfu, apitypes.WorkflowDriverDashscopeRealtime, apitypes.WorkflowDriverDoubaoRealtimeDuplex:
				if updated != nil {
					t.Fatalf("unsupported parameters stored: %+v", updated)
				}
			default:
				if updated == nil {
					t.Fatal("supported input was discarded with unsupported conversation")
				}
				discriminator, err := updated.Discriminator()
				if err != nil || discriminator != string(driver) {
					t.Fatalf("parameters discriminator = %q, %v", discriminator, err)
				}
			}
		})
	}
}

func TestSetPeerWorkspaceParametersStoresTTSSpeechRate(t *testing.T) {
	srv := newTestServer(t)
	seedEinoPortedWorkflow(t, srv, "workflow-1", "model-1")
	seedModel(t, srv, "model-1", apitypes.ModelKindLlm)
	ctx := ownership.WithOwner(t.Context(), "peer-owner")
	pushToTalk := apitypes.WorkspaceInputModePushToTalk
	created, err := srv.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{
		Name: "workspace-1", WorkflowID: "workflow-1",
		Parameters: einoInputParameters(t, apitypes.EinoWorkspaceParameters{
			AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino,
			Input:     &pushToTalk,
		}),
	})
	if err != nil {
		t.Fatalf("CreatePeerWorkspace() error = %v", err)
	}

	updated, err := srv.SetPeerWorkspaceParameters(ctx, PeerWorkspaceParametersSetRequest{ID: created.Id, TTSSpeechRatePercent: new(70)})
	if err != nil {
		t.Fatalf("SetPeerWorkspaceParameters(rate only) error = %v", err)
	}
	parameters, err := updated.Parameters.AsEinoWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if parameters.TtsSpeechRatePercent == nil || *parameters.TtsSpeechRatePercent != 70 || parameters.Input == nil || *parameters.Input != pushToTalk {
		t.Fatalf("rate-only patch = %+v", parameters)
	}

	realtime := apitypes.WorkspaceInputModeRealtime
	updated, err = srv.SetPeerWorkspaceParameters(ctx, PeerWorkspaceParametersSetRequest{ID: created.Id, Input: &realtime})
	if err != nil {
		t.Fatalf("SetPeerWorkspaceParameters(input only) error = %v", err)
	}
	parameters, err = updated.Parameters.AsEinoWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if parameters.TtsSpeechRatePercent == nil || *parameters.TtsSpeechRatePercent != 70 {
		t.Fatalf("input-only patch lost rate: %+v", parameters)
	}

	var patchErr *PeerWorkspaceParametersSetError
	for _, value := range []int{49, 201} {
		if _, err := srv.SetPeerWorkspaceParameters(ctx, PeerWorkspaceParametersSetRequest{ID: created.Id, TTSSpeechRatePercent: new(value)}); !errors.As(err, &patchErr) || patchErr.Kind != PeerWorkspaceParametersSetInvalid {
			t.Fatalf("SetPeerWorkspaceParameters(%d) error = %#v, want invalid", value, err)
		}
	}
}

func TestCreatePeerWorkspaceRejectsOutOfRangeTTSSpeechRate(t *testing.T) {
	srv := newTestServer(t)
	seedEinoPortedWorkflow(t, srv, "workflow-1", "model-1")
	seedModel(t, srv, "model-1", apitypes.ModelKindLlm)
	ctx := ownership.WithOwner(t.Context(), "peer-owner")
	_, err := srv.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{
		Name: "workspace-1", WorkflowID: "workflow-1",
		Parameters: einoInputParameters(t, apitypes.EinoWorkspaceParameters{
			AgentType:            apitypes.EinoWorkspaceParametersAgentTypeEino,
			TtsSpeechRatePercent: new(300),
		}),
	})
	if err == nil {
		t.Fatal("CreatePeerWorkspace(rate 300) error = nil")
	}
}

func TestWorkspaceParametersPatchStoresTTSSpeechRateForVoiceDrivers(t *testing.T) {
	for _, driver := range []apitypes.WorkflowDriver{
		apitypes.WorkflowDriverAstTranslate, apitypes.WorkflowDriverDoubaoRealtime,
		apitypes.WorkflowDriverEino, apitypes.WorkflowDriverEino,
		apitypes.WorkflowDriverDashscopeRealtime, apitypes.WorkflowDriverDoubaoRealtimeDuplex,
	} {
		t.Run(string(driver), func(t *testing.T) {
			updated, err := workspaceParametersWithPatch(nil, driver, nil, nil, new(150), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if updated == nil {
				t.Fatal("rate patch stored nothing")
			}
			discriminator, err := updated.Discriminator()
			if err != nil || discriminator != string(driver) {
				t.Fatalf("discriminator = %q, %v", discriminator, err)
			}
			rate, err := updated.TTSSpeechRatePercent()
			if err != nil || rate == nil || *rate != 150 {
				t.Fatalf("rate = %v, %v", rate, err)
			}
		})
	}
}

func TestWorkspaceParametersPatchStoresAudioInputForEinoOnly(t *testing.T) {
	model := apitypes.AudioInputPathModel
	updated, err := workspaceParametersWithPatch(nil, apitypes.WorkflowDriverEino, nil, nil, nil, nil, &model)
	if err != nil {
		t.Fatalf("workspaceParametersWithPatch(eino) error = %v", err)
	}
	got, err := updated.AudioInput()
	if err != nil || got == nil || *got != model {
		t.Fatalf("eino audio_input = %v, %v; want %q", got, err, model)
	}

	// A later patch of another field keeps the stored path.
	realtime := apitypes.WorkspaceInputModeRealtime
	updated, err = workspaceParametersWithPatch(updated, apitypes.WorkflowDriverEino, &realtime, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("workspaceParametersWithPatch(eino input) error = %v", err)
	}
	if got, err := updated.AudioInput(); err != nil || got == nil || *got != model {
		t.Fatalf("eino audio_input after input patch = %v, %v; want %q", got, err, model)
	}

	// Drivers without a selectable path ignore the field.
	for _, driver := range []apitypes.WorkflowDriver{
		apitypes.WorkflowDriverDoubaoRealtime, apitypes.WorkflowDriverAstTranslate,
		apitypes.WorkflowDriverDashscopeRealtime, apitypes.WorkflowDriverDoubaoRealtimeDuplex, apitypes.WorkflowDriverSfu,
	} {
		updated, err := workspaceParametersWithPatch(nil, driver, nil, nil, nil, nil, &model)
		if err != nil {
			t.Fatalf("workspaceParametersWithPatch(%s) error = %v", driver, err)
		}
		if updated == nil {
			continue
		}
		data, err := updated.MarshalJSON()
		if err != nil || strings.Contains(string(data), "audio_input") {
			t.Fatalf("%s parameters = %s, %v; want no audio_input", driver, data, err)
		}
	}
}

func TestCreatePeerWorkspaceValidatesAudioInput(t *testing.T) {
	srv := newTestServer(t)
	store := testWorkflowStore(t, srv)
	if err := store.Set(t.Context(), workflowReferenceKey("workflow-eino"), []byte(`{"id":"workflow-eino","spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{},"state":{"fields":[]},"nodes":[],"edges":[],"branches":[],"outputs":[]}}}}`)); err != nil {
		t.Fatalf("seed Eino workflow: %v", err)
	}
	ctx := ownership.WithOwner(t.Context(), "peer-owner")
	for value, wantErr := range map[string]bool{"model": false, "asr": false, "direct": true} {
		var parameters apitypes.WorkspaceParameters
		if err := parameters.UnmarshalJSON([]byte(`{"agent_type":"eino","audio_input":"` + value + `"}`)); err != nil {
			t.Fatalf("decode parameters: %v", err)
		}
		created, err := srv.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{
			Name: "workspace-" + value, WorkflowID: "workflow-eino", Parameters: &parameters,
		})
		if wantErr {
			if err == nil || !strings.Contains(err.Error(), "unsupported audio_input") {
				t.Fatalf("CreatePeerWorkspace(audio_input %q) error = %v", value, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("CreatePeerWorkspace(audio_input %q) error = %v", value, err)
		}
		if got, err := created.Parameters.AudioInput(); err != nil || got == nil || string(*got) != value {
			t.Fatalf("stored audio_input = %v, %v; want %q", got, err, value)
		}
	}
}

func TestValidateWorkspaceParametersPatchAudioInput(t *testing.T) {
	if err := validateWorkspaceParametersPatch(PeerWorkspaceParametersSetRequest{AudioInput: new(apitypes.AudioInputPathAsr)}); err != nil {
		t.Fatalf("validateWorkspaceParametersPatch(asr) error = %v", err)
	}
	err := validateWorkspaceParametersPatch(PeerWorkspaceParametersSetRequest{AudioInput: new(apitypes.AudioInputPath("direct"))})
	if err == nil || !strings.Contains(err.Error(), `unsupported audio_input "direct"`) {
		t.Fatalf("validateWorkspaceParametersPatch(direct) error = %v", err)
	}
}

func TestWorkspaceParametersPatchIgnoresTTSSpeechRateForSFU(t *testing.T) {
	updated, err := workspaceParametersWithPatch(nil, apitypes.WorkflowDriverSfu, nil, nil, new(70), nil, nil)
	if err != nil {
		t.Fatalf("workspaceParametersWithPatch(sfu) error = %v", err)
	}
	if updated != nil {
		t.Fatalf("sfu parameters = %+v, want unchanged nil", updated)
	}
}
