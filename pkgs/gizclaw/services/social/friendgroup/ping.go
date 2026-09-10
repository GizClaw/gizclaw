package friendgroup

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// ErrPingUnavailable reports that this Server cannot reach devices, so no
// rally can be pushed.
var ErrPingUnavailable = errors.New("social: ping delivery is not configured")

// ProfileService reads the rallying member's self-chosen display name.
type ProfileService interface {
	GetSelfInfo(context.Context, giznet.PublicKey) (apitypes.DeviceInfo, error)
}

// PingFriendGroup rallies the caller's Friend Group named req.Name: every
// other member whose device is connected to this Server receives
// client.social.ping carrying that member's own name for the Group. Any member
// may rally. Offline members are skipped without waiting; when none is online
// the call answers NOT_ONLINE. One rally per Group per socialutil.PingWindow,
// shared by all members; a rally that reaches no device does not consume it.
func (s *Server) PingFriendGroup(ctx context.Context, owner string, req rpcapi.FriendGroupPingRequest) (rpcapi.FriendGroupPingResponse, error) {
	if s == nil || s.Pings == nil {
		return rpcapi.FriendGroupPingResponse{}, ErrPingUnavailable
	}
	owner = strings.TrimSpace(owner)
	friendGroupID, err := s.resolveFriendGroupName(ctx, owner, req.Name)
	if err != nil {
		return rpcapi.FriendGroupPingResponse{}, err
	}
	if err := s.requireUse(ctx, owner, friendGroupID); err != nil {
		return rpcapi.FriendGroupPingResponse{}, err
	}
	store, err := s.groupsStore()
	if err != nil {
		return rpcapi.FriendGroupPingResponse{}, err
	}
	window := socialutil.GroupPingWindowKey(friendGroupID)
	now := s.now()
	remaining, err := socialutil.PingWindowRemaining(ctx, store, window, now)
	if err != nil {
		return rpcapi.FriendGroupPingResponse{}, err
	}
	if remaining > 0 {
		return groupPingRateLimited(remaining), nil
	}
	members, err := s.listAllMembers(ctx, friendGroupID)
	if err != nil {
		return rpcapi.FriendGroupPingResponse{}, err
	}
	displayName := s.pingDisplayName(ctx, owner)
	targets := make([]socialutil.PingTarget, 0, len(members))
	for _, member := range members {
		if member.PeerPublicKey == owner || !s.Pings.PeerOnline(member.PeerPublicKey) {
			continue
		}
		targets = append(targets, socialutil.PingTarget{
			PeerPublicKey: member.PeerPublicKey,
			Request: rpcapi.ClientSocialPingRequest{
				FromPeerPublicKey: owner,
				FromDisplayName:   displayName,
				FriendGroupName:   new(member.FriendGroupName),
			},
		})
	}
	if len(targets) == 0 {
		return rpcapi.FriendGroupPingResponse{Result: rpcapi.SocialPingResultNotOnline}, nil
	}
	claim, remaining, claimed, err := socialutil.ClaimPingWindow(ctx, store, window, now)
	if err != nil {
		return rpcapi.FriendGroupPingResponse{}, err
	}
	if !claimed {
		return groupPingRateLimited(remaining), nil
	}
	delivered := socialutil.DeliverPings(ctx, s.Pings, targets)
	if delivered == 0 {
		if err := socialutil.ReleasePingWindow(context.WithoutCancel(ctx), store, claim); err != nil {
			return rpcapi.FriendGroupPingResponse{}, err
		}
		return rpcapi.FriendGroupPingResponse{Result: rpcapi.SocialPingResultNotOnline}, nil
	}
	return rpcapi.FriendGroupPingResponse{Result: rpcapi.SocialPingResultDelivered, DeliveredCount: int32(delivered)}, nil
}

func groupPingRateLimited(remaining time.Duration) rpcapi.FriendGroupPingResponse {
	return rpcapi.FriendGroupPingResponse{Result: rpcapi.SocialPingResultRateLimited, RetryAfterSeconds: socialutil.RetryAfterSeconds(remaining)}
}

// pingDisplayName reads the caller's self-chosen name. A missing profile only
// leaves the name out; it never blocks the rally.
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
