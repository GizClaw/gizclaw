package gizclaw

import (
	"context"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/serverpublic"
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
		base = withServerPublicContentType(base, ctx.Get(fiber.HeaderContentType))
		ctx.SetUserContext(serverpublic.WithCallerPublicKey(base, conn.PublicKey()))
		return ctx.Next()
	})
	serverpublic.RegisterHandlers(app, serverpublic.NewStrictHandler(s.public, nil))

	server := gizhttp.NewServer(conn, ServiceServerPublic, fiberHTTPHandler(app))
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
		setServerPublicCORSHeaders(ctx)
		if ctx.Method() == http.MethodOptions && isServerPublicHTTPPath(ctx.Path()) {
			return ctx.SendStatus(http.StatusNoContent)
		}
		base = withServerPublicContentType(base, ctx.Get(fiber.HeaderContentType))
		ctx.SetUserContext(base)
		if isUnauthenticatedServerPublicHTTPRoute(ctx.Method(), ctx.Path()) {
			return ctx.Next()
		}
		publicKey, ok := authenticateFiberSession(ctx, sessions)
		if !ok {
			return nil
		}
		ctx.SetUserContext(serverpublic.WithCallerPublicKey(base, publicKey))
		return ctx.Next()
	})
	serverpublic.RegisterHandlers(app, serverpublic.NewStrictHandler(s.public, nil))
	return fiberHTTPHandler(app)
}

func setServerPublicCORSHeaders(ctx *fiber.Ctx) {
	ctx.Set(fiber.HeaderAccessControlAllowOrigin, "*")
	ctx.Set(fiber.HeaderAccessControlAllowMethods, "GET,POST,OPTIONS")
	ctx.Set(fiber.HeaderAccessControlAllowHeaders, "Authorization,Content-Type,X-Giznet-Nonce,X-Giznet-Public-Key,X-Giznet-Timestamp")
	ctx.Set(fiber.HeaderAccessControlExposeHeaders, "Content-Length,Content-Type")
}

func isServerPublicHTTPPath(path string) bool {
	switch path {
	case "/login", "/server-info", "/webrtc/v1/offer":
		return true
	default:
		return false
	}
}

func isUnauthenticatedServerPublicHTTPRoute(method, path string) bool {
	return (method == http.MethodGet && path == "/server-info") ||
		(method == http.MethodPost && path == "/login") ||
		(method == http.MethodPost && path == "/webrtc/v1/offer")
}
