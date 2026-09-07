package friend

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type adminFriendReadStore struct {
	kv.Store
	lists          atomic.Int64
	gets           atomic.Int64
	indexedMembers atomic.Int64
}

func (s *adminFriendReadStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	s.gets.Add(1)
	return s.Store.Get(ctx, key)
}

func (s *adminFriendReadStore) RangeOrderedMembers(ctx context.Context, key kv.Key, query kv.OrderedRange) ([]string, error) {
	members, err := s.Store.RangeOrderedMembers(ctx, key, query)
	s.indexedMembers.Add(int64(len(members)))
	return members, err
}

func TestAdminFriendIndexPagesCurrentRowsWithoutKVScan(t *testing.T) {
	s := newTestServer()
	const relations = 12
	for i := range relations {
		if _, err := s.AdminCreateFriend(t.Context(), fmt.Sprintf("owner-%02d", i), fmt.Sprintf("peer-%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	// Unindexed data is outside this list's domain and must never be decoded.
	if err := s.Friends.Set(t.Context(), kv.Key{"friends", "foreign", "malformed"}, []byte("invalid JSON")); err != nil {
		t.Fatal(err)
	}
	reader := &adminFriendReadStore{Store: s.Friends}
	s.Friends = reader
	var cursor *string
	seen := make(map[string]bool)
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber > relations*2 {
			t.Fatal("pagination did not terminate")
		}
		reader.gets.Store(0)
		reader.indexedMembers.Store(0)
		page, err := s.AdminListFriends(t.Context(), cursor, new(3))
		if err != nil {
			t.Fatal(err)
		}
		if reader.indexedMembers.Load() > 4 {
			t.Fatalf("index returned %d members for a page of three", reader.indexedMembers.Load())
		}
		if len(page.Items) > 3 || reader.gets.Load() > 6 {
			t.Fatalf("page rows = %d, record reads = %d", len(page.Items), reader.gets.Load())
		}
		for _, row := range page.Items {
			key := row.OwnerPublicKey + "/" + row.Id
			if seen[key] {
				t.Fatalf("duplicate row %q", key)
			}
			seen[key] = true
		}
		if !page.HasNext {
			break
		}
		if page.NextCursor == nil || (cursor != nil && *page.NextCursor == *cursor) {
			t.Fatal("cursor did not advance")
		}
		cursor = page.NextCursor
	}
	if len(seen) != relations*2 {
		t.Fatalf("rows = %d, want %d", len(seen), relations*2)
	}
	id := socialutil.RelationID("owner-00", "peer-00")
	if _, err := s.AdminDeleteFriend(t.Context(), "owner-00", id); err != nil {
		t.Fatal(err)
	}
	page, err := s.AdminListFriends(t.Context(), nil, new(100))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != (relations-1)*2 {
		t.Fatalf("rows after deletion = %d", len(page.Items))
	}
	for _, row := range page.Items {
		if row.Id == id {
			t.Fatal("deleted relation remains in index")
		}
	}
}

type blockedAdminReadStore struct {
	kv.Store
	started chan struct{}
}

func (s blockedAdminReadStore) Get(ctx context.Context, _ kv.Key) ([]byte, error) {
	s.started <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestAdminFriendReadsBoundConcurrencyAndStopOnCancellation(t *testing.T) {
	store := blockedAdminReadStore{started: make(chan struct{}, 16)}
	keys := make([]kv.Key, 16)
	for i := range keys {
		keys[i] = kv.Key{"friends", "owner", fmt.Sprint(i)}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := loadAdminFriendRows(ctx, store, keys); done <- err }()
	for range 8 {
		select {
		case <-store.started:
		case <-time.After(5 * time.Second):
			t.Fatal("page reads did not run concurrently")
		}
	}
	select {
	case <-store.started:
		t.Fatal("more than eight reads started")
	default:
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled read = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled page reads did not stop")
	}
}

func TestOwnerPagesBoundMembershipReads(t *testing.T) {
	s := newTestServer()
	for i := range 1000 {
		id := fmt.Sprintf("item-%04d", i)
		if _, err := s.AdminCreateFriend(t.Context(), "owner", id); err != nil {
			t.Fatal(err)
		}
	}
	reader := &adminFriendReadStore{Store: s.Friends}
	s.Friends = reader
	var cursor *string
	for range 2 {
		reader.indexedMembers.Store(0)
		reader.gets.Store(0)
		page, err := s.ListFriends(t.Context(), "owner", rpcapi.FriendListRequest{Cursor: cursor, Limit: new(3)})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 3 || !page.HasNext || page.NextCursor == nil {
			t.Fatalf("page=%#v", page)
		}
		if reader.indexedMembers.Load() != 4 || reader.gets.Load() != 3 {
			t.Fatalf("unbounded page: members=%d reads=%d", reader.indexedMembers.Load(), reader.gets.Load())
		}
		if reader.lists.Load() != 0 {
			t.Fatal("pagination enumerated the full set")
		}
		cursor = page.NextCursor
	}
}

func (s *adminFriendReadStore) ListMembers(ctx context.Context, key kv.Key) ([]string, error) {
	s.lists.Add(1)
	return s.Store.ListMembers(ctx, key)
}
