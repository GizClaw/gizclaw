package gizclaw_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
)

func TestIntegrationAdminServiceWorkflowLifecycle(t *testing.T) {
	ts := startTestServer(t)

	admin := newTestClient(t, ts)
	ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: strPtr("admin")})

	createDoc := mustWorkflow(t, `{
		"name": "demo-assistant",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {}
		}
	}`)
	created, err := createWorkflow(context.Background(), admin, createDoc)
	if err != nil {
		t.Fatalf("CreateWorkflow error: %v", err)
	}
	if created.Spec.Driver != apitypes.WorkflowDriverFlowcraft {
		t.Fatalf("CreateWorkflow driver = %q", created.Spec.Driver)
	}

	items, err := listWorkflows(context.Background(), admin)
	if err != nil {
		t.Fatalf("ListWorkflows error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListWorkflows len = %d", len(items))
	}

	got, err := getWorkflow(context.Background(), admin, "demo-assistant")
	if err != nil {
		t.Fatalf("GetWorkflow error: %v", err)
	}
	if got.Name != "demo-assistant" {
		t.Fatalf("GetWorkflow name = %q", got.Name)
	}

	updateDoc := mustWorkflow(t, `{
		"name": "demo-assistant",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {
				"runtime": {
					"executor_ref": "local"
				}
			}
		}
	}`)
	updated, err := putWorkflow(context.Background(), admin, "demo-assistant", updateDoc)
	if err != nil {
		t.Fatalf("PutWorkflow error: %v", err)
	}
	if updated.Spec.Flowcraft == nil || (*updated.Spec.Flowcraft)["runtime"] == nil {
		t.Fatalf("PutWorkflow spec = %#v", updated.Spec)
	}

	if _, err := deleteWorkflow(context.Background(), admin, "demo-assistant"); err != nil {
		t.Fatalf("DeleteWorkflow error: %v", err)
	}
	if _, err := getWorkflow(context.Background(), admin, "demo-assistant"); err == nil {
		t.Fatal("GetWorkflow after delete expected error")
	}
}

func TestIntegrationAdminServiceRejectsLegacyWorkflowDescription(t *testing.T) {
	ts := startTestServer(t)

	admin := newTestClient(t, ts)
	ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: strPtr("admin")})
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("ServerAdminClient() error = %v", err)
	}
	resp, err := api.CreateWorkflowWithBodyWithResponse(
		context.Background(),
		"application/json",
		strings.NewReader(`{
			"metadata":{"name":"legacy","description":"old"},
			"spec":{"driver":"flowcraft","flowcraft":{}}
		}`),
	)
	if err != nil {
		t.Fatalf("CreateWorkflowWithBodyWithResponse() error = %v", err)
	}
	if resp.StatusCode() != http.StatusBadRequest {
		t.Fatalf("CreateWorkflow status = %d, body = %s", resp.StatusCode(), resp.Body)
	}
}

func TestIntegrationAdminServiceWorkspaceLifecycle(t *testing.T) {
	ts := startTestServer(t)

	admin := newTestClient(t, ts)
	ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: strPtr("admin")})

	workflowDoc := mustWorkflow(t, `{
		"name": "demo-workflow",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {}
		}
	}`)
	if _, err := createWorkflow(context.Background(), admin, workflowDoc); err != nil {
		t.Fatalf("CreateWorkflow error: %v", err)
	}
	if _, err := createModel(context.Background(), admin, adminhttp.ModelUpsert{
		Id:     "updated",
		Kind:   apitypes.ModelKindLlm,
		Source: apitypes.ModelSourceManual,
		Provider: apitypes.ModelProvider{
			Kind: "openai-tenant",
			Name: "global",
		},
		ProviderData: mustOpenAIProviderData(t, "updated-upstream"),
	}); err != nil {
		t.Fatalf("CreateModel error: %v", err)
	}

	createBody := adminhttp.WorkspaceUpsert{
		Name:         "demo-workspace",
		WorkflowName: "demo-workflow",
		Parameters:   testFlowcraftWorkspaceParameters(),
	}
	created, err := createWorkspace(context.Background(), admin, createBody)
	if err != nil {
		t.Fatalf("CreateWorkspace error: %v", err)
	}
	if created.Name != "demo-workspace" {
		t.Fatalf("CreateWorkspace = %#v", created)
	}

	items, err := listWorkspaces(context.Background(), admin)
	if err != nil {
		t.Fatalf("ListWorkspaces error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListWorkspaces len = %d", len(items))
	}

	got, err := getWorkspace(context.Background(), admin, "demo-workspace")
	if err != nil {
		t.Fatalf("GetWorkspace error: %v", err)
	}
	if got.WorkflowName != "demo-workflow" {
		t.Fatalf("GetWorkspace workflow = %q", got.WorkflowName)
	}

	updated, err := putWorkspace(context.Background(), admin, "demo-workspace", adminhttp.WorkspaceUpsert{
		Name:         "demo-workspace",
		WorkflowName: "demo-workflow",
		Parameters:   testFlowcraftWorkspaceParameters(),
	})
	if err != nil {
		t.Fatalf("PutWorkspace error: %v", err)
	}
	params, err := updated.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil || params.GenerateModel == nil || *params.GenerateModel != "updated" {
		t.Fatalf("PutWorkspace parameters = %#v", updated.Parameters)
	}

	if _, err := deleteWorkspace(context.Background(), admin, "demo-workspace"); err != nil {
		t.Fatalf("DeleteWorkspace error: %v", err)
	}
	if _, err := getWorkspace(context.Background(), admin, "demo-workspace"); err == nil {
		t.Fatal("GetWorkspace after delete expected error")
	}
}

func TestIntegrationAdminServiceCredentialLifecycle(t *testing.T) {
	ts := startTestServer(t)

	admin := newTestClient(t, ts)
	ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: strPtr("admin")})

	createBody := mustCredentialUpsert(t, `{
		"name": "openai-primary",
		"provider": "openai",
		"description": "primary openai credential",
		"body": {"api_key": "sk-test"}
	}`)
	created, err := createCredential(context.Background(), admin, createBody)
	if err != nil {
		t.Fatalf("CreateCredential error: %v", err)
	}
	if created.Name != "openai-primary" {
		t.Fatalf("CreateCredential = %#v", created)
	}
	if testCredentialBodyString(created.Body, "api_key") != "sk-test" {
		t.Fatalf("CreateCredential body = %#v", created.Body)
	}

	items, err := listCredentials(context.Background(), admin, nil)
	if err != nil {
		t.Fatalf("ListCredentials error: %v", err)
	}
	if len(items) != 1 || items[0].Provider != "openai" {
		t.Fatalf("ListCredentials = %#v", items)
	}

	got, err := getCredential(context.Background(), admin, "openai-primary")
	if err != nil {
		t.Fatalf("GetCredential error: %v", err)
	}
	if got.Description == nil || *got.Description != "primary openai credential" {
		t.Fatalf("GetCredential description = %#v", got.Description)
	}
	if testCredentialBodyString(got.Body, "api_key") != "sk-test" {
		t.Fatalf("GetCredential body = %#v", got.Body)
	}

	updateBody := mustCredentialUpsert(t, `{
			"name": "openai-primary",
			"provider": "volc",
			"description": "volc credential",
			"body": {"ark_api_key": "volc-api-key"}
	}`)
	updated, err := putCredential(context.Background(), admin, "openai-primary", updateBody)
	if err != nil {
		t.Fatalf("PutCredential error: %v", err)
	}
	if updated.Provider != "volc" {
		t.Fatalf("PutCredential = %#v", updated)
	}
	if testCredentialBodyString(updated.Body, "ark_api_key") != "volc-api-key" {
		t.Fatalf("PutCredential body = %#v", updated.Body)
	}

	provider := string("volc")
	filtered, err := listCredentials(context.Background(), admin, &provider)
	if err != nil {
		t.Fatalf("ListCredentials(provider) error: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Name != "openai-primary" {
		t.Fatalf("ListCredentials(provider) = %#v", filtered)
	}
	if testCredentialBodyString(filtered[0].Body, "ark_api_key") != "volc-api-key" {
		t.Fatalf("ListCredentials(provider) body = %#v", filtered[0].Body)
	}

	if _, err := deleteCredential(context.Background(), admin, "openai-primary"); err != nil {
		t.Fatalf("DeleteCredential error: %v", err)
	}
	if _, err := getCredential(context.Background(), admin, "openai-primary"); err == nil {
		t.Fatal("GetCredential after delete expected error")
	}
}

func mustWorkflow(t *testing.T, raw string) apitypes.Workflow {
	t.Helper()

	var doc apitypes.Workflow
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return doc
}

func mustCredentialUpsert(t *testing.T, raw string) adminhttp.CredentialUpsert {
	t.Helper()

	var upsert adminhttp.CredentialUpsert
	if err := json.Unmarshal([]byte(raw), &upsert); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return upsert
}

func mustOpenAIProviderData(t *testing.T, upstreamModel string) apitypes.ModelProviderData {
	t.Helper()
	falseValue := false
	var data apitypes.ModelProviderData
	if err := data.FromOpenAITenantModelProviderData(apitypes.OpenAITenantModelProviderData{
		UpstreamModel:      &upstreamModel,
		SupportJsonOutput:  &falseValue,
		SupportToolCalls:   &falseValue,
		SupportTextOnly:    &falseValue,
		UseSystemRole:      &falseValue,
		SupportTemperature: &falseValue,
		SupportThinking:    &falseValue,
	}); err != nil {
		t.Fatalf("FromOpenAITenantModelProviderData() error = %v", err)
	}
	return data
}
