package socialutil

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestJSONPagingAndDeletePrefix(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	owner := " peer/a "
	firstID := "id/a"
	secondID := "id b"

	first := rpcapi.ContactObject{Id: strPtr(firstID), DisplayName: strPtr("first")}
	second := rpcapi.ContactObject{Id: strPtr(secondID), DisplayName: strPtr("second")}
	if err := WriteJSON(ctx, store, ContactKey(owner, firstID), first); err != nil {
		t.Fatalf("WriteJSON first: %v", err)
	}
	if err := WriteJSON(ctx, store, ContactKey(owner, secondID), second); err != nil {
		t.Fatalf("WriteJSON second: %v", err)
	}
	got, err := ReadJSONValue[rpcapi.ContactObject](ctx, store, ContactKey(owner, firstID))
	if err != nil {
		t.Fatalf("ReadJSONValue: %v", err)
	}
	if StringValue(got.Id) != firstID || StringValue(got.DisplayName) != "first" {
		t.Fatalf("ReadJSONValue = %#v, want first contact", got)
	}

	page, err := ListPage(ctx, store, OwnerPrefix(ContactsRoot, owner), "", 1)
	if err != nil {
		t.Fatalf("ListPage first: %v", err)
	}
	if len(page.Items) != 1 || !page.HasNext || page.NextCursor == nil || *page.NextCursor != firstID {
		t.Fatalf("ListPage first = %#v, want first item and cursor %q", page, firstID)
	}
	page, err = ListPage(ctx, store, OwnerPrefix(ContactsRoot, owner), *page.NextCursor, 1)
	if err != nil {
		t.Fatalf("ListPage second: %v", err)
	}
	if len(page.Items) != 1 || page.HasNext || page.NextCursor != nil {
		t.Fatalf("ListPage second = %#v, want final item", page)
	}

	if err := DeletePrefix(ctx, store, OwnerPrefix(ContactsRoot, owner)); err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if _, err := store.Get(ctx, ContactKey(owner, firstID)); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Get after DeletePrefix error = %v, want kv.ErrNotFound", err)
	}
}

func TestItemPagingAndVisibility(t *testing.T) {
	items := []rpcapi.ContactObject{
		{Id: strPtr("a"), DisplayName: strPtr("first")},
		{Id: strPtr("b"), DisplayName: strPtr("second")},
		{Id: strPtr("c"), DisplayName: strPtr("third")},
	}
	page := PageItems(items, "a", 1, func(item rpcapi.ContactObject) string {
		return StringValue(item.Id)
	})
	if len(page.Items) != 1 || StringValue(page.Items[0].Id) != "b" || !page.HasNext || page.NextCursor == nil || *page.NextCursor != "b" {
		t.Fatalf("PageItems after cursor = %#v, want item b with next cursor", page)
	}
	page = PageItems(items, "missing", 2, func(item rpcapi.ContactObject) string {
		return StringValue(item.Id)
	})
	if len(page.Items) != 2 || StringValue(page.Items[0].Id) != "a" {
		t.Fatalf("PageItems missing cursor = %#v, want first page", page)
	}
}

func TestScalarHelpersAndRoles(t *testing.T) {
	if err := RequireOwner(" "); err == nil {
		t.Fatal("RequireOwner empty error = nil")
	}
	cursor, limit := NormalizeListParams(" a/b ", MaxListLimit+1)
	if cursor != "a%2Fb" || limit != MaxListLimit {
		t.Fatalf("NormalizeListParams = (%q, %d), want escaped cursor and capped limit", cursor, limit)
	}
	if key := CursorAfterKey(kv.Key{"root"}, cursor); len(key) != 2 || key[1] != cursor {
		t.Fatalf("CursorAfterKey = %#v, want root/cursor", key)
	}
	if got := RelationID(" peer-b ", "peer-a"); got != "peer-a:peer-b" {
		t.Fatalf("RelationID = %q, want sorted relation", got)
	}
	if got := DirectWorkspaceName("peer-a:peer-b"); got == "" || got == DirectWorkspaceName("peer-a:peer-c") || !strings.HasPrefix(got, "social-direct-") {
		t.Fatalf("DirectWorkspaceName returned unstable value %q", got)
	}
	if got := GroupWorkspaceName("group-a"); got == "" || !strings.HasPrefix(got, "social-group-") {
		t.Fatalf("GroupWorkspaceName = %q", got)
	}
	params := ChatRoomWorkspaceParameters(apitypes.ChatRoomModeDirect)
	typed, err := params.AsChatRoomWorkspaceParameters()
	if err != nil {
		t.Fatalf("AsChatRoomWorkspaceParameters() error = %v", err)
	}
	if typed.AgentType != apitypes.ChatRoomWorkspaceParametersAgentTypeChatroom || typed.Mode == nil || *typed.Mode != apitypes.ChatRoomModeDirect {
		t.Fatalf("ChatRoomWorkspaceParameters = %#v", typed)
	}
	if got := NormalizePhone("+1 (555) 0100"); got != "15550100" {
		t.Fatalf("NormalizePhone = %q, want digits only", got)
	}
	if OptionalString("") != nil || StringValue(OptionalString("x")) != "x" {
		t.Fatal("OptionalString returned unexpected value")
	}
	if got := UnescapeStoreSegment(EscapeStoreSegment("a/b c")); got != "a/b c" {
		t.Fatalf("escaped round trip = %q, want original", got)
	}
	if got := UnescapeStoreSegment("%"); got != "%" {
		t.Fatalf("invalid unescape = %q, want original", got)
	}
	if got := GroupBelongKey("peer b", "group/a"); len(got) != 3 || got[1] != "peer+b" || got[2] != "group%2Fa" {
		t.Fatalf("GroupBelongKey = %#v, want escaped peer/group key", got)
	}
	if got := FriendInviteTokenKey("peer/a"); len(got) != 2 || got[1] != "peer%2Fa" {
		t.Fatalf("FriendInviteTokenKey = %#v, want escaped owner key", got)
	}
	if got := GroupInviteTokenKey("group/a"); len(got) != 2 || got[1] != "group%2Fa" {
		t.Fatalf("GroupInviteTokenKey = %#v, want escaped group key", got)
	}
}

func TestGroupRolesAndMessageExpiry(t *testing.T) {
	role := rpcapi.FriendGroupMemberRoleAdmin
	if got := GroupRole(rpcapi.FriendGroupMemberObject{Role: &role}); got != role {
		t.Fatalf("GroupRole = %q, want admin", got)
	}
	if got := GroupRole(rpcapi.FriendGroupMemberObject{}); got != "" {
		t.Fatalf("GroupRole nil = %q, want empty", got)
	}

	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	expiredAt := now
	future := now.Add(time.Second)
	if !MessageExpired(rpcapi.FriendGroupMessageObject{ExpiresAt: &expiredAt}, now) {
		t.Fatal("MessageExpired at boundary = false, want true")
	}
	if MessageExpired(rpcapi.FriendGroupMessageObject{ExpiresAt: &future}, now) {
		t.Fatal("MessageExpired future = true, want false")
	}
	if !TimeValue(&now).Equal(now) || !TimeValue(nil).IsZero() {
		t.Fatal("TimeValue returned unexpected value")
	}
	if !CompareByCreatedAtAsc(now, "a", future, "b") || !CompareByCreatedAtAsc(now, "a", now, "b") {
		t.Fatal("CompareByCreatedAtAsc returned unexpected ordering")
	}
	if !CompareByCreatedAtDesc(future, "b", now, "a") || !CompareByCreatedAtDesc(now, "b", now, "a") {
		t.Fatal("CompareByCreatedAtDesc returned unexpected ordering")
	}
}

func strPtr(v string) *string {
	return &v
}
