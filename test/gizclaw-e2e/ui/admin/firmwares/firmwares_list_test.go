//go:build gizclaw_e2e

// User story: As an admin operator, I can inspect firmware release lines and
// trigger release actions from the firmware detail page.
package adminui_test

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
)

func adminFirmwaresListStories() []Story {
	return []Story{{
		Name: "120-admin-firmwares-list-detail-and-release",
		Run: func(_ testing.TB, page *Page) {
			createdName := fmt.Sprintf("ui-created-firmware-%d", time.Now().UnixNano())
			page.GotoAdmin("/firmwares")
			page.ExpectText("Firmwares")
			page.ExpectText("devkit-firmware-000")
			page.ClickRole("link", "Create")
			page.ExpectText("Create Firmware")
			page.Fill(`input:not([disabled])`, createdName)
			page.Fill(`textarea`, "Created from UI test")
			page.ClickRole("button", "Edit stable slot")
			page.FillNth(`div[role="dialog"] input`, 0, "2.0.0")
			page.ClickRole("button", "Apply Slot")
			page.ClickRole("button", "Create")
			page.ExpectURLSuffix("/firmwares/" + createdName)
			page.ExpectText("Created from UI test")

			page.GotoAdmin("/firmwares/" + url.PathEscape("devkit-firmware-main"))
			page.ExpectText("Release State")
			page.ExpectText("devkit-firmware-main/stable/artifact/artifact.tar")
			page.ClickRole("button", "Files")
			page.ExpectText("firmware/main.bin")
			page.ExpectText("assets/icons/status.png")
			page.ClickNthRole("button", "Info", 0)
			page.ExpectText("Artifact entry")
			page.ExpectText("Entry Stats")
			page.ClickNthRole("button", "Close", 0)

			page.GotoAdmin("/firmwares/" + url.PathEscape(SeedFirmwareName))
			page.ClickRole("tab", "Edit")
			page.ExpectText("Firmware Info")
			page.ClickRole("tab", "Summary")
			page.ClickRole("tab", "CLI")
			page.ExpectText("Firmware Resource Spec")
			page.ExpectText("gizclaw admin firmwares --context <admin-cli-context> get '" + SeedFirmwareName + "'")
			page.ExpectText("gizclaw admin firmwares --context <admin-cli-context> upload-artifact '" + SeedFirmwareName + "' --channel stable -f artifact.tar")
		},
	}}
}
