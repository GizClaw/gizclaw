package gizclaw

import (
	"context"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
)

func (s *PeerService) servePublic(conn giznet.Conn) error {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
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

	server := gizhttp.NewServer(conn, ServicePeerHTTP, fiberHTTPHandler(app))
	defer func() {
		_ = server.Shutdown(context.Background())
	}()
	defer func() {
		_ = conn.Close()
	}()
	return server.Serve()
}

func (s *PeerService) publicHTTPHandler(sessions *publiclogin.SessionManager) http.Handler {
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
		if isUnauthenticatedPeerHTTPRoute(ctx.Method(), ctx.Path()) {
			return ctx.Next()
		}
		publicKey, ok := authenticateFiberSession(ctx, sessions)
		if !ok {
			return nil
		}
		ctx.SetUserContext(peerhttp.WithCallerPublicKey(base, publicKey))
		return ctx.Next()
	})
	peerhttp.RegisterHandlers(app, peerhttp.NewStrictHandler(s.public, nil))
	return fiberHTTPHandler(app)
}

func setPeerHTTPCORSHeaders(ctx *fiber.Ctx) {
	ctx.Set(fiber.HeaderAccessControlAllowOrigin, "*")
	ctx.Set(fiber.HeaderAccessControlAllowMethods, "GET,POST,PUT,OPTIONS")
	ctx.Set(fiber.HeaderAccessControlAllowHeaders, "Authorization,Content-Type,X-Public-Key,X-Giznet-Nonce,X-Giznet-Public-Key,X-Giznet-Timestamp")
	ctx.Set(fiber.HeaderAccessControlExposeHeaders, "Content-Length,Content-Type")
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
