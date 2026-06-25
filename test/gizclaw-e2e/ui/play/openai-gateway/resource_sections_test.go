//go:build gizclaw_e2e

// User story: As a Play UI user, I can switch between OpenAI gateway resource
// views.
package playui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"testing"
)

func playActionsStories() []Story {
	return []Story{{
		Name: "201-play-actions",
		Run: func(_ testing.TB, page *Page) {
			page.GotoPlay("/")
			page.ClickRoleLike("button", "Models")
			page.ExpectText(SeedModelID)
			page.ClickRoleLike("button", "Voices")
			page.ExpectText(SeedVoiceID)
			page.ExpectText("MiniMax Cloned Narrator")
			page.ClickRoleLike("button", "Pets")
			page.ExpectText("Seeded Pet")
			page.ClickRoleLike("button", "Transactions")
			page.ExpectText("pet_adopt")
			page.ClickRoleLike("button", "Rewards")
			page.ExpectText("Seeded reward")
			page.ClickRoleLike("button", "Models")
			page.ExpectText(SeedModelID)
		},
	}}
}
