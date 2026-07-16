package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
)

func (s *PeerService) servePublic(conn giznet.Conn) error {
	return s.servePublicService(conn, ServicePeerHTTP)
}

func (s *PeerService) serveEdgePublic(conn giznet.Conn) error {
	server := gizhttp.NewServer(conn, ServiceEdgeHTTP, s.edgeHTTPHandlerForPeer(s.sessions, conn.PublicKey().String()))
	defer func() {
		_ = server.Shutdown(context.Background())
	}()
	return server.Serve()
}

func (s *PeerService) servePublicService(conn giznet.Conn, service uint64) error {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(observeFiberRoute)
	app.Use(func(ctx *fiber.Ctx) error {
		base := ctx.UserContext()
		if base == nil {
			base = context.Background()
		}
		base = withPeerHTTPContentType(base, ctx.Get(fiber.HeaderContentType))
		ctx.SetUserContext(peerhttp.WithCallerPublicKey(base, conn.PublicKey()))
		return ctx.Next()
	})
	peerhttp.RegisterHandlers(app, peerhttp.NewStrictHandler(s.public, nil))

	surface := observability.SurfacePeerHTTP
	if service == ServiceEdgeHTTP {
		surface = observability.SurfaceEdgeHTTP
	}
	handler := observeHTTPHandler(fiberHTTPHandler(app), httpObservationOptions{
		surface:       surface,
		peerPublicKey: conn.PublicKey().String(),
	})
	server := gizhttp.NewServer(conn, service, handler)
	defer func() {
		_ = server.Shutdown(context.Background())
	}()
	return server.Serve()
}

type publicHTTPOptions struct {
	requireClientPeer bool
	login             publiclogin.PeerHTTP
}

func (s *PeerService) publicHTTPHandler(sessions *publiclogin.SessionManager) http.Handler {
	return s.publicHTTPHandlerWithOptions(sessions, publicHTTPOptions{})
}

func (s *PeerService) edgePublicHTTPHandler(sessions *publiclogin.SessionManager) http.Handler {
	return s.publicHTTPHandlerWithOptions(sessions, publicHTTPOptions{requireClientPeer: true})
}

func (s *PeerService) edgeHTTPHandler(sessions *publiclogin.SessionManager) http.Handler {
	return s.edgeHTTPHandlerForPeer(sessions, "")
}

func (s *PeerService) edgeHTTPHandlerForPeer(sessions *publiclogin.SessionManager, peerPublicKey string) http.Handler {
	mux := http.NewServeMux()
	publicHandler := s.edgePublicHTTPHandler(sessions)
	mux.Handle("/login", s.edgeLoginHTTPHandler(sessions))
	mux.Handle("/server-info", publicHandler)
	mux.Handle("/webrtc/v1/offer", publicHandler)
	mux.Handle("/me", publicHandler)
	mux.Handle("/me/status", publicHandler)
	mux.Handle("/me/runtime", publicHandler)
	mux.Handle("/openai/v1/", s.edgeOpenAIHTTPHandler(sessions))
	return observeHTTPHandler(mux, httpObservationOptions{
		surface:       observability.SurfaceEdgeHTTP,
		peerPublicKey: peerPublicKey,
		peerRole:      string(apitypes.PeerRoleEdgeNode),
	})
}

type loginWithoutAuthorizer interface {
	LoginWithoutAuthorizer(context.Context, peerhttp.LoginRequestObject) (peerhttp.LoginResponseObject, error)
}

type edgeLoginPeerHTTP struct {
	publiclogin.PeerHTTP
	allowClientPeer func(context.Context, giznet.PublicKey) bool
}

func (h edgeLoginPeerHTTP) Login(ctx context.Context, request peerhttp.LoginRequestObject) (peerhttp.LoginResponseObject, error) {
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(request.Params.XPublicKey)); err != nil || publicKey.IsZero() {
		return peerhttp.Login401JSONResponse(apitypes.NewErrorResponse("INVALID_PUBLIC_KEY", "invalid X-Public-Key")), nil
	}
	if h.allowClientPeer == nil || !h.allowClientPeer(ctx, publicKey) {
		return peerhttp.Login401JSONResponse(apitypes.NewErrorResponse("EDGE_CLIENT_REQUIRED", "edge public HTTP only proxies active client peers")), nil
	}
	if login, ok := h.PeerHTTP.(loginWithoutAuthorizer); ok {
		return login.LoginWithoutAuthorizer(ctx, request)
	}
	return h.PeerHTTP.Login(ctx, request)
}

func (s *PeerService) edgeLoginHTTPHandler(sessions *publiclogin.SessionManager) http.Handler {
	var login publiclogin.PeerHTTP
	if s != nil && s.public != nil && s.public.PeerHTTP != nil {
		login = edgeLoginPeerHTTP{
			PeerHTTP:        s.public.PeerHTTP,
			allowClientPeer: s.allowEdgeClientPeer,
		}
	}
	return s.publicHTTPHandlerWithOptions(sessions, publicHTTPOptions{
		requireClientPeer: true,
		login:             login,
	})
}

func (s *PeerService) edgeOpenAIHTTPHandler(sessions *publiclogin.SessionManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setPublicHTTPCORSHeaders(w.Header())
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		publicKey, ok := authenticateHTTPSession(w, r, sessions)
		if !ok {
			return
		}
		if !s.allowEdgeClientPeer(r.Context(), publicKey) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(apitypes.NewErrorResponse("EDGE_CLIENT_REQUIRED", "edge public HTTP only proxies active client peers"))
			return
		}
		http.StripPrefix("/openai", s.openAIHTTPHandlerForPeer(publicKey, nil)).ServeHTTP(w, r)
	})
}

func (s *PeerService) publicHTTPHandlerWithOptions(sessions *publiclogin.SessionManager, opts publicHTTPOptions) http.Handler {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(observeFiberRoute)
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
				_ = ctx.JSON(apitypes.NewErrorResponse("EDGE_CLIENT_REQUIRED", "edge public HTTP only proxies active client peers"))
				return nil
			}
			ctx.SetUserContext(peerhttp.WithCallerPublicKey(base, publicKey))
			return ctx.Next()
		}
		if isUnauthenticatedPeerHTTPRoute(ctx.Method(), ctx.Path()) {
			return ctx.Next()
		}
		publicKey, ok := authenticateFiberSession(ctx, sessions)
		if !ok {
			return nil
		}
		if opts.requireClientPeer && !s.allowEdgeClientPeer(ctx.UserContext(), publicKey) {
			ctx.Status(http.StatusForbidden)
			_ = ctx.JSON(apitypes.NewErrorResponse("EDGE_CLIENT_REQUIRED", "edge public HTTP only proxies active client peers"))
			return nil
		}
		ctx.SetUserContext(peerhttp.WithCallerPublicKey(base, publicKey))
		if !opts.requireClientPeer {
			observability.SetPeer(ctx.UserContext(), publicKey.String(), "")
		}
		return ctx.Next()
	})
	public := s.public
	if opts.login != nil && public != nil {
		copy := *public
		copy.PeerHTTP = opts.login
		public = &copy
	}
	peerhttp.RegisterHandlers(app, peerhttp.NewStrictHandler(public, nil))
	return fiberHTTPHandler(app)
}

func (s *PeerService) allowEdgeClientPeer(ctx context.Context, publicKey giznet.PublicKey) bool {
	if s == nil || s.manager == nil || s.manager.Peers == nil {
		return false
	}
	peer, err := s.manager.Peers.LoadPeer(ctx, publicKey)
	if err != nil {
		return false
	}
	return peer.Status == apitypes.PeerRegistrationStatusActive && peer.Role == apitypes.PeerRoleClient
}

func (s *PeerService) allowEdgeSignalingPeer(ctx context.Context, publicKey giznet.PublicKey) bool {
	if s == nil || s.manager == nil || s.manager.Peers == nil {
		return false
	}
	peer, err := s.manager.Peers.LoadPeer(ctx, publicKey)
	if errors.Is(err, runtimepeer.ErrPeerNotFound) {
		return true
	}
	if err != nil {
		return false
	}
	return peer.Status == apitypes.PeerRegistrationStatusActive && peer.Role == apitypes.PeerRoleClient
}

func (s *PeerService) edgeSignalingPublicKey(ctx *fiber.Ctx) (giznet.PublicKey, bool) {
	var publicKey giznet.PublicKey
	if ctx == nil {
		return publicKey, false
	}
	if err := publicKey.UnmarshalText([]byte(ctx.Get("X-Giznet-Public-Key"))); err != nil || publicKey.IsZero() {
		ctx.Status(http.StatusBadRequest)
		_ = ctx.JSON(apitypes.NewErrorResponse("INVALID_PUBLIC_KEY", "invalid X-Giznet-Public-Key"))
		return giznet.PublicKey{}, false
	}
	return publicKey, true
}

func setPeerHTTPCORSHeaders(ctx *fiber.Ctx) {
	ctx.Set(fiber.HeaderAccessControlAllowOrigin, "*")
	ctx.Set(fiber.HeaderAccessControlAllowMethods, "GET,POST,PUT,OPTIONS")
	ctx.Set(fiber.HeaderAccessControlAllowHeaders, "Authorization,Content-Type,X-Public-Key,X-Giznet-Nonce,X-Giznet-Public-Key,X-Giznet-Timestamp,X-Request-ID")
	ctx.Set(fiber.HeaderAccessControlExposeHeaders, "Content-Length,Content-Type,X-Request-ID")
}

func isPeerHTTPPath(path string) bool {
	switch path {
	case "/login", "/server-info", "/webrtc/v1/offer", "/me", "/me/status", "/me/runtime":
		return true
	default:
		return false
	}
}

func isUnauthenticatedPeerHTTPRoute(method, path string) bool {
	return (method == http.MethodGet && path == "/server-info") ||
		(method == http.MethodPost && path == "/login") ||
		(method == http.MethodPost && path == "/webrtc/v1/offer")
}
