//go:build gizclaw_e2e

// User story: As an admin operator, I can browse provider tenants that back
// AI model and voice runtime routing.
package adminui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"net/url"
	"testing"
)

func adminProviderTenantsListStories() []Story {
	return []Story{{
		Name: "133-admin-openai-tenants-list-and-detail",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/providers/openai-tenants")
			page.ExpectText("OpenAI Tenants")
			page.ExpectText(SeedOpenAITenantName)
			page.ExpectText("openai-lab-credential")

			page.GotoAdmin("/providers/openai-tenants/" + url.PathEscape(SeedOpenAITenantName))
			page.ExpectText("OpenAI-compatible endpoint configuration")
			page.ClickRole("tab", "CLI")
			page.ExpectText("OpenAITenant Resource Spec")
			page.ExpectText("gizclaw admin openai-tenants --context <admin-cli-context> get '" + SeedOpenAITenantName + "'")
		},
	}, {
		Name: "133-admin-gemini-tenants-list-and-detail",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/providers/gemini-tenants")
			page.ExpectText("Gemini Tenants")
			page.ExpectText(SeedGeminiTenantName)
			page.ExpectText("gemini-main-credential")

			page.GotoAdmin("/providers/gemini-tenants/" + url.PathEscape(SeedGeminiTenantName))
			page.ExpectText("Gemini project and credential binding")
			page.ExpectText("gemini-main-project")
			page.ClickRole("tab", "CLI")
			page.ExpectText("GeminiTenant Resource Spec")
			page.ExpectText("gizclaw admin gemini-tenants --context <admin-cli-context> get '" + SeedGeminiTenantName + "'")
		},
	}, {
		Name: "133-admin-dashscope-tenants-list-and-detail",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/providers/dashscope-tenants")
			page.ExpectText("DashScope Tenants")
			page.ExpectText(SeedDashScopeTenantName)
			page.ExpectText("dashscope-main-credential")

			page.GotoAdmin("/providers/dashscope-tenants/" + url.PathEscape(SeedDashScopeTenantName))
			page.ExpectText("DashScope endpoint and credential binding")
			page.ClickRole("tab", "CLI")
			page.ExpectText("DashScopeTenant Resource Spec")
			page.ExpectText("gizclaw admin dashscope-tenants --context <admin-cli-context> get '" + SeedDashScopeTenantName + "'")
		},
	}}
}
