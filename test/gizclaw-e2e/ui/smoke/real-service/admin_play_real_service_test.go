//go:build gizclaw_e2e

// User story: As a maintainer, I can run one smoke path that proves Admin and
// Play both serve against the same shared test service.
package smoke_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"testing"
)

func realServiceSmokeStories() []Story {
	return []Story{{
		Name: "900-real-service-smoke",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/")
			page.ExpectText("Dashboard")

			page.GotoPlay("/")
			page.ExpectText("OpenAI Gateway")
			page.ClickRoleLike("button", "Models")
			page.ExpectText(SeedModelID)
			page.ClickRoleLike("button", "Voices")
			page.ExpectText(SeedVoiceID)
			page.ExpectText("OpenAI")
		},
	}}
}
