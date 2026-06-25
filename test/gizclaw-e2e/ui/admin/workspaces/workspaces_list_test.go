//go:build gizclaw_e2e

// User story: As an admin operator, I can browse shared workspaces and verify
// their associated workflow.
package adminui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"testing"
)

func adminWorkspacesListStories() []Story {
	return []Story{{
		Name: "142-admin-workspaces-list",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/ai/workspaces")
			page.ExpectText("Workspaces")
			page.ExpectText(SeedWorkspaceName)
			page.ExpectText(SeedWorkflowName)
			page.ExpectText("Refresh")
		},
	}}
}
