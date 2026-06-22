// User story: As an admin operator, I can browse seeded workflows and
// inspect the workflow driver.
package ui_test

import (
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
