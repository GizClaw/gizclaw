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
	case errors.Is(err, friendgroup.ErrFriendGroupFull):
		// A ten-member cap is a quota, which is what RESOURCE_EXHAUSTED names.
		// The 409 this used to send maps to ABORTED in StatusCodeFromHTTP, but
		// ABORTED is for concurrency aborts and says nothing true here. The
		// reason carries the specific failure, and matches the code the Admin
		// HTTP surface already returns for the same condition.
		return rpcapi.Error{
			RequestID: id,
			Code:      rpcapi.StatusCodeResourceExhausted,
			Reason:    "FRIEND_GROUP_FULL",
			Message:   "friend group is full",
		}.RPCResponse()
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
