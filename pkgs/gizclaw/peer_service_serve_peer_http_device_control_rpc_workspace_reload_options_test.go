package gizclaw

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type reloadOptionsResources struct {
	*fakeRPCRunWorkspaceResources
	steps      *[]string
	patch      rpcapi.WorkspaceParametersSetRequest
	patchError *rpcapi.RPCStatus
	workflow   string
}

func (r *reloadOptionsResources) ResolveRunWorkspaceSelection(ctx context.Context, name string) (apitypes.Workspace, *rpcapi.RPCStatus) {
	canonical, err := r.ValidateRunWorkspaceSelection(ctx, name)
	return apitypes.Workspace{Name: canonical, WorkflowId: r.workflow}, err
}

func (r *reloadOptionsResources) Dispatch(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if req.Method != rpcapi.RPCMethodServerWorkspaceParametersSet {
		return nil, false, nil
	}
	*r.steps = append(*r.steps, "parameters")
	var err error
	r.patch, err = req.Params.AsWorkspaceParametersSetRequest()
	if err != nil {
		return nil, true, err
	}
	if r.patchError != nil {
		return &rpcapi.RPCResponse{Id: req.Id, Error: r.patchError}, true, nil
	}
	response, err := newRPCResultResponse(req.Id, rpcapi.Workspace{Name: r.patch.Name}, (*rpcapi.RPCPayload).FromWorkspaceParametersSetResponse)
	return response, true, err
}

type reloadOptionsRuntime struct {
	*fakeRPCPeerRunRuntime
	store       *peerrun.Server
	caller      giznet.PublicKey
	steps       *[]string
	reloadError error
}

func (r *reloadOptionsRuntime) SetRunAgent(ctx context.Context, selection apitypes.AgentSelection) (apitypes.PeerRunAgent, error) {
	*r.steps = append(*r.steps, "select")
	return r.store.SetRunAgent(ctx, r.caller, selection)
}
func (r *reloadOptionsRuntime) Reload(ctx context.Context) (apitypes.PeerRunStatus, error) {
	*r.steps = append(*r.steps, "reload")
	if r.reloadError != nil {
		return apitypes.PeerRunStatus{}, r.reloadError
	}
	return r.fakeRPCPeerRunRuntime.Reload(ctx)
}

func TestWorkspaceReloadWithOptions(t *testing.T) {
	for _, test := range []struct {
		name                                                                             string
		currentOnly, noParameters, sfu, invalidName, denied, invalidPatch, reloadFailure bool
		wantSteps                                                                        []string
		wantError                                                                        bool
	}{
		{name: "select configure reload", wantSteps: []string{"parameters", "select", "reload"}},
		{name: "current workspace", currentOnly: true, wantSteps: []string{"parameters", "reload"}},
		{name: "selection only", noParameters: true, wantSteps: []string{"select", "reload"}},
		{name: "SFU exactly one reload", sfu: true, wantSteps: []string{"parameters", "select", "reload"}},
		{name: "invalid name", invalidName: true, wantError: true},
		{name: "access denied", denied: true, wantError: true},
		{name: "parameter rejection keeps selection", invalidPatch: true, wantSteps: []string{"parameters"}, wantError: true},
		{name: "reload error propagates", reloadFailure: true, wantSteps: []string{"parameters", "select", "reload"}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			var steps []string
			runs := peerruntest.New(t)
			caller := giznet.PublicKey{1}
			if _, err := runs.SetRunAgent(ctx, caller, apitypes.AgentSelection{WorkspaceName: "old"}); err != nil {
				t.Fatal(err)
			}
			runtime := &reloadOptionsRuntime{
				fakeRPCPeerRunRuntime: &fakeRPCPeerRunRuntime{reload: apitypes.PeerRunStatus{State: apitypes.PeerRunStatusStateRunning, WorkspaceName: new("target")}},
				store:                 runs, caller: caller, steps: &steps,
			}
			resources := &reloadOptionsResources{fakeRPCRunWorkspaceResources: &fakeRPCRunWorkspaceResources{}, steps: &steps}
			if test.sfu {
				resources.workflow = socialutil.SFUWorkflowID
			}
			if test.denied {
				resources.rpcErr = &rpcapi.RPCStatus{Code: rpcapi.StatusCodeNotFound, Message: "not found"}
			}
			if test.invalidPatch {
				resources.patchError = &rpcapi.RPCStatus{Code: rpcapi.StatusCodeInvalidArgument, Message: "invalid parameters"}
			}
			if test.reloadFailure {
				runtime.reloadError = errors.New("attach failed")
			}
			options := rpcapi.ServerReloadRunWorkspaceWithOptionsRequest{WorkspaceName: new("target"), Parameters: &rpcapi.WorkspaceParametersPatch{Input: new(rpcapi.WorkspaceInputModeRealtime)}}
			if test.currentOnly {
				options.WorkspaceName = nil
			}
			if test.noParameters {
				options.Parameters = nil
			}
			if test.invalidName {
				options.WorkspaceName = new(" target ")
			}
			server := &rpcServer{peerRun: runs, peerRunRuntime: runtime, serverResources: resources, callerPublicKey: caller}
			payload, err := newRPCRequestParams(options, (*rpcapi.RPCPayload).FromServerReloadRunWorkspaceWithOptionsRequest)
			if err != nil {
				t.Fatal(err)
			}
			response, err := server.dispatch(ctx, newRPCRequest("reload", rpcapi.RPCMethodServerRunWorkspaceReloadWithOptions, payload))
			if err != nil {
				t.Fatal(err)
			}
			if (response.Error != nil) != test.wantError {
				t.Fatalf("response = %+v", response)
			}
			if !reflect.DeepEqual(steps, test.wantSteps) {
				t.Fatalf("steps = %v, want %v", steps, test.wantSteps)
			}
			if test.invalidPatch || test.denied || test.invalidName {
				agent, err := runs.GetRunAgent(ctx, caller)
				if err != nil || agent.Pending == nil || agent.Pending.WorkspaceName != "old" {
					t.Fatalf("selection changed on rejection: %+v %v", agent, err)
				}
			}
			if test.currentOnly && resources.patch.Name != "old" {
				t.Fatalf("updated %q instead of current workspace", resources.patch.Name)
			}
			if !test.wantError {
				state, err := response.Result.AsServerReloadRunWorkspaceWithOptionsResponse()
				if err != nil || state.RuntimeState != rpcapi.PeerRunStatusStateRunning {
					t.Fatalf("reload status: %+v, %v", state, err)
				}
			}
		})
	}
}
