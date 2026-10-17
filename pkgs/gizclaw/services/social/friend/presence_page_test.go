package friend

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
)

type pagePresenceFunc func(context.Context, string) (bool, time.Time)

func (f pagePresenceFunc) PeerPresence(ctx context.Context, peer string) (bool, time.Time) {
	return f(ctx, peer)
}

func TestListFriendsBoundsPresencePage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestServer()
		for i := range socialutil.PeerFriendLimit {
			if _, err := s.AdminCreateFriend(t.Context(), "owner", fmt.Sprintf("peer-%02d", i)); err != nil {
				t.Fatal(err)
			}
		}
		before, err := s.ListFriends(t.Context(), "owner", rpcapi.FriendListRequest{})
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		var calls atomic.Int32
		s.Presence = pagePresenceFunc(func(ctx context.Context, _ string) (bool, time.Time) {
			calls.Add(1)
			if deadline, ok := ctx.Deadline(); !ok || !deadline.Equal(started.Add(socialutil.PresencePageTimeout)) {
				t.Errorf("presence deadline = %v, %v", deadline, ok)
			}
			<-ctx.Done()
			return false, time.Time{}
		})
		page, err := s.ListFriends(t.Context(), "owner", rpcapi.FriendListRequest{})
		if err != nil || len(page.Items) != socialutil.PeerFriendLimit {
			t.Fatalf("page = %+v, %v", page, err)
		}
		if elapsed := time.Since(started); elapsed != socialutil.PresencePageTimeout || calls.Load() != 8 {
			t.Fatalf("page took %s with %d reads; want one budget and eight active reads", elapsed, calls.Load())
		}
		for i, item := range page.Items {
			if item.Name != before.Items[i].Name || item.Online == nil || *item.Online || item.LastSeenAt != nil {
				t.Fatalf("item %d = %+v; order or timeout fields changed", i, item)
			}
		}
	})
}
