package workspace

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
)

func TestSafetyFencePatchRoundTripAndPreservation(t *testing.T) {
	for _, driver := range []apitypes.WorkflowDriver{apitypes.WorkflowDriverFlowcraft, apitypes.WorkflowDriverEino, apitypes.WorkflowDriverDoubaoRealtime, apitypes.WorkflowDriverDoubaoRealtimeDuplex, apitypes.WorkflowDriverDashscopeRealtime, apitypes.WorkflowDriverAstTranslate} {
		t.Run(string(driver), func(t *testing.T) {
			var parameters *apitypes.WorkspaceParameters
			for _, level := range []apitypes.SafetyFenceLevel{apitypes.SafetyFenceLevelGeneral, apitypes.SafetyFenceLevelChild, apitypes.SafetyFenceLevelOff} {
				updated, err := workspaceParametersWithPatch(parameters, driver, nil, nil, nil, &level)
				if err != nil {
					t.Fatal(err)
				}
				if parameters != nil {
					rate, err := updated.TTSSpeechRatePercent()
					if err != nil || rate == nil || *rate != 70 {
						t.Fatalf("fence-only patch lost rate: %v, %v", rate, err)
					}
				}
				parameters, err = workspaceParametersWithPatch(updated, driver, nil, nil, new(70), nil)
				if err != nil {
					t.Fatal(err)
				}
				got, err := parameters.SafetyFenceLevel()
				if err != nil || got == nil || *got != level {
					t.Fatalf("level = %v, %v; want %s", got, err, level)
				}
			}
		})
	}
	for _, level := range []apitypes.SafetyFenceLevel{"", "unknown", "GENERAL"} {
		if err := validateWorkspaceParametersPatch(PeerWorkspaceParametersSetRequest{SafetyFenceLevel: &level}); err == nil {
			t.Fatalf("accepted %q", level)
		}
	}
	for _, level := range []apitypes.SafetyFenceLevel{apitypes.SafetyFenceLevelOff, apitypes.SafetyFenceLevelGeneral, apitypes.SafetyFenceLevelChild} {
		got, err := workspaceParametersWithPatch(nil, apitypes.WorkflowDriverSfu, nil, nil, nil, &level)
		if err != nil || got != nil {
			t.Fatalf("SFU patch = %v, %v", got, err)
		}
	}
}

func TestCreateAndPutRejectInvalidSafetyFence(t *testing.T) {
	srv := newTestServer(t)
	seedFlowcraftWorkflow(t, srv, "workflow-1", "model-1")
	seedModel(t, srv, "model-1", apitypes.ModelKindLlm)
	ctx := ownership.WithOwner(t.Context(), "peer-owner")
	invalid := flowcraftInputParameters(t, apitypes.FlowcraftWorkspaceParameters{AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft, SafetyFenceLevel: new(apitypes.SafetyFenceLevel("invalid"))})
	if _, err := srv.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{Name: "bad", WorkflowID: "workflow-1", Parameters: invalid}); err == nil {
		t.Fatal("create accepted invalid level")
	}
	created, err := srv.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{Name: "workspace", WorkflowID: "workflow-1"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: created.Id, Body: &adminhttp.WorkspaceUpsert{Id: created.Id, Name: created.Name, WorkflowId: created.WorkflowId, Parameters: invalid}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.PutWorkspace400JSONResponse); !ok {
		t.Fatalf("put = %T; want 400", response)
	}
}
