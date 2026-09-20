package gizcli

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type clientSecurityPolicy struct{}

func (clientSecurityPolicy) AllowPeer(context.Context, giznet.PeerAdmission) bool {
	return true
}

func (clientSecurityPolicy) AllowService(_ giznet.PublicKey, service uint64) bool {
	return service == ServicePeerHTTP || service == ServicePeerOpenAI || service == ServicePeerRPC || service == ServiceEdgeRPC || service == EventStreamAgent
}
