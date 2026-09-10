package friend

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// ErrPingUnavailable reports that this Server cannot reach devices, so no
// ping can be pushed.
var ErrPingUnavailable = errors.New("social: ping delivery is not configured")

// PingFriend pushes client.social.ping to the device of the caller's Friend
// named req.Name. It never waits for an offline device: a Friend that is not
// connected to this Server answers NOT_ONLINE at once. One ping per friend
// pair per socialutil.PingWindow; both directions share the window, and a
// ping that reaches no device does not consume it.
func (s *Server) PingFriend(ctx context.Context, owner string, req rpcapi.FriendPingRequest) (rpcapi.FriendPingResponse, error) {
	if s == nil || s.Pings == nil {
		return rpcapi.FriendPingResponse{}, ErrPingUnavailable
	}
	owner = strings.TrimSpace(owner)
	relation, err := s.GetFriendRelation(ctx, owner, req.Name)
	if err != nil {
		return rpcapi.FriendPingResponse{}, err
	}
	target := socialutil.StringValue(relation.PeerPublicKey)
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendPingResponse{}, err
	}
	window := socialutil.FriendPingWindowKey(socialutil.RelationID(owner, target))
	now := s.now()
	remaining, err := socialutil.PingWindowRemaining(ctx, store, window, now)
	if err != nil {
		return rpcapi.FriendPingResponse{}, err
	}
	if remaining > 0 {
		return friendPingRateLimited(remaining), nil
	}
	if !s.Pings.PeerOnline(target) {
		return rpcapi.FriendPingResponse{Result: rpcapi.SocialPingResultNotOnline}, nil
	}
	request := rpcapi.ClientSocialPingRequest{FromPeerPublicKey: owner, FromDisplayName: s.pingDisplayName(ctx, owner)}
	claim, remaining, claimed, err := socialutil.ClaimPingWindow(ctx, store, window, now)
	if err != nil {
		return rpcapi.FriendPingResponse{}, err
	}
	if !claimed {
		return friendPingRateLimited(remaining), nil
	}
	delivered := socialutil.DeliverPings(ctx, s.Pings, []socialutil.PingTarget{{PeerPublicKey: target, Request: request}})
	if delivered == 0 {
		if err := socialutil.ReleasePingWindow(context.WithoutCancel(ctx), store, claim); err != nil {
			return rpcapi.FriendPingResponse{}, err
		}
		return rpcapi.FriendPingResponse{Result: rpcapi.SocialPingResultNotOnline}, nil
	}
	return rpcapi.FriendPingResponse{Result: rpcapi.SocialPingResultDelivered, DeliveredCount: int32(delivered)}, nil
}

func friendPingRateLimited(remaining time.Duration) rpcapi.FriendPingResponse {
	return rpcapi.FriendPingResponse{Result: rpcapi.SocialPingResultRateLimited, RetryAfterSeconds: socialutil.RetryAfterSeconds(remaining)}
}

// pingDisplayName reads the caller's self-chosen name. A missing profile only
// leaves the name out; it never blocks the ping.
func (s *Server) pingDisplayName(ctx context.Context, owner string) *string {
	if s.Profiles == nil {
		return nil
	}
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(owner)); err != nil {
		return nil
	}
	info, err := s.Profiles.GetSelfInfo(ctx, publicKey)
	if err != nil {
		return nil
	}
	return socialutil.PingDisplayName(info.Name)
}
