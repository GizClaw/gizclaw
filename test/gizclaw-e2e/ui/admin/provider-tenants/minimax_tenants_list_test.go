//go:build gizclaw_e2e

// User story: As an admin operator, I can browse shared MiniMax tenants and
// verify their credential and provider metadata.
package adminui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"net/url"
	"testing"
)

func adminMiniMaxTenantsListStories() []Story {
	return []Story{{
		Name: "131-admin-minimax-tenants-list",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/providers/minimax-tenants")
			page.ExpectText("MiniMax Tenants")
			page.ExpectText(SeedMiniMaxTenantName)
			page.ExpectText("minimax-main-app")
			page.ExpectText("minimax-main-group")
			page.ExpectText(SeedCredentialName)
			page.ExpectText("https://example.invalid")
		},
	}, {
		Name: "131-admin-minimax-tenant-detail-cli",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/providers/minimax-tenants/" + url.PathEscape(SeedMiniMaxTenantName))
			page.ExpectText(SeedMiniMaxTenantName)
			page.ClickRole("tab", "CLI")
			page.ExpectText("MiniMaxTenant Resource Spec")
			page.ExpectText(`"kind": "MiniMaxTenant"`)
			page.ExpectText("gizclaw admin minimax-tenants --context <admin-cli-context> get '" + SeedMiniMaxTenantName + "'")
			page.ExpectText("gizclaw admin minimax-tenants --context <admin-cli-context> sync-voices '" + SeedMiniMaxTenantName + "'")
		},
	}}
}
