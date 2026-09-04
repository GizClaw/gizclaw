package peerresource

import (
	"database/sql"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func businessError(id string, err error) *rpcapi.RPCResponse {
	switch {
	case errors.Is(err, friend.ErrCrossServerFriendCreation):
		return statusError(id, rpcapi.StatusCodeFailedPrecondition, friend.ErrCrossServerFriendCreation.Error())
	case errors.Is(err, friendgroup.ErrCrossServerFriendGroupMembership):
		return statusError(id, rpcapi.StatusCodeFailedPrecondition, friendgroup.ErrCrossServerFriendGroupMembership.Error())
	case errors.Is(err, friend.ErrInviteTokenRequired):
		return statusError(id, rpcapi.StatusCodeInvalidArgument, "friend invite token is required")
	case errors.Is(err, friend.ErrInviteTokenUnavailable):
		return statusError(id, rpcapi.StatusCodeNotFound, "friend invite token not found")
	case errors.Is(err, friend.ErrInviteTokenSelfOwned):
		return statusError(id, rpcapi.StatusCodeInvalidArgument, "cannot add self as friend")
	case errors.Is(err, friend.ErrInviteTokenLookupFailed):
		return internalError(id, "friend invite lookup failed")
	}
	if errors.Is(err, kv.ErrNotFound) || errors.Is(err, sql.ErrNoRows) || errors.Is(err, peer.ErrPeerNotFound) {
		return statusError(id, rpcapi.StatusCodeNotFound, "not found")
	}
	return internalError(id, err.Error())
}
