//go:build gizclaw_e2e

// User story: As an admin operator, I can perform peer actions against real
// shared registrations, including refresh, role updates, block, and reset.
package adminui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"net/url"
	"testing"
)

func adminPeerActionsStories() []Story {
	return []Story{
		{
			Name: "112-admin-peer-actions",
			Run: func(t testing.TB, page *Page) {
				if page.Seed.ActionDevicePublicKey == "" {
					t.Skipf("admin peer action seed public key is unavailable")
				}
				page.GotoAdmin("/peers/" + url.PathEscape(page.Seed.ActionDevicePublicKey))
				page.ClickRole("button", "Refresh Peer")
				page.ExpectText("Peer refreshed.")
				page.ClickRole("tab", "Edit")
				page.ClickRole("button", "Save Role")
				page.ExpectText("Peer role saved as client.")
				page.ClickRole("tab", "Edit")
				page.ClickRole("button", "Block")
				page.ExpectText("Peer blocked.")
			},
		},
		{
			Name: "112-admin-peer-delete",
			Run: func(t testing.TB, page *Page) {
				if page.Seed.DeleteDevicePublicKey == "" {
					t.Skipf("admin peer delete seed public key is unavailable")
				}
				page.GotoAdmin("/peers/" + url.PathEscape(page.Seed.DeleteDevicePublicKey))
				page.ClickRole("tab", "Edit")
				page.ClickRole("button", "Reset")
				page.ExpectURLSuffix("/peers")
				page.ExpectText("Peers")
			},
		},
	}
}
