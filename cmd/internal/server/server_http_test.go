package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/pion/webrtc/v4"
)

func TestCmdServerServeHTTPNilServerReturnsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	(*CmdServer)(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("nil server status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCmdServerPrivateIngressRequiresAuthorizedSession(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	adminKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(admin) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}

	srv, err := New(Config{
		KeyPair:  serverKey,
		Listen:   "127.0.0.1:0",
		Endpoint: "127.0.0.1:0",
		Stores: map[string]stores.Config{
			defaultPeersStore: {Kind: stores.KindKeyValue, Backend: "memory"},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer srv.Close()

	peers := &peer.Server{Store: kv.Prefixed(srv.Server.PeerStore, kv.Key{"peers"})}
	for _, item := range []apitypes.Peer{
		{
			PublicKey: adminKey.Public.String(),
			Role:      apitypes.PeerRoleAdmin,
			Status:    apitypes.PeerRegistrationStatusActive,
			Device:    apitypes.DeviceInfo{},
		},
		{
			PublicKey: clientKey.Public.String(),
			Role:      apitypes.PeerRoleClient,
			Status:    apitypes.PeerRegistrationStatusActive,
			Device:    apitypes.DeviceInfo{},
		},
	} {
		if _, err := peers.SavePeer(context.Background(), item); err != nil {
			t.Fatalf("SavePeer(%s) error = %v", item.PublicKey, err)
		}
	}

	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/server-info", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	assertHTTPError(t, rec, http.StatusUnauthorized, "INVALID_SESSION")

	srv.Server.WebRTCSignalingHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("WebRTC offer Authorization = %q, want signed offer without bearer session", r.Header.Get("Authorization"))
			http.Error(w, "unexpected bearer session", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	req = httptest.NewRequest(http.MethodPost, gizwebrtc.SignalingPath, nil)
	req.Header.Set("X-Giznet-Public-Key", clientKey.Public.String())
	req.Header.Set("X-Giznet-Timestamp", "1")
	req.Header.Set("X-Giznet-Nonce", "nonce")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s status = %d body=%s, want %d", gizwebrtc.SignalingPath, rec.Code, rec.Body.String(), http.StatusOK)
	}
	req = httptest.NewRequest(http.MethodOptions, gizwebrtc.SignalingPath, nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("OPTIONS %s status = %d body=%s, want signaling handler without private-ingress auth", gizwebrtc.SignalingPath, rec.Code, rec.Body.String())
	}

	clientLogin := cmdServerTestLogin(t, srv, serverKey.Public, clientKey)
	assertHTTPError(t, clientLogin, http.StatusUnauthorized, "INVALID_ASSERTION")

	adminLogin := cmdServerTestLogin(t, srv, serverKey.Public, adminKey)
	if adminLogin.Code != http.StatusOK {
		t.Fatalf("admin POST /login status = %d body=%s", adminLogin.Code, adminLogin.Body.String())
	}
	var session peerhttp.LoginResult
	if err := json.Unmarshal(adminLogin.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode admin login response: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/server-info", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set(publiclogin.PublicKeyHeader, clientKey.Public.String())
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	assertHTTPError(t, rec, http.StatusUnauthorized, "PUBLIC_KEY_MISMATCH")

	req = httptest.NewRequest(http.MethodGet, "/server-info", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set(publiclogin.PublicKeyHeader, adminKey.Public.String())
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authorized GET /server-info status = %d body=%s", rec.Code, rec.Body.String())
	}

	deviceToken := cmdServerTestCreateDeviceToken(t, srv.Server, session.AccessToken, adminKey.Public)
	sideLogin := cmdServerTestSideControlLogin(t, srv.Server, serverKey.Public, adminKey, deviceToken.Token)
	if sideLogin.Code != http.StatusOK {
		t.Fatalf("embedded server side-control login status = %d body=%s", sideLogin.Code, sideLogin.Body.String())
	}
	var sideSession peerhttp.LoginResult
	if err := json.Unmarshal(sideLogin.Body.Bytes(), &sideSession); err != nil {
		t.Fatalf("decode side-control login response: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/side-control/info", nil)
	req.Header.Set("Authorization", "Bearer "+sideSession.AccessToken)
	req.Header.Set(publiclogin.PublicKeyHeader, adminKey.Public.String())
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	assertHTTPError(t, rec, http.StatusForbidden, "PRIVATE_INGRESS_DENIED")

	unusedToken := cmdServerTestCreateDeviceToken(t, srv.Server, session.AccessToken, adminKey.Public)
	assertion, err := publiclogin.NewLoginAssertion(adminKey, serverKey.Public, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion(side control) error = %v", err)
	}
	loginBody, err := json.Marshal(peerhttp.LoginJSONRequestBody{GrantType: peerhttp.SideControl, DeviceToken: unusedToken.Token})
	if err != nil {
		t.Fatalf("marshal side-control login: %v", err)
	}
	newSideLoginRequest := func() *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(loginBody))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(publiclogin.PublicKeyHeader, adminKey.Public.String())
		request.Header.Set("Authorization", "Bearer "+assertion)
		return request
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, newSideLoginRequest())
	assertHTTPError(t, rec, http.StatusForbidden, "PRIVATE_INGRESS_DENIED")
	rec = httptest.NewRecorder()
	srv.Server.ServeHTTP(rec, newSideLoginRequest())
	if rec.Code != http.StatusOK {
		t.Fatalf("rejected direct grant consumed credentials: embedded login status = %d body=%s", rec.Code, rec.Body.String())
	}

	if _, err := peers.SavePeer(context.Background(), apitypes.Peer{
		PublicKey: adminKey.Public.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusActive,
		Device:    apitypes.DeviceInfo{},
	}); err != nil {
		t.Fatalf("SavePeer(demoted admin) error = %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/server-info", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set(publiclogin.PublicKeyHeader, adminKey.Public.String())
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	assertHTTPError(t, rec, http.StatusForbidden, "PRIVATE_INGRESS_DENIED")
}

func assertHTTPError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), status)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	if body.Error.Code != code {
		t.Fatalf("error code = %q body=%s, want %q", body.Error.Code, rec.Body.String(), code)
	}
}

func TestSideControlRoutesUsePublicHTTPIngressPolicy(t *testing.T) {
	for _, path := range []string{
		"/me/side-control/device-tokens",
		"/me/side-control/sessions/session-id",
		"/side-control/info",
		"/side-control/telemetry/aggregate",
		"/side-control/contacts/contact-id",
	} {
		if !isPublicHTTPRoute(path) {
			t.Fatalf("isPublicHTTPRoute(%q) = false", path)
		}
	}
}

func TestSideControlDirectTCPWhenServeToClientsEnabled(t *testing.T) {
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
	srv, err := New(Config{
		KeyPair:        serverKey,
		Listen:         "127.0.0.1:0",
		Endpoint:       "127.0.0.1:0",
		ServeToClients: true,
		Stores: map[string]stores.Config{
			defaultPeersStore: {Kind: stores.KindKeyValue, Backend: "memory"},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer srv.Close()
	peers := &peer.Server{Store: kv.Prefixed(srv.Server.PeerStore, kv.Key{"peers"})}
	if _, err := peers.SavePeer(context.Background(), apitypes.Peer{
		PublicKey: targetKey.Public.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusActive,
		Device:    apitypes.DeviceInfo{},
	}); err != nil {
		t.Fatalf("SavePeer(target) error = %v", err)
	}
	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	httpServer := httptest.NewServer(srv)
	defer httpServer.Close()

	targetLogin := cmdServerTestLoginURL(t, httpServer.URL, serverKey.Public, targetKey, nil)
	deviceToken := cmdServerTestCreateDeviceTokenURL(t, httpServer.URL, targetLogin.AccessToken, targetKey.Public)
	sideLogin := cmdServerTestLoginURL(t, httpServer.URL, serverKey.Public, controllerKey, &peerhttp.LoginJSONRequestBody{
		GrantType:   peerhttp.SideControl,
		DeviceToken: deviceToken.Token,
	})

	req, err := http.NewRequest(http.MethodGet, httpServer.URL+"/side-control/info", nil)
	if err != nil {
		t.Fatalf("NewRequest(side info) error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+sideLogin.AccessToken)
	req.Header.Set(publiclogin.PublicKeyHeader, controllerKey.Public.String())
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET side-control info error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET side-control info status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func cmdServerTestLogin(t *testing.T, srv *CmdServer, serverPublicKey giznet.PublicKey, keyPair *giznet.KeyPair) *httptest.ResponseRecorder {
	t.Helper()
	assertion, err := publiclogin.NewLoginAssertion(keyPair, serverPublicKey, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.Header.Set(publiclogin.PublicKeyHeader, keyPair.Public.String())
	req.Header.Set("Authorization", "Bearer "+assertion)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func cmdServerTestCreateDeviceToken(t *testing.T, handler http.Handler, accessToken string, publicKey giznet.PublicKey) peerhttp.SideControlDeviceToken {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/me/side-control/device-tokens", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set(publiclogin.PublicKeyHeader, publicKey.String())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create device token status = %d body=%s", rec.Code, rec.Body.String())
	}
	var token peerhttp.SideControlDeviceToken
	if err := json.Unmarshal(rec.Body.Bytes(), &token); err != nil {
		t.Fatalf("decode device token: %v", err)
	}
	return token
}

func cmdServerTestSideControlLogin(t *testing.T, handler http.Handler, serverPublicKey giznet.PublicKey, keyPair *giznet.KeyPair, deviceToken string) *httptest.ResponseRecorder {
	t.Helper()
	assertion, err := publiclogin.NewLoginAssertion(keyPair, serverPublicKey, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion(side control) error = %v", err)
	}
	body, err := json.Marshal(peerhttp.LoginJSONRequestBody{GrantType: peerhttp.SideControl, DeviceToken: deviceToken})
	if err != nil {
		t.Fatalf("marshal side-control login: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(publiclogin.PublicKeyHeader, keyPair.Public.String())
	req.Header.Set("Authorization", "Bearer "+assertion)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func cmdServerTestLoginURL(t *testing.T, baseURL string, serverPublicKey giznet.PublicKey, keyPair *giznet.KeyPair, body *peerhttp.LoginJSONRequestBody) peerhttp.LoginResult {
	t.Helper()
	assertion, err := publiclogin.NewLoginAssertion(keyPair, serverPublicKey, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	var requestBody []byte
	if body != nil {
		requestBody, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal login body: %v", err)
		}
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/login", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("NewRequest(login) error = %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(publiclogin.PublicKeyHeader, keyPair.Public.String())
	req.Header.Set("Authorization", "Bearer "+assertion)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST login error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST login status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var result peerhttp.LoginResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return result
}

func cmdServerTestCreateDeviceTokenURL(t *testing.T, baseURL, accessToken string, publicKey giznet.PublicKey) peerhttp.SideControlDeviceToken {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/me/side-control/device-tokens", nil)
	if err != nil {
		t.Fatalf("NewRequest(device token) error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set(publiclogin.PublicKeyHeader, publicKey.String())
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST device token error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("POST device token status = %d, want %d", response.StatusCode, http.StatusCreated)
	}
	var token peerhttp.SideControlDeviceToken
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		t.Fatalf("decode device token: %v", err)
	}
	return token
}

func TestNewWithOptionsWiresLegacyMetricsStore(t *testing.T) {
	srv, err := newWithOptions(Config{
		Listen:   "127.0.0.1:0",
		Endpoint: "127.0.0.1:0",
		Stores: map[string]stores.Config{
			defaultPeersStore: {Kind: stores.KindKeyValue, Backend: "memory"},
			defaultMetricsStore: {
				Kind: stores.KindMetrics,
				Prometheus: &metrics.PrometheusConfig{
					RemoteWriteURL: "http://127.0.0.1:1/api/v1/write",
					QueryURL:       "http://127.0.0.1:1",
				},
			},
		},
	}, newServerOptions{})
	if err != nil {
		t.Fatalf("newWithOptions() error = %v", err)
	}
	defer srv.Close()
	if srv.Server.MetricsStore == nil {
		t.Fatal("MetricsStore is nil")
	}
}

func TestNewWithOptionsInstallsAndFlushesMetricsRecorder(t *testing.T) {
	srv, err := newWithOptions(Config{
		Listen:   "127.0.0.1:0",
		Endpoint: "127.0.0.1:0",
		Stores: map[string]stores.Config{
			defaultPeersStore:   {Kind: stores.KindKeyValue, Backend: "memory"},
			defaultMetricsStore: {Kind: stores.KindMetrics, Backend: "memory"},
		},
	}, newServerOptions{})
	if err != nil {
		t.Fatalf("newWithOptions() error = %v", err)
	}
	t.Cleanup(func() {
		if err := srv.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	gizmetrics.AddCounter(context.Background(), "gizclaw_server_test_total", 2,
		gizmetrics.Label{Name: "result", Value: "ok"},
	)
	if err := srv.shutdownMetrics(context.Background()); err != nil {
		t.Fatalf("shutdownMetrics() error = %v", err)
	}
	series, err := srv.Server.MetricsStore.Latest(context.Background(), metrics.LatestQuery{
		Selector: metrics.Selector{
			Name: "gizclaw_server_test_total",
			Matchers: []metrics.LabelMatcher{
				{Name: "result", Op: metrics.MatchEqual, Value: "ok"},
			},
		},
		At:       time.Now(),
		Lookback: time.Minute,
	})
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 || series[0].Points[0].Value != 2 {
		t.Fatalf("Latest() = %#v, want one sample with value 2", series)
	}
}

func TestNewWithOptionsWithoutMetricsStoreLeavesRecorderDisabled(t *testing.T) {
	srv, err := newWithOptions(Config{
		Listen:   "127.0.0.1:0",
		Endpoint: "127.0.0.1:0",
		Stores: map[string]stores.Config{
			defaultPeersStore: {Kind: stores.KindKeyValue, Backend: "memory"},
		},
	}, newServerOptions{})
	if err != nil {
		t.Fatalf("newWithOptions() error = %v", err)
	}
	defer srv.Close()

	if srv.metricsShutdown != nil {
		t.Fatal("metricsShutdown is configured without a metrics store")
	}
	gizmetrics.AddCounter(context.Background(), "gizclaw_server_no_store_total", 1)
}

func TestCmdServerCloseStopsServerBeforeMetricsRecorder(t *testing.T) {
	srv := &CmdServer{Server: &gizclaw.Server{}}
	srv.metricsShutdown = func(context.Context) error {
		if srv.Server != nil {
			return errors.New("metrics recorder stopped before gizclaw server")
		}
		return nil
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestNewWithOptionsConcurrentMetricsInstallPreservesExistingRecorder(t *testing.T) {
	existing := metrics.NewMemoryStore()
	shutdown, err := gizmetrics.InstallStore(existing, gizmetrics.WithFlushInterval(time.Hour))
	if err != nil {
		t.Fatalf("InstallStore(existing) error = %v", err)
	}
	t.Cleanup(func() {
		_ = shutdown(context.Background())
		_ = existing.Close()
	})

	_, err = newWithOptions(Config{
		Listen:   "127.0.0.1:0",
		Endpoint: "127.0.0.1:0",
		Stores: map[string]stores.Config{
			defaultPeersStore:   {Kind: stores.KindKeyValue, Backend: "memory"},
			defaultMetricsStore: {Kind: stores.KindMetrics, Backend: "memory"},
		},
	}, newServerOptions{})
	if !errors.Is(err, gizmetrics.ErrAlreadyInstalled) {
		t.Fatalf("newWithOptions() error = %v, want %v", err, gizmetrics.ErrAlreadyInstalled)
	}

	gizmetrics.AddCounter(context.Background(), "gizclaw_existing_recorder_total", 1)
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown(existing) error = %v", err)
	}
	series, err := existing.Latest(context.Background(), metrics.LatestQuery{
		Selector: metrics.Selector{Name: "gizclaw_existing_recorder_total"},
		At:       time.Now(),
		Lookback: time.Minute,
	})
	if err != nil {
		t.Fatalf("Latest(existing) error = %v", err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 || series[0].Points[0].Value != 1 {
		t.Fatalf("existing recorder series = %#v", series)
	}
}

func TestConfigListenAddrs(t *testing.T) {
	cfg := Config{Listen: "0.0.0.0:9820", Endpoint: "192.168.1.20:9820"}
	if got := cfg.PublicAPIListenAddr(); got != "0.0.0.0:9820" {
		t.Fatalf("PublicAPIListenAddr = %q", got)
	}
	if got := cfg.ICEListenAddr(); got != "0.0.0.0:9820" {
		t.Fatalf("ICEListenAddr = %q", got)
	}
}

func TestWebRTCListenConfigUsesListenAndPublicEndpoint(t *testing.T) {
	policy := testSecurityPolicy{}
	handler := testPeerEventHandler{}
	iceTCPListener := &testListener{addr: testAddr("0.0.0.0:9820")}
	cfg := webRTCListenConfig(Config{Listen: "0.0.0.0:9820", Endpoint: "192.168.1.20:19820"}, gizclaw.PeerListenerOptions{
		SecurityPolicy:   policy,
		PeerEventHandler: handler,
	}, iceTCPListener)

	if cfg.ICEUDPAddr != "0.0.0.0:9820" || cfg.ICETCPAddr != "" {
		t.Fatalf("ICE addrs = %q, %q", cfg.ICEUDPAddr, cfg.ICETCPAddr)
	}
	if cfg.ICETCPListener != iceTCPListener {
		t.Fatal("ICETCPListener not preserved")
	}
	if cfg.PublicICEUDPAddr != "192.168.1.20:19820" {
		t.Fatalf("PublicICEUDPAddr = %q", cfg.PublicICEUDPAddr)
	}
	if cfg.PublicICETCPAddr != "192.168.1.20:19820" {
		t.Fatalf("PublicICETCPAddr = %q", cfg.PublicICETCPAddr)
	}
	if len(cfg.NAT1To1IPs) != 0 {
		t.Fatalf("NAT1To1IPs = %#v", cfg.NAT1To1IPs)
	}
	if cfg.ICELite {
		t.Fatal("ICELite = true, want false")
	}
	if cfg.SecurityPolicy != policy {
		t.Fatal("SecurityPolicy not preserved")
	}
	if cfg.PeerEventHandler != handler {
		t.Fatal("PeerEventHandler not preserved")
	}
}

func TestWebRTCListenConfigSkipsUnspecifiedPublicEndpoint(t *testing.T) {
	cfg := webRTCListenConfig(Config{Listen: "0.0.0.0:9820", Endpoint: "0.0.0.0:9820"}, gizclaw.PeerListenerOptions{}, nil)
	if cfg.PublicICEUDPAddr != "" {
		t.Fatalf("PublicICEUDPAddr = %q, want empty", cfg.PublicICEUDPAddr)
	}
	if cfg.PublicICETCPAddr != "" {
		t.Fatalf("PublicICETCPAddr = %q, want empty", cfg.PublicICETCPAddr)
	}
}

func TestWebRTCListenConfigSkipsHostnamePublicEndpoint(t *testing.T) {
	cfg := webRTCListenConfig(Config{Listen: "0.0.0.0:9820", Endpoint: "example.com:9820"}, gizclaw.PeerListenerOptions{}, nil)
	if cfg.PublicICEUDPAddr != "" {
		t.Fatalf("PublicICEUDPAddr = %q, want empty", cfg.PublicICEUDPAddr)
	}
	if cfg.PublicICETCPAddr != "" {
		t.Fatalf("PublicICETCPAddr = %q, want empty", cfg.PublicICETCPAddr)
	}
}

func TestWebRTCListenConfigUsesRelayOnlyWithICEServers(t *testing.T) {
	cfg := webRTCListenConfig(Config{
		Listen:   "0.0.0.0:9820",
		Endpoint: "192.168.1.20:19820",
		ICEServers: []gizwebrtc.ICEServer{{
			URLs:       []string{"turn:edge.example.com:3478?transport=udp"},
			Username:   "edge",
			Credential: "secret",
		}},
	}, gizclaw.PeerListenerOptions{}, nil)

	if cfg.ICETransportPolicy != webrtc.ICETransportPolicyRelay {
		t.Fatalf("ICETransportPolicy = %s, want relay", cfg.ICETransportPolicy)
	}
}

func TestWebRTCListenConfigKeepsTURNRESTCredentialsForPerAnswerMinting(t *testing.T) {
	cfg := webRTCListenConfig(Config{
		Listen:   "0.0.0.0:9820",
		Endpoint: "192.168.1.20:19820",
		ICEServers: []gizwebrtc.ICEServer{{
			URLs:           []string{"turn:edge.example.com:3478?transport=udp"},
			Username:       "edge",
			Credential:     "long-term-secret",
			CredentialMode: gizwebrtc.ICECredentialModeTURNREST,
		}},
	}, gizclaw.PeerListenerOptions{}, nil)
	if len(cfg.ICEServers) != 1 {
		t.Fatalf("ICEServers len = %d, want 1", len(cfg.ICEServers))
	}
	got := cfg.ICEServers[0]
	if got.Username != "edge" || got.Credential != "long-term-secret" || got.CredentialMode != gizwebrtc.ICECredentialModeTURNREST {
		t.Fatalf("ICEServers[0] = %+v, want raw TURN REST config", got)
	}
}

func TestWebRTCListenConfigRejectsEmptyTURNRESTSecret(t *testing.T) {
	cfg := webRTCListenConfig(Config{
		Listen:   "0.0.0.0:9820",
		Endpoint: "192.168.1.20:19820",
		ICEServers: []gizwebrtc.ICEServer{{
			URLs:           []string{"turn:edge.example.com:3478?transport=udp"},
			Username:       "edge",
			CredentialMode: gizwebrtc.ICECredentialModeTURNREST,
		}},
	}, gizclaw.PeerListenerOptions{}, nil)
	if _, err := cfg.Listen(testKeyPair(t, 0x44)); err == nil {
		t.Fatal("Listen error = nil, want empty TURN REST credential rejection")
	}
}

func TestWebRTCListenConfigKeepsDefaultPolicyWithSTUNOnlyICEServers(t *testing.T) {
	cfg := webRTCListenConfig(Config{
		Listen:   "0.0.0.0:9820",
		Endpoint: "192.168.1.20:19820",
		ICEServers: []gizwebrtc.ICEServer{{
			URLs: []string{"stun:edge.example.com:3478"},
		}},
	}, gizclaw.PeerListenerOptions{}, nil)

	if cfg.ICETransportPolicy != webrtc.ICETransportPolicyAll {
		t.Fatalf("ICETransportPolicy = %s, want all", cfg.ICETransportPolicy)
	}
	if cfg.PublicICEUDPAddr != "192.168.1.20:19820" {
		t.Fatalf("PublicICEUDPAddr = %q, want endpoint", cfg.PublicICEUDPAddr)
	}
	if cfg.PublicICETCPAddr != "192.168.1.20:19820" {
		t.Fatalf("PublicICETCPAddr = %q, want endpoint", cfg.PublicICETCPAddr)
	}
}

type testSecurityPolicy struct{}

func (testSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (testSecurityPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}

type testPeerEventHandler struct{}

func (testPeerEventHandler) HandlePeerEvent(giznet.PeerEvent) {}

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

type testListener struct {
	addr testAddr
}

func (l *testListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (l *testListener) Close() error              { return nil }
func (l *testListener) Addr() net.Addr            { return l.addr }
