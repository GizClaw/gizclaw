package peerresource

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	_ "modernc.org/sqlite"
)

func TestServerAllowedCRUD(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}

	flowCreate := callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("workflow-a1")))
	requireNoRPCError(t, flowCreate)
	if got := mustResult(t, flowCreate.Result.AsWorkflowCreateResponse).Metadata.Name; got != "workflow-a1" {
		t.Fatalf("workflow.create name = %q", got)
	}

	flowList := callRPC(t, srv, "workflow-list", rpcapi.RPCMethodServerWorkflowList, nil)
	if got := mustResult(t, flowList.Result.AsWorkflowListResponse); len(got.Items) != 1 || got.Items[0].Metadata.Name != "workflow-a1" {
		t.Fatalf("workflow.list = %#v", got)
	}

	flowGet := callRPC(t, srv, "workflow-get", rpcapi.RPCMethodServerWorkflowGet, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowGetRequest, rpcapi.WorkflowGetRequest{Name: "workflow-a1"}))
	if got := mustResult(t, flowGet.Result.AsWorkflowGetResponse).Metadata.Name; got != "workflow-a1" {
		t.Fatalf("workflow.get name = %q", got)
	}

	flowPut := callRPC(t, srv, "workflow-put", rpcapi.RPCMethodServerWorkflowPut, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowPutRequest, rpcapi.WorkflowPutRequest{
		Name: "workflow-a1",
		Body: workflowDoc("workflow-a1"),
	}))
	requireNoRPCError(t, flowPut)

	createInput := rpcapi.WorkspaceInputModePushToTalk
	var createParams rpcapi.WorkspaceParameters
	if err := createParams.FromFlowcraftWorkspaceParameters(rpcapi.FlowcraftWorkspaceParameters{Input: &createInput}); err != nil {
		t.Fatalf("create workspace parameters: %v", err)
	}
	workspaceCreate := callRPC(t, srv, "workspace-create", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-a",
		WorkflowName: "workflow-a1",
		Parameters:   &createParams,
	}))
	if got := mustResult(t, workspaceCreate.Result.AsWorkspaceCreateResponse); got.Name != "workspace-a" || got.WorkflowName != "workflow-a1" {
		t.Fatalf("workspace.create = %#v", got)
	} else if got.Parameters == nil {
		t.Fatalf("workspace.create parameters are nil: %#v", got)
	} else if typed, err := got.Parameters.AsFlowcraftWorkspaceParameters(); err != nil {
		t.Fatalf("workspace.create parameters decode: %v", err)
	} else if typed.Input == nil || *typed.Input != rpcapi.WorkspaceInputModePushToTalk {
		t.Fatalf("workspace.create input = %#v, want push-to-talk", typed.Input)
	}

	workspaceList := callRPC(t, srv, "workspace-list", rpcapi.RPCMethodServerWorkspaceList, nil)
	if got := mustResult(t, workspaceList.Result.AsWorkspaceListResponse); len(got.Items) != 1 || got.Items[0].Name != "workspace-a" {
		t.Fatalf("workspace.list = %#v", got)
	}

	workspaceGet := callRPC(t, srv, "workspace-get", rpcapi.RPCMethodServerWorkspaceGet, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceGetRequest, rpcapi.WorkspaceGetRequest{Name: "workspace-a"}))
	if got := mustResult(t, workspaceGet.Result.AsWorkspaceGetResponse).Name; got != "workspace-a" {
		t.Fatalf("workspace.get name = %q", got)
	}

	updateInput := rpcapi.WorkspaceInputModeRealtime
	var updateParams rpcapi.WorkspaceParameters
	if err := updateParams.FromFlowcraftWorkspaceParameters(rpcapi.FlowcraftWorkspaceParameters{Input: &updateInput}); err != nil {
		t.Fatalf("update workspace parameters: %v", err)
	}
	workspacePut := callRPC(t, srv, "workspace-put", rpcapi.RPCMethodServerWorkspacePut, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspacePutRequest, rpcapi.WorkspacePutRequest{
		Name: "workspace-a",
		Body: rpcapi.Workspace{Name: "workspace-a", WorkflowName: "workflow-a1", Parameters: &updateParams},
	}))
	if got := mustResult(t, workspacePut.Result.AsWorkspacePutResponse); got.Parameters == nil {
		t.Fatalf("workspace.put parameters are nil: %#v", got)
	} else if typed, err := got.Parameters.AsFlowcraftWorkspaceParameters(); err != nil {
		t.Fatalf("workspace.put parameters decode: %v", err)
	} else if typed.Input == nil || *typed.Input != rpcapi.WorkspaceInputModeRealtime {
		t.Fatalf("workspace.put input = %#v, want realtime", typed.Input)
	}

	modelCreate := callRPC(t, srv, "model-create", rpcapi.RPCMethodServerModelCreate, rpcParams(t, (*rpcapi.RPCPayload).FromModelCreateRequest, rpcModel("model-a")))
	if got := mustResult(t, modelCreate.Result.AsModelCreateResponse).Id; got != "model-a" {
		t.Fatalf("model.create id = %q", got)
	}

	modelList := callRPC(t, srv, "model-list", rpcapi.RPCMethodServerModelList, nil)
	if got := mustResult(t, modelList.Result.AsModelListResponse); len(got.Items) != 1 || got.Items[0].Id != "model-a" {
		t.Fatalf("model.list = %#v", got)
	}

	modelGet := callRPC(t, srv, "model-get", rpcapi.RPCMethodServerModelGet, rpcParams(t, (*rpcapi.RPCPayload).FromModelGetRequest, rpcapi.ModelGetRequest{Id: "model-a"}))
	if got := mustResult(t, modelGet.Result.AsModelGetResponse).Id; got != "model-a" {
		t.Fatalf("model.get id = %q", got)
	}

	updatedModel := rpcModel("model-a")
	modelName := "updated model"
	updatedModel.Name = &modelName
	modelPut := callRPC(t, srv, "model-put", rpcapi.RPCMethodServerModelPut, rpcParams(t, (*rpcapi.RPCPayload).FromModelPutRequest, rpcapi.ModelPutRequest{
		Id:   "model-a",
		Body: updatedModel,
	}))
	if got := mustResult(t, modelPut.Result.AsModelPutResponse); got.Name == nil || *got.Name != modelName {
		t.Fatalf("model.put = %#v", got)
	}

	credentialCreate := callRPC(t, srv, "credential-create", rpcapi.RPCMethodServerCredentialCreate, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialCreateRequest, rpcCredential("credential-a", "sk-a")))
	requireNoRPCError(t, credentialCreate)
	if got := mustResult(t, credentialCreate.Result.AsCredentialCreateResponse).Name; got != "credential-a" {
		t.Fatalf("credential.create name = %q", got)
	}

	credentialList := callRPC(t, srv, "credential-list", rpcapi.RPCMethodServerCredentialList, nil)
	if got := mustResult(t, credentialList.Result.AsCredentialListResponse); len(got.Items) != 1 || got.Items[0].Name != "credential-a" {
		t.Fatalf("credential.list = %#v", got)
	}

	credentialGet := callRPC(t, srv, "credential-get", rpcapi.RPCMethodServerCredentialGet, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialGetRequest, rpcapi.CredentialGetRequest{Name: "credential-a"}))
	if got := mustResult(t, credentialGet.Result.AsCredentialGetResponse).Name; got != "credential-a" {
		t.Fatalf("credential.get name = %q", got)
	}

	credentialPut := callRPC(t, srv, "credential-put", rpcapi.RPCMethodServerCredentialPut, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialPutRequest, rpcapi.CredentialPutRequest{
		Name: "credential-a",
		Body: rpcCredential("credential-a", "sk-b"),
	}))
	if got := testRPCCredentialBodyString(mustResult(t, credentialPut.Result.AsCredentialPutResponse).Body, "api_key"); got != "sk-b" {
		t.Fatalf("credential.put body api_key = %#v", got)
	}

	requireNoRPCError(t, callRPC(t, srv, "credential-delete", rpcapi.RPCMethodServerCredentialDelete, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialDeleteRequest, rpcapi.CredentialDeleteRequest{Name: "credential-a"})))
	requireNoRPCError(t, callRPC(t, srv, "model-delete", rpcapi.RPCMethodServerModelDelete, rpcParams(t, (*rpcapi.RPCPayload).FromModelDeleteRequest, rpcapi.ModelDeleteRequest{Id: "model-a"})))
	requireNoRPCError(t, callRPC(t, srv, "workspace-delete", rpcapi.RPCMethodServerWorkspaceDelete, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceDeleteRequest, rpcapi.WorkspaceDeleteRequest{Name: "workspace-a"})))
	requireNoRPCError(t, callRPC(t, srv, "workflow-delete", rpcapi.RPCMethodServerWorkflowDelete, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowDeleteRequest, rpcapi.WorkflowDeleteRequest{Name: "workflow-a1"})))
}

func TestPeerCreateGrantsResourceOwnerBindings(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	bindings := &recordingToolACL{}
	srv.ResourceACL = bindings

	requireNoRPCError(t, callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("workflow-owner"))))
	requireNoRPCError(t, callRPC(t, srv, "workspace-create", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-owner",
		WorkflowName: "workflow-owner",
	})))
	requireNoRPCError(t, callRPC(t, srv, "model-create", rpcapi.RPCMethodServerModelCreate, rpcParams(t, (*rpcapi.RPCPayload).FromModelCreateRequest, rpcModel("model-owner"))))
	requireNoRPCError(t, callRPC(t, srv, "credential-create", rpcapi.RPCMethodServerCredentialCreate, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialCreateRequest, rpcCredential("credential-owner", "sk-owner"))))

	if bindings.role != resourceOwnerRole || !permissionListsEqual(bindings.permissions, resourceOwnerPermissions) {
		t.Fatalf("resource owner role = %q %#v", bindings.role, bindings.permissions)
	}
	for _, resource := range []apitypes.ACLResource{
		workflowResource("workflow-owner"),
		acl.WorkspaceResource("workspace-owner"),
		acl.ModelResource("model-owner"),
		acl.CredentialResource("credential-owner"),
	} {
		policy, ok := bindings.policyBinding(resourceOwnerBindingID(resource))
		if !ok {
			t.Fatalf("owner binding for %#v was not created; policies = %#v", resource, bindings.policies)
		}
		if policy.Subject != acl.PublicKeySubject(srv.Caller.String()) ||
			policy.Resource != resource ||
			policy.Role != resourceOwnerRole {
			t.Fatalf("owner policy for %#v = %#v", resource, policy)
		}
	}

	requireNoRPCError(t, callRPC(t, srv, "workspace-delete", rpcapi.RPCMethodServerWorkspaceDelete, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceDeleteRequest, rpcapi.WorkspaceDeleteRequest{Name: "workspace-owner"})))
	requireNoRPCError(t, callRPC(t, srv, "credential-delete", rpcapi.RPCMethodServerCredentialDelete, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialDeleteRequest, rpcapi.CredentialDeleteRequest{Name: "credential-owner"})))
	requireNoRPCError(t, callRPC(t, srv, "model-delete", rpcapi.RPCMethodServerModelDelete, rpcParams(t, (*rpcapi.RPCPayload).FromModelDeleteRequest, rpcapi.ModelDeleteRequest{Id: "model-owner"})))
	requireNoRPCError(t, callRPC(t, srv, "workflow-delete", rpcapi.RPCMethodServerWorkflowDelete, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowDeleteRequest, rpcapi.WorkflowDeleteRequest{Name: "workflow-owner"})))
	for _, resource := range []apitypes.ACLResource{
		acl.WorkspaceResource("workspace-owner"),
		acl.CredentialResource("credential-owner"),
		acl.ModelResource("model-owner"),
		workflowResource("workflow-owner"),
	} {
		if !bindings.deletedBinding(resourceOwnerBindingID(resource)) {
			t.Fatalf("owner binding for %#v was not deleted; deleted = %#v", resource, bindings.deletedIDs)
		}
	}
}

func TestModelPeerCreateRollsBackWhenOwnerGrantFails(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	srv.ResourceACL = &recordingToolACL{policyErr: errors.New("write owner binding")}

	created := callRPC(t, srv, "model-create", rpcapi.RPCMethodServerModelCreate, rpcParams(t, (*rpcapi.RPCPayload).FromModelCreateRequest, rpcModel("model-rollback")))
	requireRPCError(t, created, rpcapi.RPCErrorCodeInternalError)

	missing := callRPC(t, srv, "model-get", rpcapi.RPCMethodServerModelGet, rpcParams(t, (*rpcapi.RPCPayload).FromModelGetRequest, rpcapi.ModelGetRequest{Id: "model-rollback"}))
	requireRPCError(t, missing, rpcapi.RPCErrorCodeNotFound)
}

func TestModelPeerDeleteRollsBackWhenOwnerBindingDeleteFails(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	bindings := &recordingToolACL{}
	srv.ResourceACL = bindings

	requireNoRPCError(t, callRPC(t, srv, "model-create", rpcapi.RPCMethodServerModelCreate, rpcParams(t, (*rpcapi.RPCPayload).FromModelCreateRequest, rpcModel("model-delete-rollback"))))

	bindings.deleteErr = errors.New("delete owner binding")
	deleted := callRPC(t, srv, "model-delete", rpcapi.RPCMethodServerModelDelete, rpcParams(t, (*rpcapi.RPCPayload).FromModelDeleteRequest, rpcapi.ModelDeleteRequest{Id: "model-delete-rollback"}))
	requireRPCError(t, deleted, rpcapi.RPCErrorCodeInternalError)
	if !bindings.deletedBinding(resourceOwnerBindingID(acl.ModelResource("model-delete-rollback"))) {
		t.Fatalf("model owner binding was not deleted; deleted = %#v", bindings.deletedIDs)
	}
	got := callRPC(t, srv, "model-get", rpcapi.RPCMethodServerModelGet, rpcParams(t, (*rpcapi.RPCPayload).FromModelGetRequest, rpcapi.ModelGetRequest{Id: "model-delete-rollback"}))
	requireNoRPCError(t, got)
	if model := mustResult(t, got.Result.AsModelGetResponse); model.Id != "model-delete-rollback" {
		t.Fatalf("model after rollback = %#v", model)
	}
}

func TestCredentialPeerCreateRollsBackWhenOwnerGrantFails(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	srv.ResourceACL = &recordingToolACL{policyErr: errors.New("write owner binding")}

	created := callRPC(t, srv, "credential-create", rpcapi.RPCMethodServerCredentialCreate, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialCreateRequest, rpcCredential("credential-rollback", "sk-rollback")))
	requireRPCError(t, created, rpcapi.RPCErrorCodeInternalError)

	missing := callRPC(t, srv, "credential-get", rpcapi.RPCMethodServerCredentialGet, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialGetRequest, rpcapi.CredentialGetRequest{Name: "credential-rollback"}))
	requireRPCError(t, missing, rpcapi.RPCErrorCodeNotFound)
}

func TestCredentialPeerDeleteRollsBackWhenOwnerBindingDeleteFails(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	bindings := &recordingToolACL{}
	srv.ResourceACL = bindings

	requireNoRPCError(t, callRPC(t, srv, "credential-create", rpcapi.RPCMethodServerCredentialCreate, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialCreateRequest, rpcCredential("credential-delete-rollback", "sk-rollback"))))

	bindings.deleteErr = errors.New("delete owner binding")
	deleted := callRPC(t, srv, "credential-delete", rpcapi.RPCMethodServerCredentialDelete, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialDeleteRequest, rpcapi.CredentialDeleteRequest{Name: "credential-delete-rollback"}))
	requireRPCError(t, deleted, rpcapi.RPCErrorCodeInternalError)
	if !bindings.deletedBinding(resourceOwnerBindingID(acl.CredentialResource("credential-delete-rollback"))) {
		t.Fatalf("credential owner binding was not deleted; deleted = %#v", bindings.deletedIDs)
	}
	got := callRPC(t, srv, "credential-get", rpcapi.RPCMethodServerCredentialGet, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialGetRequest, rpcapi.CredentialGetRequest{Name: "credential-delete-rollback"}))
	requireNoRPCError(t, got)
	if credential := mustResult(t, got.Result.AsCredentialGetResponse); credential.Name != "credential-delete-rollback" {
		t.Fatalf("credential after rollback = %#v", credential)
	}
}

func TestWorkflowPeerCreateRollsBackWhenOwnerGrantFails(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	srv.ResourceACL = &recordingToolACL{policyErr: errors.New("write owner binding")}

	created := callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("workflow-rollback")))
	requireRPCError(t, created, rpcapi.RPCErrorCodeInternalError)

	missing := callRPC(t, srv, "workflow-get", rpcapi.RPCMethodServerWorkflowGet, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowGetRequest, rpcapi.WorkflowGetRequest{Name: "workflow-rollback"}))
	requireRPCError(t, missing, rpcapi.RPCErrorCodeNotFound)
}

func TestWorkflowPeerDeleteRollsBackWhenOwnerBindingDeleteFails(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	bindings := &recordingToolACL{}
	srv.ResourceACL = bindings

	requireNoRPCError(t, callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("workflow-delete-rollback"))))

	bindings.deleteErr = errors.New("delete owner binding")
	deleted := callRPC(t, srv, "workflow-delete", rpcapi.RPCMethodServerWorkflowDelete, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowDeleteRequest, rpcapi.WorkflowDeleteRequest{Name: "workflow-delete-rollback"}))
	requireRPCError(t, deleted, rpcapi.RPCErrorCodeInternalError)
	if !bindings.deletedBinding(resourceOwnerBindingID(workflowResource("workflow-delete-rollback"))) {
		t.Fatalf("workflow owner binding was not deleted; deleted = %#v", bindings.deletedIDs)
	}
	got := callRPC(t, srv, "workflow-get", rpcapi.RPCMethodServerWorkflowGet, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowGetRequest, rpcapi.WorkflowGetRequest{Name: "workflow-delete-rollback"}))
	requireNoRPCError(t, got)
	if workflow := mustResult(t, got.Result.AsWorkflowGetResponse); workflow.Metadata.Name != "workflow-delete-rollback" {
		t.Fatalf("workflow after rollback = %#v", workflow)
	}
}

func TestWorkspacePeerCreateRollsBackWhenOwnerGrantFails(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	bindings := &recordingToolACL{}
	srv.ResourceACL = bindings

	requireNoRPCError(t, callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("workflow-rollback"))))
	bindings.policyErr = errors.New("write owner binding")
	created := callRPC(t, srv, "workspace-create", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-rollback",
		WorkflowName: "workflow-rollback",
	}))
	requireRPCError(t, created, rpcapi.RPCErrorCodeInternalError)

	missing := callRPC(t, srv, "workspace-get", rpcapi.RPCMethodServerWorkspaceGet, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceGetRequest, rpcapi.WorkspaceGetRequest{Name: "workspace-rollback"}))
	requireRPCError(t, missing, rpcapi.RPCErrorCodeNotFound)
}

func TestWorkspacePeerDeleteRollsBackWhenOwnerBindingDeleteFails(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	bindings := &recordingToolACL{}
	srv.ResourceACL = bindings

	requireNoRPCError(t, callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("workflow-delete-rollback"))))
	requireNoRPCError(t, callRPC(t, srv, "workspace-create", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-delete-rollback",
		WorkflowName: "workflow-delete-rollback",
	})))

	bindings.deleteErr = errors.New("delete owner binding")
	deleted := callRPC(t, srv, "workspace-delete", rpcapi.RPCMethodServerWorkspaceDelete, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceDeleteRequest, rpcapi.WorkspaceDeleteRequest{Name: "workspace-delete-rollback"}))
	requireRPCError(t, deleted, rpcapi.RPCErrorCodeInternalError)
	if !bindings.deletedBinding(resourceOwnerBindingID(acl.WorkspaceResource("workspace-delete-rollback"))) {
		t.Fatalf("workspace owner binding was not deleted; deleted = %#v", bindings.deletedIDs)
	}
	got := callRPC(t, srv, "workspace-get", rpcapi.RPCMethodServerWorkspaceGet, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceGetRequest, rpcapi.WorkspaceGetRequest{Name: "workspace-delete-rollback"}))
	requireNoRPCError(t, got)
	if workspace := mustResult(t, got.Result.AsWorkspaceGetResponse); workspace.Name != "workspace-delete-rollback" {
		t.Fatalf("workspace after rollback = %#v", workspace)
	}
}

func TestWorkspacePeerDeleteRestoresOwnerBindingWhenResourceDeleteFails(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	bindings := &recordingToolACL{}
	srv.ResourceACL = bindings
	runtimeStore := &failingWorkspaceRuntimeStore{}
	srv.Workspaces.(*workspace.Server).RuntimeStore = runtimeStore

	requireNoRPCError(t, callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("workflow-delete-fail"))))
	requireNoRPCError(t, callRPC(t, srv, "workspace-create", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-delete-fail",
		WorkflowName: "workflow-delete-fail",
	})))
	bindingID := resourceOwnerBindingID(acl.WorkspaceResource("workspace-delete-fail"))
	originalPolicy, ok := bindings.policyBinding(bindingID)
	if !ok {
		t.Fatalf("workspace owner binding was not created; policies = %#v", bindings.policies)
	}

	runtimeStore.deleteErr = errors.New("delete runtime failed")
	deleted := callRPC(t, srv, "workspace-delete", rpcapi.RPCMethodServerWorkspaceDelete, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceDeleteRequest, rpcapi.WorkspaceDeleteRequest{Name: "workspace-delete-fail"}))
	requireRPCError(t, deleted, rpcapi.RPCErrorCodeInternalError)
	if restoredPolicy, ok := bindings.policyBinding(bindingID); !ok || restoredPolicy != originalPolicy {
		t.Fatalf("workspace owner binding after failed delete = %#v, %v; want %#v", restoredPolicy, ok, originalPolicy)
	}
	got := callRPC(t, srv, "workspace-get", rpcapi.RPCMethodServerWorkspaceGet, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceGetRequest, rpcapi.WorkspaceGetRequest{Name: "workspace-delete-fail"}))
	requireNoRPCError(t, got)
	if workspace := mustResult(t, got.Result.AsWorkspaceGetResponse); workspace.Name != "workspace-delete-fail" {
		t.Fatalf("workspace after failed delete = %#v", workspace)
	}
}

func TestServerRejectsInvalidCustomIDs(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}

	workflowCreate := callRPC(t, srv, "workflow-create-invalid", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("flow-a")))
	requireRPCError(t, workflowCreate, rpcapi.RPCErrorCodeBadRequest)

	workspaceCreate := callRPC(t, srv, "workspace-create-invalid", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "bad",
		WorkflowName: "workflow-a1",
	}))
	requireRPCError(t, workspaceCreate, rpcapi.RPCErrorCodeBadRequest)
}

func TestServerACLBoundaries(t *testing.T) {
	ctx := context.Background()
	auth := newRuleAuthorizer()
	srv := newTestResourceServer()
	srv.ACL = auth

	auth.allow(acl.ResourceKindWorkflow, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	requireNoRPCError(t, callRPC(t, srv, "workflow-create-a", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("workflow-a1"))))
	requireNoRPCError(t, callRPC(t, srv, "workflow-create-b", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("workflow-b1"))))

	auth.allow(acl.ResourceKindWorkspace, "workspace-a", apitypes.ACLPermissionAdmin)
	denied := callRPC(t, srv, "workspace-create-denied", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-a",
		WorkflowName: "workflow-a1",
	}))
	if denied.Error == nil || denied.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("workspace.create denied response = %#v", denied)
	}

	auth.allow(acl.ResourceKindWorkspace, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	auth.allow(acl.ResourceKindWorkflow, "workflow-a1", apitypes.ACLPermissionUse)
	requireNoRPCError(t, callRPC(t, srv, "workspace-create-allowed", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-a",
		WorkflowName: "workflow-a1",
	})))

	auth.allow(acl.ResourceKindWorkspace, "workspace-b", apitypes.ACLPermissionAdmin)
	auth.allow(acl.ResourceKindWorkflow, "workflow-b1", apitypes.ACLPermissionUse)
	requireNoRPCError(t, callRPC(t, srv, "workspace-create-b", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-b",
		WorkflowName: "workflow-b1",
	})))

	auth.allow(acl.ResourceKindWorkspace, "workspace-a", apitypes.ACLPermissionRead)
	auth.allow(acl.ResourceKindWorkflow, "workflow-a1", apitypes.ACLPermissionRead)

	workspaceList := callRPC(t, srv, "workspace-list-filtered", rpcapi.RPCMethodServerWorkspaceList, nil)
	if got := mustResult(t, workspaceList.Result.AsWorkspaceListResponse); len(got.Items) != 1 || got.Items[0].Name != "workspace-a" {
		t.Fatalf("filtered workspace.list = %#v", got)
	}
	workflowList := callRPC(t, srv, "workflow-list-filtered", rpcapi.RPCMethodServerWorkflowList, nil)
	if got := mustResult(t, workflowList.Result.AsWorkflowListResponse); len(got.Items) != 1 || got.Items[0].Metadata.Name != "workflow-a1" {
		t.Fatalf("filtered workflow.list = %#v", got)
	}

	if got := auth.count(ctx, acl.ResourceKindWorkflow, "workflow-a1", apitypes.ACLPermissionUse); got == 0 {
		t.Fatal("workspace.create did not check use")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkspace, acl.CollectionResourceID, apitypes.ACLPermissionCreate); got == 0 {
		t.Fatal("workspace.create did not check collection create")
	}
}

func TestServerWorkspaceListPrefixUsesACLDiscovery(t *testing.T) {
	ctx := context.Background()
	auth := newListingAuthorizer()
	srv := newTestResourceServer()
	srv.ACL = auth

	auth.allow(acl.ResourceKindWorkflow, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	auth.allow(acl.ResourceKindWorkflow, "workflow-a1", apitypes.ACLPermissionUse)
	auth.allow(acl.ResourceKindWorkspace, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	requireNoRPCError(t, callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("workflow-a1"))))
	for _, name := range []string{"social-direct-visible", "social-direct-hidden", "social-group-visible"} {
		requireNoRPCError(t, callRPC(t, srv, "workspace-create-"+name, rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
			Name:         name,
			WorkflowName: "workflow-a1",
		})))
	}

	auth.bindings = []apitypes.ACLPolicyBinding{
		{Id: "binding-hidden", Policy: apitypes.ACLPolicy{Subject: acl.PublicKeySubject(srv.Caller.String()), Resource: acl.WorkspaceResource("social-direct-hidden"), Role: "workspace-member"}},
		{Id: "binding-missing", Policy: apitypes.ACLPolicy{Subject: acl.PublicKeySubject(srv.Caller.String()), Resource: acl.WorkspaceResource("social-direct-missing"), Role: "workspace-member"}},
		{Id: "binding-visible", Policy: apitypes.ACLPolicy{Subject: acl.PublicKeySubject(srv.Caller.String()), Resource: acl.WorkspaceResource("social-direct-visible"), Role: "workspace-member"}},
		{Id: "binding-group", Policy: apitypes.ACLPolicy{Subject: acl.PublicKeySubject(srv.Caller.String()), Resource: acl.WorkspaceResource("social-group-visible"), Role: "workspace-member"}},
	}
	auth.allow(acl.ResourceKindWorkspace, "social-direct-missing", apitypes.ACLPermissionRead)
	auth.allow(acl.ResourceKindWorkspace, "social-direct-visible", apitypes.ACLPermissionRead)

	limit := 1
	resp := callRPC(t, srv, "workspace-list-prefix", rpcapi.RPCMethodServerWorkspaceList, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceListRequest, rpcapi.WorkspaceListRequest{
		Prefix: stringPtr("social-direct-"),
		Limit:  &limit,
	}))
	got := mustResult(t, resp.Result.AsWorkspaceListResponse)
	if len(got.Items) != 1 || got.Items[0].Name != "social-direct-visible" {
		t.Fatalf("workspace.list prefix = %#v", got)
	}
	if got.HasNext || got.NextCursor != nil {
		t.Fatalf("workspace.list prefix pagination = hasNext:%v next:%v", got.HasNext, got.NextCursor)
	}
	if len(auth.listRequests) == 0 {
		t.Fatal("workspace.list prefix did not list ACL policy bindings")
	}
	req := auth.listRequests[0]
	if req.SubjectKind != acl.SubjectKindPublicKey || req.SubjectID != srv.Caller.String() || req.ResourceKind != acl.ResourceKindWorkspace ||
		req.ResourceIDPrefix != "social-direct-" || req.Permission != apitypes.ACLPermissionRead {
		t.Fatalf("ACL discovery request = %+v", req)
	}
	if got := auth.count(ctx, acl.ResourceKindWorkspace, "social-direct-visible", apitypes.ACLPermissionRead); got == 0 {
		t.Fatal("workspace.list prefix did not authorize visible workspace")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkspace, "social-direct-hidden", apitypes.ACLPermissionRead); got == 0 {
		t.Fatal("workspace.list prefix did not authorize hidden workspace")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkspace, "social-group-visible", apitypes.ACLPermissionRead); got != 0 {
		t.Fatal("workspace.list prefix checked workspace outside requested prefix")
	}
}

func TestServerWorkspaceWorkflowCreateUsesCollectionACL(t *testing.T) {
	ctx := context.Background()
	auth := newRuleAuthorizer()
	srv := newTestResourceServer()
	srv.ACL = auth

	auth.allow(acl.ResourceKindWorkflow, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	auth.allow(acl.ResourceKindWorkflow, "flow-dynamic", apitypes.ACLPermissionUse)
	auth.allow(acl.ResourceKindWorkspace, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	auth.allow(acl.ResourceKindModel, acl.CollectionResourceID, apitypes.ACLPermissionCreate)
	auth.allow(acl.ResourceKindCredential, acl.CollectionResourceID, apitypes.ACLPermissionCreate)

	requireNoRPCError(t, callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("flow-dynamic"))))
	requireNoRPCError(t, callRPC(t, srv, "workspace-create", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-dynamic",
		WorkflowName: "flow-dynamic",
	})))
	requireNoRPCError(t, callRPC(t, srv, "model-create", rpcapi.RPCMethodServerModelCreate, rpcParams(t, (*rpcapi.RPCPayload).FromModelCreateRequest, rpcModel("model-dynamic"))))
	requireNoRPCError(t, callRPC(t, srv, "credential-create", rpcapi.RPCMethodServerCredentialCreate, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialCreateRequest, rpcCredential("credential-dynamic", "sk-dynamic"))))

	if got := auth.count(ctx, acl.ResourceKindWorkflow, "flow-dynamic", apitypes.ACLPermissionAdmin); got != 0 {
		t.Fatal("workflow.create checked concrete workflow admin")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkflow, acl.CollectionResourceID, apitypes.ACLPermissionCreate); got == 0 {
		t.Fatal("workflow.create did not check workflow collection create")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkspace, acl.CollectionResourceID, apitypes.ACLPermissionCreate); got == 0 {
		t.Fatal("workspace.create did not check workspace collection create")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkflow, "flow-dynamic", apitypes.ACLPermissionUse); got == 0 {
		t.Fatal("workspace.create did not check concrete workflow use")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkflow, acl.CollectionResourceID, apitypes.ACLPermissionUse); got != 0 {
		t.Fatal("workspace.create checked workflow collection use")
	}
	if got := auth.count(ctx, acl.ResourceKindModel, "model-dynamic", apitypes.ACLPermissionAdmin); got != 0 {
		t.Fatal("model.create checked concrete model admin")
	}
	if got := auth.count(ctx, acl.ResourceKindModel, acl.CollectionResourceID, apitypes.ACLPermissionCreate); got == 0 {
		t.Fatal("model.create did not check model collection create")
	}
	if got := auth.count(ctx, acl.ResourceKindCredential, "credential-dynamic", apitypes.ACLPermissionAdmin); got != 0 {
		t.Fatal("credential.create checked concrete credential admin")
	}
	if got := auth.count(ctx, acl.ResourceKindCredential, acl.CollectionResourceID, apitypes.ACLPermissionCreate); got == 0 {
		t.Fatal("credential.create did not check credential collection create")
	}
}

func TestServerWorkspaceHistoryRPC(t *testing.T) {
	workflowStore := kv.NewMemory(nil)
	objects := objectstore.Dir(t.TempDir())
	workspaceServer := &workspace.Server{
		Store:         kv.NewMemory(nil),
		WorkflowStore: workflowStore,
		RuntimeStore:  workspace.NewObjectRuntimeStore(objects),
	}
	srv := &Server{
		Caller:      giznet.PublicKey{1},
		ACL:         allowAllAuthorizer{},
		Workflows:   &workflow.Server{Store: workflowStore},
		Workspaces:  workspaceServer,
		ResourceACL: &recordingToolACL{},
	}
	requireNoRPCError(t, callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, workflowDoc("flow-history"))))
	requireNoRPCError(t, callRPC(t, srv, "workspace-create", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-history",
		WorkflowName: "flow-history",
	})))
	createdAt := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	entry, err := workspaceServer.AppendWorkspaceHistory(context.Background(), "workspace-history", workspace.AppendHistoryRequest{
		Type:      "agent",
		Name:      "assistant",
		Text:      "历史回复",
		CreatedAt: createdAt,
		Asset:     &workspace.AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("opus")},
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory() error = %v", err)
	}
	latest, err := workspaceServer.AppendWorkspaceHistory(context.Background(), "workspace-history", workspace.AppendHistoryRequest{
		Type:      "agent",
		Name:      "assistant",
		Text:      "最新回复",
		CreatedAt: createdAt.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory(latest) error = %v", err)
	}

	limit := 1
	list := callRPC(t, srv, "workspace-history-list", rpcapi.RPCMethodServerWorkspaceHistoryList, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceHistoryListRequest, rpcapi.WorkspaceHistoryListRequest{
		WorkspaceName: "workspace-history",
		Limit:         &limit,
	}))
	listResult := mustResult(t, list.Result.AsWorkspaceHistoryListResponse)
	if len(listResult.Items) != 1 || listResult.Items[0].Id != entry.ID || listResult.Items[0].Text != "历史回复" {
		t.Fatalf("workspace.history.list = %+v", listResult)
	}
	desc := rpcapi.WorkspaceHistoryListRequestOrderDesc
	list = callRPC(t, srv, "workspace-history-list-desc", rpcapi.RPCMethodServerWorkspaceHistoryList, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceHistoryListRequest, rpcapi.WorkspaceHistoryListRequest{
		WorkspaceName: "workspace-history",
		Limit:         &limit,
		Order:         &desc,
	}))
	listResult = mustResult(t, list.Result.AsWorkspaceHistoryListResponse)
	if len(listResult.Items) != 1 || listResult.Items[0].Id != latest.ID || listResult.Items[0].Text != "最新回复" {
		t.Fatalf("workspace.history.list desc = %+v", listResult)
	}

	get := callRPC(t, srv, "workspace-history-get", rpcapi.RPCMethodServerWorkspaceHistoryGet, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceHistoryGetRequest, rpcapi.WorkspaceHistoryGetRequest{
		WorkspaceName: "workspace-history",
		HistoryId:     entry.ID,
	}))
	if got := mustResult(t, get.Result.AsWorkspaceHistoryGetResponse); got.Id != entry.ID || got.Text != "历史回复" {
		t.Fatalf("workspace.history.get = %+v", got)
	}

	asset := callRPC(t, srv, "workspace-history-audio-get", rpcapi.RPCMethodServerWorkspaceHistoryAudioGet, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceHistoryAudioGetRequest, rpcapi.WorkspaceHistoryAudioGetRequest{
		WorkspaceName: "workspace-history",
		HistoryId:     entry.ID,
	}))
	assetResult := mustResult(t, asset.Result.AsWorkspaceHistoryAudioGetResponse)
	if assetResult.WorkspaceName != "workspace-history" || assetResult.HistoryId != entry.ID || assetResult.MimeType != "audio/opus" || assetResult.SizeBytes != int64(len("opus")) {
		t.Fatalf("workspace.history.audio.get = %+v", assetResult)
	}
	assetMetadata, reader, rpcErr, err := srv.PrepareWorkspaceHistoryAudioGet(context.Background(), rpcapi.WorkspaceHistoryAudioGetRequest{
		WorkspaceName: "workspace-history",
		HistoryId:     entry.ID,
	})
	if err != nil || rpcErr != nil {
		t.Fatalf("PrepareWorkspaceHistoryAudioGet() error = %v rpcErr = %+v", err, rpcErr)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(workspace history audio) error = %v", err)
	}
	if assetMetadata != assetResult || string(data) != "opus" {
		t.Fatalf("PrepareWorkspaceHistoryAudioGet() = %+v data=%q", assetMetadata, data)
	}

	textAssetEntry, err := workspaceServer.AppendWorkspaceHistory(context.Background(), "workspace-history", workspace.AppendHistoryRequest{
		Type:  "agent",
		Name:  "assistant",
		Text:  "text asset",
		Asset: &workspace.AppendHistoryAsset{MIMEType: "application/octet-stream", Data: []byte("not audio")},
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory(text asset) error = %v", err)
	}
	_, reader, rpcErr, err = srv.PrepareWorkspaceHistoryAudioGet(context.Background(), rpcapi.WorkspaceHistoryAudioGetRequest{
		WorkspaceName: "workspace-history",
		HistoryId:     textAssetEntry.ID,
	})
	if err != nil || rpcErr == nil || rpcErr.Code != rpcapi.RPCErrorCodeNotFound || reader != nil {
		t.Fatalf("PrepareWorkspaceHistoryAudioGet(non-audio) err = %v rpcErr = %+v reader = %v", err, rpcErr, reader)
	}

	missingAssetEntry, err := workspaceServer.AppendWorkspaceHistory(context.Background(), "workspace-history", workspace.AppendHistoryRequest{
		Type:  "agent",
		Name:  "assistant",
		Text:  "missing audio",
		Asset: &workspace.AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("gone")},
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory(missing asset) error = %v", err)
	}
	if err := objects.Delete(missingAssetEntry.Assets[0].Name); err != nil {
		t.Fatalf("Delete missing asset fixture: %v", err)
	}
	_, reader, rpcErr, err = srv.PrepareWorkspaceHistoryAudioGet(context.Background(), rpcapi.WorkspaceHistoryAudioGetRequest{
		WorkspaceName: "workspace-history",
		HistoryId:     missingAssetEntry.ID,
	})
	if err != nil || rpcErr == nil || rpcErr.Code != rpcapi.RPCErrorCodeNotFound || reader != nil {
		t.Fatalf("PrepareWorkspaceHistoryAudioGet(missing asset) err = %v rpcErr = %+v reader = %v", err, rpcErr, reader)
	}
}

func TestServerListVoicesFiltersByACL(t *testing.T) {
	ctx := context.Background()
	auth := newRuleAuthorizer()
	srv := newTestResourceServer()
	srv.ACL = auth

	for _, id := range []string{"voice-a", "voice-b", "provider:tenant:voice-c"} {
		body := testVoiceUpsert(id)
		resp, err := srv.Voices.CreateVoice(ctx, adminhttp.CreateVoiceRequestObject{Body: &body})
		if err != nil {
			t.Fatalf("CreateVoice(%s) error = %v", id, err)
		}
		if _, ok := resp.(adminhttp.CreateVoice200JSONResponse); !ok {
			t.Fatalf("CreateVoice(%s) response = %#v", id, resp)
		}
	}

	auth.allow(acl.ResourceKindVoice, "voice-a", apitypes.ACLPermissionRead)
	auth.allow(acl.ResourceKindVoice, "provider:tenant:voice-c", apitypes.ACLPermissionRead)
	resp, err := srv.ListVoices(ctx, adminhttp.ListVoicesRequestObject{})
	if err != nil {
		t.Fatalf("ListVoices() error = %v", err)
	}
	list, ok := resp.(adminhttp.ListVoices200JSONResponse)
	if !ok {
		t.Fatalf("ListVoices() response = %#v", resp)
	}
	if len(list.Items) != 2 || list.Items[0].Id != "provider:tenant:voice-c" || list.Items[1].Id != "voice-a" {
		t.Fatalf("ListVoices() items = %#v", list.Items)
	}
	if got := auth.count(ctx, acl.ResourceKindVoice, "voice-b", apitypes.ACLPermissionRead); got == 0 {
		t.Fatal("ListVoices() did not check denied voice")
	}

	rpcList := callRPC(t, srv, "voice-list", rpcapi.RPCMethodServerVoiceList, rpcParams(t, (*rpcapi.RPCPayload).FromVoiceListRequest, rpcapi.VoiceListRequest{}))
	requireNoRPCError(t, rpcList)
	rpcVoiceList := mustResult(t, rpcList.Result.AsVoiceListResponse)
	if len(rpcVoiceList.Items) != 2 || rpcVoiceList.Items[0].Id != "provider:tenant:voice-c" || rpcVoiceList.Items[1].Id != "voice-a" {
		t.Fatalf("server.voice.list items = %#v", rpcVoiceList.Items)
	}

	rpcGet := callRPC(t, srv, "voice-get", rpcapi.RPCMethodServerVoiceGet, rpcParams(t, (*rpcapi.RPCPayload).FromVoiceGetRequest, rpcapi.VoiceGetRequest{Id: "voice-a"}))
	requireNoRPCError(t, rpcGet)
	if got := mustResult(t, rpcGet.Result.AsVoiceGetResponse); got.Id != "voice-a" {
		t.Fatalf("server.voice.get = %#v", got)
	}
}

func TestServerVoiceRPCErrorPaths(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}

	missingService := newTestResourceServer()
	missingService.Voices = nil
	resp := callRPC(t, missingService, "voice-list-no-service", rpcapi.RPCMethodServerVoiceList, rpcParams(t, (*rpcapi.RPCPayload).FromVoiceListRequest, rpcapi.VoiceListRequest{}))
	requireRPCError(t, resp, rpcapi.RPCErrorCodeInternalError)

	resp = callRPC(t, missingService, "voice-get-no-service", rpcapi.RPCMethodServerVoiceGet, rpcParams(t, (*rpcapi.RPCPayload).FromVoiceGetRequest, rpcapi.VoiceGetRequest{Id: "voice-a"}))
	requireRPCError(t, resp, rpcapi.RPCErrorCodeInternalError)

	resp = callRPC(t, srv, "voice-get-missing-params", rpcapi.RPCMethodServerVoiceGet, nil)
	requireRPCError(t, resp, rpcapi.RPCErrorCodeInvalidParams)

	body := testVoiceUpsert("voice-a")
	if _, err := srv.Voices.CreateVoice(context.Background(), adminhttp.CreateVoiceRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateVoice() error = %v", err)
	}
	denied := newRuleAuthorizer()
	srv.ACL = denied
	resp = callRPC(t, srv, "voice-get-denied", rpcapi.RPCMethodServerVoiceGet, rpcParams(t, (*rpcapi.RPCPayload).FromVoiceGetRequest, rpcapi.VoiceGetRequest{Id: "voice-a"}))
	requireRPCError(t, resp, rpcapi.RPCErrorCodeInternalError)

	srv.ACL = errorAuthorizer{err: errors.New("acl backend down")}
	resp = callRPC(t, srv, "voice-list-acl-error", rpcapi.RPCMethodServerVoiceList, rpcParams(t, (*rpcapi.RPCPayload).FromVoiceListRequest, rpcapi.VoiceListRequest{}))
	requireRPCError(t, resp, rpcapi.RPCErrorCodeInternalError)

	upstreamError := newTestResourceServer()
	upstreamError.ACL = allowAllAuthorizer{}
	upstreamError.Voices = fakeVoiceAdminService{
		list: adminhttp.ListVoices500JSONResponse(apitypes.NewErrorResponse("VOICE_ERROR", "failed")),
	}
	resp = callRPC(t, upstreamError, "voice-list-upstream-error", rpcapi.RPCMethodServerVoiceList, rpcParams(t, (*rpcapi.RPCPayload).FromVoiceListRequest, rpcapi.VoiceListRequest{}))
	requireRPCError(t, resp, rpcapi.RPCErrorCodeInternalError)
}

func TestServerFirmwareRPCUsesFirmwareReadACL(t *testing.T) {
	ctx := context.Background()
	auth := newRuleAuthorizer()
	description := "main stable firmware"
	firmwareServer := &firmware.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir()), Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	create := adminhttp.FirmwareUpsert{
		Name: "devkit",
		Slots: apitypes.FirmwareSlots{
			Stable: apitypes.FirmwareSlot{
				Description: &description,
			},
		},
	}
	if resp, err := firmwareServer.CreateFirmware(ctx, adminhttp.CreateFirmwareRequestObject{Body: &create}); err != nil {
		t.Fatalf("CreateFirmware error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateFirmware200JSONResponse); !ok {
		t.Fatalf("CreateFirmware response = %T", resp)
	}
	other := adminhttp.FirmwareUpsert{
		Name: "otherkit",
		Slots: apitypes.FirmwareSlots{
			Stable: apitypes.FirmwareSlot{Description: stringPtr("other stable firmware")},
		},
	}
	if resp, err := firmwareServer.CreateFirmware(ctx, adminhttp.CreateFirmwareRequestObject{Body: &other}); err != nil {
		t.Fatalf("CreateFirmware other error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateFirmware200JSONResponse); !ok {
		t.Fatalf("CreateFirmware other response = %T", resp)
	}
	if resp, err := firmwareServer.UploadFirmwareArtifact(ctx, adminhttp.UploadFirmwareArtifactRequestObject{
		Name:    "devkit",
		Channel: "stable",
		Body:    bytes.NewReader(peerresourceTarPayload(t, map[string]string{"firmware.bin": "firmware payload"})),
	}); err != nil {
		t.Fatalf("UploadFirmwareArtifact error = %v", err)
	} else if _, ok := resp.(adminhttp.UploadFirmwareArtifact200JSONResponse); !ok {
		t.Fatalf("UploadFirmwareArtifact response = %T", resp)
	}

	srv := &Server{
		Caller:    giznet.PublicKey{1},
		ACL:       auth,
		Firmwares: firmwareServer,
	}

	denied := callRPC(t, srv, "firmware-get-denied", rpcapi.RPCMethodServerFirmwareGet, rpcParams(t, (*rpcapi.RPCPayload).FromFirmwareGetRequest, rpcapi.FirmwareGetRequest{
		FirmwareId: "devkit",
	}))
	requireRPCError(t, denied, rpcapi.RPCErrorCodeForbidden)
	if got := auth.count(ctx, acl.ResourceKindFirmware, "devkit", apitypes.ACLPermissionRead); got == 0 {
		t.Fatal("firmware.get did not check read")
	}

	auth.allow(acl.ResourceKindFirmware, "devkit", apitypes.ACLPermissionRead)
	listResp := callRPC(t, srv, "firmware-list", rpcapi.RPCMethodServerFirmwareList, nil)
	gotList := mustResult(t, listResp.Result.AsFirmwareListResponse)
	if len(gotList.Items) != 1 || gotList.Items[0].Name != "devkit" {
		t.Fatalf("firmware.list = %#v", gotList)
	}
	if got := auth.count(ctx, acl.ResourceKindFirmware, "otherkit", apitypes.ACLPermissionRead); got == 0 {
		t.Fatal("firmware.list did not check denied firmware")
	}

	getResp := callRPC(t, srv, "firmware-get", rpcapi.RPCMethodServerFirmwareGet, rpcParams(t, (*rpcapi.RPCPayload).FromFirmwareGetRequest, rpcapi.FirmwareGetRequest{
		FirmwareId: "devkit",
	}))
	gotFirmware := mustResult(t, getResp.Result.AsFirmwareGetResponse)
	if gotFirmware.Name != "devkit" || gotFirmware.Slots.Stable.Description == nil || *gotFirmware.Slots.Stable.Description != description {
		t.Fatalf("firmware.get = %#v", gotFirmware)
	}
	if gotFirmware.Slots.Stable.Artifact == nil || gotFirmware.Slots.Stable.Artifact.Size == 0 {
		t.Fatalf("firmware.get artifact = %#v", gotFirmware.Slots.Stable.Artifact)
	}

	bin := callRPC(t, srv, "firmware-download", rpcapi.RPCMethodServerFirmwareFilesDownload, rpcParams(t, (*rpcapi.RPCPayload).FromFirmwareFilesDownloadRequest, rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: "devkit",
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "firmware.bin",
	}))
	gotBin := mustResult(t, bin.Result.AsFirmwareFilesDownloadResponse)
	if gotBin.FirmwareId != "devkit" || gotBin.File.Path != "firmware.bin" || gotBin.File.Size == 0 {
		t.Fatalf("firmware.download = %#v", gotBin)
	}

	missingBin := callRPC(t, srv, "firmware-artifact-missing", rpcapi.RPCMethodServerFirmwareFilesDownload, rpcParams(t, (*rpcapi.RPCPayload).FromFirmwareFilesDownloadRequest, rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: "devkit",
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "missing.bin",
	}))
	requireRPCError(t, missingBin, rpcapi.RPCErrorCodeNotFound)
}

func TestServerGameplayPixaDownloads(t *testing.T) {
	ctx := context.Background()
	caller := giznet.PublicKey{9}
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	catalog := &gameplay.Catalog{
		GameRulesets: kv.NewMemory(nil),
		PetDefs:      kv.NewMemory(nil),
		BadgeDefs:    kv.NewMemory(nil),
		GameDefs:     kv.NewMemory(nil),
		Assets:       objectstore.Dir(t.TempDir()),
		Now:          func() time.Time { return now },
	}
	petPixa := peerresourceTestPixa(t, []string{"default", "feed"})
	badgePixa := peerresourceTestPixa(t, []string{"icon"})
	petDefI18n := peerresourcePetDefI18n("Pet A")
	if resp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{Body: &adminhttp.PetDefUpsert{Id: "petdef-a", Spec: peerresourcePetDefSpec("Pet A"), I18n: &petDefI18n}}); err != nil {
		t.Fatalf("CreatePetDef error = %v", err)
	} else if _, ok := resp.(adminhttp.CreatePetDef200JSONResponse); !ok {
		t.Fatalf("CreatePetDef response = %T", resp)
	}
	if resp, err := catalog.UploadPetDefPixa(ctx, adminhttp.UploadPetDefPixaRequestObject{Id: "petdef-a", Body: bytes.NewReader(petPixa)}); err != nil {
		t.Fatalf("UploadPetDefPixa error = %v", err)
	} else if _, ok := resp.(adminhttp.UploadPetDefPixa200JSONResponse); !ok {
		t.Fatalf("UploadPetDefPixa response = %T", resp)
	}
	if resp, err := catalog.CreateBadgeDef(ctx, adminhttp.CreateBadgeDefRequestObject{Body: &adminhttp.BadgeDefUpsert{Id: "badge-a", Spec: apitypes.BadgeDefSpec{DisplayName: "Badge A"}}}); err != nil {
		t.Fatalf("CreateBadgeDef error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateBadgeDef200JSONResponse); !ok {
		t.Fatalf("CreateBadgeDef response = %T", resp)
	}
	if resp, err := catalog.UploadBadgeDefPixa(ctx, adminhttp.UploadBadgeDefPixaRequestObject{Id: "badge-a", Body: bytes.NewReader(badgePixa)}); err != nil {
		t.Fatalf("UploadBadgeDefPixa error = %v", err)
	} else if _, ok := resp.(adminhttp.UploadBadgeDefPixa200JSONResponse); !ok {
		t.Fatalf("UploadBadgeDefPixa response = %T", resp)
	}
	badgeDelta := map[string]int64{"badge-a": 100}
	if resp, err := catalog.CreateGameRuleset(ctx, adminhttp.CreateGameRulesetRequestObject{Body: &adminhttp.GameRulesetUpsert{
		Name: "default",
		Spec: apitypes.GameRulesetSpec{
			Enabled: true,
			PetPool: []apitypes.GameRulesetPetPoolEntry{{
				PetdefId: "petdef-a",
				Weight:   1,
			}},
			BadgeDefIds: &[]string{"badge-a"},
			Drive:       &apitypes.GameRulesetDriveSpec{DefaultReward: &apitypes.GameRewardSpec{BadgeExpDelta: &badgeDelta}},
		},
	}}); err != nil {
		t.Fatalf("CreateGameRuleset error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateGameRuleset200JSONResponse); !ok {
		t.Fatalf("CreateGameRuleset response = %T", resp)
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	workflowStore := kv.NewMemory(nil)
	workflowServer := &workflow.Server{Store: workflowStore}
	chatroomWorkflow, err := convertType[apitypes.WorkflowDocument](workflowDoc("chatroom"))
	if err != nil {
		t.Fatalf("convert workflow: %v", err)
	}
	if resp, err := workflowServer.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &chatroomWorkflow}); err != nil {
		t.Fatalf("CreateWorkflow error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateWorkflow200JSONResponse); !ok {
		t.Fatalf("CreateWorkflow response = %T", resp)
	}
	ids := []string{"pet-a", "adopt-txn", "grant-a"}
	runtime := &gameplay.Runtime{
		DB:         db,
		Catalog:    catalog,
		Workspaces: &workspace.Server{Store: kv.NewMemory(nil), WorkflowStore: workflowStore},
		Now:        func() time.Time { return now },
		PickWeight: func(int64) int64 { return 0 },
		NewID: func() string {
			if len(ids) == 0 {
				t.Fatal("unexpected id allocation")
			}
			id := ids[0]
			ids = ids[1:]
			return id
		},
	}
	adopted, err := runtime.AdoptPet(ctx, caller.String(), apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet error = %v", err)
	}
	if _, err := runtime.DrivePet(ctx, caller.String(), apitypes.PetDriveRequest{PetId: adopted.Pet.Id}); err != nil {
		t.Fatalf("DrivePet error = %v", err)
	}
	auth := newRuleAuthorizer()
	auth.allow(acl.ResourceKindGameRuleset, "default", apitypes.ACLPermissionRead)
	srv := &Server{Caller: caller, ACL: auth, Gameplay: runtime}
	actionsResp := callRPC(t, srv, "pet-actions-get", rpcapi.RPCMethodServerPetActionsGet, rpcParams(t, (*rpcapi.RPCPayload).FromServerPetActionsGetRequest, rpcapi.ServerPetActionsGetRequest{Id: adopted.Pet.Id}))
	gotActions := mustResult(t, actionsResp.Result.AsServerPetActionsGetResponse)
	if gotActions.PetId != adopted.Pet.Id || gotActions.PetdefId != "petdef-a" || gotActions.DefaultLocale != "en" {
		t.Fatalf("pet actions identity = %#v", gotActions)
	}
	if len(gotActions.Actions) != 2 || gotActions.Actions[1].Id != "feed" || gotActions.Actions[1].VisualClipId == nil || *gotActions.Actions[1].VisualClipId != "feed" {
		t.Fatalf("pet actions = %#v", gotActions.Actions)
	}
	if gotActions.Actions[1].PixaClipName == nil || *gotActions.Actions[1].PixaClipName != "feed" {
		t.Fatalf("pet action pixa clip = %#v", gotActions.Actions[1])
	}
	if gotActions.I18n["en"].Actions["feed"].Name != "Feed" {
		t.Fatalf("pet actions i18n = %#v", gotActions.I18n)
	}
	petPixaResp := callRPC(t, srv, "pet-pixa-download", rpcapi.RPCMethodServerPetPixaDownload, rpcParams(t, (*rpcapi.RPCPayload).FromServerPetPixaDownloadRequest, rpcapi.PetPixaDownloadRequest{PetId: adopted.Pet.Id}))
	gotPetPixa := mustResult(t, petPixaResp.Result.AsServerPetPixaDownloadResponse)
	if gotPetPixa.PetId != adopted.Pet.Id || gotPetPixa.PetdefId != "petdef-a" || gotPetPixa.SizeBytes != int64(len(petPixa)) || valueOrZero(gotPetPixa.PixaPath) != "pet-defs/petdef-a/pixa" {
		t.Fatalf("pet pixa metadata = %#v", gotPetPixa)
	}
	gotPetPixaMetadata, petPixaReader, rpcErr, err := srv.PreparePetPixaDownload(ctx, rpcapi.PetPixaDownloadRequest{PetId: adopted.Pet.Id})
	if err != nil || rpcErr != nil {
		t.Fatalf("PreparePetPixaDownload err = %v rpcErr = %+v", err, rpcErr)
	}
	defer petPixaReader.Close()
	if data, err := io.ReadAll(petPixaReader); err != nil || !bytes.Equal(data, petPixa) || gotPetPixaMetadata.SizeBytes != int64(len(petPixa)) {
		t.Fatalf("pet pixa data len=%d metadata=%#v err=%v", len(data), gotPetPixaMetadata, err)
	}
	badgeResp := callRPC(t, srv, "badgedef-pixa-download", rpcapi.RPCMethodServerBadgeDefPixaDownload, rpcParams(t, (*rpcapi.RPCPayload).FromBadgeDefPixaDownloadRequest, rpcapi.BadgeDefPixaDownloadRequest{Id: "badge-a"}))
	gotBadge := mustResult(t, badgeResp.Result.AsBadgeDefPixaDownloadResponse)
	if gotBadge.Id != "badge-a" || gotBadge.SizeBytes != int64(len(badgePixa)) || valueOrZero(gotBadge.PixaPath) != "badge-defs/badge-a/pixa" {
		t.Fatalf("badgedef pixa metadata = %#v", gotBadge)
	}

	other := &Server{Caller: giznet.PublicKey{8}, ACL: newRuleAuthorizer(), Gameplay: runtime}
	actionsDenied := callRPC(t, other, "pet-actions-denied", rpcapi.RPCMethodServerPetActionsGet, rpcParams(t, (*rpcapi.RPCPayload).FromServerPetActionsGetRequest, rpcapi.ServerPetActionsGetRequest{Id: adopted.Pet.Id}))
	requireRPCError(t, actionsDenied, rpcapi.RPCErrorCodeNotFound)
	petPixaDenied := callRPC(t, other, "pet-pixa-denied", rpcapi.RPCMethodServerPetPixaDownload, rpcParams(t, (*rpcapi.RPCPayload).FromServerPetPixaDownloadRequest, rpcapi.PetPixaDownloadRequest{PetId: adopted.Pet.Id}))
	requireRPCError(t, petPixaDenied, rpcapi.RPCErrorCodeNotFound)
}

func TestServerErrorPaths(t *testing.T) {
	requiredMethods := []rpcapi.RPCMethod{
		rpcapi.RPCMethodServerWorkspaceGet,
		rpcapi.RPCMethodServerWorkspaceCreate,
		rpcapi.RPCMethodServerWorkspacePut,
		rpcapi.RPCMethodServerWorkspaceDelete,
		rpcapi.RPCMethodServerWorkflowGet,
		rpcapi.RPCMethodServerWorkflowCreate,
		rpcapi.RPCMethodServerWorkflowPut,
		rpcapi.RPCMethodServerWorkflowDelete,
		rpcapi.RPCMethodServerModelGet,
		rpcapi.RPCMethodServerModelCreate,
		rpcapi.RPCMethodServerModelPut,
		rpcapi.RPCMethodServerModelDelete,
		rpcapi.RPCMethodServerCredentialGet,
		rpcapi.RPCMethodServerCredentialCreate,
		rpcapi.RPCMethodServerCredentialPut,
		rpcapi.RPCMethodServerCredentialDelete,
	}

	for _, method := range []rpcapi.RPCMethod{
		rpcapi.RPCMethodServerWorkspaceList,
		rpcapi.RPCMethodServerWorkspaceGet,
		rpcapi.RPCMethodServerWorkspaceCreate,
		rpcapi.RPCMethodServerWorkspacePut,
		rpcapi.RPCMethodServerWorkspaceDelete,
		rpcapi.RPCMethodServerWorkflowList,
		rpcapi.RPCMethodServerWorkflowGet,
		rpcapi.RPCMethodServerWorkflowCreate,
		rpcapi.RPCMethodServerWorkflowPut,
		rpcapi.RPCMethodServerWorkflowDelete,
		rpcapi.RPCMethodServerModelList,
		rpcapi.RPCMethodServerModelGet,
		rpcapi.RPCMethodServerModelCreate,
		rpcapi.RPCMethodServerModelPut,
		rpcapi.RPCMethodServerModelDelete,
		rpcapi.RPCMethodServerCredentialList,
		rpcapi.RPCMethodServerCredentialGet,
		rpcapi.RPCMethodServerCredentialCreate,
		rpcapi.RPCMethodServerCredentialPut,
		rpcapi.RPCMethodServerCredentialDelete,
	} {
		resp, handled, err := (&Server{}).Dispatch(context.Background(), &rpcapi.RPCRequest{Id: string(method), Method: method})
		if err != nil || !handled {
			t.Fatalf("unconfigured Dispatch(%s) handled=%v err=%v", method, handled, err)
		}
		requireRPCError(t, resp, rpcapi.RPCErrorCodeInternalError)
	}

	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	for _, method := range requiredMethods {
		resp := callRPC(t, srv, "invalid-"+string(method), method, nil)
		requireRPCError(t, resp, rpcapi.RPCErrorCodeInvalidParams)
	}
	for _, method := range []rpcapi.RPCMethod{
		rpcapi.RPCMethodServerWorkspaceList,
		rpcapi.RPCMethodServerWorkflowList,
		rpcapi.RPCMethodServerModelList,
		rpcapi.RPCMethodServerCredentialList,
	} {
		resp := callRPC(t, srv, "invalid-"+string(method), method, &rpcapi.RPCPayload{})
		requireRPCError(t, resp, rpcapi.RPCErrorCodeInvalidParams)
	}

	for _, tc := range []struct {
		name   string
		method rpcapi.RPCMethod
		params *rpcapi.RPCPayload
	}{
		{"workspace", rpcapi.RPCMethodServerWorkspaceGet, rpcParams(t, (*rpcapi.RPCPayload).FromWorkspaceGetRequest, rpcapi.WorkspaceGetRequest{Name: "missing"})},
		{"workflow", rpcapi.RPCMethodServerWorkflowGet, rpcParams(t, (*rpcapi.RPCPayload).FromWorkflowGetRequest, rpcapi.WorkflowGetRequest{Name: "missing"})},
		{"model", rpcapi.RPCMethodServerModelGet, rpcParams(t, (*rpcapi.RPCPayload).FromModelGetRequest, rpcapi.ModelGetRequest{Id: "missing"})},
		{"credential", rpcapi.RPCMethodServerCredentialGet, rpcParams(t, (*rpcapi.RPCPayload).FromCredentialGetRequest, rpcapi.CredentialGetRequest{Name: "missing"})},
	} {
		t.Run(tc.name+"-not-found", func(t *testing.T) {
			resp := callRPC(t, srv, tc.name+"-not-found", tc.method, tc.params)
			requireRPCError(t, resp, rpcapi.RPCErrorCodeNotFound)
		})
	}

	authless := newTestResourceServer()
	resp := callRPC(t, authless, "acl-missing", rpcapi.RPCMethodServerModelGet, rpcParams(t, (*rpcapi.RPCPayload).FromModelGetRequest, rpcapi.ModelGetRequest{Id: "model-a"}))
	requireRPCError(t, resp, rpcapi.RPCErrorCodeInternalError)
}

func peerresourceTarPayload(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	modTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for name, body := range files {
		data := []byte(body)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data)), ModTime: modTime}); err != nil {
			t.Fatalf("WriteHeader(%s): %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("Write(%s): %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("Close tar: %v", err)
	}
	return buf.Bytes()
}

func peerresourceTestPixa(t *testing.T, clips []string) []byte {
	t.Helper()
	if len(clips) == 0 {
		t.Fatal("peerresourceTestPixa requires at least one clip")
	}
	const (
		headerSize       = 40
		clipEntrySize    = 56
		frameEntrySize   = 16
		clipNameSize     = 32
		paletteByteCount = 2
	)
	paletteOffset := headerSize
	clipOffset := paletteOffset + paletteByteCount
	frameOffset := clipOffset + len(clips)*clipEntrySize
	payload := []byte{0x00, 0xf8, 0xe0, 0x07}
	payloadOffset := frameOffset + frameEntrySize
	data := make([]byte, payloadOffset+len(payload))
	copy(data[:4], "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], 16)
	binary.LittleEndian.PutUint16(data[10:12], 16)
	binary.LittleEndian.PutUint16(data[12:14], 1)
	binary.LittleEndian.PutUint16(data[14:16], uint16(len(clips)))
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[20:24], uint32(paletteOffset))
	binary.LittleEndian.PutUint32(data[24:28], uint32(clipOffset))
	binary.LittleEndian.PutUint32(data[28:32], uint32(frameOffset))
	binary.LittleEndian.PutUint32(data[32:36], uint32(payloadOffset))
	binary.LittleEndian.PutUint32(data[36:40], uint32(len(payload)))
	for i, clip := range clips {
		base := clipOffset + i*clipEntrySize
		copy(data[base:base+clipNameSize], []byte(clip))
		binary.LittleEndian.PutUint32(data[base+36:base+40], 0)
		binary.LittleEndian.PutUint32(data[base+40:base+44], 1)
		binary.LittleEndian.PutUint32(data[base+44:base+48], 120)
		binary.LittleEndian.PutUint16(data[base+48:base+50], 1)
	}
	binary.LittleEndian.PutUint16(data[frameOffset:frameOffset+2], 120)
	data[frameOffset+2] = 0
	binary.LittleEndian.PutUint32(data[frameOffset+4:frameOffset+8], 0)
	binary.LittleEndian.PutUint32(data[frameOffset+8:frameOffset+12], uint32(len(payload)))
	copy(data[payloadOffset:], payload)
	return data
}

func TestHelpers(t *testing.T) {
	if IsMethod(rpcapi.RPCMethodAllPing) {
		t.Fatal("IsMethod(all.ping) = true")
	}
	value := 7
	if got := int32Ptr(&value); got == nil || *got != 7 {
		t.Fatalf("int32Ptr() = %#v", got)
	}
	if got := int32Ptr(nil); got != nil {
		t.Fatalf("int32Ptr(nil) = %#v", got)
	}
	if got := peerListLimit(nil); got != 50 {
		t.Fatalf("peerListLimit(nil) = %d, want 50", got)
	}
	zero := 0
	if got := peerListLimit(&zero); got != 50 {
		t.Fatalf("peerListLimit(0) = %d, want 50", got)
	}
	tooHigh := 201
	if got := peerListLimit(&tooHigh); got != 200 {
		t.Fatalf("peerListLimit(201) = %d, want 200", got)
	}
	inRange := 7
	if got := peerListLimit(&inRange); got != 7 {
		t.Fatalf("peerListLimit(7) = %d", got)
	}
	if resp := statusError("status", http.StatusTeapot, ""); resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInternalError {
		t.Fatalf("statusError(418) = %#v", resp)
	}
	if resp := withRequestID("id", nil); resp != nil {
		t.Fatalf("withRequestID(nil) = %#v", resp)
	}
	if got := newTestResourceServer().String(); got == "" {
		t.Fatal("String() = empty")
	}
	if _, err := convertType[struct{}](func() {}); err == nil {
		t.Fatal("convertType(function) error = nil")
	}
}

func newTestResourceServer() *Server {
	workflowStore := kv.NewMemory(nil)
	return &Server{
		Caller:      giznet.PublicKey{1},
		Workflows:   &workflow.Server{Store: workflowStore},
		Workspaces:  &workspace.Server{Store: kv.NewMemory(nil), WorkflowStore: workflowStore},
		Models:      &model.Server{Store: kv.NewMemory(nil), Now: func() time.Time { return time.Unix(1, 0).UTC() }},
		Credentials: &credential.Server{Store: kv.NewMemory(nil)},
		Voices:      &voice.Server{Store: kv.NewMemory(nil), Now: func() time.Time { return time.Unix(1, 0).UTC() }},
		ResourceACL: &recordingToolACL{},
	}
}

type failingWorkspaceRuntimeStore struct {
	deleteErr error
}

func (s *failingWorkspaceRuntimeStore) PrepareWorkspace(context.Context, string) (workspace.Runtime, error) {
	return workspace.Runtime{}, nil
}

func (s *failingWorkspaceRuntimeStore) GetWorkspaceRuntime(context.Context, string) (workspace.Runtime, error) {
	return workspace.Runtime{}, nil
}

func (s *failingWorkspaceRuntimeStore) DeleteWorkspaceRuntime(context.Context, string) error {
	return s.deleteErr
}

type fixedPeerConfigService struct {
	peer apitypes.Peer
	err  error
}

func (s fixedPeerConfigService) LoadPeer(context.Context, giznet.PublicKey) (apitypes.Peer, error) {
	if s.err != nil {
		return apitypes.Peer{}, s.err
	}
	return s.peer, nil
}

func peerresourcePetDefI18n(displayName string) apitypes.PetDefI18nSpec {
	description := "Peer resource pet."
	return apitypes.PetDefI18nSpec{
		DefaultLocale: "en",
		AdditionalProperties: map[string]apitypes.PetDefI18nCatalog{
			"en": {
				DisplayName: &displayName,
				Description: &description,
				Attr: &apitypes.PetDefI18nAttrSpec{
					Life:        &apitypes.PetDefI18nAttrGroup{"hunger": {DisplayName: "Hunger"}},
					Progression: &apitypes.PetDefI18nAttrGroup{"xp": {DisplayName: "XP"}},
				},
				Drive: &apitypes.PetDefI18nDriveSpec{Actions: &map[string]apitypes.PetDefI18nDisplayText{
					"idle": {DisplayName: "Idle"},
					"feed": {DisplayName: "Feed"},
				}},
			},
		},
	}
}

func stringPtr(value string) *string {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func peerresourcePetDefSpec(displayName string) apitypes.PetDefSpec {
	return apitypes.PetDefSpec{
		Attr: apitypes.PetDefAttrSpec{
			Life: apitypes.PetAttrGroupSpec{
				"hunger": {Initial: 100},
			},
			Progression: apitypes.PetAttrGroupSpec{
				"xp": {Initial: 0},
			},
		},
		Character: apitypes.PetDefCharacterSpec{Prompt: "Small friendly pixel pet."},
		Voice:     apitypes.PetDefVoiceSpec{VoiceId: "gizclaw-soft", Prompt: "Soft and curious."},
		Drive: apitypes.PetDefDriveSpec{Actions: []apitypes.PetDefActionSpec{
			{Id: "idle", Cost: 0, VisualClipId: stringPtr("idle")},
			{Id: "feed", Cost: 0, VisualClipId: stringPtr("feed")},
		}},
		Visual: apitypes.PetDefVisualSpec{
			Refs: apitypes.PetDefVisualRefsSpec{},
			Pixa: apitypes.PetDefPixaSpec{
				AssetRef: "asset://pets/test/pet.pixa",
				Metadata: apitypes.PetDefPixaMetadata{
					Version: "1",
					Canvas:  apitypes.PetDefPixaCanvasMetadata{Width: 16, Height: 16},
					Clips: []apitypes.PetDefPixaClipMetadata{
						{Id: "idle", ActionId: stringPtr("idle"), PixaClipName: "default"},
						{Id: "feed", ActionId: stringPtr("feed"), PixaClipName: "feed"},
					},
				},
			},
		},
	}
}

func TestPetActionsListFallsBackToClipID(t *testing.T) {
	actions := petActionsList(
		apitypes.PetDefDriveSpec{Actions: []apitypes.PetDefActionSpec{{Id: "feed"}}},
		apitypes.PetDefPixaMetadata{Clips: []apitypes.PetDefPixaClipMetadata{{Id: "feed", PixaClipName: "eat-animation"}}},
	)
	if len(actions) != 1 || actions[0].PixaClipName == nil || *actions[0].PixaClipName != "eat-animation" {
		t.Fatalf("actions = %#v, want clip-id fallback", actions)
	}
}

func TestPetClipNamesPreservesMetadataMapping(t *testing.T) {
	got := petClipNames(apitypes.PetDefPixaMetadata{Clips: []apitypes.PetDefPixaClipMetadata{
		{Id: "hungry", PixaClipName: "low-energy"},
	}})
	if got["hungry"] != "low-energy" {
		t.Fatalf("clip names = %#v, want hungry -> low-energy", got)
	}
}

func callRPC(t *testing.T, srv *Server, id string, method rpcapi.RPCMethod, params *rpcapi.RPCPayload) *rpcapi.RPCResponse {
	t.Helper()

	resp, handled, err := srv.Dispatch(context.Background(), &rpcapi.RPCRequest{V: rpcapi.RPCVersionV1, Id: id, Method: method, Params: params})
	if err != nil {
		t.Fatalf("Dispatch(%s) error = %v", method, err)
	}
	if !handled {
		t.Fatalf("Dispatch(%s) handled = false", method)
	}
	if resp == nil {
		t.Fatalf("Dispatch(%s) response = nil", method)
	}
	return resp
}

func rpcParams[T any](t *testing.T, encode func(*rpcapi.RPCPayload, T) error, value T) *rpcapi.RPCPayload {
	t.Helper()

	var params rpcapi.RPCPayload
	if err := encode(&params, value); err != nil {
		t.Fatalf("encode params error = %v", err)
	}
	return &params
}

func mustResult[T any](t *testing.T, decode func() (T, error)) T {
	t.Helper()

	value, err := decode()
	if err != nil {
		t.Fatalf("decode result error = %v", err)
	}
	return value
}

func requireNoRPCError(t *testing.T, resp *rpcapi.RPCResponse) {
	t.Helper()

	if resp.Error != nil {
		t.Fatalf("RPC error = %#v", resp.Error)
	}
}

func requireRPCError(t *testing.T, resp *rpcapi.RPCResponse, code rpcapi.RPCErrorCode) {
	t.Helper()

	if resp == nil || resp.Error == nil {
		t.Fatalf("RPC response error = nil, response = %#v", resp)
	}
	if resp.Error.Code != code {
		t.Fatalf("RPC error code = %v, want %v, response = %#v", resp.Error.Code, code, resp)
	}
}

func workflowDoc(name string) rpcapi.WorkflowDocument {
	spec := rpcapi.FlowcraftWorkflowSpec{"entry_agent": ""}
	return rpcapi.WorkflowDocument{
		Metadata: rpcapi.WorkflowMetadata{Name: name},
		Spec: rpcapi.WorkflowSpec{
			Driver:    rpcapi.WorkflowDriverFlowcraft,
			Flowcraft: &spec,
		},
	}
}

func rpcModel(id string) rpcapi.Model {
	return rpcapi.Model{
		Id:     id,
		Kind:   rpcapi.ModelKindLlm,
		Source: rpcapi.ModelSourceManual,
		Provider: rpcapi.ModelProvider{
			Kind: rpcapi.ModelProviderKind("openai-tenant"),
			Name: "global",
		},
	}
}

func rpcCredential(name, key string) rpcapi.Credential {
	return rpcapi.Credential{
		Name:     name,
		Provider: "openai",
		Body:     testRPCOpenAICredentialBody(key),
	}
}

func testVoiceUpsert(id string) adminhttp.VoiceUpsert {
	return adminhttp.VoiceUpsert{
		Id:     id,
		Source: apitypes.VoiceSourceManual,
		Provider: apitypes.VoiceProvider{
			Kind: apitypes.VoiceProviderKindOpenaiTenant,
			Name: "global",
		},
	}
}

type allowAllAuthorizer struct{}

func (allowAllAuthorizer) Authorize(context.Context, acl.AuthorizeRequest) error {
	return nil
}

type errorAuthorizer struct {
	err error
}

func (a errorAuthorizer) Authorize(context.Context, acl.AuthorizeRequest) error {
	return a.err
}

type fakeVoiceAdminService struct {
	list adminhttp.ListVoicesResponseObject
}

func (s fakeVoiceAdminService) CreateVoice(context.Context, adminhttp.CreateVoiceRequestObject) (adminhttp.CreateVoiceResponseObject, error) {
	return nil, errors.New("unexpected CreateVoice")
}

func (s fakeVoiceAdminService) ListVoices(context.Context, adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error) {
	return s.list, nil
}

func (s fakeVoiceAdminService) DeleteVoice(context.Context, adminhttp.DeleteVoiceRequestObject) (adminhttp.DeleteVoiceResponseObject, error) {
	return nil, errors.New("unexpected DeleteVoice")
}

func (s fakeVoiceAdminService) GetVoice(context.Context, adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	return nil, errors.New("unexpected GetVoice")
}

func (s fakeVoiceAdminService) PutVoice(context.Context, adminhttp.PutVoiceRequestObject) (adminhttp.PutVoiceResponseObject, error) {
	return nil, errors.New("unexpected PutVoice")
}

type ruleAuthorizer struct {
	allowed map[authKey]struct{}
	calls   map[authKey]int
}

type authKey struct {
	kind       apitypes.ACLResourceKind
	id         string
	permission apitypes.ACLPermission
}

func newRuleAuthorizer() *ruleAuthorizer {
	return &ruleAuthorizer{
		allowed: make(map[authKey]struct{}),
		calls:   make(map[authKey]int),
	}
}

func (a *ruleAuthorizer) allow(kind apitypes.ACLResourceKind, id string, permission apitypes.ACLPermission) {
	a.allowed[authKey{kind: kind, id: id, permission: permission}] = struct{}{}
}

func (a *ruleAuthorizer) count(_ context.Context, kind apitypes.ACLResourceKind, id string, permission apitypes.ACLPermission) int {
	return a.calls[authKey{kind: kind, id: id, permission: permission}]
}

func (a *ruleAuthorizer) Authorize(_ context.Context, request acl.AuthorizeRequest) error {
	key := authKey{kind: request.Resource.Kind, id: request.Resource.Id, permission: request.Permission}
	a.calls[key]++
	if _, ok := a.allowed[key]; !ok {
		return acl.ErrDenied
	}
	return nil
}

type listingAuthorizer struct {
	*ruleAuthorizer
	bindings     []apitypes.ACLPolicyBinding
	listRequests []acl.ListPolicyBindingsRequest
}

func newListingAuthorizer() *listingAuthorizer {
	return &listingAuthorizer{ruleAuthorizer: newRuleAuthorizer()}
}

func (a *listingAuthorizer) ListPolicyBindings(_ context.Context, request acl.ListPolicyBindingsRequest) ([]apitypes.ACLPolicyBinding, bool, *string, error) {
	a.listRequests = append(a.listRequests, request)
	limit := request.Limit
	if limit <= 0 {
		limit = 50
	}
	cursorPassed := request.Cursor == ""
	filtered := make([]apitypes.ACLPolicyBinding, 0, len(a.bindings))
	for _, binding := range a.bindings {
		if !cursorPassed {
			if binding.Id == request.Cursor {
				cursorPassed = true
			}
			continue
		}
		if request.SubjectKind != "" && binding.Policy.Subject.Kind != request.SubjectKind {
			continue
		}
		if request.SubjectID != "" && binding.Policy.Subject.Id != request.SubjectID {
			continue
		}
		if request.ResourceKind != "" && binding.Policy.Resource.Kind != request.ResourceKind {
			continue
		}
		if request.ResourceIDPrefix != "" && !strings.HasPrefix(binding.Policy.Resource.Id, request.ResourceIDPrefix) {
			continue
		}
		filtered = append(filtered, binding)
	}
	if len(filtered) <= limit {
		return filtered, false, nil, nil
	}
	nextCursor := filtered[limit-1].Id
	return filtered[:limit], true, &nextCursor, nil
}
