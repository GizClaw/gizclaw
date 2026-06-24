//go:build gizclaw_e2e

// User story: As a Play UI user, I can use the social pages and chat drawer
// built around invite tokens and workspace history.
package playui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"testing"
)

func playSocialStories() []Story {
	return []Story{{
		Name: "205-play-social",
		Run: func(_ testing.TB, page *Page) {
			page.GotoPlay("/")

			page.ClickRoleLike("button", "Friends")
			page.ExpectText("Invite Token")
			page.ExpectText("Add Friend")
			page.ExpectNoText("Request")

			page.ClickRoleLike("tab", "Invite Token")
			page.ExpectText("Invite token")
			page.ClickRoleLike("tab", "Add Friend")
			page.ExpectText("Add Friend")

			page.ClickRoleLike("button", "Groups")
			page.ExpectText("Create Group")
			page.ExpectText("Join Group")
			page.ExpectNoText("Request")

			page.ClickRole("button", "Chat")
			page.ExpectText("Append voice messages and replay history through the selected social workspace.")
		},
	}}
}

func TestPlaySocialStories(t *testing.T) {
	RunPlayStories(t, playSocialStories())
}
