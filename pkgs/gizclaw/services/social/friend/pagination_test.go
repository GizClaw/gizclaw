package friend

import (
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestListFriendsPaginatesRelationIDs(t *testing.T) {
	s := newTestServer()
	want := []string{"peer-b", "peer-c", "peer-d"}
	for _, peer := range want {
		if _, err := s.AdminCreateFriend(t.Context(), "peer-a", peer); err != nil {
			t.Fatal(err)
		}
	}
	var cursor *string
	var got []string
	for i := range want {
		page, err := s.ListFriends(t.Context(), "peer-a", rpcapi.FriendListRequest{Cursor: cursor, Limit: new(1)})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 || page.HasNext != (i < len(want)-1) {
			t.Fatalf("page %d = %#v", i, page)
		}
		got = append(got, page.Items[0].Name)
		cursor = page.NextCursor
	}
	if !slices.Equal(got, want) {
		t.Fatalf("paginated friends = %q, want %q", got, want)
	}
}
