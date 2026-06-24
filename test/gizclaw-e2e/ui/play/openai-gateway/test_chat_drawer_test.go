//go:build gizclaw_e2e

// User story: As a Play UI user, I can refresh OpenAI gateway resources and
// open the example-style chat tester sheet.
package playui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"testing"
)

func playAllActionsStories() []Story {
	return []Story{{
		Name: "202-play-all-actions",
		Run: func(_ testing.TB, page *Page) {
			page.GotoPlay("/")
			page.ClickRoleLike("button", "Models")
			page.ClickRole("button", "Refresh")
			page.ExpectText(SeedModelID)
			page.ClickRole("button", "OpenAI")
			page.ExpectText("Ready to test")
			page.ExpectText("/v1/chat/completions")
		},
	}}
}
