package friend

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// fakePings records every client.social.ping pushed to an online Peer.
type fakePings struct {
	mu        sync.Mutex
	online    map[string]bool
	unreached map[string]bool
	pushed    []pushedPing
}

type pushedPing struct {
	peer    string
	request rpcapi.ClientSocialPingRequest
}

func (f *fakePings) PeerOnline(peer string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.online[peer]
}

func (f *fakePings) DeliverSocialPing(_ context.Context, peer string, request rpcapi.ClientSocialPingRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.online[peer] || f.unreached[peer] {
		return errors.New("device did not acknowledge")
	}
	f.pushed = append(f.pushed, pushedPing{peer: peer, request: request})
	return nil
}

func (f *fakePings) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.pushed)
}

func newPingFriends(t *testing.T) (*Server, *fakePings, string, string, *time.Time) {
	t.Helper()
	ctx := t.Context()
	alice, bob := giznet.PublicKey{1}.String(), giznet.PublicKey{2}.String()
	s := newTestServer()
	now := time.Now().UTC()
	s.Now = func() time.Time { return now }
	name := "Alice"
	s.Profiles = profileStub{want: giznet.PublicKey{1}, info: apitypes.DeviceInfo{Name: &name}}
	pings := &fakePings{online: map[string]bool{}, unreached: map[string]bool{}}
	s.Pings = pings
	token, err := s.CreateFriendInviteToken(ctx, bob, rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFriend(ctx, alice, rpcapi.FriendAddRequest{InviteToken: token.InviteToken}); err != nil {
		t.Fatal(err)
	}
	return s, pings, alice, bob, &now
}

func TestPingFriendDeliversToOnlineFriend(t *testing.T) {
	s, pings, alice, bob, _ := newPingFriends(t)
	pings.online[bob] = true

	got, err := s.PingFriend(t.Context(), alice, rpcapi.FriendPingRequest{Name: bob})
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != rpcapi.SocialPingResultDelivered || got.DeliveredCount != 1 || got.RetryAfterSeconds != nil {
		t.Fatalf("PingFriend() = %+v", got)
	}
	if len(pings.pushed) != 1 {
		t.Fatalf("pushed = %+v", pings.pushed)
	}
	push := pings.pushed[0]
	if push.peer != bob || push.request.FromPeerPublicKey != alice || push.request.FromDisplayName == nil || *push.request.FromDisplayName != "Alice" || push.request.FriendGroupName != nil {
		t.Fatalf("pushed ping = %+v", push)
	}
}

func TestPingFriendAnswersNotOnlineWithoutConsumingTheWindow(t *testing.T) {
	s, pings, alice, bob, _ := newPingFriends(t)

	got, err := s.PingFriend(t.Context(), alice, rpcapi.FriendPingRequest{Name: bob})
	if err != nil || got.Result != rpcapi.SocialPingResultNotOnline || got.DeliveredCount != 0 {
		t.Fatalf("offline PingFriend() = %+v, %v", got, err)
	}
	pings.online[bob] = true
	got, err = s.PingFriend(t.Context(), alice, rpcapi.FriendPingRequest{Name: bob})
	if err != nil || got.Result != rpcapi.SocialPingResultDelivered {
		t.Fatalf("PingFriend() after friend came online = %+v, %v", got, err)
	}
}

func TestPingFriendReleasesTheWindowWhenTheDeviceDoesNotAcknowledge(t *testing.T) {
	s, pings, alice, bob, _ := newPingFriends(t)
	pings.online[bob] = true
	pings.unreached[bob] = true

	got, err := s.PingFriend(t.Context(), alice, rpcapi.FriendPingRequest{Name: bob})
	if err != nil || got.Result != rpcapi.SocialPingResultNotOnline {
		t.Fatalf("unacknowledged PingFriend() = %+v, %v", got, err)
	}
	pings.unreached[bob] = false
	got, err = s.PingFriend(t.Context(), alice, rpcapi.FriendPingRequest{Name: bob})
	if err != nil || got.Result != rpcapi.SocialPingResultDelivered {
		t.Fatalf("PingFriend() after a failed push = %+v, %v", got, err)
	}
}

func TestPingFriendRateLimitsThePairInBothDirections(t *testing.T) {
	s, pings, alice, bob, now := newPingFriends(t)
	pings.online[alice] = true
	pings.online[bob] = true
	ctx := t.Context()

	if got, err := s.PingFriend(ctx, alice, rpcapi.FriendPingRequest{Name: bob}); err != nil || got.Result != rpcapi.SocialPingResultDelivered {
		t.Fatalf("first ping = %+v, %v", got, err)
	}
	*now = now.Add(15*time.Second + 300*time.Millisecond)
	for _, tc := range []struct{ from, to string }{{alice, bob}, {bob, alice}} {
		got, err := s.PingFriend(ctx, tc.from, rpcapi.FriendPingRequest{Name: tc.to})
		if err != nil {
			t.Fatal(err)
		}
		if got.Result != rpcapi.SocialPingResultRateLimited || got.RetryAfterSeconds == nil || *got.RetryAfterSeconds != 45 || got.DeliveredCount != 0 {
			t.Fatalf("ping %s -> %s within the window = %+v", tc.from, tc.to, got)
		}
	}
	if pings.count() != 1 {
		t.Fatalf("rate-limited pings reached devices: %+v", pings.pushed)
	}
}

func TestPingFriendRejectsStrangers(t *testing.T) {
	s, pings, _, bob, _ := newPingFriends(t)
	pings.online[bob] = true
	stranger := giznet.PublicKey{3}.String()
	if _, err := s.PingFriend(t.Context(), stranger, rpcapi.FriendPingRequest{Name: bob}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("stranger PingFriend() error = %v, want not found", err)
	}
	if pings.count() != 0 {
		t.Fatalf("stranger ping reached a device: %+v", pings.pushed)
	}
}

func TestPingFriendWithoutDeliveryIsUnavailable(t *testing.T) {
	s, _, alice, bob, _ := newPingFriends(t)
	s.Pings = nil
	if _, err := s.PingFriend(t.Context(), alice, rpcapi.FriendPingRequest{Name: bob}); !errors.Is(err, ErrPingUnavailable) {
		t.Fatalf("PingFriend() error = %v, want %v", err, ErrPingUnavailable)
	}
}
