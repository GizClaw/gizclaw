//go:build gizclaw_e2e

// User story: As an admin operator, I can manage social Friend and Friend
// Group resources from top-level Admin UI pages.
package adminui_test

import (
	"fmt"
	"testing"
	"time"

	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
)

const (
	adminUISocialOwnerPublicKey = "6Ww6ANsXDCf91Yp7Tvi65hqpywjMmXqAoZDiq33kfCee"
	adminUISocialPeerPublicKey  = "8rAUkTyxLHDa5o3VajtzWcQdNJq1thrjAGtpwQkEsaEu"
	adminUISocialContactID      = "living-room"
	adminUISocialContactName    = "Living Room Device"
	adminUISocialGroupID        = "family-circle"
	adminUISocialGroupName      = "Family Circle"
	adminUISocialGroupToken     = "family-circle-token"
)

func adminSocialStories() []Story {
	return []Story{
		{
			Name: "160-admin-social-routes",
			Run: func(_ testing.TB, page *Page) {
				page.GotoAdmin("/social/contacts")
				page.ExpectURLSuffix("/social/contacts")
				page.ExpectText("Contacts")
				page.ExpectText("New Contact")

				page.GotoAdmin("/social/friends")
				page.ExpectURLSuffix("/social/friends")
				page.ExpectText("Friends")
				page.ExpectText("New Friend")

				page.GotoAdmin("/social/friend-groups")
				page.ExpectURLSuffix("/social/friend-groups")
				page.ExpectText("Friend Groups")
				page.ExpectText("New Friend Group")
			},
		},
		{
			Name: "161-admin-social-contacts-list-and-detail",
			Run: func(t testing.TB, page *Page) {
				page.GotoAdmin("/social/contacts")
				page.ExpectText(adminUISocialContactName)
				clickContactRow(t, page, adminUISocialOwnerPublicKey, adminUISocialContactID)
				page.ExpectText(adminUISocialOwnerPublicKey)
				page.ExpectText(adminUISocialContactID)
				page.ExpectText("Contact Row")
				page.ExpectText("Edit Contact")
			},
		},
		{
			Name: "162-admin-social-friends-list-and-detail",
			Run: func(t testing.TB, page *Page) {
				page.GotoAdmin("/social/friends")
				page.ExpectText(adminUISocialPeerPublicKey)
				clickFriendRow(t, page, adminUISocialOwnerPublicKey, adminUISocialPeerPublicKey)
				page.ExpectText(adminUISocialOwnerPublicKey)
				page.ExpectText(adminUISocialPeerPublicKey)
				page.ExpectText("Friend Row")
				page.ExpectText("Workspace History")
			},
		},
		{
			Name: "163-admin-social-friend-groups-detail",
			Run: func(t testing.TB, page *Page) {
				page.GotoAdmin("/social/friend-groups")
				page.ExpectText(adminUISocialGroupName)
				clickFriendGroupRow(t, page, adminUISocialGroupID)
				page.ExpectText("Info")
				page.ExpectText("Members")
				page.ExpectText("Invite Token")
				page.ExpectText("History")

				page.ClickRole("tab", "Members")
				page.ExpectText(adminUISocialOwnerPublicKey)
				page.ExpectText(adminUISocialPeerPublicKey)

				page.ClickRole("tab", "Invite Token")
				expectInputValue(t, page, `input[placeholder="Invite token"]`, adminUISocialGroupToken)

				page.ClickRole("tab", "History")
				page.ExpectText("Workspace History")
			},
		},
	}
}

func clickContactRow(t testing.TB, page *Page, ownerPublicKey string, contactID string) {
	t.Helper()
	if err := page.Raw().Locator(fmt.Sprintf(`table tbody tr:has-text(%q):has-text(%q)`, ownerPublicKey, contactID)).First().Click(); err != nil {
		t.Fatalf("click contact row %q/%q: %v", ownerPublicKey, contactID, err)
	}
}

func clickFriendRow(t testing.TB, page *Page, ownerPublicKey string, peerPublicKey string) {
	t.Helper()
	if err := page.Raw().Locator(fmt.Sprintf(`table tbody tr:has-text(%q):has-text(%q)`, ownerPublicKey, peerPublicKey)).First().Click(); err != nil {
		t.Fatalf("click friend row %q <-> %q: %v", ownerPublicKey, peerPublicKey, err)
	}
}

func clickFriendGroupRow(t testing.TB, page *Page, groupName string) {
	t.Helper()
	if err := page.Raw().Locator(fmt.Sprintf(`table tbody tr:has-text(%q)`, groupName)).First().Click(); err != nil {
		t.Fatalf("click friend group row %q: %v", groupName, err)
	}
}

func expectInputValue(t testing.TB, page *Page, selector, want string) {
	t.Helper()
	if err := WaitUntil(10*time.Second, func() error {
		got, err := page.Raw().Locator(selector).InputValue()
		if err != nil {
			return err
		}
		if got == want {
			return nil
		}
		return fmt.Errorf("%s value = %q, want %q", selector, got, want)
	}); err != nil {
		t.Fatal(err)
	}
}
