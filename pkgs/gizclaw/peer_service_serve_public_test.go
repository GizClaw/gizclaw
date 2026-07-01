package gizclaw

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/serverpublic"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giznoise"
)

func TestPublicFiberAdapterServerInfo(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(ctx *fiber.Ctx) error {
		base := ctx.UserContext()
		if base == nil {
			base = context.Background()
		}
		ctx.SetUserContext(serverpublic.WithCallerPublicKey(base, giznet.PublicKey{1}))
		return ctx.Next()
	})
	serverpublic.RegisterHandlers(app, serverpublic.NewStrictHandler(&serverPublic{
		ServerPublicService: &peer.Server{
			BuildCommit:     "test-build",
			ServerPublicKey: giznet.PublicKey{1},
		},
	}, nil))

	req := httptest.NewRequest(http.MethodGet, "/server-info", nil)
	rec := httptest.NewRecorder()
	adaptor.FiberApp(app).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPeerServicePublicHTTPHandlerAllowsBrowserPreflight(t *testing.T) {
	service := &PeerService{
		public: &serverPublic{
			ServerPublicService: &peer.Server{
				BuildCommit:     "test-build",
				ServerPublicKey: giznet.PublicKey{1},
			},
		},
	}
	handler := service.publicHTTPHandler(nil)

	req := httptest.NewRequest(http.MethodOptions, "/webrtc/v1/offer", nil)
	req.Header.Set("Origin", "wails://wails.localhost")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type,x-giznet-nonce,x-giznet-public-key,x-giznet-timestamp")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatal("Access-Control-Allow-Headers is empty")
	}
}

func TestPeerServicePublicHTTPHandlerAddsCORSHeaders(t *testing.T) {
	service := &PeerService{
		public: &serverPublic{
			ServerPublicService: &peer.Server{
				BuildCommit:     "test-build",
				ServerPublicKey: giznet.PublicKey{1},
			},
		},
	}
	handler := service.publicHTTPHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/server-info", nil)
	req.Header.Set("Origin", "wails://wails.localhost")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
}

func TestPeerServicePublicRoundTrip(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}

	serverListener, err := (&giznoise.ListenConfig{
		Addr: "127.0.0.1:0",
		SecurityPolicy: testGiznetSecurityPolicy{
			allowService: func(_ giznet.PublicKey, service uint64) bool {
				return service == ServiceServerPublic
			},
		},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("giznoise.Listen(server) error = %v", err)
	}
	defer serverListener.Close()
	go drainUDP(serverListener.UDP())

	clientListener, err := (&giznoise.ListenConfig{
		Addr:           "127.0.0.1:0",
		SecurityPolicy: testGiznetSecurityPolicy{},
	}).Listen(clientKey)
	if err != nil {
		t.Fatalf("giznoise.Listen(client) error = %v", err)
	}
	defer clientListener.Close()
	go drainUDP(clientListener.UDP())

	connCh := make(chan giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := serverListener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()

	conn, err := clientListener.Dial(serverKey.Public, serverListener.HostInfo().Addr)
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer conn.Close()

	var serverConn giznet.Conn
	select {
	case serverConn = <-connCh:
	case err := <-errCh:
		t.Fatalf("Accept error = %v", err)
	}
	defer serverConn.Close()

	peersServer := &peer.Server{
		BuildCommit:     "test-build",
		ServerPublicKey: serverKey.Public,
	}
	service := &PeerService{
		manager: NewManager(peersServer),
		public: &serverPublic{
			ServerPublicService: peersServer,
		},
	}
	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- service.servePublic(serverConn)
	}()

	client := &http.Client{Transport: gizhttp.NewRoundTripper(conn, ServiceServerPublic)}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://gizclaw/server-info", nil)
	if err != nil {
		t.Fatalf("http.NewRequest error = %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		select {
		case serveErr := <-serveErrCh:
			t.Fatalf("client.Do error = %v; servePublic error = %v", err, serveErr)
		default:
		}
		t.Fatalf("client.Do error = %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.StatusCode, string(body))
	}
}
