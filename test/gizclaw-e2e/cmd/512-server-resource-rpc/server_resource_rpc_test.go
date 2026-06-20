package peerresourcerpc_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

const serverResourceRole = "server-resource-rpc-admin"

func TestServerResourceRPCUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "512-server-resource-rpc")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)
	h.CreateContext("peer-a").MustSucceed(t)
	h.RegisterContext("peer-a", "--sn", "peer-a-sn").MustSucceed(t)
	h.CreateContext("peer-denied").MustSucceed(t)
	h.RegisterContext("peer-denied", "--sn", "peer-denied-sn").MustSucceed(t)

	seedPeerResources(t, h)

	peer := h.ConnectClientFromContext("peer-a")
	defer peer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	workflowList, err := peer.ListWorkflows(ctx, "workflow.list.seeded", rpcapi.WorkflowListRequest{})
	if err != nil {
		t.Fatalf("workflow.list seeded: %v", err)
	}
	if !hasWorkflow(workflowList.Items, "seed-flow") {
		t.Fatalf("workflow.list missing seed-flow: %#v", workflowList.Items)
	}
	seedFlow, err := peer.GetWorkflow(ctx, "workflow.get.seeded", rpcapi.WorkflowGetRequest{Name: "seed-flow"})
	if err != nil {
		t.Fatalf("workflow.get seeded: %v", err)
	}
	if seedFlow.Metadata.Name != "seed-flow" {
		t.Fatalf("workflow.get seeded name = %q", seedFlow.Metadata.Name)
	}

	createdFlow, err := peer.CreateWorkflow(ctx, "workflow.create", rpcWorkflow("peer-flow", "created by peer rpc"))
	if err != nil {
		t.Fatalf("workflow.create: %v", err)
	}
	if createdFlow.Metadata.Name != "peer-flow" {
		t.Fatalf("workflow.create name = %q", createdFlow.Metadata.Name)
	}
	updatedFlowDoc := rpcWorkflow("peer-flow", "updated by peer rpc")
	updatedFlow, err := peer.PutWorkflow(ctx, "workflow.put", rpcapi.WorkflowPutRequest{Name: "peer-flow", Body: updatedFlowDoc})
	if err != nil {
		t.Fatalf("workflow.put: %v", err)
	}
	if updatedFlow.Metadata.Description == nil || *updatedFlow.Metadata.Description != "updated by peer rpc" {
		t.Fatalf("workflow.put description = %#v", updatedFlow.Metadata.Description)
	}

	workspaceList, err := peer.ListWorkspaces(ctx, "workspace.list.seeded", rpcapi.WorkspaceListRequest{})
	if err != nil {
		t.Fatalf("workspace.list seeded: %v", err)
	}
	if !hasWorkspace(workspaceList.Items, "seed-workspace") {
		t.Fatalf("workspace.list missing seed-workspace: %#v", workspaceList.Items)
	}
	seedWorkspace, err := peer.GetWorkspace(ctx, "workspace.get.seeded", rpcapi.WorkspaceGetRequest{Name: "seed-workspace"})
	if err != nil {
		t.Fatalf("workspace.get seeded: %v", err)
	}
	if seedWorkspace.Name != "seed-workspace" || seedWorkspace.WorkflowName != "seed-flow" {
		t.Fatalf("workspace.get seeded = %#v", seedWorkspace)
	}
	workspace, err := peer.CreateWorkspace(ctx, "workspace.create", rpcapi.WorkspaceCreateRequest{
		Name:         "peer-workspace",
		WorkflowName: "peer-flow",
		Parameters:   testRPCFlowcraftWorkspaceParameters("create"),
	})
	if err != nil {
		t.Fatalf("workspace.create: %v", err)
	}
	if workspace.Name != "peer-workspace" || workspace.WorkflowName != "peer-flow" {
		t.Fatalf("workspace.create = %#v", workspace)
	}
	workspace, err = peer.PutWorkspace(ctx, "workspace.put", rpcapi.WorkspacePutRequest{
		Name: "peer-workspace",
		Body: rpcapi.Workspace{
			Name:         "peer-workspace",
			WorkflowName: "peer-flow",
			Parameters:   testRPCFlowcraftWorkspaceParameters("update"),
		},
	})
	if err != nil {
		t.Fatalf("workspace.put: %v", err)
	}
	if testRPCFlowcraftGenerateModel(t, workspace.Parameters) != "update" {
		t.Fatalf("workspace.put parameters = %#v", workspace.Parameters)
	}
	workspace, err = peer.GetWorkspace(ctx, "workspace.get.updated", rpcapi.WorkspaceGetRequest{Name: "peer-workspace"})
	if err != nil {
		t.Fatalf("workspace.get updated: %v", err)
	}
	if testRPCFlowcraftGenerateModel(t, workspace.Parameters) != "update" {
		t.Fatalf("workspace.get updated parameters = %#v", workspace.Parameters)
	}

	modelList, err := peer.ListModels(ctx, "model.list.seeded", rpcapi.ModelListRequest{})
	if err != nil {
		t.Fatalf("model.list seeded: %v", err)
	}
	if !hasModel(modelList.Items, "seed-model") {
		t.Fatalf("model.list missing seed-model: %#v", modelList.Items)
	}
	seedModel, err := peer.GetModel(ctx, "model.get.seeded", rpcapi.ModelGetRequest{Id: "seed-model"})
	if err != nil {
		t.Fatalf("model.get seeded: %v", err)
	}
	if seedModel.Id != "seed-model" {
		t.Fatalf("model.get seeded id = %q", seedModel.Id)
	}
	model, err := peer.CreateModel(ctx, "model.create", rpcModel("peer-model", "peer-provider"))
	if err != nil {
		t.Fatalf("model.create: %v", err)
	}
	if model.Id != "peer-model" {
		t.Fatalf("model.create id = %q", model.Id)
	}
	modelName := "peer model updated"
	model, err = peer.PutModel(ctx, "model.put", rpcapi.ModelPutRequest{
		Id: "peer-model",
		Body: func() rpcapi.Model {
			body := rpcModel("peer-model", "peer-provider")
			body.Name = &modelName
			return body
		}(),
	})
	if err != nil {
		t.Fatalf("model.put: %v", err)
	}
	if model.Name == nil || *model.Name != modelName {
		t.Fatalf("model.put name = %#v", model.Name)
	}
	model, err = peer.GetModel(ctx, "model.get.updated", rpcapi.ModelGetRequest{Id: "peer-model"})
	if err != nil {
		t.Fatalf("model.get updated: %v", err)
	}
	if model.Name == nil || *model.Name != modelName {
		t.Fatalf("model.get updated name = %#v", model.Name)
	}

	credentialList, err := peer.ListCredentials(ctx, "credential.list.seeded", rpcapi.CredentialListRequest{})
	if err != nil {
		t.Fatalf("credential.list seeded: %v", err)
	}
	if !hasCredential(credentialList.Items, "seed-credential") {
		t.Fatalf("credential.list missing seed-credential: %#v", credentialList.Items)
	}
	seedCredential, err := peer.GetCredential(ctx, "credential.get.seeded", rpcapi.CredentialGetRequest{Name: "seed-credential"})
	if err != nil {
		t.Fatalf("credential.get seeded: %v", err)
	}
	if seedCredential.Name != "seed-credential" {
		t.Fatalf("credential.get seeded name = %q", seedCredential.Name)
	}
	credential, err := peer.CreateCredential(ctx, "credential.create", rpcCredential("peer-credential", "sk-created"))
	if err != nil {
		t.Fatalf("credential.create: %v", err)
	}
	if credential.Name != "peer-credential" {
		t.Fatalf("credential.create name = %q", credential.Name)
	}
	credential, err = peer.PutCredential(ctx, "credential.put", rpcapi.CredentialPutRequest{
		Name: "peer-credential",
		Body: rpcCredential("peer-credential", "sk-updated"),
	})
	if err != nil {
		t.Fatalf("credential.put: %v", err)
	}
	if testRPCOpenAIAPIKey(t, credential.Body) != "sk-updated" {
		t.Fatalf("credential.put body = %#v", credential.Body)
	}
	credential, err = peer.GetCredential(ctx, "credential.get.updated", rpcapi.CredentialGetRequest{Name: "peer-credential"})
	if err != nil {
		t.Fatalf("credential.get updated: %v", err)
	}
	if testRPCOpenAIAPIKey(t, credential.Body) != "sk-updated" {
		t.Fatalf("credential.get updated body = %#v", credential.Body)
	}

	assertWorkflowPagination(t, ctx, peer, "seed-flow", "peer-flow")
	assertWorkspacePagination(t, ctx, peer, "seed-workspace", "peer-workspace")
	assertModelPagination(t, ctx, peer, "seed-model", "peer-model")
	assertCredentialPagination(t, ctx, peer, "seed-credential", "peer-credential")

	if _, err := peer.DeleteCredential(ctx, "credential.delete", rpcapi.CredentialDeleteRequest{Name: "peer-credential"}); err != nil {
		t.Fatalf("credential.delete: %v", err)
	}
	if _, err := peer.DeleteModel(ctx, "model.delete", rpcapi.ModelDeleteRequest{Id: "peer-model"}); err != nil {
		t.Fatalf("model.delete: %v", err)
	}
	if _, err := peer.DeleteWorkspace(ctx, "workspace.delete", rpcapi.WorkspaceDeleteRequest{Name: "peer-workspace"}); err != nil {
		t.Fatalf("workspace.delete: %v", err)
	}
	if _, err := peer.DeleteWorkflow(ctx, "workflow.delete", rpcapi.WorkflowDeleteRequest{Name: "peer-flow"}); err != nil {
		t.Fatalf("workflow.delete: %v", err)
	}

	denied := h.ConnectClientFromContext("peer-denied")
	defer denied.Close()
	if _, err := denied.GetWorkflow(ctx, "workflow.get.denied", rpcapi.WorkflowGetRequest{Name: "seed-flow"}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied peer workflow.get error = %v", err)
	}
	if _, err := denied.GetWorkspace(ctx, "workspace.get.denied", rpcapi.WorkspaceGetRequest{Name: "seed-workspace"}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied peer workspace.get error = %v", err)
	}
	if _, err := denied.GetModel(ctx, "model.get.denied", rpcapi.ModelGetRequest{Id: "seed-model"}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied peer model.get error = %v", err)
	}
	if _, err := denied.GetCredential(ctx, "credential.get.denied", rpcapi.CredentialGetRequest{Name: "seed-credential"}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied peer credential.get error = %v", err)
	}
	assertDeniedListsAreEmpty(t, ctx, denied)
}

func assertWorkflowPagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := peer.ListWorkflows(ctx, "workflow.list.page1", rpcapi.WorkflowListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("workflow.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("workflow.list page1 = %#v", first)
	}
	second, err := peer.ListWorkflows(ctx, "workflow.list.page2", rpcapi.WorkflowListRequest{Limit: &limit, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("workflow.list page2: %v", err)
	}
	got := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		got[item.Metadata.Name] = true
	}
	if !got[wantA] || !got[wantB] {
		t.Fatalf("workflow list pagination got names %#v, want %q and %q", got, wantA, wantB)
	}
}

func assertWorkspacePagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := peer.ListWorkspaces(ctx, "workspace.list.page1", rpcapi.WorkspaceListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("workspace.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("workspace.list page1 = %#v", first)
	}
	second, err := peer.ListWorkspaces(ctx, "workspace.list.page2", rpcapi.WorkspaceListRequest{Limit: &limit, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("workspace.list page2: %v", err)
	}
	got := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		got[item.Name] = true
	}
	if !got[wantA] || !got[wantB] {
		t.Fatalf("workspace list pagination got names %#v, want %q and %q", got, wantA, wantB)
	}
}

func assertModelPagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := peer.ListModels(ctx, "model.list.page1", rpcapi.ModelListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("model.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("model.list page1 = %#v", first)
	}
	second, err := peer.ListModels(ctx, "model.list.page2", rpcapi.ModelListRequest{Limit: &limit, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("model.list page2: %v", err)
	}
	got := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		got[item.Id] = true
	}
	if !got[wantA] || !got[wantB] {
		t.Fatalf("model list pagination got ids %#v, want %q and %q", got, wantA, wantB)
	}
}

func assertCredentialPagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := peer.ListCredentials(ctx, "credential.list.page1", rpcapi.CredentialListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("credential.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("credential.list page1 = %#v", first)
	}
	second, err := peer.ListCredentials(ctx, "credential.list.page2", rpcapi.CredentialListRequest{Limit: &limit, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("credential.list page2: %v", err)
	}
	got := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		got[item.Name] = true
	}
	if !got[wantA] || !got[wantB] {
		t.Fatalf("credential list pagination got names %#v, want %q and %q", got, wantA, wantB)
	}
}

func assertDeniedListsAreEmpty(t *testing.T, ctx context.Context, denied *gizcli.Client) {
	t.Helper()

	workflows, err := denied.ListWorkflows(ctx, "workflow.list.denied", rpcapi.WorkflowListRequest{})
	if err != nil {
		t.Fatalf("denied workflow.list: %v", err)
	}
	if len(workflows.Items) != 0 {
		t.Fatalf("denied workflow.list items = %#v", workflows.Items)
	}
	workspaces, err := denied.ListWorkspaces(ctx, "workspace.list.denied", rpcapi.WorkspaceListRequest{})
	if err != nil {
		t.Fatalf("denied workspace.list: %v", err)
	}
	if len(workspaces.Items) != 0 {
		t.Fatalf("denied workspace.list items = %#v", workspaces.Items)
	}
	models, err := denied.ListModels(ctx, "model.list.denied", rpcapi.ModelListRequest{})
	if err != nil {
		t.Fatalf("denied model.list: %v", err)
	}
	if len(models.Items) != 0 {
		t.Fatalf("denied model.list items = %#v", models.Items)
	}
	credentials, err := denied.ListCredentials(ctx, "credential.list.denied", rpcapi.CredentialListRequest{})
	if err != nil {
		t.Fatalf("denied credential.list: %v", err)
	}
	if len(credentials.Items) != 0 {
		t.Fatalf("denied credential.list items = %#v", credentials.Items)
	}
}

func seedPeerResources(t *testing.T, h *clitest.Harness) {
	t.Helper()

	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	roleResp, err := api.CreateACLRoleWithResponse(ctx, adminservice.ACLRoleUpsert{
		Name: serverResourceRole,
		Permissions: apitypes.ACLPermissionList{
			apitypes.ACLPermissionWorkspaceAdmin,
			apitypes.ACLPermissionWorkspaceRead,
			apitypes.ACLPermissionWorkflowAdmin,
			apitypes.ACLPermissionWorkflowRead,
			apitypes.ACLPermissionWorkflowUse,
			apitypes.ACLPermissionModelAdmin,
			apitypes.ACLPermissionModelRead,
			apitypes.ACLPermissionCredentialAdmin,
			apitypes.ACLPermissionCredentialRead,
		},
	})
	if err != nil {
		t.Fatalf("create ACL role: %v", err)
	}
	if roleResp.JSON200 == nil {
		t.Fatalf("create ACL role status %d: %s", roleResp.StatusCode(), strings.TrimSpace(string(roleResp.Body)))
	}

	subject := apitypes.ACLSubject{Kind: apitypes.ACLSubjectKindPk, Id: h.ContextPublicKey("peer-a")}
	for _, resource := range []apitypes.ACLResource{
		{Kind: apitypes.ACLResourceKindWorkflow, Id: "seed-flow"},
		{Kind: apitypes.ACLResourceKindWorkflow, Id: "peer-flow"},
		{Kind: apitypes.ACLResourceKindWorkspace, Id: "seed-workspace"},
		{Kind: apitypes.ACLResourceKindWorkspace, Id: "peer-workspace"},
		{Kind: apitypes.ACLResourceKindModel, Id: "seed-model"},
		{Kind: apitypes.ACLResourceKindModel, Id: "peer-model"},
		{Kind: apitypes.ACLResourceKindCredential, Id: "seed-credential"},
		{Kind: apitypes.ACLResourceKindCredential, Id: "peer-credential"},
	} {
		id := fmt.Sprintf("peer-resource-rpc-%s-%s", resource.Kind, resource.Id)
		resp, err := api.CreateACLPolicyBindingWithResponse(ctx, adminservice.ACLPolicyBindingUpsert{
			Id: &id,
			Policy: apitypes.ACLPolicy{
				Subject:  subject,
				Resource: resource,
				Role:     serverResourceRole,
			},
		})
		if err != nil {
			t.Fatalf("create ACL policy binding %s: %v", id, err)
		}
		if resp.JSON200 == nil {
			t.Fatalf("create ACL policy binding %s status %d: %s", id, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
		}
	}

	if resp, err := api.CreateWorkflowWithResponse(ctx, apiWorkflow("seed-flow", "seeded workflow")); err != nil {
		t.Fatalf("seed workflow: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("seed workflow status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
	if resp, err := api.CreateWorkspaceWithResponse(ctx, adminservice.WorkspaceUpsert{
		Name:         "seed-workspace",
		WorkflowName: "seed-flow",
		Parameters:   testAPIFlowcraftWorkspaceParameters("seed"),
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("seed workspace status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
	if resp, err := api.CreateModelWithResponse(ctx, adminservice.ModelUpsert{
		Id:     "seed-model",
		Kind:   apitypes.ModelKindLlm,
		Source: apitypes.ModelSourceManual,
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKindOpenaiTenant,
			Name: "seed-provider",
		},
	}); err != nil {
		t.Fatalf("seed model: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("seed model status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
	if resp, err := api.CreateCredentialWithResponse(ctx, adminservice.CredentialUpsert{
		Name:     "seed-credential",
		Provider: "openai",
		Body:     testAPIOpenAICredentialBody("sk-seed"),
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("seed credential status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
}

func apiWorkflow(name, description string) apitypes.WorkflowDocument {
	return apitypes.WorkflowDocument{
		ApiVersion: apitypes.WorkflowAPIVersionGizclawFlowcraftv1alpha1,
		Kind:       apitypes.FlowcraftWorkflowKindFlowcraftWorkflow,
		Metadata: apitypes.WorkflowMetadata{
			Name:        name,
			Description: &description,
		},
		Spec: apitypes.FlowcraftWorkflowSpec{
			"entry_agent": "",
		},
	}
}

func rpcWorkflow(name, description string) rpcapi.WorkflowDocument {
	return rpcapi.WorkflowDocument{
		ApiVersion: rpcapi.WorkflowAPIVersionGizclawFlowcraftv1alpha1,
		Kind:       rpcapi.FlowcraftWorkflowKindFlowcraftWorkflow,
		Metadata: rpcapi.WorkflowMetadata{
			Name:        name,
			Description: &description,
		},
		Spec: rpcapi.FlowcraftWorkflowSpec{
			"entry_agent": "",
		},
	}
}

func rpcModel(id, providerName string) rpcapi.Model {
	return rpcapi.Model{
		Id:     id,
		Kind:   rpcapi.ModelKindLlm,
		Source: rpcapi.ModelSourceManual,
		Provider: rpcapi.ModelProvider{
			Kind: rpcapi.ModelProviderKindOpenaiTenant,
			Name: providerName,
		},
	}
}

func rpcCredential(name, apiKey string) rpcapi.Credential {
	return rpcapi.Credential{
		Name:     name,
		Provider: "openai",
		Body:     testRPCOpenAICredentialBody(apiKey),
	}
}

func testAPIFlowcraftWorkspaceParameters(generateModel string) *apitypes.WorkspaceParameters {
	var params apitypes.WorkspaceParameters
	if err := params.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{
		AgentType:     apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		GenerateModel: ptr(generateModel),
	}); err != nil {
		panic(err)
	}
	return &params
}

func testRPCFlowcraftWorkspaceParameters(generateModel string) *rpcapi.WorkspaceParameters {
	var params rpcapi.WorkspaceParameters
	if err := params.FromFlowcraftWorkspaceParameters(rpcapi.FlowcraftWorkspaceParameters{
		AgentType:     rpcapi.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		GenerateModel: ptr(generateModel),
	}); err != nil {
		panic(err)
	}
	return &params
}

func testRPCFlowcraftGenerateModel(t *testing.T, params *rpcapi.WorkspaceParameters) string {
	t.Helper()
	if params == nil {
		t.Fatal("workspace parameters are nil")
	}
	typed, err := params.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatalf("decode flowcraft workspace parameters: %v", err)
	}
	if typed.GenerateModel == nil {
		return ""
	}
	return *typed.GenerateModel
}

func testAPIOpenAICredentialBody(apiKey string) apitypes.CredentialBody {
	var body apitypes.CredentialBody
	if err := body.FromOpenAICredentialBody(apitypes.OpenAICredentialBody{ApiKey: ptr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}

func testRPCOpenAICredentialBody(apiKey string) rpcapi.CredentialBody {
	var body rpcapi.CredentialBody
	if err := body.FromOpenAICredentialBody(rpcapi.OpenAICredentialBody{ApiKey: ptr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}

func testRPCOpenAIAPIKey(t *testing.T, body rpcapi.CredentialBody) string {
	t.Helper()
	typed, err := body.AsOpenAICredentialBody()
	if err != nil {
		t.Fatalf("decode OpenAI credential body: %v", err)
	}
	if typed.ApiKey == nil {
		return ""
	}
	return *typed.ApiKey
}

func ptr[T any](v T) *T {
	return &v
}

func hasWorkflow(items []rpcapi.WorkflowDocument, name string) bool {
	for _, item := range items {
		if item.Metadata.Name == name {
			return true
		}
	}
	return false
}

func hasWorkspace(items []rpcapi.Workspace, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func hasModel(items []rpcapi.Model, id string) bool {
	for _, item := range items {
		if item.Id == id {
			return true
		}
	}
	return false
}

func hasCredential(items []rpcapi.Credential, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
