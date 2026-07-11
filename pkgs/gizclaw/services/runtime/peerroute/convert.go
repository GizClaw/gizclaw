package peerroute

import (
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func ToRPC(assignment apitypes.PeerAssignment) *rpcapi.PeerAssignment {
	return &rpcapi.PeerAssignment{
		PeerPublicKey:   assignment.PeerPublicKey,
		ServerPublicKey: assignment.ServerPublicKey,
		ServerEndpoint:  assignment.ServerEndpoint,
		Role:            peerRoleToRPC(assignment.Role),
		Version:         assignment.Version,
		UpdatedAt:       assignment.UpdatedAt.Format(time.RFC3339Nano),
	}
}

func peerRoleToRPC(role apitypes.PeerRole) rpcpb.PeerRole {
	switch role {
	case apitypes.PeerRoleAdmin:
		return rpcpb.PeerRole_PEER_ROLE_ADMIN
	case apitypes.PeerRoleServer:
		return rpcpb.PeerRole_PEER_ROLE_SERVER
	case apitypes.PeerRoleEdgeNode:
		return rpcpb.PeerRole_PEER_ROLE_EDGE_NODE
	case apitypes.PeerRoleClient:
		return rpcpb.PeerRole_PEER_ROLE_CLIENT
	default:
		return rpcpb.PeerRole_PEER_ROLE_UNSPECIFIED
	}
}
