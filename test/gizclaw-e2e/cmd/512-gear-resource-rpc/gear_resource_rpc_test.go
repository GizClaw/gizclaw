package gearresourcerpc_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

const gearResourceRole = "gear-resource-rpc-admin"

func TestGearResourceRPCUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "512-gear-resource-rpc")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)
	h.CreateContext("gear-a").MustSucceed(t)
	h.RegisterContext("gear-a", "--sn", "gear-a-sn").MustSucceed(t)
	h.CreateContext("gear-denied").MustSucceed(t)
	h.RegisterContext("gear-denied", "--sn", "gear-denied-sn").MustSucceed(t)

	seedGearResources(t, h)

	gear := h.ConnectClientFromContext("gear-a")
	defer gear.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	workflowList, err := gear.ListWorkflows(ctx, "workflow.list.seeded", rpcapi.WorkflowListRequest{})
	if err != nil {
		t.Fatalf("workflow.list seeded: %v", err)
	}
	if !hasWorkflow(workflowList.Items, "seed-flow") {
		t.Fatalf("workflow.list missing seed-flow: %#v", workflowList.Items)
	}
	seedFlow, err := gear.GetWorkflow(ctx, "workflow.get.seeded", rpcapi.WorkflowGetRequest{Name: "seed-flow"})
	if err != nil {
		t.Fatalf("workflow.get seeded: %v", err)
	}
	if seedFlow.Metadata.Name != "seed-flow" {
		t.Fatalf("workflow.get seeded name = %q", seedFlow.Metadata.Name)
	}

	createdFlow, err := gear.CreateWorkflow(ctx, "workflow.create", rpcWorkflow("gear-flow", "created by gear rpc"))
	if err != nil {
		t.Fatalf("workflow.create: %v", err)
	}
	if createdFlow.Metadata.Name != "gear-flow" {
		t.Fatalf("workflow.create name = %q", createdFlow.Metadata.Name)
	}
	updatedFlowDoc := rpcWorkflow("gear-flow", "updated by gear rpc")
	updatedFlow, err := gear.PutWorkflow(ctx, "workflow.put", rpcapi.WorkflowPutRequest{Name: "gear-flow", Body: updatedFlowDoc})
	if err != nil {
		t.Fatalf("workflow.put: %v", err)
	}
	if updatedFlow.Metadata.Description == nil || *updatedFlow.Metadata.Description != "updated by gear rpc" {
		t.Fatalf("workflow.put description = %#v", updatedFlow.Metadata.Description)
	}

	workspaceList, err := gear.ListWorkspaces(ctx, "workspace.list.seeded", rpcapi.WorkspaceListRequest{})
	if err != nil {
		t.Fatalf("workspace.list seeded: %v", err)
	}
	if !hasWorkspace(workspaceList.Items, "seed-workspace") {
		t.Fatalf("workspace.list missing seed-workspace: %#v", workspaceList.Items)
	}
	seedWorkspace, err := gear.GetWorkspace(ctx, "workspace.get.seeded", rpcapi.WorkspaceGetRequest{Name: "seed-workspace"})
	if err != nil {
		t.Fatalf("workspace.get seeded: %v", err)
	}
	if seedWorkspace.Name != "seed-workspace" || seedWorkspace.WorkflowName != "seed-flow" {
		t.Fatalf("workspace.get seeded = %#v", seedWorkspace)
	}
	workspace, err := gear.CreateWorkspace(ctx, "workspace.create", rpcapi.WorkspaceCreateRequest{
		Name:         "gear-workspace",
		WorkflowName: "gear-flow",
		Parameters:   &map[string]interface{}{"mode": "create"},
	})
	if err != nil {
		t.Fatalf("workspace.create: %v", err)
	}
	if workspace.Name != "gear-workspace" || workspace.WorkflowName != "gear-flow" {
		t.Fatalf("workspace.create = %#v", workspace)
	}
	workspace, err = gear.PutWorkspace(ctx, "workspace.put", rpcapi.WorkspacePutRequest{
		Name: "gear-workspace",
		Body: rpcapi.Workspace{
			Name:         "gear-workspace",
			WorkflowName: "gear-flow",
			Parameters:   &map[string]interface{}{"mode": "update"},
		},
	})
	if err != nil {
		t.Fatalf("workspace.put: %v", err)
	}
	if workspace.Parameters == nil || (*workspace.Parameters)["mode"] != "update" {
		t.Fatalf("workspace.put parameters = %#v", workspace.Parameters)
	}
	workspace, err = gear.GetWorkspace(ctx, "workspace.get.updated", rpcapi.WorkspaceGetRequest{Name: "gear-workspace"})
	if err != nil {
		t.Fatalf("workspace.get updated: %v", err)
	}
	if workspace.Parameters == nil || (*workspace.Parameters)["mode"] != "update" {
		t.Fatalf("workspace.get updated parameters = %#v", workspace.Parameters)
	}

	modelList, err := gear.ListModels(ctx, "model.list.seeded", rpcapi.ModelListRequest{})
	if err != nil {
		t.Fatalf("model.list seeded: %v", err)
	}
	if !hasModel(modelList.Items, "seed-model") {
		t.Fatalf("model.list missing seed-model: %#v", modelList.Items)
	}
	seedModel, err := gear.GetModel(ctx, "model.get.seeded", rpcapi.ModelGetRequest{Id: "seed-model"})
	if err != nil {
		t.Fatalf("model.get seeded: %v", err)
	}
	if seedModel.Id != "seed-model" {
		t.Fatalf("model.get seeded id = %q", seedModel.Id)
	}
	model, err := gear.CreateModel(ctx, "model.create", rpcModel("gear-model", "gear-provider"))
	if err != nil {
		t.Fatalf("model.create: %v", err)
	}
	if model.Id != "gear-model" {
		t.Fatalf("model.create id = %q", model.Id)
	}
	modelName := "gear model updated"
	model, err = gear.PutModel(ctx, "model.put", rpcapi.ModelPutRequest{
		Id: "gear-model",
		Body: func() rpcapi.Model {
			body := rpcModel("gear-model", "gear-provider")
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
	model, err = gear.GetModel(ctx, "model.get.updated", rpcapi.ModelGetRequest{Id: "gear-model"})
	if err != nil {
		t.Fatalf("model.get updated: %v", err)
	}
	if model.Name == nil || *model.Name != modelName {
		t.Fatalf("model.get updated name = %#v", model.Name)
	}

	credentialList, err := gear.ListCredentials(ctx, "credential.list.seeded", rpcapi.CredentialListRequest{})
	if err != nil {
		t.Fatalf("credential.list seeded: %v", err)
	}
	if !hasCredential(credentialList.Items, "seed-credential") {
		t.Fatalf("credential.list missing seed-credential: %#v", credentialList.Items)
	}
	seedCredential, err := gear.GetCredential(ctx, "credential.get.seeded", rpcapi.CredentialGetRequest{Name: "seed-credential"})
	if err != nil {
		t.Fatalf("credential.get seeded: %v", err)
	}
	if seedCredential.Name != "seed-credential" {
		t.Fatalf("credential.get seeded name = %q", seedCredential.Name)
	}
	credential, err := gear.CreateCredential(ctx, "credential.create", rpcCredential("gear-credential", "sk-created"))
	if err != nil {
		t.Fatalf("credential.create: %v", err)
	}
	if credential.Name != "gear-credential" {
		t.Fatalf("credential.create name = %q", credential.Name)
	}
	credential, err = gear.PutCredential(ctx, "credential.put", rpcapi.CredentialPutRequest{
		Name: "gear-credential",
		Body: rpcCredential("gear-credential", "sk-updated"),
	})
	if err != nil {
		t.Fatalf("credential.put: %v", err)
	}
	if credential.Body["api_key"] != "sk-updated" {
		t.Fatalf("credential.put body = %#v", credential.Body)
	}
	credential, err = gear.GetCredential(ctx, "credential.get.updated", rpcapi.CredentialGetRequest{Name: "gear-credential"})
	if err != nil {
		t.Fatalf("credential.get updated: %v", err)
	}
	if credential.Body["api_key"] != "sk-updated" {
		t.Fatalf("credential.get updated body = %#v", credential.Body)
	}

	assertWorkflowPagination(t, ctx, gear, "seed-flow", "gear-flow")
	assertWorkspacePagination(t, ctx, gear, "seed-workspace", "gear-workspace")
	assertModelPagination(t, ctx, gear, "seed-model", "gear-model")
	assertCredentialPagination(t, ctx, gear, "seed-credential", "gear-credential")

	if _, err := gear.DeleteCredential(ctx, "credential.delete", rpcapi.CredentialDeleteRequest{Name: "gear-credential"}); err != nil {
		t.Fatalf("credential.delete: %v", err)
	}
	if _, err := gear.DeleteModel(ctx, "model.delete", rpcapi.ModelDeleteRequest{Id: "gear-model"}); err != nil {
		t.Fatalf("model.delete: %v", err)
	}
	if _, err := gear.DeleteWorkspace(ctx, "workspace.delete", rpcapi.WorkspaceDeleteRequest{Name: "gear-workspace"}); err != nil {
		t.Fatalf("workspace.delete: %v", err)
	}
	if _, err := gear.DeleteWorkflow(ctx, "workflow.delete", rpcapi.WorkflowDeleteRequest{Name: "gear-flow"}); err != nil {
		t.Fatalf("workflow.delete: %v", err)
	}

	denied := h.ConnectClientFromContext("gear-denied")
	defer denied.Close()
	if _, err := denied.GetWorkflow(ctx, "workflow.get.denied", rpcapi.WorkflowGetRequest{Name: "seed-flow"}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied gear workflow.get error = %v", err)
	}
	if _, err := denied.GetWorkspace(ctx, "workspace.get.denied", rpcapi.WorkspaceGetRequest{Name: "seed-workspace"}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied gear workspace.get error = %v", err)
	}
	if _, err := denied.GetModel(ctx, "model.get.denied", rpcapi.ModelGetRequest{Id: "seed-model"}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied gear model.get error = %v", err)
	}
	if _, err := denied.GetCredential(ctx, "credential.get.denied", rpcapi.CredentialGetRequest{Name: "seed-credential"}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("denied gear credential.get error = %v", err)
	}
	assertDeniedListsAreEmpty(t, ctx, denied)
}

func assertWorkflowPagination(t *testing.T, ctx context.Context, gear *gizclaw.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := gear.ListWorkflows(ctx, "workflow.list.page1", rpcapi.WorkflowListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("workflow.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("workflow.list page1 = %#v", first)
	}
	second, err := gear.ListWorkflows(ctx, "workflow.list.page2", rpcapi.WorkflowListRequest{Limit: &limit, Cursor: first.NextCursor})
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

func assertWorkspacePagination(t *testing.T, ctx context.Context, gear *gizclaw.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := gear.ListWorkspaces(ctx, "workspace.list.page1", rpcapi.WorkspaceListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("workspace.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("workspace.list page1 = %#v", first)
	}
	second, err := gear.ListWorkspaces(ctx, "workspace.list.page2", rpcapi.WorkspaceListRequest{Limit: &limit, Cursor: first.NextCursor})
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

func assertModelPagination(t *testing.T, ctx context.Context, gear *gizclaw.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := gear.ListModels(ctx, "model.list.page1", rpcapi.ModelListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("model.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("model.list page1 = %#v", first)
	}
	second, err := gear.ListModels(ctx, "model.list.page2", rpcapi.ModelListRequest{Limit: &limit, Cursor: first.NextCursor})
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

func assertCredentialPagination(t *testing.T, ctx context.Context, gear *gizclaw.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := gear.ListCredentials(ctx, "credential.list.page1", rpcapi.CredentialListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("credential.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("credential.list page1 = %#v", first)
	}
	second, err := gear.ListCredentials(ctx, "credential.list.page2", rpcapi.CredentialListRequest{Limit: &limit, Cursor: first.NextCursor})
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

func assertDeniedListsAreEmpty(t *testing.T, ctx context.Context, denied *gizclaw.Client) {
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

func seedGearResources(t *testing.T, h *clitest.Harness) {
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
		Name: gearResourceRole,
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

	subject := apitypes.ACLSubject{Kind: apitypes.ACLSubjectKindPk, Id: h.ContextPublicKey("gear-a")}
	for _, resource := range []apitypes.ACLResource{
		{Kind: apitypes.ACLResourceKindWorkflow, Id: "seed-flow"},
		{Kind: apitypes.ACLResourceKindWorkflow, Id: "gear-flow"},
		{Kind: apitypes.ACLResourceKindWorkspace, Id: "seed-workspace"},
		{Kind: apitypes.ACLResourceKindWorkspace, Id: "gear-workspace"},
		{Kind: apitypes.ACLResourceKindModel, Id: "seed-model"},
		{Kind: apitypes.ACLResourceKindModel, Id: "gear-model"},
		{Kind: apitypes.ACLResourceKindCredential, Id: "seed-credential"},
		{Kind: apitypes.ACLResourceKindCredential, Id: "gear-credential"},
	} {
		id := fmt.Sprintf("gear-resource-rpc-%s-%s", resource.Kind, resource.Id)
		resp, err := api.CreateACLPolicyBindingWithResponse(ctx, adminservice.ACLPolicyBindingUpsert{
			Id: &id,
			Policy: apitypes.ACLPolicy{
				Subject:  subject,
				Resource: resource,
				Role:     gearResourceRole,
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
		Parameters:   &map[string]interface{}{"seeded": true},
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
		Method:   apitypes.CredentialMethodApiKey,
		Body:     apitypes.CredentialBody{"api_key": "sk-seed"},
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
		Method:   rpcapi.CredentialMethodApiKey,
		Body:     rpcapi.CredentialBody{"api_key": apiKey},
	}
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
