package gizclaw

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/gearservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/serverpublic"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

const publicKeyHeader = "X-Public-Key"

// PublicHTTPHandler exposes server-public and gear APIs over ordinary HTTP.
// Requests that mutate or read gear state authenticate with Bearer sessions
// issued by /api/public/login.
func (s *Server) PublicHTTPHandler() (http.Handler, error) {
	if s == nil {
		return nil, fmt.Errorf("gizclaw: nil server")
	}
	if s.KeyPair == nil {
		return nil, fmt.Errorf("gizclaw: nil key pair")
	}
	if err := s.init(); err != nil {
		return nil, err
	}
	peerService := s.peerService
	sessions := s.sessions
	if peerService == nil || sessions == nil {
		return nil, fmt.Errorf("gizclaw: public http runtime not initialized")
	}

	mux := http.NewServeMux()
	mux.Handle("/api/public/", http.StripPrefix("/api/public", peerService.publicHTTPHandler(sessions)))
	mux.Handle("/api/gear/", http.StripPrefix("/api/gear", peerService.gearHTTPHandler(sessions)))
	mux.HandleFunc("/api/public", redirectProxyPrefix("/api/public/"))
	mux.HandleFunc("/api/gear", redirectProxyPrefix("/api/gear/"))
	return mux, nil
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

func (s *PeerService) gearHTTPHandler(sessions *publiclogin.SessionManager) http.Handler {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(ctx *fiber.Ctx) error {
		publicKey, ok := authenticateFiberSession(ctx, sessions)
		if !ok {
			return nil
		}
		base := ctx.UserContext()
		if base == nil {
			base = context.Background()
		}
		ctx.SetUserContext(gearservice.WithCallerPublicKey(base, publicKey))
		return ctx.Next()
	})
	registerGearDownloadHandler(app, s.gear)
	return fiberHTTPHandler(app)
}

func authenticateFiberSession(ctx *fiber.Ctx, sessions *publiclogin.SessionManager) (giznet.PublicKey, bool) {
	publicKey, err := sessions.Authenticate(ctx.Get("Authorization"))
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		_ = ctx.JSON(map[string]any{
			"error": map[string]string{
				"code":    "INVALID_SESSION",
				"message": "missing or invalid bearer session",
			},
		})
		return giznet.PublicKey{}, false
	}
	if headerPublicKey := ctx.Get(publicKeyHeader); headerPublicKey != "" && headerPublicKey != publicKey.String() {
		ctx.Status(http.StatusUnauthorized)
		_ = ctx.JSON(map[string]any{
			"error": map[string]string{
				"code":    "PUBLIC_KEY_MISMATCH",
				"message": "x-public-key does not match bearer session",
			},
		})
		return giznet.PublicKey{}, false
	}
	return publicKey, true
}
