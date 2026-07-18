package peerresource

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func businessError(id string, err error) *rpcapi.RPCResponse {
	if errors.Is(err, kv.ErrNotFound) || errors.Is(err, sql.ErrNoRows) || errors.Is(err, peer.ErrPeerNotFound) {
		return statusError(id, http.StatusNotFound, "not found")
	}
	return internalError(id, err.Error())
}
