//go:build gizclaw_e2e

// User story: As an admin operator, I can review ACL roles, bindings, and
// content views from the Settings area.
package adminui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"net/url"
	"testing"
)

func adminACLStories() []Story {
	return []Story{
		{
			Name: "160-admin-acl-views-list-and-detail-cli",
			Run: func(_ testing.TB, page *Page) {
				page.GotoAdmin("/settings/acl")
				page.ExpectText("Access Control")
				page.ClickRole("tab", "Views")
				page.ExpectText(SeedACLViewName)
				page.ExpectText("Seeded child-safe content view")

				page.GotoAdmin("/settings/acl/views/" + url.PathEscape(SeedACLViewName))
				page.ExpectText("Named content view")
				page.ClickRole("tab", "CLI")
				page.ExpectText("ACLView Resource Spec")
				page.ExpectText("gizclaw admin acl --context <admin-cli-context> views get '" + SeedACLViewName + "'")
			},
		},
	}
}
