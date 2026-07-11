package gizclaw

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
)

func TestPublicFiberAdapterServerInfo(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(ctx *fiber.Ctx) error {
		base := ctx.UserContext()
		if base == nil {
			base = context.Background()
		}
		ctx.SetUserContext(peerhttp.WithCallerPublicKey(base, giznet.PublicKey{1}))
		return ctx.Next()
	})
	peerhttp.RegisterHandlers(app, peerhttp.NewStrictHandler(&peerHTTP{
		PeerHTTPService: &peer.Server{
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
		public: &peerHTTP{
			PeerHTTPService: &peer.Server{
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

	req = httptest.NewRequest(http.MethodOptions, "/me/status", nil)
	req.Header.Set("Origin", "wails://wails.localhost")
	req.Header.Set("Access-Control-Request-Method", http.MethodPut)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-public-key")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS /me/status status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPut) {
		t.Fatalf("Access-Control-Allow-Methods = %q, want PUT", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Public-Key") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want X-Public-Key", got)
	}
}

func TestPeerServicePublicHTTPHandlerAddsCORSHeaders(t *testing.T) {
	service := &PeerService{
		public: &peerHTTP{
			PeerHTTPService: &peer.Server{
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

	conn, serverConn := newTestWebRTCConnPair(t, serverKey, clientKey,
		testGiznetSecurityPolicy{
			allowService: func(_ giznet.PublicKey, service uint64) bool {
				return service == ServicePeerHTTP
			},
		},
		testGiznetSecurityPolicy{})
	defer conn.Close()
	defer serverConn.Close()

	peersServer := &peer.Server{
		BuildCommit:     "test-build",
		ServerPublicKey: serverKey.Public,
	}
	service := &PeerService{
		manager: NewManager(peersServer),
		public: &peerHTTP{
			PeerHTTPService: peersServer,
		},
	}
	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- service.servePublic(serverConn)
	}()

	client := &http.Client{Transport: gizhttp.NewRoundTripper(conn, ServicePeerHTTP)}
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

func TestPeerServiceEdgePublicRequiresActiveClientPeer(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	peersServer := &peer.Server{
		Store:           mustBadgerInMemory(t, nil),
		BuildCommit:     "test-build",
		ServerPublicKey: serverKey.Public,
	}
	loginServer := publiclogin.NewServer(serverKey, mustBadgerInMemory(t, nil))
	service := &PeerService{
		manager:  NewManager(peersServer),
		sessions: loginServer.SessionManager(),
		public: &peerHTTP{
			PeerHTTPService: peersServer,
			Self:            peersServer,
		},
	}
	handler := service.edgePublicHTTPHandler(service.sessions)

	tests := []struct {
		name       string
		role       apitypes.PeerRole
		status     apitypes.PeerRegistrationStatus
		wantStatus int
	}{
		{name: "client", role: apitypes.PeerRoleClient, status: apitypes.PeerRegistrationStatusActive, wantStatus: http.StatusOK},
		{name: "admin", role: apitypes.PeerRoleAdmin, status: apitypes.PeerRegistrationStatusActive, wantStatus: http.StatusForbidden},
		{name: "server", role: apitypes.PeerRoleServer, status: apitypes.PeerRegistrationStatusActive, wantStatus: http.StatusForbidden},
		{name: "edge", role: apitypes.PeerRoleEdgeNode, status: apitypes.PeerRegistrationStatusActive, wantStatus: http.StatusForbidden},
		{name: "blocked client", role: apitypes.PeerRoleClient, status: apitypes.PeerRegistrationStatusBlocked, wantStatus: http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keyPair, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatalf("GenerateKeyPair(peer) error = %v", err)
			}
			if _, err := peersServer.SavePeer(context.Background(), apitypes.Peer{
				PublicKey:     keyPair.Public.String(),
				Role:          tc.role,
				Status:        tc.status,
				Device:        apitypes.DeviceInfo{},
				Configuration: apitypes.Configuration{},
			}); err != nil {
				t.Fatalf("SavePeer error = %v", err)
			}

			accessToken := issuePeerHTTPSession(t, loginServer, keyPair, serverKey.Public)
			req := httptest.NewRequest(http.MethodGet, "/me", nil)
			req.Header.Set(publiclogin.PublicKeyHeader, keyPair.Public.String())
			req.Header.Set("Authorization", "Bearer "+accessToken)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), tc.wantStatus)
			}
		})
	}
}

func issuePeerHTTPSession(t testing.TB, loginServer *publiclogin.Server, keyPair *giznet.KeyPair, serverPublicKey giznet.PublicKey) string {
	t.Helper()

	assertion, err := publiclogin.NewLoginAssertion(keyPair, serverPublicKey, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	resp, err := loginServer.Login(context.Background(), peerhttp.LoginRequestObject{
		Params: peerhttp.LoginParams{
			XPublicKey:    keyPair.Public.String(),
			Authorization: "Bearer " + assertion,
		},
	})
	if err != nil {
		t.Fatalf("Login error = %v", err)
	}
	ok, isOK := resp.(peerhttp.Login200JSONResponse)
	if !isOK {
		t.Fatalf("Login response = %T", resp)
	}
	return ok.AccessToken
}

func TestPeerServiceEdgePublicRoundTrip(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(edge) error = %v", err)
	}

	conn, serverConn := newTestWebRTCConnPair(t, serverKey, edgeKey,
		testGiznetSecurityPolicy{
			allowService: func(_ giznet.PublicKey, service uint64) bool {
				return service == ServiceEdgeHTTP
			},
		},
		testGiznetSecurityPolicy{})
	defer conn.Close()
	defer serverConn.Close()

	peersServer := &peer.Server{
		BuildCommit:     "test-build",
		ServerPublicKey: serverKey.Public,
	}
	service := &PeerService{
		manager: NewManager(peersServer),
		public: &peerHTTP{
			PeerHTTPService: peersServer,
		},
	}
	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- service.serveEdgePublic(serverConn)
	}()

	client := &http.Client{Transport: gizhttp.NewRoundTripper(conn, ServiceEdgeHTTP)}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://gizclaw/server-info", nil)
	if err != nil {
		t.Fatalf("http.NewRequest error = %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		select {
		case serveErr := <-serveErrCh:
			t.Fatalf("client.Do error = %v; serveEdgePublic error = %v", err, serveErr)
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
