//go:build gizclaw_e2e

// User story: As an admin operator, I can browse shared workflows and
// inspect the workflow driver.
package adminui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"testing"
)

func adminWorkflowsListStories() []Story {
	return []Story{{
		Name: "141-admin-workflows-list",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/ai/workflows")
			page.ExpectText("Workflows")
			page.ExpectText(SeedWorkflowName)
			page.ExpectText("Resource")
		},
	}}
}
