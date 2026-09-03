package peerresource

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func businessError(id string, err error) *rpcapi.RPCResponse {
	switch {
	case errors.Is(err, friendgroup.ErrFriendGroupFull):
		return statusError(id, http.StatusConflict, "friend group is full")
	case errors.Is(err, friend.ErrInviteTokenRequired):
		return statusError(id, http.StatusBadRequest, "friend invite token is required")
	case errors.Is(err, friend.ErrInviteTokenUnavailable):
		return statusError(id, http.StatusNotFound, "friend invite token not found")
	case errors.Is(err, friend.ErrInviteTokenSelfOwned):
		return statusError(id, http.StatusConflict, "cannot add self as friend")
	case errors.Is(err, friend.ErrInviteTokenLookupFailed):
		return internalError(id, "friend invite lookup failed")
	}
	if errors.Is(err, kv.ErrNotFound) || errors.Is(err, sql.ErrNoRows) || errors.Is(err, peer.ErrPeerNotFound) {
		return statusError(id, http.StatusNotFound, "not found")
	}
	return internalError(id, err.Error())
}
