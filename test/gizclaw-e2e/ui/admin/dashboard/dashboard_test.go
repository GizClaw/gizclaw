//go:build gizclaw_e2e

// User story: As an admin operator, I can open the dashboard and see the
// high-level server and device overview backed by real shared data.
package adminui_test

import (
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"testing"
)

func adminDashboardStories() []Story {
	return []Story{{
		Name: "100-admin-dashboard",
		Run: func(_ testing.TB, page *Page) {
			page.GotoAdmin("/")
			page.ExpectText("Dashboard")
			page.ExpectText("Server Build")
			page.ExpectText("Peers This Page")
			page.ExpectText("Peers")
		},
	}}
}
