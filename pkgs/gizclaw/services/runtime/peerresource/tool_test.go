package peerresource

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestToolPeerCRUDNamespaceACLAndOwnerBinding(t *testing.T) {
	caller := giznet.PublicKey{1}
	callerID := caller.String()
	id := "peer." + callerID + ".music.play"
	auth := newRuleAuthorizer()
	auth.allow(acl.ResourceKindTool, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	bindings := &recordingToolACL{}
	srv := &Server{
		Caller:  caller,
		ACL:     auth,
		Tools:   &toolkit.Server{Store: kv.NewMemory(nil)},
		ToolACL: bindings,
	}

	createRequest := rpcTool(id, callerID)
	createRequest.Enabled = nil
	createdResp := callRPC(t, srv, "create", rpcapi.RPCMethodServerToolCreate, rpcParams(t, (*rpcapi.RPCPayload).FromToolCreateRequest, createRequest))
	requireNoRPCError(t, createdResp)
	created := mustResult(t, createdResp.Result.AsToolCreateResponse)
	if created.Id != id || created.Enabled == nil || !*created.Enabled || created.OwnerPeer == nil || *created.OwnerPeer != callerID || created.CreatedAt.IsZero() {
		t.Fatalf("created Tool = %#v", created)
	}
	if bindings.role != toolOwnerRole || bindings.policy.Resource != acl.ToolResource(id) || bindings.policy.Subject != acl.PublicKeySubject(callerID) {
		t.Fatalf("owner binding = role %q policy %#v", bindings.role, bindings.policy)
	}
	if len(bindings.permissions) != 3 {
		t.Fatalf("owner permissions = %#v", bindings.permissions)
	}

	auth.allow(acl.ResourceKindTool, id, apitypes.ACLPermissionRead)
	auth.allow(acl.ResourceKindTool, id, apitypes.ACLPermissionAdmin)
	getResp := callRPC(t, srv, "get", rpcapi.RPCMethodServerToolGet, rpcParams(t, (*rpcapi.RPCPayload).FromToolGetRequest, rpcapi.ToolGetRequest{Id: id}))
	requireNoRPCError(t, getResp)
	if got := mustResult(t, getResp.Result.AsToolGetResponse); got.Id != id {
		t.Fatalf("get Tool = %#v", got)
	}

	updated := rpcTool(id, callerID)
	description := "updated"
	updated.Description = &description
	putResp := callRPC(t, srv, "put", rpcapi.RPCMethodServerToolPut, rpcParams(t, (*rpcapi.RPCPayload).FromToolPutRequest, rpcapi.ToolPutRequest{Id: id, Body: updated}))
	requireNoRPCError(t, putResp)
	if got := mustResult(t, putResp.Result.AsToolPutResponse); got.Description == nil || *got.Description != description || got.CreatedAt != created.CreatedAt {
		t.Fatalf("put Tool = %#v", got)
	}

	other := toolkit.Tool{ID: "system.hidden", Source: toolkit.ToolSourceBuiltin, Enabled: true, InputSchema: jsonschema.Schema{Type: "object"}, Executor: toolkit.ToolExecutor{Kind: toolkit.ToolExecutorKindBuiltin, Name: stringPointer("hidden")}}
	if _, err := srv.Tools.PutTool(context.Background(), other); err != nil {
		t.Fatalf("PutTool(hidden) error = %v", err)
	}
	listResp := callRPC(t, srv, "list", rpcapi.RPCMethodServerToolList, rpcParams(t, (*rpcapi.RPCPayload).FromToolListRequest, rpcapi.ToolListRequest{}))
	requireNoRPCError(t, listResp)
	if got := mustResult(t, listResp.Result.AsToolListResponse); len(got.Items) != 1 || got.Items[0].Id != id {
		t.Fatalf("list Tools = %#v", got)
	}

	deleteResp := callRPC(t, srv, "delete", rpcapi.RPCMethodServerToolDelete, rpcParams(t, (*rpcapi.RPCPayload).FromToolDeleteRequest, rpcapi.ToolDeleteRequest{Id: id}))
	requireNoRPCError(t, deleteResp)
	if bindings.deleted == "" {
		t.Fatal("owner binding was not deleted")
	}
}

func TestToolPeerCreateRejectsNonDeviceAndForeignNamespace(t *testing.T) {
	caller := giznet.PublicKey{2}
	callerID := caller.String()
	auth := newRuleAuthorizer()
	auth.allow(acl.ResourceKindTool, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	srv := &Server{Caller: caller, ACL: auth, Tools: &toolkit.Server{Store: kv.NewMemory(nil)}, ToolACL: &recordingToolACL{}}

	foreign := rpcTool("peer.other.music.play", callerID)
	resp := callRPC(t, srv, "foreign", rpcapi.RPCMethodServerToolCreate, rpcParams(t, (*rpcapi.RPCPayload).FromToolCreateRequest, foreign))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("foreign namespace response = %#v", resp)
	}
	foreign.Id = "peer." + callerID + ".music.play"
	foreign.Source = rpcapi.ToolSourceAdmin
	resp = callRPC(t, srv, "admin", rpcapi.RPCMethodServerToolCreate, rpcParams(t, (*rpcapi.RPCPayload).FromToolCreateRequest, foreign))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("admin source response = %#v", resp)
	}
}

func rpcTool(id, peer string) rpcapi.Tool {
	method := "music.play"
	enabled := true
	return rpcapi.Tool{
		Id:          id,
		Source:      rpcapi.ToolSourceDevice,
		Enabled:     &enabled,
		OwnerPeer:   &peer,
		InputSchema: jsonschema.Schema{Type: "object"},
		Executor:    rpcapi.ToolExecutor{Kind: rpcapi.ToolExecutorKindDeviceRpc, Method: &method, PeerId: &peer},
	}
}

type recordingToolACL struct {
	role        string
	permissions apitypes.ACLPermissionList
	policy      apitypes.ACLPolicy
	deleted     string
}

func (a *recordingToolACL) PutRole(_ context.Context, name string, permissions apitypes.ACLPermissionList) (apitypes.ACLRole, error) {
	a.role = name
	a.permissions = append(apitypes.ACLPermissionList(nil), permissions...)
	return apitypes.ACLRole{Name: name, Permissions: permissions}, nil
}

func (a *recordingToolACL) PutPolicyBinding(_ context.Context, id string, _ float64, policy apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error) {
	a.policy = policy
	return apitypes.ACLPolicyBinding{Id: id, Policy: policy}, nil
}

func (a *recordingToolACL) DeletePolicyBinding(_ context.Context, id string) (apitypes.ACLPolicyBinding, error) {
	a.deleted = id
	return apitypes.ACLPolicyBinding{Id: id}, nil
}

func stringPointer(value string) *string { return &value }
