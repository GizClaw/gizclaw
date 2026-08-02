package gizclaw

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type ServerSecurityPolicy Server

var _ giznet.SecurityPolicy = (*ServerSecurityPolicy)(nil)

func (p *ServerSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return p != nil
}

// AllowGatewaySCTP identifies bounded Edge-to-Server upstream associations.
// Public clients retain the transport's default SCTP receive window.
func (p *ServerSecurityPolicy) AllowGatewaySCTP(publicKey giznet.PublicKey) bool {
	if p == nil {
		return false
	}
	s := (*Server)(p)
	return s.manager != nil && s.manager.allowActivePeerRole(
		context.Background(),
		publicKey,
		apitypes.PeerRoleEdgeNode,
	)
}

func (p *ServerSecurityPolicy) AllowService(publicKey giznet.PublicKey, service uint64) bool {
	if p == nil {
		return false
	}
	s := (*Server)(p)
	if m := s.manager; m != nil && m.allowService(context.Background(), publicKey, service) {
		return true
	}
	return s.SecurityPolicy != nil && s.SecurityPolicy.AllowService(publicKey, service)
}
