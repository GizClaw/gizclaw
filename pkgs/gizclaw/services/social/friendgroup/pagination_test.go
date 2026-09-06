package friendgroup

import (
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestListFriendGroupsPaginatesOpaqueIDs(t *testing.T) {
	s := newTestServer(t)
	want := []string{"group:a", "group:b", "group:c"}
	for _, id := range want {
		if _, err := s.AdminCreateFriendGroup(t.Context(), id, "peer-a", id, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	var cursor *string
	var got []string
	for i := range want {
		page, err := s.ListFriendGroups(t.Context(), "peer-a", rpcapi.FriendGroupListRequest{Cursor: cursor, Limit: new(1)})
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
		t.Fatalf("paginated groups = %q, want %q", got, want)
	}
}
