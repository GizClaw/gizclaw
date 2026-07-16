//go:build gizclaw_e2e

package rpc_test

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerWorkspaceRPC(t *testing.T) {
	env := newServerResourceHarness(t)
	admin := serverResourceAdminClient(t, env)

	_, _ = env.peer.DeleteWorkspace(env.ctx, "workspace.delete.preclean", rpcapi.WorkspaceDeleteRequest{Name: mutationWorkspace})
	_, _ = admin.DeleteWorkflowWithResponse(env.ctx, mutationWorkflow)
	if response, err := admin.CreateWorkflowWithResponse(env.ctx, adminWorkflow(mutationWorkflow, "workspace test flow")); err != nil || response.JSON200 == nil {
		t.Fatalf("create workflow for workspace test: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.DeleteWorkflowWithResponse(env.ctx, mutationWorkflow) })
	workspaceList, err := env.peer.ListWorkspaces(env.ctx, "workspace.list.shared", rpcapi.WorkspaceListRequest{})
	if err != nil {
		t.Fatalf("workspace.list shared: %v", err)
	}
	if len(workspaceList.Items) == 0 {
		t.Fatalf("workspace.list returned no items")
	}
	assertWorkspacePrefixList(t, env.ctx, env.peer)
	sharedWorkspaceObject, err := env.peer.GetWorkspace(env.ctx, "workspace.get.shared", rpcapi.WorkspaceGetRequest{Name: sharedWorkspace})
	if err != nil {
		t.Fatalf("workspace.get shared: %v", err)
	}
	if sharedWorkspaceObject.Name != sharedWorkspace || sharedWorkspaceObject.WorkflowName != sharedWorkflow {
		t.Fatalf("workspace.get shared = %#v", sharedWorkspaceObject)
	}

	createInput := rpcapi.WorkspaceInputModePushToTalk
	var createParams rpcapi.WorkspaceParameters
	if err := createParams.FromFlowcraftWorkspaceParameters(rpcapi.FlowcraftWorkspaceParameters{
		AgentType: rpcapi.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		Input:     &createInput,
	}); err != nil {
		t.Fatalf("FromFlowcraftWorkspaceParameters(create) error = %v", err)
	}
	workspace, err := env.peer.CreateWorkspace(env.ctx, "workspace.create", rpcapi.WorkspaceCreateRequest{
		Name:         mutationWorkspace,
		WorkflowName: mutationWorkflow,
		Parameters:   &createParams,
	})
	if err != nil {
		t.Fatalf("workspace.create: %v", err)
	}
	if workspace.Name != mutationWorkspace || workspace.WorkflowName != mutationWorkflow {
		t.Fatalf("workspace.create = %#v", workspace)
	}

	updateInput := rpcapi.WorkspaceInputModeRealtime
	var updateParams rpcapi.WorkspaceParameters
	if err := updateParams.FromFlowcraftWorkspaceParameters(rpcapi.FlowcraftWorkspaceParameters{
		AgentType: rpcapi.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		Input:     &updateInput,
	}); err != nil {
		t.Fatalf("FromFlowcraftWorkspaceParameters(update) error = %v", err)
	}
	workspace, err = env.peer.PutWorkspace(env.ctx, "workspace.put", rpcapi.WorkspacePutRequest{
		Name: mutationWorkspace,
		Body: rpcapi.WorkspaceUpsert{
			Name:         mutationWorkspace,
			WorkflowName: mutationWorkflow,
			Parameters:   &updateParams,
		},
	})
	if err != nil {
		t.Fatalf("workspace.put: %v", err)
	}
	if workspace.Name != mutationWorkspace || workspace.WorkflowName != mutationWorkflow {
		t.Fatalf("workspace.put = %#v", workspace)
	}
	workspace, err = env.peer.GetWorkspace(env.ctx, "workspace.get.updated", rpcapi.WorkspaceGetRequest{Name: mutationWorkspace})
	if err != nil {
		t.Fatalf("workspace.get updated: %v", err)
	}
	if workspace.Parameters == nil {
		t.Fatalf("workspace.get updated parameters are nil: %#v", workspace)
	}
	typed, err := workspace.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatalf("workspace.get updated parameters decode: %v", err)
	}
	if typed.Input == nil || *typed.Input != rpcapi.WorkspaceInputModeRealtime {
		t.Fatalf("workspace.get updated input = %#v, want realtime", typed.Input)
	}
	assertWorkspacePagination(t, env.ctx, env.peer, sharedWorkspace, mutationWorkspace)
	if _, err := env.peer.DeleteWorkspace(env.ctx, "workspace.delete", rpcapi.WorkspaceDeleteRequest{Name: mutationWorkspace}); err != nil {
		t.Fatalf("workspace.delete: %v", err)
	}
}

func TestServerResourceACLRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	denied := env.h.ConnectClientFromContext("peer-denied")
	defer denied.Close()
	if _, err := denied.GetWorkflow(env.ctx, "workflow.get.denied", rpcapi.WorkflowGetRequest{Name: sharedWorkflow}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied peer workflow.get error = %v", err)
	}
	if _, err := denied.GetWorkspace(env.ctx, "workspace.get.denied", rpcapi.WorkspaceGetRequest{Name: sharedWorkspace}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied peer workspace.get error = %v", err)
	}
	if _, err := denied.GetModel(env.ctx, "model.get.denied", rpcapi.ModelGetRequest{Id: sharedModel}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied peer model.get error = %v", err)
	}
	if _, err := denied.GetCredential(env.ctx, "credential.get.denied", rpcapi.CredentialGetRequest{Name: sharedCredential}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied peer credential.get error = %v", err)
	}
	assertDeniedListsAreEmpty(t, env.ctx, denied)
}

func TestServerResourceCreatorOwnsConcreteResources(t *testing.T) {
	env := newServerResourceHarness(t)
	admin := serverResourceAdminClient(t, env)

	workspaceName := "acl-owner-workspace"
	unownedWorkspaceName := "acl-unowned-workspace"
	modelID := "acl-owner-model"
	credentialName := "acl-owner-credential"
	t.Cleanup(func() {
		_, _ = admin.DeleteWorkspaceWithResponse(env.ctx, workspaceName)
		_, _ = admin.DeleteWorkspaceWithResponse(env.ctx, unownedWorkspaceName)
		_, _ = admin.DeleteModelWithResponse(env.ctx, modelID)
		_, _ = admin.DeleteCredentialWithResponse(env.ctx, credentialName)
	})
	_, _ = admin.DeleteWorkspaceWithResponse(env.ctx, workspaceName)
	_, _ = admin.DeleteWorkspaceWithResponse(env.ctx, unownedWorkspaceName)
	_, _ = admin.DeleteModelWithResponse(env.ctx, modelID)
	_, _ = admin.DeleteCredentialWithResponse(env.ctx, credentialName)

	created, err := admin.CreateWorkspaceWithResponse(env.ctx, adminhttp.WorkspaceUpsert{
		Name:         unownedWorkspaceName,
		WorkflowName: sharedWorkflow,
	})
	if err != nil || created.JSON200 == nil {
		t.Fatalf("create unowned workspace: response=%#v error=%v", created, err)
	}
	if _, err := env.peer.DeleteWorkspace(env.ctx, "acl.owner.workspace.unowned.delete", rpcapi.WorkspaceDeleteRequest{Name: unownedWorkspaceName}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("workspace.delete unowned error = %v", err)
	}

	if _, err := env.peer.CreateWorkspace(env.ctx, "acl.owner.workspace.create", rpcapi.WorkspaceCreateRequest{
		Name:         workspaceName,
		WorkflowName: sharedWorkflow,
	}); err != nil {
		t.Fatalf("workspace.create owner: %v", err)
	}
	if _, err := env.peer.PutWorkspace(env.ctx, "acl.owner.workspace.put", rpcapi.WorkspacePutRequest{
		Name: workspaceName,
		Body: rpcapi.WorkspaceUpsert{Name: workspaceName, WorkflowName: sharedWorkflow},
	}); err != nil {
		t.Fatalf("workspace.put owner: %v", err)
	}
	if _, err := env.peer.DeleteWorkspace(env.ctx, "acl.owner.workspace.delete", rpcapi.WorkspaceDeleteRequest{Name: workspaceName}); err != nil {
		t.Fatalf("workspace.delete owner: %v", err)
	}

	if _, err := env.peer.CreateModel(env.ctx, "acl.owner.model.create", rpcModel(modelID, "openai-main")); err != nil {
		t.Fatalf("model.create owner: %v", err)
	}
	if _, err := env.peer.PutModel(env.ctx, "acl.owner.model.put", rpcapi.ModelPutRequest{
		Id:   modelID,
		Body: rpcModel(modelID, "openai-main"),
	}); err != nil {
		t.Fatalf("model.put owner: %v", err)
	}
	if _, err := env.peer.DeleteModel(env.ctx, "acl.owner.model.delete", rpcapi.ModelDeleteRequest{Id: modelID}); err != nil {
		t.Fatalf("model.delete owner: %v", err)
	}

	if _, err := env.peer.CreateCredential(env.ctx, "acl.owner.credential.create", rpcCredential(credentialName, "sk-created")); err != nil {
		t.Fatalf("credential.create owner: %v", err)
	}
	if _, err := env.peer.PutCredential(env.ctx, "acl.owner.credential.put", rpcapi.CredentialPutRequest{
		Name: credentialName,
		Body: rpcCredential(credentialName, "sk-updated"),
	}); err != nil {
		t.Fatalf("credential.put owner: %v", err)
	}
	if _, err := env.peer.DeleteCredential(env.ctx, "acl.owner.credential.delete", rpcapi.CredentialDeleteRequest{Name: credentialName}); err != nil {
		t.Fatalf("credential.delete owner: %v", err)
	}
}

func serverResourceAdminClient(t *testing.T, env *serverResourceHarness) *adminhttp.ClientWithResponses {
	t.Helper()

	adminClient := env.h.ConnectClientFromContext("admin-a")
	t.Cleanup(func() { adminClient.Close() })
	api, err := adminClient.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin API client: %v", err)
	}
	return api
}
