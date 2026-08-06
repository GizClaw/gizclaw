//go:build gizclaw_e2e

package admin_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminAIProviderCatalogUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "510-admin-ai-provider-resources")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	openAIList := h.RunCLI("admin", "openai-tenants", "list", "--context", "admin-a")
	openAIList.MustSucceed(t)
	openAITenantID := adminResourceID(t, openAIList.Stdout, "fake-openai")
	credentials := h.RunCLI("admin", "credentials", "list", "--context", "admin-a")
	credentials.MustSucceed(t)
	openAICredentialID := adminResourceID(t, credentials.Stdout, "fake-openai-credential-000")
	assertOutputContains(t, openAIList.Stdout, `"id":"fake-openai"`, `"credential_id":"`+openAICredentialID+`"`)

	openAIGet := h.RunCLI("admin", "openai-tenants", "get", openAITenantID, "--context", "admin-a")
	openAIGet.MustSucceed(t)
	assertOutputContains(t, openAIGet.Stdout, `"kind":"compatible"`, `"api_mode":"chat_completions"`)

	geminiList := h.RunCLI("admin", "gemini-tenants", "list", "--context", "admin-a")
	geminiList.MustSucceed(t)
	if strings.Contains(geminiList.Stdout, `"id":"gemini-main"`) {
		geminiTenantID := adminResourceID(t, geminiList.Stdout, "gemini-main")
		geminiCredentialID := adminResourceID(t, credentials.Stdout, "gemini-main-credential")
		geminiGet := h.RunCLI("admin", "gemini-tenants", "get", geminiTenantID, "--context", "admin-a")
		geminiGet.MustSucceed(t)
		assertOutputContains(t, geminiGet.Stdout, `"credential_id":"`+geminiCredentialID+`"`, `"location":"global"`)
	}

	dashScopeList := h.RunCLI("admin", "dashscope-tenants", "list", "--context", "admin-a")
	dashScopeList.MustSucceed(t)
	if strings.Contains(dashScopeList.Stdout, `"id":"qwen-dashscope-main"`) {
		dashScopeTenantID := adminResourceID(t, dashScopeList.Stdout, "qwen-dashscope-main")
		dashScopeCredentialID := adminResourceID(t, credentials.Stdout, "qwen-dashscope-credential")
		assertOutputContains(t, dashScopeList.Stdout, `"credential_id":"`+dashScopeCredentialID+`"`)
		dashScopeGet := h.RunCLI("admin", "dashscope-tenants", "get", dashScopeTenantID, "--context", "admin-a")
		dashScopeGet.MustSucceed(t)
		assertOutputContains(t, dashScopeGet.Stdout, `"id":"qwen-dashscope-main"`, `"base_url":"`)
	}

	modelsList := h.RunCLI("admin", "models", "list", "--provider-kind", "openai-tenant", "--provider-id", openAITenantID, "--context", "admin-a")
	modelsList.MustSucceed(t)
	assertOutputContains(t, modelsList.Stdout, `"id":"fake-openai-chat-000"`, `"upstream_model":"fake-gpt-000"`)
	modelID := adminResourceID(t, modelsList.Stdout, "fake-openai-chat-000")

	rpcModelsList := h.RunCLI("admin", "models", "list", "--provider-kind", "openai-tenant", "--provider-id", openAITenantID, "--context", "admin-a")
	rpcModelsList.MustSucceed(t)
	assertOutputContains(t, rpcModelsList.Stdout, `"id":"fake-openai-chat-000"`, `"id":"fake-openai-chat-079"`)

	modelGet := h.RunCLI("admin", "models", "get", modelID, "--context", "admin-a")
	modelGet.MustSucceed(t)
	assertOutputContains(t, modelGet.Stdout, `"kind":"llm"`, `"id":"fake-openai-chat-000"`)

	rpcModelGet := h.RunCLI("admin", "models", "get", modelID, "--context", "admin-a")
	rpcModelGet.MustSucceed(t)
	assertOutputContains(t, rpcModelGet.Stdout, `"upstream_model":"fake-gpt-000"`)

	profilesList := h.RunCLI("admin", "runtime-profiles", "list", "--context", "admin-a")
	profilesList.MustSucceed(t)
	assertOutputContains(t, profilesList.Stdout, `"id":"default-gameplay"`)
	profileID := adminResourceID(t, profilesList.Stdout, "default-gameplay")

	profileGet := h.RunCLI("admin", "runtime-profiles", "get", profileID, "--context", "admin-a")
	profileGet.MustSucceed(t)
	assertOutputContains(t, profileGet.Stdout, `"pet_defs":{"starter-pet":{`, `"resource_id":`)
}

func assertOutputContains(t *testing.T, output string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(output, value) {
			t.Fatalf("output missing %s:\n%s", value, output)
		}
	}
}
