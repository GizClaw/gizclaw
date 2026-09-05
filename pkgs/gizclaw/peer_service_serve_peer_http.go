package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (s *PeerService) servePublic(conn giznet.Conn) error {
	return s.servePublicWithRetiring(conn, nil)
}

func (s *PeerService) servePublicWithRetiring(conn giznet.Conn, isRetiring func() bool) error {
	return s.servePublicService(conn, ServicePeerHTTP, isRetiring)
}

func (s *PeerService) serveEdgePublic(conn giznet.Conn) error {
	return s.serveEdgePublicWithRetiring(conn, nil)
}

func (s *PeerService) serveEdgePublicWithRetiring(conn giznet.Conn, isRetiring func() bool) error {
	server := gizhttp.NewServer(conn, ServiceEdgeHTTP, rejectRetiringHTTP(isRetiring, s.edgeHTTPHandlerForPeer(s.apiKeys, conn.PublicKey().String())))
	defer func() { _ = server.Shutdown(context.Background()) }()
	return server.Serve()
}

func (s *PeerService) servePublicService(conn giznet.Conn, service uint64, isRetiring func() bool) error {
	handler := s.publicHTTPHandler(s.apiKeys)
	surface := observability.SurfacePeerHTTP
	if service == ServiceEdgeHTTP {
		surface = observability.SurfaceEdgeHTTP
	}
	handler = rejectRetiringHTTP(isRetiring, observeHTTPHandler(handler, httpObservationOptions{surface: surface, peerPublicKey: conn.PublicKey().String()}))
	server := gizhttp.NewServer(conn, service, handler)
	defer func() { _ = server.Shutdown(context.Background()) }()
	return server.Serve()
}

func rejectRetiringHTTP(isRetiring func() bool, next http.Handler) http.Handler {
	if isRetiring == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isRetiring() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(apitypes.NewErrorResponse(runtimepeer.PeerPendingDeletionCode, ErrPeerConnRetiring.Error()))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type publicHTTPOptions struct{ requireClientPeer bool }

var errAPIKeyOwnerUnavailable = errors.New("API key owner is unavailable")

func (s *PeerService) publicHTTPHandler(apiKeys *apikey.Server) http.Handler {
	return s.publicHTTPHandlerWithOptions(apiKeys, publicHTTPOptions{})
}

func (s *PeerService) edgePublicHTTPHandler(apiKeys *apikey.Server) http.Handler {
	return s.publicHTTPHandlerWithOptions(apiKeys, publicHTTPOptions{requireClientPeer: true})
}

func (s *PeerService) edgeHTTPHandler(apiKeys *apikey.Server) http.Handler {
	return s.edgeHTTPHandlerForPeer(apiKeys, "")
}

func (s *PeerService) edgeHTTPHandlerForPeer(apiKeys *apikey.Server, peerPublicKey string) http.Handler {
	mux := http.NewServeMux()
	publicHandler := s.edgePublicHTTPHandler(apiKeys)
	mux.Handle("/server-info", publicHandler)
	mux.Handle("/webrtc/v1/offer", publicHandler)
	mux.Handle("/gizclaw/v1/", publicHandler)
	mux.Handle("/openai/v1/", s.edgeOpenAIHTTPHandler(apiKeys))
	return observeHTTPHandler(mux, httpObservationOptions{surface: observability.SurfaceEdgeHTTP, peerPublicKey: peerPublicKey, peerRole: string(apitypes.PeerRoleEdgeNode)})
}

func (s *PeerService) edgeOpenAIHTTPHandler(apiKeys *apikey.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setPublicHTTPCORSHeaders(w.Header(), r.Header.Get("Origin"))
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		principal, ok := authenticateHTTPAPIKey(w, r, apiKeys)
		if !ok {
			return
		}
		publicKey, err := parsePeerPublicKey(principal.Key.Owner)
		if err != nil {
			writeHTTPAPIKeyError(w, err)
			return
		}
		if err := s.validateAPIKeyOwner(r.Context(), publicKey); err != nil {
			writeHTTPAPIKeyOwnerError(w, err)
			return
		}
		http.StripPrefix("/openai", s.openAIHTTPHandlerForPeer(publicKey, nil, s.peerResourcesForAPIKey(publicKey))).ServeHTTP(w, r)
	})
}

func (s *PeerService) publicHTTPHandlerWithOptions(apiKeys *apikey.Server, opts publicHTTPOptions) http.Handler {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(ctx *fiber.Ctx) error {
		base := ctx.UserContext()
		if base == nil {
			base = context.Background()
		}
		setPeerHTTPCORSHeaders(ctx)
		if ctx.Method() == http.MethodOptions && isPeerHTTPPath(ctx.Path()) {
			return ctx.SendStatus(http.StatusNoContent)
		}
		base = withPeerHTTPContentType(base, ctx.Get(fiber.HeaderContentType))
		ctx.SetUserContext(base)
		if opts.requireClientPeer && ctx.Method() == http.MethodPost && ctx.Path() == "/webrtc/v1/offer" {
			publicKey, ok := s.edgeSignalingPublicKey(ctx)
			if !ok {
				return nil
			}
			if !s.allowEdgeSignalingPeer(ctx.UserContext(), publicKey) {
				ctx.Status(http.StatusForbidden)
				return ctx.JSON(apitypes.NewErrorResponse("EDGE_CLIENT_REQUIRED", "edge public HTTP only proxies client peers"))
			}
			return ctx.Next()
		}
		if peerhttp.IsIdentifierLookup(ctx.Method(), ctx.Path()) {
			ctx.Set("Cache-Control", "no-store")
			return ctx.Next()
		}
		if isUnauthenticatedPeerHTTPRoute(ctx.Method(), ctx.Path()) {
			return ctx.Next()
		}
		if !strings.HasPrefix(ctx.Path(), "/gizclaw/v1/") {
			return ctx.Next()
		}
		normalizeOptionalJSONBody(ctx)
		if ctx.Get(fiber.HeaderAuthorization) == "" && ctx.Query("public_key") != "" && peerhttp.IsDebugDataPath(ctx.Path()) {
			publicKey, err := parsePeerPublicKey(ctx.Query("public_key"))
			if err != nil {
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(apitypes.NewErrorResponse("INVALID_REQUEST", "invalid public key"))
			}
			if err := s.validateAPIKeyOwner(ctx.UserContext(), publicKey); err != nil {
				writeFiberAPIKeyOwnerError(ctx, err)
				return nil
			}
			item, err := s.manager.Peers.LoadPeer(ctx.UserContext(), publicKey)
			if err != nil {
				writeFiberAPIKeyOwnerError(ctx, err)
				return nil
			}
			mode := "off"
			if item.Device.DebugMode != nil {
				mode = *item.Device.DebugMode
			}
			if mode != "fullcontrol" && !(mode == "readonly" && ctx.Method() == http.MethodGet) {
				ctx.Status(http.StatusForbidden)
				return ctx.JSON(apitypes.NewErrorResponse("DEBUG_ACCESS_FORBIDDEN", "device debug mode does not permit this operation"))
			}
			ctx.Set("Cache-Control", "no-store")
			ctx.SetUserContext(peerhttp.WithCallerPublicKey(base, publicKey))
			observability.SetPeer(ctx.UserContext(), publicKey.String(), "")
			return ctx.Next()
		}
		principal, ok := authenticateFiberAPIKey(ctx, apiKeys)
		if !ok {
			return nil
		}
		publicKey, err := parsePeerPublicKey(principal.Key.Owner)
		if err != nil {
			writeFiberAPIKeyError(ctx, err)
			return nil
		}
		if err := s.validateAPIKeyOwner(ctx.UserContext(), publicKey); err != nil {
			writeFiberAPIKeyOwnerError(ctx, err)
			return nil
		}
		ctx.SetUserContext(peerhttp.WithCallerPublicKey(apikey.WithPrincipal(base, principal), publicKey))
		observability.SetPeer(ctx.UserContext(), publicKey.String(), "")
		return ctx.Next()
	})
	app.Use(observeFiberRoute)
	peerhttp.RegisterHandlers(app, peerhttp.NewStrictHandler(s.public, nil))
	return fiberHTTPHandler(app)
}

func (s *PeerService) validateAPIKeyOwner(ctx context.Context, publicKey giznet.PublicKey) error {
	if s == nil || s.manager == nil || s.manager.Peers == nil || s.manager.RuntimeProfiles == nil {
		return errors.New("API key owner services are not configured")
	}
	if err := s.manager.Peers.EnsureAvailable(ctx, publicKey); err != nil {
		return err
	}
	item, err := s.manager.Peers.LoadPeer(ctx, publicKey)
	if err != nil {
		return err
	}
	if item.Status != apitypes.PeerRegistrationStatusActive || item.Role != apitypes.PeerRoleClient {
		return errAPIKeyOwnerUnavailable
	}
	if _, err := s.manager.RuntimeProfiles.ResolveOwnerProfile(ctx, publicKey.String()); err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return errAPIKeyOwnerUnavailable
		}
		return err
	}
	return nil
}

func writeFiberAPIKeyOwnerError(ctx *fiber.Ctx, err error) {
	status, code := apiKeyOwnerHTTPError(err)
	ctx.Status(status)
	_ = ctx.JSON(apitypes.NewErrorResponse(code, http.StatusText(status)))
}

func writeHTTPAPIKeyOwnerError(w http.ResponseWriter, err error) {
	status, code := apiKeyOwnerHTTPError(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apitypes.NewErrorResponse(code, http.StatusText(status)))
}

func apiKeyOwnerHTTPError(err error) (int, string) {
	if errors.Is(err, runtimepeer.ErrPeerPendingDeletion) {
		return http.StatusConflict, runtimepeer.PeerPendingDeletionCode
	}
	if errors.Is(err, runtimepeer.ErrPeerNotFound) || errors.Is(err, runtimepeer.ErrPeerDeleted) || errors.Is(err, errAPIKeyOwnerUnavailable) {
		return http.StatusForbidden, "API_KEY_OWNER_UNAVAILABLE"
	}
	return http.StatusInternalServerError, "INTERNAL_ERROR"
}

func (s *PeerService) allowEdgeSignalingPeer(ctx context.Context, publicKey giznet.PublicKey) bool {
	if s == nil || s.manager == nil || s.manager.Peers == nil {
		return false
	}
	item, err := s.manager.Peers.LoadPeer(ctx, publicKey)
	if errors.Is(err, runtimepeer.ErrPeerNotFound) {
		return true
	}
	return err == nil && item.Status == apitypes.PeerRegistrationStatusActive && item.Role == apitypes.PeerRoleClient
}

func (s *PeerService) edgeSignalingPublicKey(ctx *fiber.Ctx) (giznet.PublicKey, bool) {
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(ctx.Get("X-Giznet-Public-Key"))); err != nil || publicKey.IsZero() {
		ctx.Status(http.StatusBadRequest)
		_ = ctx.JSON(apitypes.NewErrorResponse("INVALID_PUBLIC_KEY", "invalid X-Giznet-Public-Key"))
		return giznet.PublicKey{}, false
	}
	return publicKey, true
}

// normalizeOptionalJSONBody lets routes with an optional JSON request body
// accept an empty POST: the generated strict handler only tolerates a JSON
// body, so an absent body becomes the empty object.
func normalizeOptionalJSONBody(ctx *fiber.Ctx) {
	if ctx.Method() != http.MethodPost ||
		(ctx.Path() != "/gizclaw/v1/device/actions/reboot" && ctx.Path() != "/gizclaw/v1/device/wifi/scan") ||
		len(ctx.Body()) != 0 {
		return
	}
	ctx.Request().Header.SetContentType(fiber.MIMEApplicationJSON)
	ctx.Request().SetBodyString("{}")
}

func setPeerHTTPCORSHeaders(ctx *fiber.Ctx) {
	origin := ctx.Get(fiber.HeaderOrigin)
	if origin == "" {
		origin = "*"
	} else {
		ctx.Vary(fiber.HeaderOrigin)
	}
	ctx.Set(fiber.HeaderAccessControlAllowOrigin, origin)
	ctx.Set(fiber.HeaderAccessControlAllowMethods, "GET,POST,PUT,DELETE,OPTIONS")
	ctx.Set(fiber.HeaderAccessControlAllowHeaders, "Authorization,Content-Type,X-Giznet-Nonce,X-Giznet-Public-Key,X-Giznet-Timestamp,X-Request-ID")
	ctx.Set(fiber.HeaderAccessControlExposeHeaders, "Content-Length,Content-Type,X-Request-ID")
}

func isPeerHTTPPath(path string) bool {
	return path == "/server-info" || path == "/webrtc/v1/offer" || strings.HasPrefix(path, "/gizclaw/v1/")
}

func isUnauthenticatedPeerHTTPRoute(method, path string) bool {
	return (method == http.MethodGet && path == "/server-info") || (method == http.MethodPost && path == "/webrtc/v1/offer")
}
