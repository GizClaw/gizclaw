package peerroute

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func ToRPC(assignment apitypes.PeerAssignment) rpcapi.EdgePeerAssignment {
	return rpcapi.EdgePeerAssignment{
		PeerPublicKey:   assignment.PeerPublicKey,
		ServerPublicKey: assignment.ServerPublicKey,
		ServerEndpoint:  assignment.ServerEndpoint,
		Role:            string(assignment.Role),
		Version:         assignment.Version,
		UpdatedAt:       assignment.UpdatedAt,
	}
}
