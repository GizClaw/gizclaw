package gizclaw

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type ServerSecurityPolicy Server

var _ giznet.SecurityPolicy = (*ServerSecurityPolicy)(nil)

// AllowPeer delegates admission to the host policy, defaulting to open admission.
func (p *ServerSecurityPolicy) AllowPeer(ctx context.Context, admission giznet.PeerAdmission) bool {
	if p == nil {
		return false
	}
	s := (*Server)(p)
	return s.SecurityPolicy == nil || s.SecurityPolicy.AllowPeer(ctx, admission)
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
	return s.manager.allowServiceWithPolicy(context.Background(), publicKey, service, s.SecurityPolicy)
}
