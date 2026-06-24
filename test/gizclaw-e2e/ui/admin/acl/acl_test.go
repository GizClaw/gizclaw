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
		{
			Name: "161-admin-acl-social-resource-controls",
			Run: func(_ testing.TB, page *Page) {
				page.GotoAdmin("/social/friend-groups")
				page.ExpectURLSuffix("/social/friend-groups")
				page.ExpectText("friend_group")

				page.ClickRole("button", "New Binding")
				page.ClickRole("combobox", "Policy binding resource kind")
				page.ExpectText("contact")
				page.ExpectText("friend")
				page.ExpectText("friend_group")
				page.ExpectNoText("call.admin")

				page.GotoAdmin("/settings/acl")
				page.ClickRole("tab", "Roles")
				page.ClickRole("button", "New Role")
				page.ExpectText("contact.admin")
				page.ExpectText("friend.admin")
				page.ExpectText("friend_group.admin")
				page.ExpectNoText("call.admin")
			},
		},
	}
}
