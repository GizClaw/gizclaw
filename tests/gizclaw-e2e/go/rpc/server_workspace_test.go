//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerWorkspaceRPC(t *testing.T) {
	env := newServerResourceHarness(t)
	admin := serverResourceAdminClient(t, env)
	pageWorkspace := mutationWorkspace + "-page"

	_, _ = env.peer.DeleteWorkspace(env.ctx, "workspace.delete.preclean", rpcapi.WorkspaceDeleteRequest{Name: mutationWorkspace})
	_, _ = env.peer.DeleteWorkspace(env.ctx, "workspace.delete.page.preclean", rpcapi.WorkspaceDeleteRequest{Name: pageWorkspace})
	_, _ = admin.DeleteWorkflowWithResponse(env.ctx, mutationWorkflow)
	if response, err := admin.CreateWorkflowWithResponse(env.ctx, adminWorkflow(mutationWorkflow, "workspace test flow")); err != nil || response.JSON200 == nil {
		t.Fatalf("create workflow for workspace test: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.DeleteWorkflowWithResponse(env.ctx, mutationWorkflow) })
	createInput := rpcapi.WorkspaceInputModePushToTalk
	workspace, err := env.peer.CreateWorkspace(env.ctx, "workspace.create", rpcapi.WorkspaceCreateRequest{
		Name:           mutationWorkspace,
		WorkflowName:   "mutation",
		WorkflowSource: runtimeSourcePtr(),
		Parameters:     rpcFlowcraftWorkspaceParameters(t, createInput),
	})
	if err != nil {
		t.Fatalf("workspace.create: %v", err)
	}
	if workspace.Name != mutationWorkspace || workspace.WorkflowName != "mutation" || workspace.WorkflowSource == nil || *workspace.WorkflowSource != rpcapi.ResourceSourceRuntime {
		t.Fatalf("workspace.create = %#v", workspace)
	}
	if _, err := env.peer.CreateWorkspace(env.ctx, "workspace.create.page", rpcapi.WorkspaceCreateRequest{
		Name:           pageWorkspace,
		WorkflowName:   "mutation",
		WorkflowSource: runtimeSourcePtr(),
		Parameters:     rpcFlowcraftWorkspaceParameters(t, createInput),
	}); err != nil {
		t.Fatalf("workspace.create page item: %v", err)
	}
	workspaceList, err := env.peer.ListWorkspaces(env.ctx, "workspace.list.owned", rpcapi.WorkspaceListRequest{})
	if err != nil {
		t.Fatalf("workspace.list owned: %v", err)
	}
	if len(workspaceList.Items) != 2 {
		t.Fatalf("workspace.list returned %#v, want two owner Workspaces", workspaceList.Items)
	}
	assertWorkspacePrefixList(t, env.ctx, env.peer)

	updateInput := rpcapi.WorkspaceInputModeRealtime
	workspace, err = env.peer.PutWorkspace(env.ctx, "workspace.put", rpcapi.WorkspacePutRequest{
		Name: mutationWorkspace,
		Body: rpcapi.WorkspaceUpsert{
			Name:           mutationWorkspace,
			WorkflowName:   "mutation",
			WorkflowSource: runtimeSourcePtr(),
			Parameters:     rpcFlowcraftWorkspaceParameters(t, updateInput),
		},
	})
	if err != nil {
		t.Fatalf("workspace.put: %v", err)
	}
	if workspace.Name != mutationWorkspace || workspace.WorkflowName != "mutation" || workspace.WorkflowSource == nil || *workspace.WorkflowSource != rpcapi.ResourceSourceRuntime {
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
	assertWorkspacePagination(t, env.ctx, env.peer, mutationWorkspace, pageWorkspace)
	if _, err := env.peer.DeleteWorkspace(env.ctx, "workspace.delete", rpcapi.WorkspaceDeleteRequest{Name: mutationWorkspace}); err != nil {
		t.Fatalf("workspace.delete: %v", err)
	}
	if _, err := env.peer.DeleteWorkspace(env.ctx, "workspace.delete.page", rpcapi.WorkspaceDeleteRequest{Name: pageWorkspace}); err != nil {
		t.Fatalf("workspace.delete page item: %v", err)
	}
}

func TestServerResourceUnavailableWithoutProfileOrOwnership(t *testing.T) {
	env := newServerResourceHarness(t)

	denied := env.h.ConnectClientFromContext("peer-denied")
	defer denied.Close()
	if _, err := denied.GetWorkflow(env.ctx, "workflow.get.denied", rpcapi.WorkflowGetRequest{Name: "shared", Source: rpcapi.ResourceSourceRuntime}); err == nil {
		t.Fatalf("denied peer workflow.get error = %v", err)
	}
	if _, err := denied.GetWorkspace(env.ctx, "workspace.get.denied", rpcapi.WorkspaceGetRequest{Name: sharedWorkspace}); err == nil {
		t.Fatalf("denied peer workspace.get error = %v", err)
	}
	if _, err := denied.GetModel(env.ctx, "model.get.denied", rpcapi.ModelGetRequest{Id: sharedModel}); err == nil {
		t.Fatalf("denied peer model.get error = %v", err)
	}
	if _, err := denied.GetCredential(env.ctx, "credential.get.denied", rpcapi.CredentialGetRequest{Name: sharedCredential}); err == nil {
		t.Fatalf("denied peer credential.get error = %v", err)
	}
	assertDeniedListsAreEmpty(t, env.ctx, denied)
}

func TestServerResourceCreatorOwnsConcreteResources(t *testing.T) {
	env := newServerResourceHarness(t)
	admin := serverResourceAdminClient(t, env)

	workspaceName := "owner-workspace"
	unownedWorkspaceName := "unowned-workspace"
	modelID := "owner-model"
	credentialName := "owner-credential"
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

	input := apitypes.WorkspaceInputModePushToTalk
	var adminParameters apitypes.WorkspaceParameters
	generateModel := sharedModel
	if err := adminParameters.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{
		AgentType:     apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		Input:         &input,
		GenerateModel: &generateModel,
	}); err != nil {
		t.Fatalf("build unowned Workspace parameters: %v", err)
	}
	created, err := admin.CreateWorkspaceWithResponse(env.ctx, adminhttp.WorkspaceUpsert{
		Name:         unownedWorkspaceName,
		WorkflowName: sharedWorkflow,
		Parameters:   &adminParameters,
	})
	if err != nil || created.JSON200 == nil {
		t.Fatalf("create unowned workspace: response=%#v error=%v", created, err)
	}
	if _, err := env.peer.DeleteWorkspace(env.ctx, "owner.workspace.unowned.delete", rpcapi.WorkspaceDeleteRequest{Name: unownedWorkspaceName}); err == nil {
		t.Fatalf("workspace.delete unowned error = %v", err)
	}

	if _, err := env.peer.CreateWorkspace(env.ctx, "owner.workspace.create", rpcapi.WorkspaceCreateRequest{
		Name:           workspaceName,
		WorkflowName:   "shared",
		WorkflowSource: runtimeSourcePtr(),
		Parameters:     rpcFlowcraftWorkspaceParameters(t, rpcapi.WorkspaceInputModePushToTalk),
	}); err != nil {
		t.Fatalf("workspace.create owner: %v", err)
	}
	if _, err := env.peer.PutWorkspace(env.ctx, "owner.workspace.put", rpcapi.WorkspacePutRequest{
		Name: workspaceName,
		Body: rpcapi.WorkspaceUpsert{
			Name:           workspaceName,
			WorkflowName:   "shared",
			WorkflowSource: runtimeSourcePtr(),
			Parameters:     rpcFlowcraftWorkspaceParameters(t, rpcapi.WorkspaceInputModePushToTalk),
		},
	}); err != nil {
		t.Fatalf("workspace.put owner: %v", err)
	}
	if _, err := env.peer.DeleteWorkspace(env.ctx, "owner.workspace.delete", rpcapi.WorkspaceDeleteRequest{Name: workspaceName}); err != nil {
		t.Fatalf("workspace.delete owner: %v", err)
	}

	if _, err := env.peer.CreateModel(env.ctx, "owner.model.create", rpcModel(modelID, "openai-main")); err != nil {
		t.Fatalf("model.create owner: %v", err)
	}
	if _, err := env.peer.PutModel(env.ctx, "owner.model.put", rpcapi.ModelPutRequest{
		Id:   modelID,
		Body: rpcModel(modelID, "openai-main"),
	}); err != nil {
		t.Fatalf("model.put owner: %v", err)
	}
	if _, err := env.peer.DeleteModel(env.ctx, "owner.model.delete", rpcapi.ModelDeleteRequest{Id: modelID}); err != nil {
		t.Fatalf("model.delete owner: %v", err)
	}

	if _, err := env.peer.CreateCredential(env.ctx, "owner.credential.create", rpcCredential(credentialName, "sk-created")); err != nil {
		t.Fatalf("credential.create owner: %v", err)
	}
	if _, err := env.peer.PutCredential(env.ctx, "owner.credential.put", rpcapi.CredentialPutRequest{
		Name: credentialName,
		Body: rpcCredential(credentialName, "sk-updated"),
	}); err != nil {
		t.Fatalf("credential.put owner: %v", err)
	}
	if _, err := env.peer.DeleteCredential(env.ctx, "owner.credential.delete", rpcapi.CredentialDeleteRequest{Name: credentialName}); err != nil {
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
