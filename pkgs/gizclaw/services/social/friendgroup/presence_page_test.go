package friendgroup

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type pagePresenceFunc func(context.Context, string, string) (bool, time.Time, bool)

func (f pagePresenceFunc) PeerPresence(ctx context.Context, peer string) (bool, time.Time) {
	online, seen, _ := f(ctx, peer, "")
	return online, seen
}

func (f pagePresenceFunc) PeerRoomPresence(ctx context.Context, peer, workspace string) (bool, time.Time, bool) {
	return f(ctx, peer, workspace)
}

func (f pagePresenceFunc) PeerRunningWorkspace(ctx context.Context, peer, workspace string) bool {
	_, _, room := f(ctx, peer, workspace)
	return room
}

func TestListFriendGroupMembersBoundsPresencePage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestServer(t)
		group, err := s.CreateFriendGroup(t.Context(), "owner", rpcapi.FriendGroupCreateRequest{Name: "room"})
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < socialutil.FriendGroupMemberLimit; i++ {
			if _, err := s.AddFriendGroupMember(t.Context(), "owner", rpcapi.FriendGroupMemberAddRequest{
				FriendGroupName: group.Name, PeerPublicKey: fmt.Sprintf("peer-%02d", i), MemberName: "room", Role: "member",
			}); err != nil {
				t.Fatal(err)
			}
		}
		req := rpcapi.FriendGroupMemberListRequest{FriendGroupName: new(group.Name)}
		before, err := s.ListFriendGroupMembers(t.Context(), "owner", req)
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		var calls atomic.Int32
		presence := pagePresenceFunc(func(ctx context.Context, _, workspace string) (bool, time.Time, bool) {
			calls.Add(1)
			if deadline, ok := ctx.Deadline(); !ok || !deadline.Equal(started.Add(socialutil.PresencePageTimeout)) {
				t.Errorf("presence deadline = %v, %v", deadline, ok)
			}
			if workspace != socialutil.StringValue(group.WorkspaceName) {
				t.Errorf("workspace = %q", workspace)
			}
			<-ctx.Done()
			return false, time.Time{}, false
		})
		s.Presence, s.Rooms = presence, presence
		page, err := s.ListFriendGroupMembers(t.Context(), "owner", req)
		if err != nil || len(page.Items) != socialutil.FriendGroupMemberLimit {
			t.Fatalf("page = %+v, %v", page, err)
		}
		if elapsed := time.Since(started); elapsed != socialutil.PresencePageTimeout || calls.Load() != 8 {
			t.Fatalf("page took %s with %d reads; want one budget and eight active reads", elapsed, calls.Load())
		}
		for i, item := range page.Items {
			if item.Name != before.Items[i].Name || item.Online == nil || *item.Online || item.LastSeenAt != nil || item.InRoom == nil || *item.InRoom {
				t.Fatalf("item %d = %+v; order or timeout fields changed", i, item)
			}
		}
	})
}

type stalledBindingStore struct {
	kv.Store
	key kv.Key
}

func (s stalledBindingStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	if slices.Equal(key, s.key) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return s.Store.Get(ctx, key)
}

func TestRoomBindingConsumesPresencePageBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestServer(t)
		group, err := s.CreateFriendGroup(t.Context(), "owner", rpcapi.FriendGroupCreateRequest{Name: "room"})
		if err != nil {
			t.Fatal(err)
		}
		id := mustGroupID(t, s, "owner", group.Name)
		s.RelationshipStore = stalledBindingStore{Store: s.RelationshipStore, key: workspaceBindingKey(id)}
		presence := pagePresenceFunc(func(context.Context, string, string) (bool, time.Time, bool) {
			t.Error("no presence read should start after the binding consumes the page budget")
			return true, time.Now(), true
		})
		s.Presence, s.Rooms = presence, presence
		started := time.Now()
		page, err := s.ListFriendGroupMembers(t.Context(), "owner", rpcapi.FriendGroupMemberListRequest{FriendGroupName: new(group.Name)})
		if err != nil || len(page.Items) != 1 || time.Since(started) != socialutil.PresencePageTimeout {
			t.Fatalf("page = %+v, %v, elapsed = %s", page, err, time.Since(started))
		}
		item := page.Items[0]
		if item.Online == nil || *item.Online || item.LastSeenAt != nil || item.InRoom != nil {
			t.Fatalf("binding timeout fields = %+v", item)
		}
	})
}
