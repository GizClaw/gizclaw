package gizclaw

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/gofiber/fiber/v2"
)

type signedPeerTargetKey struct{}

type signedPeerAccess struct {
	caller, target giznet.PublicKey
	role           apitypes.PeerRole
	assignment     apitypes.PeerAssignment
}

// Both Edge route resolution and final HTTP authorization check live authority.
func (s *PeerService) resolveSignedPeerAccess(ctx context.Context, token string) (signedPeerAccess, error) {
	access := signedPeerAccess{}
	caller, target, err := peerhttp.VerifySignedBearerToken(token, time.Now())
	if err != nil {
		return access, apikey.ErrInvalidAPIKey
	}
	m := s.manager
	if m == nil || m.Peers == nil || m.PeerRoutes == nil {
		return access, errors.New("Peer authorization services unavailable")
	}
	if err := m.Peers.EnsureAvailable(ctx, caller); err != nil {
		return access, err
	}
	record, err := m.Peers.LoadPeer(ctx, caller)
	if err != nil {
		return access, err
	}
	admin := record.Role == apitypes.PeerRoleAdmin || s.allowAdminHTTP != nil && s.allowAdminHTTP(caller)
	if record.Status != apitypes.PeerRegistrationStatusActive || !admin && record.Role != apitypes.PeerRoleClient {
		return access, errAPIKeyOwnerUnavailable
	}
	role := apitypes.PeerRoleAdmin
	if !admin {
		if target != caller {
			return access, errAPIKeyOwnerUnavailable
		}
		if err := s.validateAPIKeyOwner(ctx, caller); err != nil {
			return access, err
		}
		role = apitypes.PeerRoleClient
	}
	assignment, err := m.PeerRoutes.Resolve(ctx, target)
	if errors.Is(err, peerroute.ErrAssignmentNotFound) || errors.Is(err, peerroute.ErrPeerInactive) || errors.Is(err, peerroute.ErrPeerNotAssignable) {
		return access, errAPIKeyOwnerUnavailable
	}
	if err != nil {
		return access, err
	}
	return signedPeerAccess{caller: caller, target: target, role: role, assignment: assignment}, nil
}

func (s *PeerService) authorizeSignedPeerHTTP(ctx *fiber.Ctx, token string) error {
	ctx.Set("Cache-Control", "no-store")
	access, err := s.resolveSignedPeerAccess(ctx.UserContext(), token)
	if err != nil {
		if errors.Is(err, apikey.ErrInvalidAPIKey) {
			writeFiberAPIKeyError(ctx, err)
		} else {
			writeFiberAPIKeyOwnerError(ctx, err)
		}
		return nil
	}
	if !peerhttp.IsDebugDataPath(ctx.Path()) {
		writeFiberAPIKeyOwnerError(ctx, errAPIKeyOwnerUnavailable)
		return nil
	}
	if access.assignment.ServerPublicKey != s.manager.Peers.ServerPublicKey.String() {
		return ctx.Status(http.StatusServiceUnavailable).JSON(apiError("API_KEY_SERVER_UNAVAILABLE", "target Server unavailable"))
	}
	base := gizlog.WithPeerPublicKey(ctx.UserContext(), access.caller.String())
	base = peerhttp.WithCallerPublicKey(base, access.caller)
	ctx.SetUserContext(context.WithValue(base, signedPeerTargetKey{}, access.target))
	if trusted, _ := base.Value(trustedEdgeHTTPKey{}).(bool); trusted {
		ctx.Set(gizlog.AuthenticatedPeerHeader, access.caller.String())
	}
	observability.SetPeer(ctx.UserContext(), access.caller.String(), string(access.role))
	return ctx.Next()
}
