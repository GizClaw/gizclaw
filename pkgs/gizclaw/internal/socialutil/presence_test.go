package socialutil

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence"
)

type presenceFunc func(context.Context, string) (bool, time.Time)

func (f presenceFunc) PeerPresence(ctx context.Context, peer string) (bool, time.Time) {
	return f(ctx, peer)
}

func TestReadPresencePageBoundsConcurrency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, release, finished := make(chan struct{}, 10), make(chan struct{}), make(chan struct{})
		visited := make([]int, 10)
		go func() {
			ReadPresencePage(t.Context(), len(visited), func(_ context.Context, i int) {
				entered <- struct{}{}
				<-release
				visited[i]++
			})
			close(finished)
		}()
		for range presencePageConcurrency {
			<-entered
		}
		synctest.Wait()
		if len(entered) != 0 {
			t.Fatal("more than eight reads started while the first eight were blocked")
		}
		close(release)
		<-finished
		for i, count := range visited {
			if count != 1 {
				t.Fatalf("item %d visited %d times", i, count)
			}
		}
	})
}

func TestReadPresencePageSharesDeadline(t *testing.T) {
	for _, budget := range []time.Duration{PresencePageTimeout, 50 * time.Millisecond} {
		t.Run(budget.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), budget)
				defer cancel()
				started := time.Now()
				var calls atomic.Int32
				service := presenceFunc(func(ctx context.Context, _ string) (bool, time.Time) {
					calls.Add(1)
					if deadline, ok := ctx.Deadline(); !ok || !deadline.Equal(started.Add(budget)) {
						t.Errorf("read deadline = %v, %v; want one shared page deadline", deadline, ok)
					}
					<-ctx.Done()
					return false, time.Time{}
				})
				items := make([]*bool, 10)
				ReadPresencePage(ctx, len(items), func(ctx context.Context, i int) {
					if i == 0 { // A successful item remains present when other reads time out.
						items[i] = new(true)
						return
					}
					items[i], _ = PresenceFields(ctx, service, "peer")
				})
				if elapsed := time.Since(started); elapsed != budget {
					t.Fatalf("page took %s, want %s", elapsed, budget)
				}
				if calls.Load() != presencePageConcurrency {
					t.Fatalf("provider calls = %d, want eight; expired queued items must skip I/O", calls.Load())
				}
				for i, online := range items {
					if online == nil || *online != (i == 0) {
						t.Fatalf("item %d online = %v", i, online)
					}
				}
			})
		})
	}
}

type canceledRoomPresence struct{ calls int }

func (s *canceledRoomPresence) PeerRunningWorkspace(context.Context, string, string) bool {
	s.calls++
	return true
}

func (s *canceledRoomPresence) PeerRoomPresence(context.Context, string, string) (bool, time.Time, bool) {
	s.calls++
	return true, time.Now(), true
}

func (s *canceledRoomPresence) PeerPresence(context.Context, string) (bool, time.Time) {
	s.calls++
	return true, time.Now()
}

func TestCanceledPresenceSkipsProviders(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	service := &canceledRoomPresence{}
	online, seen, room := MemberPresenceFields(ctx, service, service, "peer", "room")
	if online == nil || *online || seen != nil || room == nil || *room || service.calls != 0 {
		t.Fatalf("canceled fields = %v, %v, %v; provider calls = %d", online, seen, room, service.calls)
	}
	online, seen, room = MemberPresenceFields(ctx, nil, nil, "peer", "room")
	if online != nil || seen != nil || room != nil {
		t.Fatal("nil services must still omit presence fields after cancellation")
	}
}

type batchPresenceStub struct {
	keys      []string
	workspace string
	deadline  time.Time
}

func (*batchPresenceStub) PeerPresence(context.Context, string) (bool, time.Time) {
	panic("batch provider should not be queried one Peer at a time")
}

func (*batchPresenceStub) PeerRunningWorkspace(context.Context, string, string) bool {
	panic("batch provider should not be queried one room at a time")
}

func (s *batchPresenceStub) PeerRoomPresenceBatch(ctx context.Context, keys []string, workspace string) []peerpresence.Snapshot {
	s.keys, s.workspace = keys, workspace
	s.deadline, _ = ctx.Deadline()
	return []peerpresence.Snapshot{{Online: true, LastSeenAt: time.Unix(1, 0), WorkspaceName: "room"}, {}}
}

func TestPresencePagesPreferBatchProvider(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		presence := &batchPresenceStub{}
		friends := []rpcapi.FriendObject{{PeerPublicKey: new("first")}, {PeerPublicKey: new("second")}}
		AddFriendPresence(t.Context(), presence, friends)
		if len(presence.keys) != 2 || presence.keys[0] != "first" || presence.keys[1] != "second" || presence.workspace != "" || !presence.deadline.Equal(time.Now().Add(PresencePageTimeout)) || friends[0].Online == nil || !*friends[0].Online || friends[0].LastSeenAt == nil || friends[1].Online == nil || *friends[1].Online || friends[1].LastSeenAt != nil {
			t.Fatalf("batch Friend fields = %+v, provider = %+v", friends, presence)
		}
		members := []rpcapi.FriendGroupMemberObject{{PeerPublicKey: new("first")}, {PeerPublicKey: new("second")}}
		AddMemberPresence(t.Context(), presence, presence, "room", members)
		if presence.workspace != "room" || members[0].InRoom == nil || !*members[0].InRoom || members[1].InRoom == nil || *members[1].InRoom {
			t.Fatalf("batch member fields = %+v, provider = %+v", members, presence)
		}
		members[0].InRoom = nil
		AddMemberPresence(t.Context(), presence, nil, "room", members)
		if members[0].InRoom != nil {
			t.Fatal("nil Rooms provider must omit in_room even when Presence supports batch reads")
		}
	})
}
