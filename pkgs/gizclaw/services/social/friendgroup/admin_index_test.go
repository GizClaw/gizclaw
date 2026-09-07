package friendgroup

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type adminGroupReadStore struct {
	kv.Store
	lists   atomic.Int64
	gets    atomic.Int64
	ranges  atomic.Int64
	members atomic.Int64
}

func (s *adminGroupReadStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	s.gets.Add(1)
	return s.Store.Get(ctx, key)
}

func (s *adminGroupReadStore) RangeOrderedMembers(ctx context.Context, key kv.Key, query kv.OrderedRange) ([]string, error) {
	s.ranges.Add(1)
	members, err := s.Store.RangeOrderedMembers(ctx, key, query)
	s.members.Add(int64(len(members)))
	return members, err
}

func TestAdminGroupIndexPaginationWithOpaqueIDs(t *testing.T) {
	store := kv.NewMemory(nil)
	ids := []string{"a/b", "a%2Fb", "a+b", "a b", "群组"}
	for i := range 1000 {
		ids = append(ids, fmt.Sprintf("group-%04d", i))
	}
	for _, id := range ids {
		ok, err := createGroupRecord(t.Context(), store, id, []byte(id))
		if err != nil || !ok {
			t.Fatalf("create %q: %v, %v", id, ok, err)
		}
	}
	if err := store.Set(t.Context(), socialutil.GroupKey("unindexed"), []byte("foreign")); err != nil {
		t.Fatal(err)
	}
	reader := &adminGroupReadStore{Store: store}
	var got []string
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > len(ids) {
			t.Fatal("pagination did not terminate")
		}
		reader.gets.Store(0)
		reader.ranges.Store(0)
		reader.members.Store(0)
		page, err := listAdminGroupRecords(t.Context(), reader, cursor, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) > 3 || reader.gets.Load() > 3 || reader.ranges.Load() != adminGroupShards || reader.members.Load() > adminGroupShards*4 {
			t.Fatalf("unbounded page: rows=%d gets=%d ranges=%d members=%d", len(page.Items), reader.gets.Load(), reader.ranges.Load(), reader.members.Load())
		}
		for _, entry := range page.Items {
			got = append(got, string(entry.Value))
		}
		if !page.HasNext {
			break
		}
		if page.NextCursor == nil || *page.NextCursor == cursor {
			t.Fatal("cursor did not advance")
		}
		cursor = *page.NextCursor
	}
	slices.Sort(ids)
	if !slices.Equal(got, ids) {
		t.Fatalf("pagination lost, repeated or reordered IDs: got %d, want %d", len(got), len(ids))
	}
}

// createGroupRecord publishes the primary record and management index together.
func createGroupRecord(ctx context.Context, store kv.Store, id string, data []byte) (bool, error) {
	return store.ApplyMutation(ctx, kv.Mutation{
		Conditions:        []kv.Condition{{Key: socialutil.GroupKey(id)}},
		Entries:           []kv.Entry{{Key: socialutil.GroupKey(id), Value: data}},
		AddOrderedMembers: []kv.SetMembers{adminGroupMembership(id)},
	})
}

func TestOwnerPagesBoundMembershipReads(t *testing.T) {
	s := newTestServer(t)
	for i := range 1000 {
		id := fmt.Sprintf("item-%04d", i)
		if _, err := s.AdminCreateFriendGroup(t.Context(), id, "owner", id, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	reader := &adminGroupReadStore{Store: s.Belongs}
	s.Belongs = reader
	var cursor *string
	for range 2 {
		reader.members.Store(0)
		reader.gets.Store(0)
		page, err := s.ListFriendGroups(t.Context(), "owner", rpcapi.FriendGroupListRequest{Cursor: cursor, Limit: new(3)})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 3 || !page.HasNext || page.NextCursor == nil {
			t.Fatalf("page=%#v", page)
		}
		if reader.members.Load() != 4 || reader.gets.Load() != 3 {
			t.Fatalf("unbounded page: members=%d reads=%d", reader.members.Load(), reader.gets.Load())
		}
		if reader.lists.Load() != 0 {
			t.Fatal("pagination enumerated the full set")
		}
		cursor = page.NextCursor
	}
}

func (s *adminGroupReadStore) ListMembers(ctx context.Context, key kv.Key) ([]string, error) {
	s.lists.Add(1)
	return s.Store.ListMembers(ctx, key)
}
