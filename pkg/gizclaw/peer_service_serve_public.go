package gizclaw

import (
	"context"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/serverpublic"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizhttp"
)

func (s *PeerService) servePublic(conn *giznet.Conn) error {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(ctx *fiber.Ctx) error {
		base := ctx.UserContext()
		if base == nil {
			base = context.Background()
		}
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
		if (ctx.Method() == http.MethodGet && ctx.Path() == "/server-info") ||
			(ctx.Method() == http.MethodPost && ctx.Path() == "/login") {
			return ctx.Next()
		}
		publicKey, ok := authenticateFiberSession(ctx, sessions)
		if !ok {
			return nil
		}
		base := ctx.UserContext()
		if base == nil {
			base = context.Background()
		}
		ctx.SetUserContext(serverpublic.WithCallerPublicKey(base, publicKey))
		return ctx.Next()
	})
	serverpublic.RegisterHandlers(app, serverpublic.NewStrictHandler(s.public, nil))
	return fiberHTTPHandler(app)
}
