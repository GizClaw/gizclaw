package peerresource

import (
	"context"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type publicProfileService interface {
	GetPublicProfile(context.Context, giznet.PublicKey) (peer.PublicProfile, error)
}

func (s *Server) handleFriendPing(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendPingRequest)
	if !ok || !canonicalName(params.Name) {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.PingFriend(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendPingResponse)
}

func (s *Server) handleFriendGroupPing(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupPingRequest)
	if !ok || !canonicalName(params.Name) {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.PingFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupPingResponse)
}

// handleProfileGet answers the public profile of up to
// rpcapi.MaxProfileGetKeys Peers. Any registered caller may look up any Peer,
// so the projection is limited to display name and emoji.
func (s *Server) handleProfileGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Profiles == nil {
		return internalError(req.Id, "profile service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsProfileGetRequest)
	if !ok || len(params.PeerPublicKeys) == 0 || len(params.PeerPublicKeys) > rpcapi.MaxProfileGetKeys {
		return invalidParams(req.Id)
	}
	keys := make([]giznet.PublicKey, 0, len(params.PeerPublicKeys))
	seen := make(map[giznet.PublicKey]struct{}, len(params.PeerPublicKeys))
	for _, text := range params.PeerPublicKeys {
		var key giznet.PublicKey
		if err := key.UnmarshalText([]byte(text)); err != nil || key.IsZero() || key.String() != text {
			return invalidParams(req.Id)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	items := make([]rpcapi.PublicProfile, 0, len(keys))
	for _, key := range keys {
		item := rpcapi.PublicProfile{PeerPublicKey: key.String()}
		profile, err := s.Profiles.GetPublicProfile(ctx, key)
		switch {
		case errors.Is(err, peer.ErrPeerNotFound):
		case err != nil:
			return internalError(req.Id, "profile lookup failed")
		default:
			item.DisplayName = profile.DisplayName
			item.Emoji = profile.Emoji
		}
		items = append(items, item)
	}
	return resultResponse(req.Id, rpcapi.ProfileGetResponse{Items: items}, (*rpcapi.RPCPayload).FromProfileGetResponse)
}

func canonicalName(name string) bool {
	return name != "" && name == strings.TrimSpace(name)
}
