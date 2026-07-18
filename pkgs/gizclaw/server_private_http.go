package gizclaw

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

var ErrPrivateHTTPIngressDenied = errors.New("gizclaw: private http ingress denied")

func (s *Server) AuthenticateHTTPSessionHeaders(authorization, publicKeyHeader string) (giznet.PublicKey, error) {
	principal, err := s.AuthenticateHTTPSessionPrincipalHeaders(authorization, publicKeyHeader)
	if err != nil {
		return giznet.PublicKey{}, err
	}
	return principal.PublicKey, nil
}

// AuthenticateHTTPSessionPrincipalHeaders resolves the typed principal for a public HTTP bearer.
func (s *Server) AuthenticateHTTPSessionPrincipalHeaders(authorization, publicKeyHeader string) (publiclogin.Principal, error) {
	if s == nil || s.sessions == nil {
		return publiclogin.Principal{}, errors.New("gizclaw: session manager not configured")
	}
	return s.sessions.AuthenticateHeadersPrincipal(authorization, publicKeyHeader)
}

func (s *Server) AuthorizePrivateHTTPIngress(ctx context.Context, publicKey giznet.PublicKey) error {
	if s == nil || s.manager == nil {
		return ErrPrivateHTTPIngressDenied
	}
	if s.manager.allowService(ctx, publicKey, ServiceAdminHTTP) {
		return nil
	}
	if s.manager.Peers == nil {
		return ErrPrivateHTTPIngressDenied
	}
	peer, err := s.manager.Peers.LoadPeer(ctx, publicKey)
	if err != nil {
		return ErrPrivateHTTPIngressDenied
	}
	if peer.Status == apitypes.PeerRegistrationStatusActive &&
		(peer.Role == apitypes.PeerRoleServer || peer.Role == apitypes.PeerRoleEdgeNode) {
		return nil
	}
	return ErrPrivateHTTPIngressDenied
}

func PrivateHTTPIngressLoginAuthorizer(s *Server) publiclogin.SessionAuthorizer {
	return func(ctx context.Context, publicKey giznet.PublicKey) error {
		return s.AuthorizePrivateHTTPIngress(ctx, publicKey)
	}
}
