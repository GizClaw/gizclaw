package friendgroup

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

type fakePings struct {
	mu        sync.Mutex
	online    map[string]bool
	unreached map[string]bool
	pushed    map[string]rpcapi.ClientSocialPingRequest
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
	f.pushed[peer] = request
	return nil
}

type profileStub map[string]string

func (p profileStub) GetSelfInfo(_ context.Context, key giznet.PublicKey) (apitypes.DeviceInfo, error) {
	name, ok := p[key.String()]
	if !ok {
		return apitypes.DeviceInfo{}, errors.New("no profile")
	}
	return apitypes.DeviceInfo{Name: &name}, nil
}

type pingGroup struct {
	s                   *Server
	pings               *fakePings
	owner, member, late string
	now                 *time.Time
}

// newPingGroup makes a group the owner calls "team", which member joined as
// "my-team" and late was added to as "their-team".
func newPingGroup(t *testing.T) pingGroup {
	t.Helper()
	ctx := t.Context()
	s := newTestServer(t)
	now := time.Now().UTC()
	s.Now = func() time.Time { return now }
	owner, member, late := giznet.PublicKey{1}.String(), giznet.PublicKey{2}.String(), giznet.PublicKey{3}.String()
	s.Profiles = profileStub{member: "Member"}
	pings := &fakePings{online: map[string]bool{}, unreached: map[string]bool{}, pushed: map[string]rpcapi.ClientSocialPingRequest{}}
	s.Pings = pings
	if _, err := s.CreateFriendGroup(ctx, owner, rpcapi.FriendGroupCreateRequest{Name: "team"}); err != nil {
		t.Fatal(err)
	}
	token, err := s.CreateFriendGroupInviteToken(ctx, owner, rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: "team"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinFriendGroup(ctx, member, rpcapi.FriendGroupJoinRequest{Name: "my-team", InviteToken: token.InviteToken}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFriendGroupMember(ctx, owner, rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: "team", PeerPublicKey: late, MemberName: "their-team", Role: rpcapi.FriendGroupMemberMutableRole("member"),
	}); err != nil {
		t.Fatal(err)
	}
	return pingGroup{s: s, pings: pings, owner: owner, member: member, late: late, now: &now}
}

func TestPingFriendGroupLetsAnyMemberRallyEveryOtherOnlineMember(t *testing.T) {
	g := newPingGroup(t)
	g.pings.online[g.owner] = true
	g.pings.online[g.member] = true
	g.pings.online[g.late] = true

	got, err := g.s.PingFriendGroup(t.Context(), g.member, rpcapi.FriendGroupPingRequest{Name: "my-team"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != rpcapi.SocialPingResultDelivered || got.DeliveredCount != 2 || got.RetryAfterSeconds != nil {
		t.Fatalf("PingFriendGroup() = %+v", got)
	}
	if _, rallied := g.pings.pushed[g.member]; rallied {
		t.Fatal("the rallying member received its own rally")
	}
	for peer, groupName := range map[string]string{g.owner: "team", g.late: "their-team"} {
		push, ok := g.pings.pushed[peer]
		if !ok {
			t.Fatalf("%s was not rallied; pushed = %+v", peer, g.pings.pushed)
		}
		if push.FromPeerPublicKey != g.member || push.FromDisplayName == nil || *push.FromDisplayName != "Member" ||
			push.FriendGroupName == nil || *push.FriendGroupName != groupName {
			t.Fatalf("rally to %s = %+v, want the recipient's own group name %q", peer, push, groupName)
		}
	}
}

func TestPingFriendGroupSkipsOfflineMembers(t *testing.T) {
	g := newPingGroup(t)
	g.pings.online[g.late] = true

	got, err := g.s.PingFriendGroup(t.Context(), g.owner, rpcapi.FriendGroupPingRequest{Name: "team"})
	if err != nil || got.Result != rpcapi.SocialPingResultDelivered || got.DeliveredCount != 1 {
		t.Fatalf("PingFriendGroup() = %+v, %v", got, err)
	}
	if _, ok := g.pings.pushed[g.late]; !ok || len(g.pings.pushed) != 1 {
		t.Fatalf("pushed = %+v, want only the online member", g.pings.pushed)
	}
}

func TestPingFriendGroupAnswersNotOnlineWithoutConsumingTheWindow(t *testing.T) {
	g := newPingGroup(t)
	g.pings.online[g.owner] = true

	got, err := g.s.PingFriendGroup(t.Context(), g.owner, rpcapi.FriendGroupPingRequest{Name: "team"})
	if err != nil || got.Result != rpcapi.SocialPingResultNotOnline || got.DeliveredCount != 0 {
		t.Fatalf("PingFriendGroup() with only the caller online = %+v, %v", got, err)
	}
	g.pings.online[g.member] = true
	g.pings.unreached[g.member] = true
	got, err = g.s.PingFriendGroup(t.Context(), g.owner, rpcapi.FriendGroupPingRequest{Name: "team"})
	if err != nil || got.Result != rpcapi.SocialPingResultNotOnline {
		t.Fatalf("PingFriendGroup() nobody acknowledged = %+v, %v", got, err)
	}
	g.pings.unreached[g.member] = false
	got, err = g.s.PingFriendGroup(t.Context(), g.owner, rpcapi.FriendGroupPingRequest{Name: "team"})
	if err != nil || got.Result != rpcapi.SocialPingResultDelivered || got.DeliveredCount != 1 {
		t.Fatalf("PingFriendGroup() after a member came online = %+v, %v", got, err)
	}
}

func TestPingFriendGroupRateLimitsTheWholeGroup(t *testing.T) {
	g := newPingGroup(t)
	g.pings.online[g.owner] = true
	g.pings.online[g.member] = true
	ctx := t.Context()

	if got, err := g.s.PingFriendGroup(ctx, g.owner, rpcapi.FriendGroupPingRequest{Name: "team"}); err != nil || got.Result != rpcapi.SocialPingResultDelivered {
		t.Fatalf("first rally = %+v, %v", got, err)
	}
	*g.now = g.now.Add(59*time.Second + 100*time.Millisecond)
	got, err := g.s.PingFriendGroup(ctx, g.member, rpcapi.FriendGroupPingRequest{Name: "my-team"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != rpcapi.SocialPingResultRateLimited || got.RetryAfterSeconds == nil || *got.RetryAfterSeconds != 1 {
		t.Fatalf("another member rallies within the window = %+v", got)
	}
}

func TestPingFriendGroupRejectsNonMembers(t *testing.T) {
	g := newPingGroup(t)
	g.pings.online[g.owner] = true
	stranger := giznet.PublicKey{9}.String()
	if _, err := g.s.PingFriendGroup(t.Context(), stranger, rpcapi.FriendGroupPingRequest{Name: "team"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("non-member PingFriendGroup() error = %v, want not found", err)
	}
	if len(g.pings.pushed) != 0 {
		t.Fatalf("non-member rally reached devices: %+v", g.pings.pushed)
	}
}
