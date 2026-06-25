//go:build gizclaw_e2e

// User story: As an admin operator, I can list registered peers and filter
// the current inventory page using real peer registrations.
package adminui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"testing"
)

func adminPeersListStories() []Story {
	return []Story{{
		Name: "110-admin-peers-list",
		Run: func(t testing.TB, page *Page) {
			if len(page.Seed.DevicePublicKey) < 12 {
				t.Skipf("admin peer seed public key is unavailable")
			}
			page.GotoAdmin("/peers")
			page.ExpectText("Peers")
			page.ExpectText("Peer Inventory")
			page.ExpectText(page.Seed.DevicePublicKey)
			page.ExpectText("peer")
			page.ExpectText("Active")

			page.Fill(`input[placeholder="Filter current page by key, role, or status"]`, "missing")
			page.ExpectText("No matching peers")
			page.Fill(`input[placeholder="Filter current page by key, role, or status"]`, page.Seed.DevicePublicKey[:12])
			page.ExpectText(page.Seed.DevicePublicKey)
		},
	}}
}
