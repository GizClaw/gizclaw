package peerresource

import (
	"context"
	"errors"
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
		Caller:      caller,
		ACL:         auth,
		Tools:       &toolkit.Server{Store: kv.NewMemory(nil)},
		ResourceACL: bindings,
	}

	createRequest := rpcTool(id, callerID)
	createRequest.Enabled = nil
	createdResp := callRPC(t, srv, "create", rpcapi.RPCMethodServerToolCreate, rpcParams(t, (*rpcapi.RPCPayload).FromToolCreateRequest, createRequest))
	requireNoRPCError(t, createdResp)
	created := mustResult(t, createdResp.Result.AsToolCreateResponse)
	if created.Id != id || created.Enabled == nil || !*created.Enabled || created.OwnerPeer == nil || *created.OwnerPeer != callerID || created.CreatedAt.IsZero() {
		t.Fatalf("created Tool = %#v", created)
	}
	if bindings.role != resourceOwnerRole || bindings.policy.Resource != acl.ToolResource(id) || bindings.policy.Subject != acl.PublicKeySubject(callerID) {
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
	if !bindings.deletedBinding(toolOwnerBindingID(id, callerID)) {
		t.Fatalf("owner binding was not deleted; deleted = %#v", bindings.deletedIDs)
	}
	if !bindings.deletedBinding(legacyToolOwnerBindingID(id, callerID)) {
		t.Fatalf("legacy owner binding was not deleted; deleted = %#v", bindings.deletedIDs)
	}
	wantDeleted := []string{legacyToolOwnerBindingID(id, callerID), toolOwnerBindingID(id, callerID)}
	if len(bindings.deletedIDs) < len(wantDeleted) || bindings.deletedIDs[0] != wantDeleted[0] || bindings.deletedIDs[1] != wantDeleted[1] {
		t.Fatalf("deleted owner binding order = %#v, want prefix %#v", bindings.deletedIDs, wantDeleted)
	}
}

func TestToolPeerCreateRejectsNonDeviceAndForeignNamespace(t *testing.T) {
	caller := giznet.PublicKey{2}
	callerID := caller.String()
	auth := newRuleAuthorizer()
	auth.allow(acl.ResourceKindTool, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	srv := &Server{Caller: caller, ACL: auth, Tools: &toolkit.Server{Store: kv.NewMemory(nil)}, ResourceACL: &recordingToolACL{}}

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

func TestToolPeerPutRejectsExistingNonOwnedTool(t *testing.T) {
	caller := giznet.PublicKey{4}
	callerID := caller.String()
	id := "peer." + callerID + ".admin-owned"
	auth := newRuleAuthorizer()
	auth.allow(acl.ResourceKindTool, id, apitypes.ACLPermissionAdmin)
	srv := &Server{Caller: caller, ACL: auth, Tools: &toolkit.Server{Store: kv.NewMemory(nil)}, ResourceACL: &recordingToolACL{}}
	existing := toolkit.Tool{ID: id, Source: toolkit.ToolSourceAdmin, Enabled: true, InputSchema: jsonschema.Schema{Type: "object"}, Executor: toolkit.ToolExecutor{Kind: toolkit.ToolExecutorKindBuiltin, Name: stringPointer("admin")}}
	if _, err := srv.Tools.PutTool(context.Background(), existing); err != nil {
		t.Fatalf("PutTool(existing) error = %v", err)
	}

	resp := callRPC(t, srv, "put", rpcapi.RPCMethodServerToolPut, rpcParams(t, (*rpcapi.RPCPayload).FromToolPutRequest, rpcapi.ToolPutRequest{Id: id, Body: rpcTool(id, callerID)}))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeForbidden {
		t.Fatalf("put non-owned Tool response = %#v", resp)
	}
	stored, err := srv.Tools.GetTool(context.Background(), id)
	if err != nil || stored.Source != toolkit.ToolSourceAdmin {
		t.Fatalf("stored Tool after rejected put = %#v, %v", stored, err)
	}
}

func TestToolPeerCreateCreatesResourceOwnerRoleWhenMissing(t *testing.T) {
	caller := giznet.PublicKey{5}
	callerID := caller.String()
	id := "peer." + callerID + ".music.play"
	auth := newRuleAuthorizer()
	auth.allow(acl.ResourceKindTool, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	bindings := &recordingToolACL{}
	srv := &Server{Caller: caller, ACL: auth, Tools: &toolkit.Server{Store: kv.NewMemory(nil)}, ResourceACL: bindings}

	resp := callRPC(t, srv, "create", rpcapi.RPCMethodServerToolCreate, rpcParams(t, (*rpcapi.RPCPayload).FromToolCreateRequest, rpcTool(id, callerID)))
	requireNoRPCError(t, resp)
	if bindings.rolePuts != 0 || bindings.roleCreates != 1 || bindings.roleGets != 1 || bindings.policy.Role != resourceOwnerRole {
		t.Fatalf("owner role handling = puts %d creates %d gets %d policy %#v", bindings.rolePuts, bindings.roleCreates, bindings.roleGets, bindings.policy)
	}
}

func TestToolPeerCreateReusesCurrentResourceOwnerRole(t *testing.T) {
	caller := giznet.PublicKey{8}
	callerID := caller.String()
	id := "peer." + callerID + ".music.play"
	auth := newRuleAuthorizer()
	auth.allow(acl.ResourceKindTool, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	bindings := &recordingToolACL{
		existingRole: apitypes.ACLRole{Name: resourceOwnerRole, Permissions: append(apitypes.ACLPermissionList(nil), resourceOwnerPermissions...)},
	}
	srv := &Server{Caller: caller, ACL: auth, Tools: &toolkit.Server{Store: kv.NewMemory(nil)}, ResourceACL: bindings}

	resp := callRPC(t, srv, "create", rpcapi.RPCMethodServerToolCreate, rpcParams(t, (*rpcapi.RPCPayload).FromToolCreateRequest, rpcTool(id, callerID)))
	requireNoRPCError(t, resp)
	if bindings.rolePuts != 0 || bindings.roleCreates != 0 || bindings.roleGets != 1 || bindings.policy.Role != resourceOwnerRole {
		t.Fatalf("owner role handling = puts %d creates %d gets %d policy %#v", bindings.rolePuts, bindings.roleCreates, bindings.roleGets, bindings.policy)
	}
}

func TestToolPeerCreateRejectsDriftedResourceOwnerRole(t *testing.T) {
	caller := giznet.PublicKey{9}
	callerID := caller.String()
	id := "peer." + callerID + ".music.play"
	auth := newRuleAuthorizer()
	auth.allow(acl.ResourceKindTool, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	bindings := &recordingToolACL{
		existingRole: apitypes.ACLRole{Name: resourceOwnerRole, Permissions: apitypes.ACLPermissionList{apitypes.ACLPermissionRead}},
	}
	srv := &Server{Caller: caller, ACL: auth, Tools: &toolkit.Server{Store: kv.NewMemory(nil)}, ResourceACL: bindings}

	resp := callRPC(t, srv, "create", rpcapi.RPCMethodServerToolCreate, rpcParams(t, (*rpcapi.RPCPayload).FromToolCreateRequest, rpcTool(id, callerID)))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInternalError {
		t.Fatalf("create with drifted owner role response = %#v", resp)
	}
	if _, err := srv.Tools.GetTool(context.Background(), id); !errors.Is(err, toolkit.ErrToolNotFound) {
		t.Fatalf("GetTool(rolled back) error = %v, want %v", err, toolkit.ErrToolNotFound)
	}
	if bindings.rolePuts != 0 || bindings.roleCreates != 0 || bindings.policy.Role != "" {
		t.Fatalf("owner role handling = puts %d creates %d policy %#v", bindings.rolePuts, bindings.roleCreates, bindings.policy)
	}
}

func TestToolPeerCreateRollsBackWhenOwnerRoleFails(t *testing.T) {
	caller := giznet.PublicKey{6}
	callerID := caller.String()
	id := "peer." + callerID + ".music.play"
	auth := newRuleAuthorizer()
	auth.allow(acl.ResourceKindTool, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	bindings := &recordingToolACL{
		roleErr: errors.New("owner role failed"),
	}
	srv := &Server{Caller: caller, ACL: auth, Tools: &toolkit.Server{Store: kv.NewMemory(nil)}, ResourceACL: bindings}

	resp := callRPC(t, srv, "create", rpcapi.RPCMethodServerToolCreate, rpcParams(t, (*rpcapi.RPCPayload).FromToolCreateRequest, rpcTool(id, callerID)))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInternalError {
		t.Fatalf("create with incompatible owner role response = %#v", resp)
	}
	if _, err := srv.Tools.GetTool(context.Background(), id); !errors.Is(err, toolkit.ErrToolNotFound) {
		t.Fatalf("GetTool(rolled back) error = %v, want %v", err, toolkit.ErrToolNotFound)
	}
	if bindings.policy.Role != "" {
		t.Fatalf("owner binding created with incompatible role = %#v", bindings.policy)
	}
}

func TestToolPeerDeleteRollsBackWhenOwnerBindingCleanupFails(t *testing.T) {
	caller := giznet.PublicKey{7}
	callerID := caller.String()
	id := "peer." + callerID + ".music.play"
	auth := newRuleAuthorizer()
	auth.allow(acl.ResourceKindTool, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	auth.allow(acl.ResourceKindTool, id, apitypes.ACLPermissionAdmin)
	bindings := &recordingToolACL{}
	srv := &Server{Caller: caller, ACL: auth, Tools: &toolkit.Server{Store: kv.NewMemory(nil)}, ResourceACL: bindings}

	createResp := callRPC(t, srv, "create", rpcapi.RPCMethodServerToolCreate, rpcParams(t, (*rpcapi.RPCPayload).FromToolCreateRequest, rpcTool(id, callerID)))
	requireNoRPCError(t, createResp)
	bindings.deleteErr = errors.New("ACL cleanup failed")
	deleteResp := callRPC(t, srv, "delete", rpcapi.RPCMethodServerToolDelete, rpcParams(t, (*rpcapi.RPCPayload).FromToolDeleteRequest, rpcapi.ToolDeleteRequest{Id: id}))
	if deleteResp.Error == nil || deleteResp.Error.Code != rpcapi.RPCErrorCodeInternalError {
		t.Fatalf("delete with cleanup failure response = %#v", deleteResp)
	}
	if stored, err := srv.Tools.GetTool(context.Background(), id); err != nil || stored.ID != id {
		t.Fatalf("GetTool(rolled back) = %#v, %v", stored, err)
	}
}

func TestToolPeerListUsesStorageCursorOrdering(t *testing.T) {
	caller := giznet.PublicKey{3}
	auth := newRuleAuthorizer()
	srv := &Server{Caller: caller, ACL: auth, Tools: &toolkit.Server{Store: kv.NewMemory(nil)}}
	for _, id := range []string{"a/b", "a-b"} {
		tool := toolkit.Tool{ID: id, Source: toolkit.ToolSourceBuiltin, Enabled: true, InputSchema: jsonschema.Schema{Type: "object"}, Executor: toolkit.ToolExecutor{Kind: toolkit.ToolExecutorKindBuiltin, Name: stringPointer("test")}}
		if _, err := srv.Tools.PutTool(context.Background(), tool); err != nil {
			t.Fatalf("PutTool(%q) error = %v", id, err)
		}
		auth.allow(acl.ResourceKindTool, id, apitypes.ACLPermissionRead)
	}

	limit := 1
	firstResp := callRPC(t, srv, "list-first", rpcapi.RPCMethodServerToolList, rpcParams(t, (*rpcapi.RPCPayload).FromToolListRequest, rpcapi.ToolListRequest{Limit: &limit}))
	requireNoRPCError(t, firstResp)
	first := mustResult(t, firstResp.Result.AsToolListResponse)
	if len(first.Items) != 1 || first.Items[0].Id != "a/b" || first.NextCursor == nil {
		t.Fatalf("first Tool page = %#v", first)
	}

	secondResp := callRPC(t, srv, "list-second", rpcapi.RPCMethodServerToolList, rpcParams(t, (*rpcapi.RPCPayload).FromToolListRequest, rpcapi.ToolListRequest{Cursor: first.NextCursor, Limit: &limit}))
	requireNoRPCError(t, secondResp)
	second := mustResult(t, secondResp.Result.AsToolListResponse)
	if len(second.Items) != 1 || second.Items[0].Id != "a-b" || second.HasNext {
		t.Fatalf("second Tool page = %#v", second)
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
	role         string
	permissions  apitypes.ACLPermissionList
	policy       apitypes.ACLPolicy
	policies     map[string]apitypes.ACLPolicy
	deleted      string
	deletedIDs   []string
	roleErr      error
	putRoleErr   error
	roleCreates  int
	roleGets     int
	rolePuts     int
	existingRole apitypes.ACLRole
	getRoleErr   error
	deleteErr    error
	policyErr    error
}

func (a *recordingToolACL) CreateRole(_ context.Context, name string, permissions apitypes.ACLPermissionList) (apitypes.ACLRole, error) {
	a.roleCreates++
	a.role = name
	a.permissions = append(apitypes.ACLPermissionList(nil), permissions...)
	return apitypes.ACLRole{Name: name, Permissions: permissions}, a.roleErr
}

func (a *recordingToolACL) GetRole(_ context.Context, _ string) (apitypes.ACLRole, error) {
	a.roleGets++
	if a.getRoleErr != nil {
		return apitypes.ACLRole{}, a.getRoleErr
	}
	if a.existingRole.Name == "" {
		return apitypes.ACLRole{}, acl.ErrRoleNotFound
	}
	return a.existingRole, a.getRoleErr
}

func (a *recordingToolACL) PutRole(_ context.Context, name string, permissions apitypes.ACLPermissionList) (apitypes.ACLRole, error) {
	a.rolePuts++
	a.role = name
	a.permissions = append(apitypes.ACLPermissionList(nil), permissions...)
	return apitypes.ACLRole{Name: name, Permissions: permissions}, a.putRoleErr
}

func (a *recordingToolACL) PutPolicyBinding(_ context.Context, id string, _ float64, policy apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error) {
	a.policy = policy
	if a.policies == nil {
		a.policies = make(map[string]apitypes.ACLPolicy)
	}
	a.policies[id] = policy
	return apitypes.ACLPolicyBinding{Id: id, Policy: policy}, a.policyErr
}

func (a *recordingToolACL) DeletePolicyBinding(_ context.Context, id string) (apitypes.ACLPolicyBinding, error) {
	a.deleted = id
	a.deletedIDs = append(a.deletedIDs, id)
	return apitypes.ACLPolicyBinding{Id: id}, a.deleteErr
}

func (a *recordingToolACL) deletedBinding(id string) bool {
	for _, deleted := range a.deletedIDs {
		if deleted == id {
			return true
		}
	}
	return false
}

func (a *recordingToolACL) policyBinding(id string) (apitypes.ACLPolicy, bool) {
	policy, ok := a.policies[id]
	return policy, ok
}

func stringPointer(value string) *string { return &value }
