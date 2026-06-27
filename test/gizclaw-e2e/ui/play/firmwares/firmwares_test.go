//go:build gizclaw_e2e

package playui_test

import (
	"testing"

	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
)

func TestFirmwaresSection(t *testing.T) {
	RunPlayStories(t, []Story{{
		Name: "play-firmwares-list-and-detail",
		Run: func(_ testing.TB, page *Page) {
			page.GotoPlay("/")
			page.ClickRoleLike("button", "Firmwares")
			page.ExpectText("Firmwares")
			page.ExpectText("devkit-firmware-000")
			page.ClickRole("button", "Next")
			page.ExpectText("devkit-firmware-main")
			page.ExpectText("Artifact")
			if err := page.Raw().Locator(`tr:has-text("devkit-firmware-main") button:has-text("Open")`).Click(); err != nil {
				t.Fatalf("open devkit-firmware-main: %v", err)
			}
			page.ExpectText("Artifact Summary")
			page.ExpectText("Channels")
			page.ExpectText("devkit-firmware-main/stable/artifact/artifact.tar")
		},
	}})
}
