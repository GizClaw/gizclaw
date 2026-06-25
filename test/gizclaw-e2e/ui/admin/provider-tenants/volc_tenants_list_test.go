//go:build gizclaw_e2e

// User story: As an admin operator, I can inspect shared Volcengine tenants
// and verify their CLI/resource metadata.
package adminui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"net/url"
	"testing"
)

func adminVolcTenantsListStories() []Story {
	return []Story{{
		Name: "132-admin-volc-tenant-detail-cli",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/providers/volc-tenants/" + url.PathEscape(SeedVolcTenantName))
			page.ExpectText(SeedVolcTenantName)
			page.ClickRole("tab", "CLI")
			page.ExpectText("VolcTenant Resource Spec")
			page.ExpectText(`"kind": "VolcTenant"`)
			page.ExpectText("gizclaw admin volc-tenants --context <admin-cli-context> get '" + SeedVolcTenantName + "'")
			page.ExpectText("gizclaw admin volc-tenants --context <admin-cli-context> sync-voices '" + SeedVolcTenantName + "'")
		},
	}}
}
