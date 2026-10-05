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
	ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: new("admin")})

	createDoc := mustWorkflow(t, `{"id":"demo-assistant","spec":{"driver":"eino","eino":{"graph":{"name":"Assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"updated","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}}}`)
	created, err := createWorkflow(context.Background(), admin, createDoc)
	if err != nil {
		t.Fatalf("CreateWorkflow error: %v", err)
	}
	if created.Spec.Driver != apitypes.WorkflowDriverEino {
		t.Fatalf("CreateWorkflow driver = %q", created.Spec.Driver)
	}

	items, err := listWorkflows(context.Background(), admin)
	if err != nil {
		t.Fatalf("ListWorkflows error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListWorkflows len = %d", len(items))
	}

	got, err := getWorkflow(context.Background(), admin, created.Id)
	if err != nil {
		t.Fatalf("GetWorkflow error: %v", err)
	}
	if got.Id != "demo-assistant" {
		t.Fatalf("GetWorkflow id = %q", got.Id)
	}

	updateDoc := mustWorkflow(t, `{"id":"demo-assistant","spec":{"driver":"eino","eino":{"graph":{"name":"Updated Assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"updated","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}}}`)
	updated, err := putWorkflow(context.Background(), admin, created.Id, updateDoc)
	if err != nil {
		t.Fatalf("PutWorkflow error: %v", err)
	}
	if updated.Spec.Eino == nil || updated.Spec.Eino.Graph.Name != "Updated Assistant" {
		t.Fatalf("PutWorkflow spec = %#v", updated.Spec)
	}

	if _, err := deleteWorkflow(context.Background(), admin, created.Id); err != nil {
		t.Fatalf("DeleteWorkflow error: %v", err)
	}
	if _, err := getWorkflow(context.Background(), admin, created.Id); err == nil {
		t.Fatal("GetWorkflow after delete expected error")
	}
}

func TestIntegrationAdminServiceRejectsLegacyWorkflowDescription(t *testing.T) {
	ts := startTestServer(t)

	admin := newTestClient(t, ts)
	ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: new("admin")})
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("ServerAdminClient() error = %v", err)
	}
	resp, err := api.CreateWorkflowWithBodyWithResponse(
		context.Background(),
		"application/json",
		strings.NewReader(`{"metadata":{"name":"legacy","description":"old"},"spec":{"driver":"eino"}}`),
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
	ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: new("admin")})

	workflowDoc := mustWorkflow(t, `{"id":"demo-workflow","spec":{"driver":"eino","eino":{"graph":{"name":"Assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"updated","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}}}`)
	workflow, err := createWorkflow(context.Background(), admin, workflowDoc)
	if err != nil {
		t.Fatalf("CreateWorkflow error: %v", err)
	}
	if _, err := createModel(context.Background(), admin, adminhttp.ModelUpsert{
		Id:     "updated",
		Kind:   apitypes.ModelKindLlm,
		Source: apitypes.ModelSourceManual,
		Provider: apitypes.ModelProvider{
			Kind: "openai-tenant",
			Id:   "global",
		},
		ProviderData: mustOpenAIProviderData(t, "updated-upstream"),
	}); err != nil {
		t.Fatalf("CreateModel error: %v", err)
	}

	createBody := adminhttp.WorkspaceUpsert{
		Id:         "demo-workspace-id",
		Name:       "demo-workspace",
		WorkflowId: workflow.Id,
		Parameters: testEinoWorkspaceParameters(),
	}
	created, err := createWorkspace(context.Background(), ts, admin, createBody)
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

	got, err := getWorkspace(context.Background(), admin, created.Id)
	if err != nil {
		t.Fatalf("GetWorkspace error: %v", err)
	}
	if got.WorkflowId != workflow.Id {
		t.Fatalf("GetWorkspace workflow = %q", got.WorkflowId)
	}

	updated, err := putWorkspace(context.Background(), admin, created.Id, adminhttp.WorkspaceUpsert{
		Id:         created.Id,
		Name:       "demo-workspace",
		WorkflowId: workflow.Id,
		Parameters: testEinoWorkspaceParameters(),
	})
	if err != nil {
		t.Fatalf("PutWorkspace error: %v", err)
	}
	params, err := updated.Parameters.AsEinoWorkspaceParameters()
	if err != nil || params.AgentType != apitypes.EinoWorkspaceParametersAgentTypeEino {
		t.Fatalf("PutWorkspace parameters = %#v", updated.Parameters)
	}

	if _, err := deleteWorkspace(context.Background(), admin, created.Id); err != nil {
		t.Fatalf("DeleteWorkspace error: %v", err)
	}
	if _, err := getWorkspace(context.Background(), admin, created.Id); err != nil {
		t.Fatalf("GetWorkspace after delete: %v", err)
	}
}

func TestIntegrationAdminServiceCredentialLifecycle(t *testing.T) {
	ts := startTestServer(t)

	admin := newTestClient(t, ts)
	ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: new("admin")})

	createBody := mustCredentialUpsert(t, `{
		"id": "openai-primary",
		"provider": "openai",
		"description": "primary openai credential",
		"body": {"api_key": "sk-test"}
	}`)
	created, err := createCredential(context.Background(), admin, createBody)
	if err != nil {
		t.Fatalf("CreateCredential error: %v", err)
	}
	if created.Id != "openai-primary" {
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

	got, err := getCredential(context.Background(), admin, created.Id)
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
			"id": "openai-primary",
			"provider": "volc",
			"description": "volc credential",
			"body": {"ark_api_key": "volc-api-key"}
	}`)
	updated, err := putCredential(context.Background(), admin, created.Id, updateBody)
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
	if len(filtered) != 1 || filtered[0].Id != "openai-primary" {
		t.Fatalf("ListCredentials(provider) = %#v", filtered)
	}
	if testCredentialBodyString(filtered[0].Body, "ark_api_key") != "volc-api-key" {
		t.Fatalf("ListCredentials(provider) body = %#v", filtered[0].Body)
	}

	if _, err := deleteCredential(context.Background(), admin, created.Id); err != nil {
		t.Fatalf("DeleteCredential error: %v", err)
	}
	if _, err := getCredential(context.Background(), admin, created.Id); err == nil {
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
		UpstreamModel:      upstreamModel,
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
