//go:build gizclaw_e2e

// User story: As a Play UI user, I can open the OpenAI gateway shell and see
// model resources plus the chat tester.
package playui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"testing"
)

func playShellStories() []Story {
	return []Story{{
		Name: "200-play-shell",
		Run: func(_ testing.TB, page *Page) {
			page.GotoPlay("/")
			page.ExpectText("OpenAI Gateway")
			page.ExpectText("GizClaw runtime")
			page.ExpectText("Models")
			page.ExpectText("Credentials")
			page.ExpectText("Voices")
			page.ExpectText("OpenAI")
			page.ClickRoleLike("button", "Models")
			page.ExpectText(SeedModelID)
			page.ClickRoleLike("button", "Credentials")
			page.ExpectText("openai-lab-credential")
		},
	}}
}
