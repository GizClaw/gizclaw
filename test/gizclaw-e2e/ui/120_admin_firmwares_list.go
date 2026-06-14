// User story: As an admin operator, I can inspect firmware release lines and
// trigger release actions from the firmware detail page.
package ui_test

import (
	"net/url"
	"testing"
)

func adminFirmwaresListStories() []Story {
	return []Story{{
		Name: "120-admin-firmwares-list-detail-and-release",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/firmwares")
			page.ExpectText("Firmwares")
			page.ExpectText(SeedFirmwareName)
			page.ExpectText("1.0.0")
			page.ClickRole("link", "Create")
			page.ExpectText("Create Firmware")
			page.Fill(`input:not([disabled])`, "ui-created-firmware")
			page.Fill(`textarea`, "Created from UI test")
			page.ClickRole("button", "Edit stable slot")
			page.FillNth(`div[role="dialog"] input`, 0, "2.0.0")
			page.ClickRole("button", "Apply Slot")
			page.ClickRole("button", "Create")
			page.ExpectURLSuffix("/firmwares/ui-created-firmware")
			page.ExpectText("Created from UI test")

			page.GotoAdmin("/firmwares/" + url.PathEscape(SeedFirmwareName))
			page.ClickRole("tab", "Edit")
			page.ExpectText("Firmware Info")
			page.ExpectText("1.3.0")
			page.ClickRole("tab", "Summary")
			page.ClickRole("button", "Release")
			page.ExpectText("1.1.0")
			page.ClickRole("tab", "CLI")
			page.ExpectText("Firmware Resource Spec")
			page.ExpectText("gizclaw admin firmwares --context <admin-cli-context> get '" + SeedFirmwareName + "'")
			page.ExpectText("gizclaw admin firmwares --context <admin-cli-context> upload-bin '" + SeedFirmwareName + "' --channel stable --bin app -f app.bin")
		},
	}}
}
