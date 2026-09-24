package gizclaw

import (
	"context"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workspacetest"
)

func TestDeviceFactoryResetMarksTransition(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	var got rpcapi.ClientDeviceFactoryResetRequest
	device := newFakeToolConn(func(_ context.Context, tool rpcpb.ClientTool, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		if tool != rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FACTORY_RESET {
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "unexpected"}.RPCResponse(), nil
		}
		params, err := req.Params.AsClientDeviceFactoryResetRequest()
		if err != nil {
			return nil, err
		}
		got = params
		return newRPCResultResponse(req.Id, rpcapi.ClientDeviceFactoryResetResponse{}, (*rpcapi.RPCPayload).FromClientDeviceFactoryResetResponse)
	})
	f.manager.SetPeerUp(f.owner, device)
	if response := f.invoke(t, "device.factory_reset", `{"keep_network":true}`); response.Code != http.StatusOK {
		t.Fatalf("factory reset status = %d body=%s", response.Code, response.Body.String())
	}
	if got.KeepNetwork == nil || !*got.KeepNetwork {
		t.Fatalf("factory reset request = %+v", got)
	}
	// The acknowledging connection is resetting, so later commands answer
	// offline instead of reaching a device that is erasing its state.
	response := f.invoke(t, "device.status.get", `{}`)
	if response.Code != http.StatusConflict || errorCode(t, response) != deviceOfflineCode {
		t.Fatalf("after reset status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestDeviceToolRunWorkspace(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	var sets []rpcapi.ClientRunWorkspaceSetRequest
	device := newFakeToolConn(func(_ context.Context, tool rpcpb.ClientTool, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		switch tool {
		case rpcpb.ClientTool_CLIENT_TOOL_RUN_WORKSPACE_SET:
			params, err := req.Params.AsClientRunWorkspaceSetRequest()
			if err != nil {
				return nil, err
			}
			sets = append(sets, params)
			return newRPCResultResponse(req.Id, rpcapi.ClientRunWorkspaceSetResponse{}, (*rpcapi.RPCPayload).FromClientRunWorkspaceSetResponse)
		default:
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "unexpected"}.RPCResponse(), nil
		}
	})
	f.manager.SetPeerUp(f.owner, device)

	created := seedDeviceWorkspaces(t, f)
	// A second save of the same workflow, active more recently than aesop-save,
	// is the one a workflow target resolves to.
	workspacetest.Seed(t, f.workspaces, apitypes.Workspace{
		Id: "ws-aesop-2", Name: "aesop-later", WorkflowId: "secret-workflow-aesop", OwnerPublicKey: new(f.owner.String()),
		System: new(false), Labels: &map[string]string{"collection": "story-teller"},
		CreatedAt: created, UpdatedAt: created, LastActiveAt: created.Add(3 * time.Hour),
	})
	// A system Workspace whose workflow resolves is available, and more recent
	// than every save, yet is never a switch target by name or by workflow.
	workspacetest.Seed(t, f.workspaces, apitypes.Workspace{
		Id: "ws-system-aesop", Name: "system-aesop", WorkflowId: "secret-workflow-aesop", OwnerPublicKey: new(f.owner.String()),
		System: new(true), Labels: &map[string]string{"collection": "story-teller"},
		CreatedAt: created, UpdatedAt: created, LastActiveAt: created.Add(4 * time.Hour),
	})

	for _, tc := range []struct {
		body, want string
		kickoff    bool
	}{
		{`{"workspace_name":"riddle-save"}`, "riddle-save", false},
		{`{"collection":"story-teller","workflow_name":"story.aesop","kickoff":true}`, "aesop-later", true},
	} {
		sets = nil
		if response := f.invoke(t, "run.workspace.set", tc.body); response.Code != http.StatusOK {
			t.Fatalf("workspace set %s status = %d body=%s", tc.body, response.Code, response.Body.String())
		}
		if len(sets) != 1 || sets[0].WorkspaceName != tc.want || (sets[0].Kickoff != nil && *sets[0].Kickoff) != tc.kickoff {
			t.Fatalf("workspace set %s forwarded %+v, want %s", tc.body, sets, tc.want)
		}
	}
	sets = nil
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{}`, http.StatusBadRequest},
		{`{"workspace_name":"a","collection":"b","workflow_name":"c"}`, http.StatusBadRequest},
		{`{"collection":"stories"}`, http.StatusBadRequest},
		{`{"workspace_name":""}`, http.StatusBadRequest},
		// Unknown, unavailable (dangling binding), system, and foreign
		// Workspaces never reach the device.
		{`{"workspace_name":"missing"}`, http.StatusNotFound},
		{`{"workspace_name":"orphan-save"}`, http.StatusNotFound},
		{`{"workspace_name":"pet"}`, http.StatusNotFound},
		{`{"workspace_name":"system-aesop"}`, http.StatusNotFound},
		{`{"collection":"games","workflow_name":"story.aesop"}`, http.StatusNotFound},
	} {
		response := f.invoke(t, "run.workspace.set", tc.body)
		if response.Code != tc.status {
			t.Fatalf("workspace set %s status = %d body=%s", tc.body, response.Code, response.Body.String())
		}
		if tc.status == http.StatusNotFound && errorCode(t, response) != "WORKSPACE_NOT_FOUND" {
			t.Fatalf("workspace set %s code = %s", tc.body, errorCode(t, response))
		}
	}
	if len(sets) != 0 {
		t.Fatalf("rejected targets reached the device: %+v", sets)
	}
}

func TestDeviceRuntimeReportsPendingWorkspace(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	if _, err := f.manager.PeerRun.SetRunAgent(context.Background(), f.owner, apitypes.AgentSelection{WorkspaceName: "bedtime"}); err != nil {
		t.Fatal(err)
	}
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/runtime", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"pending_workspace_name":"bedtime"`) ||
		strings.Contains(response.Body.String(), "active_workspace_name") {
		t.Fatalf("runtime status = %d body=%s", response.Code, response.Body.String())
	}
}

type fakeControlProfiles struct {
	tools map[string]apitypes.RuntimeProfileBinding
}

func (p fakeControlProfiles) ResolveOwnerProfile(context.Context, string) (apitypes.RuntimeProfile, error) {
	return apitypes.RuntimeProfile{Id: "p", Revision: "1", Spec: apitypes.RuntimeProfileSpec{
		Resources: apitypes.RuntimeProfileResources{Tools: &p.tools},
	}}, nil
}
