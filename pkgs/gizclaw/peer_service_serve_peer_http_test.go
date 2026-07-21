package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
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
	req.Header.Set("Access-Control-Request-Headers", "content-type,x-registration-token,x-giznet-nonce,x-giznet-public-key,x-giznet-timestamp")
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
	} else if !strings.Contains(got, publiclogin.RegistrationTokenHeader) {
		t.Fatalf("Access-Control-Allow-Headers = %q, want %s", got, publiclogin.RegistrationTokenHeader)
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
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodDelete) {
		t.Fatalf("Access-Control-Allow-Methods = %q, want DELETE", got)
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
				PublicKey: keyPair.Public.String(),
				Role:      tc.role,
				Status:    tc.status,
				Device:    apitypes.DeviceInfo{},
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

func TestPeerServiceEdgeSignalingRequiresActiveClientPeer(t *testing.T) {
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
		{name: "client passes edge gate", role: apitypes.PeerRoleClient, status: apitypes.PeerRegistrationStatusActive, wantStatus: http.StatusBadRequest},
		{name: "admin forbidden", role: apitypes.PeerRoleAdmin, status: apitypes.PeerRegistrationStatusActive, wantStatus: http.StatusForbidden},
		{name: "server forbidden", role: apitypes.PeerRoleServer, status: apitypes.PeerRegistrationStatusActive, wantStatus: http.StatusForbidden},
		{name: "edge forbidden", role: apitypes.PeerRoleEdgeNode, status: apitypes.PeerRegistrationStatusActive, wantStatus: http.StatusForbidden},
		{name: "blocked client forbidden", role: apitypes.PeerRoleClient, status: apitypes.PeerRegistrationStatusBlocked, wantStatus: http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keyPair, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatalf("GenerateKeyPair(peer) error = %v", err)
			}
			if _, err := peersServer.SavePeer(context.Background(), apitypes.Peer{
				PublicKey: keyPair.Public.String(),
				Role:      tc.role,
				Status:    tc.status,
				Device:    apitypes.DeviceInfo{},
			}); err != nil {
				t.Fatalf("SavePeer error = %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/webrtc/v1/offer", strings.NewReader("offer"))
			req.Header.Set("X-Giznet-Public-Key", keyPair.Public.String())
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), tc.wantStatus)
			}
		})
	}

	t.Run("unknown client reaches signed offer handling", func(t *testing.T) {
		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair(peer) error = %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/webrtc/v1/offer", strings.NewReader("offer"))
		req.Header.Set("X-Giznet-Public-Key", keyPair.Public.String())
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), http.StatusBadRequest)
		}
	})

	req := httptest.NewRequest(http.MethodPost, "/webrtc/v1/offer", strings.NewReader("offer"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing public key status = %d body=%s, want %d", rec.Code, rec.Body.String(), http.StatusBadRequest)
	}
}

func TestPeerServiceEdgeLoginRequiresActiveClientBeforeBypass(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	peersServer := &peer.Server{
		Store:           mustBadgerInMemory(t, nil),
		BuildCommit:     "test-build",
		ServerPublicKey: serverKey.Public,
	}
	if _, err := peersServer.SavePeer(context.Background(), apitypes.Peer{
		PublicKey: clientKey.Public.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusActive,
		Device:    apitypes.DeviceInfo{},
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}
	loginServer := publiclogin.NewServer(serverKey, mustBadgerInMemory(t, nil))
	loginServer.SessionAuthorizer = func(context.Context, giznet.PublicKey) error {
		return errors.New("private ingress rejects clients")
	}
	service := &PeerService{
		manager:  NewManager(peersServer),
		sessions: loginServer.SessionManager(),
		public: &peerHTTP{
			PeerHTTPService: peersServer,
			PeerHTTP:        loginServer,
		},
	}
	handler := service.edgeHTTPHandler(service.sessions)

	login := func(t *testing.T, keyPair *giznet.KeyPair) int {
		t.Helper()
		assertion, err := publiclogin.NewLoginAssertion(keyPair, serverKey.Public, time.Minute)
		if err != nil {
			t.Fatalf("NewLoginAssertion error = %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.Header.Set(publiclogin.PublicKeyHeader, keyPair.Public.String())
		req.Header.Set("Authorization", "Bearer "+assertion)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := login(t, clientKey); got != http.StatusOK {
		t.Fatalf("active edge login status = %d, want %d", got, http.StatusOK)
	}

	unregisteredKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(unregistered) error = %v", err)
	}
	if got := login(t, unregisteredKey); got != http.StatusUnauthorized {
		t.Fatalf("unregistered edge login status = %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestPeerServiceEdgeSideControlUsesDeviceGrantForUnregisteredController(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	targetKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(target) error = %v", err)
	}
	controllerKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(controller) error = %v", err)
	}
	peersServer := &peer.Server{Store: mustBadgerInMemory(t, nil), ServerPublicKey: serverKey.Public}
	if _, err := peersServer.SavePeer(context.Background(), apitypes.Peer{
		PublicKey: targetKey.Public.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusActive,
		Device:    apitypes.DeviceInfo{},
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}
	loginServer := publiclogin.NewServer(serverKey, kv.NewMemory(nil))
	deviceToken, err := loginServer.SessionManager().CreateSideControlDeviceToken(context.Background(), targetKey.Public)
	if err != nil {
		t.Fatalf("CreateSideControlDeviceToken error = %v", err)
	}
	contacts := &contact.Server{Store: kv.NewMemory(nil)}
	service := &PeerService{
		manager:  NewManager(peersServer),
		sessions: loginServer.SessionManager(),
		public: &peerHTTP{
			PeerHTTPService: peersServer,
			Self:            peersServer,
			Status:          &peerrun.Server{Store: kv.NewMemory(nil)},
			Contacts:        contacts,
			PeerHTTP:        loginServer,
		},
	}
	handler := service.edgeHTTPHandler(service.sessions)
	assertion, err := publiclogin.NewLoginAssertion(controllerKey, serverKey.Public, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	loginBody, err := json.Marshal(peerhttp.LoginRequest{GrantType: peerhttp.SideControl, DeviceToken: deviceToken.Token})
	if err != nil {
		t.Fatalf("Marshal login body error = %v", err)
	}
	loginRequest := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(string(loginBody)))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRequest.Header.Set(publiclogin.PublicKeyHeader, controllerKey.Public.String())
	loginRequest.Header.Set("Authorization", "Bearer "+assertion)
	loginRecorder := httptest.NewRecorder()
	handler.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("side login status = %d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	var login peerhttp.LoginResult
	if err := json.Unmarshal(loginRecorder.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode login response error = %v", err)
	}
	if _, err := peersServer.LoadPeer(context.Background(), controllerKey.Public); !errors.Is(err, peer.ErrPeerNotFound) {
		t.Fatalf("side controller was registered as a peer: err=%v", err)
	}

	do := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+login.AccessToken)
		req.Header.Set(publiclogin.PublicKeyHeader, controllerKey.Public.String())
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if rec := do(http.MethodGet, "/side-control/info", ""); rec.Code != http.StatusOK {
		t.Fatalf("info status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/side-control/runtime", ""); rec.Code != http.StatusOK {
		t.Fatalf("runtime status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/side-control/status", ""); rec.Code != http.StatusOK {
		t.Fatalf("status status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/me", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("side session /me status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/openai/v1/models", ""); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"PRIMARY_SESSION_REQUIRED"`) {
		t.Fatalf("side session /openai status = %d body=%s", rec.Code, rec.Body.String())
	}
	createContact := do(http.MethodPost, "/side-control/contacts", `{"display_name":"Alice"}`)
	if createContact.Code != http.StatusCreated {
		t.Fatalf("create contact status = %d body=%s", createContact.Code, createContact.Body.String())
	}
	var createdContact rpcapi.ContactObject
	if err := json.Unmarshal(createContact.Body.Bytes(), &createdContact); err != nil || createdContact.Id == nil {
		t.Fatalf("decode created contact = %+v err=%v", createdContact, err)
	}
	contactPath := "/side-control/contacts/" + *createdContact.Id
	if rec := do(http.MethodGet, contactPath, ""); rec.Code != http.StatusOK {
		t.Fatalf("get contact status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPut, contactPath, `{"display_name":"Bob","phone_number":"10086"}`); rec.Code != http.StatusOK {
		t.Fatalf("put contact status = %d body=%s", rec.Code, rec.Body.String())
	}
	targetContacts, err := contacts.ListContacts(context.Background(), targetKey.Public.String(), rpcapi.ContactListRequest{})
	if err != nil || len(targetContacts.Items) != 1 || targetContacts.Items[0].DisplayName == nil || *targetContacts.Items[0].DisplayName != "Bob" {
		t.Fatalf("target contacts = %+v err=%v", targetContacts, err)
	}
	controllerContacts, err := contacts.ListContacts(context.Background(), controllerKey.Public.String(), rpcapi.ContactListRequest{})
	if err != nil || len(controllerContacts.Items) != 0 {
		t.Fatalf("controller contacts = %+v err=%v", controllerContacts, err)
	}
	if rec := do(http.MethodDelete, contactPath, ""); rec.Code != http.StatusOK {
		t.Fatalf("delete contact status = %d body=%s", rec.Code, rec.Body.String())
	}
	targetContacts, err = contacts.ListContacts(context.Background(), targetKey.Public.String(), rpcapi.ContactListRequest{})
	if err != nil || len(targetContacts.Items) != 0 {
		t.Fatalf("target contacts after delete = %+v err=%v", targetContacts, err)
	}

	sessions, err := loginServer.SessionManager().ListSideControlSessions(context.Background(), targetKey.Public)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("side sessions = %+v err=%v", sessions, err)
	}
	if err := loginServer.SessionManager().RevokeSideControlSession(context.Background(), targetKey.Public, sessions[0].Id); err != nil {
		t.Fatalf("RevokeSideControlSession error = %v", err)
	}
	if rec := do(http.MethodGet, "/side-control/info", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPeerServiceEdgeOpenAIRequiresActiveClientPeer(t *testing.T) {
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
	models := &model.Server{Store: kv.NewMemory(nil)}
	createdModel, err := models.CreateModel(context.Background(), adminhttp.CreateModelRequestObject{Body: &adminhttp.ModelUpsert{
		Id:     "profile-model",
		Kind:   apitypes.ModelKindLlm,
		Source: apitypes.ModelSourceManual,
		Provider: apitypes.ModelProvider{
			Kind: "openai-tenant",
			Name: "global",
		},
		ProviderData: mustOpenAIModelProviderData(t, "profile-model-upstream"),
	}})
	if err != nil {
		t.Fatalf("CreateModel error = %v", err)
	}
	if _, ok := createdModel.(adminhttp.CreateModel200JSONResponse); !ok {
		t.Fatalf("CreateModel response = %#v", createdModel)
	}
	profileModels := map[string]apitypes.RuntimeProfileBinding{
		"primary": {ResourceId: "profile-model", I18n: map[string]apitypes.RuntimeProfileI18nText{
			"en": {DisplayName: "Primary"}, "zh-CN": {DisplayName: "主要模型"},
		}},
	}
	runtimeProfiles := &runtimeprofile.Server{Store: kv.NewMemory(nil)}
	profileResponse, err := runtimeProfiles.PutRuntimeProfile(context.Background(), adminhttp.PutRuntimeProfileRequestObject{
		Name: "edge-runtime",
		Body: &adminhttp.RuntimeProfileUpsert{
			Name: "edge-runtime",
			Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Models: &profileModels}},
		},
	})
	if err != nil {
		t.Fatalf("PutRuntimeProfile error = %v", err)
	}
	if _, ok := profileResponse.(adminhttp.PutRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("PutRuntimeProfile response = %#v", profileResponse)
	}
	loginServer.RegistrationResolver = func(_ context.Context, rawToken string) (runtimeprofile.Registration, error) {
		if rawToken != "edge-runtime-token" {
			return runtimeprofile.Registration{}, errors.New("invalid token")
		}
		return runtimeprofile.Registration{
			TokenName: "edge-runtime",
			RuntimeProfile: apitypes.RuntimeProfile{
				Name: "edge-runtime",
				Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Models: &profileModels}},
			},
		}, nil
	}
	manager := NewManager(peersServer)
	manager.Models = models
	manager.RuntimeProfiles = runtimeProfiles
	service := &PeerService{
		manager:  manager,
		sessions: loginServer.SessionManager(),
		public: &peerHTTP{
			PeerHTTPService: peersServer,
			Self:            peersServer,
		},
	}
	handler := service.edgeHTTPHandler(service.sessions)

	tests := []struct {
		name              string
		role              apitypes.PeerRole
		status            apitypes.PeerRegistrationStatus
		registrationToken string
		wantStatus        int
		wantRuntimeModel  bool
	}{
		{name: "client session keeps runtime profile", role: apitypes.PeerRoleClient, status: apitypes.PeerRegistrationStatusActive, registrationToken: "edge-runtime-token", wantStatus: http.StatusOK, wantRuntimeModel: true},
		{name: "admin forbidden", role: apitypes.PeerRoleAdmin, status: apitypes.PeerRegistrationStatusActive, wantStatus: http.StatusForbidden},
		{name: "server forbidden", role: apitypes.PeerRoleServer, status: apitypes.PeerRegistrationStatusActive, wantStatus: http.StatusForbidden},
		{name: "edge forbidden", role: apitypes.PeerRoleEdgeNode, status: apitypes.PeerRegistrationStatusActive, wantStatus: http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keyPair, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatalf("GenerateKeyPair(peer) error = %v", err)
			}
			if _, err := peersServer.SavePeer(context.Background(), apitypes.Peer{
				PublicKey: keyPair.Public.String(),
				Role:      tc.role,
				Status:    tc.status,
				Device:    apitypes.DeviceInfo{},
			}); err != nil {
				t.Fatalf("SavePeer error = %v", err)
			}

			accessToken := issuePeerHTTPSession(t, loginServer, keyPair, serverKey.Public, tc.registrationToken)
			req := httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil)
			req.Header.Set(publiclogin.PublicKeyHeader, keyPair.Public.String())
			req.Header.Set("Authorization", "Bearer "+accessToken)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), tc.wantStatus)
			}
			if tc.wantRuntimeModel {
				var result struct {
					Data []apitypes.Model `json:"data"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
					t.Fatalf("decode models response: %v", err)
				}
				if len(result.Data) != 1 || result.Data[0].Id != "primary" {
					t.Fatalf("runtime models = %#v", result.Data)
				}
			}
		})
	}
}

func issuePeerHTTPSession(t testing.TB, loginServer *publiclogin.Server, keyPair *giznet.KeyPair, serverPublicKey giznet.PublicKey, registrationTokens ...string) string {
	t.Helper()

	assertion, err := publiclogin.NewLoginAssertion(keyPair, serverPublicKey, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	params := peerhttp.LoginParams{
		XPublicKey:    keyPair.Public.String(),
		Authorization: "Bearer " + assertion,
	}
	if len(registrationTokens) != 0 && strings.TrimSpace(registrationTokens[0]) != "" {
		token := strings.TrimSpace(registrationTokens[0])
		params.XRegistrationToken = &token
	}
	resp, err := loginServer.Login(context.Background(), peerhttp.LoginRequestObject{Params: params})
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
