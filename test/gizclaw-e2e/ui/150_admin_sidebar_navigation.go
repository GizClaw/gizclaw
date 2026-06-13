// User story: As an admin operator, I can use the sidebar to move between the
// main Admin UI sections and land on each expected page.
package ui_test

import (
	"testing"
)

func adminSidebarNavigationStories() []Story {
	return []Story{{
		Name: "150-admin-sidebar-navigation",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/overview")
			for _, destination := range []struct {
				label   string
				heading string
				path    string
			}{
				{label: "Overview", heading: "Dashboard", path: "/overview"},
				{label: "Peers", heading: "Peers", path: "/peers"},
				{label: "Firmwares", heading: "Firmwares", path: "/firmwares"},
				{label: "Credentials", heading: "Credentials", path: "/providers/credentials"},
				{label: "OpenAI Tenants", heading: "OpenAI Tenants", path: "/providers/openai-tenants"},
				{label: "Gemini Tenants", heading: "Gemini Tenants", path: "/providers/gemini-tenants"},
				{label: "DashScope Tenants", heading: "DashScope Tenants", path: "/providers/dashscope-tenants"},
				{label: "MiniMax Tenants", heading: "MiniMax Tenants", path: "/providers/minimax-tenants"},
				{label: "Volcengine Tenants", heading: "Volcengine Tenants", path: "/providers/volc-tenants"},
				{label: "Voices", heading: "Voices", path: "/ai/voices"},
				{label: "Models", heading: "Models", path: "/ai/models"},
				{label: "Workflows", heading: "Workflows", path: "/ai/workflows"},
				{label: "Workspaces", heading: "Workspaces", path: "/ai/workspaces"},
				{label: "Contacts", heading: "Access Control", path: "/social/contacts"},
				{label: "Friends", heading: "Access Control", path: "/social/friends"},
				{label: "Friend Requests", heading: "Access Control", path: "/social/friend-requests"},
				{label: "Friend Groups", heading: "Access Control", path: "/social/friend-groups"},
				{label: "Access Control", heading: "Access Control", path: "/settings/acl"},
			} {
				page.ClickNavigationLink(destination.label)
				page.ExpectURLSuffix(destination.path)
				page.ExpectText(destination.heading)
			}
		},
	}}
}
