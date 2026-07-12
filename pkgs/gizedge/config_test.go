package gizedge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

func TestPrepareWorkspaceConfigLoadsStaticUpstream(t *testing.T) {
	edgeKey := testKeyPair(t, 0x11)
	upstreamKey := testKeyPair(t, 0x22)
	dir := t.TempDir()
	writeConfig(t, dir, `
identity:
  private-key: `+edgeKey.Private.String()+`
listen: 127.0.0.1:9821
endpoint: edge.example.com:9821
upstream:
  endpoint: server-a.example.com:9820
  public-key: `+upstreamKey.Public.String()+`
tls:
  cert-source: disabled
`)

	cfg, err := PrepareWorkspaceConfig(dir)
	if err != nil {
		t.Fatalf("PrepareWorkspaceConfig error = %v", err)
	}
	if cfg.KeyPair == nil || !cfg.KeyPair.Public.Equal(edgeKey.Public) {
		t.Fatalf("edge public key = %v, want %v", cfg.KeyPair.Public, edgeKey.Public)
	}
	if cfg.Listen != "127.0.0.1:9821" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.Endpoint != "edge.example.com:9821" {
		t.Fatalf("Endpoint = %q", cfg.Endpoint)
	}
	if cfg.Upstream.Endpoint != "server-a.example.com:9820" {
		t.Fatalf("Upstream.Endpoint = %q", cfg.Upstream.Endpoint)
	}
	if !cfg.Upstream.PublicKey.Equal(upstreamKey.Public) {
		t.Fatalf("Upstream.PublicKey = %v, want %v", cfg.Upstream.PublicKey, upstreamKey.Public)
	}
	if cfg.TLS.CertSource != TLSCertSourceDisabled {
		t.Fatalf("TLS.CertSource = %q", cfg.TLS.CertSource)
	}
}

func TestConfigUpstreamURLDefaultsHTTP(t *testing.T) {
	cfg := Config{Upstream: UpstreamConfig{Endpoint: "server-a.example.com:9822"}}
	upstreamURL, err := cfg.UpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	if got := upstreamURL.String(); got != "http://server-a.example.com:9822" {
		t.Fatalf("UpstreamURL = %q", got)
	}
}

func TestPrepareWorkspaceConfigDefaultsEndpointAndTLS(t *testing.T) {
	edgeKey := testKeyPair(t, 0x33)
	upstreamKey := testKeyPair(t, 0x44)
	dir := t.TempDir()
	writeConfig(t, dir, `
identity:
  private-key: `+edgeKey.Private.String()+`
upstream:
  endpoint: server-a.example.com:9820
  public-key: `+upstreamKey.Public.String()+`
`)

	cfg, err := PrepareWorkspaceConfig(dir)
	if err != nil {
		t.Fatalf("PrepareWorkspaceConfig error = %v", err)
	}
	if cfg.Listen != "0.0.0.0:9821" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.Endpoint != cfg.Listen {
		t.Fatalf("Endpoint = %q, want listen %q", cfg.Endpoint, cfg.Listen)
	}
	if cfg.TLS.CertSource != TLSCertSourceDisabled {
		t.Fatalf("TLS.CertSource = %q", cfg.TLS.CertSource)
	}
}

func TestPrepareWorkspaceConfigRejectsMissingOrInvalidFields(t *testing.T) {
	edgeKey := testKeyPair(t, 0x55)
	upstreamKey := testKeyPair(t, 0x66)
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing identity",
			body: `
upstream:
  endpoint: server-a.example.com:9820
  public-key: ` + upstreamKey.Public.String() + `
`,
			want: "identity.private-key",
		},
		{
			name: "missing upstream endpoint",
			body: `
identity:
  private-key: ` + edgeKey.Private.String() + `
upstream:
  public-key: ` + upstreamKey.Public.String() + `
`,
			want: "upstream.endpoint",
		},
		{
			name: "missing upstream public key",
			body: `
identity:
  private-key: ` + edgeKey.Private.String() + `
upstream:
  endpoint: server-a.example.com:9820
`,
			want: "upstream.public-key",
		},
		{
			name: "invalid tls source",
			body: `
identity:
  private-key: ` + edgeKey.Private.String() + `
upstream:
  endpoint: server-a.example.com:9820
  public-key: ` + upstreamKey.Public.String() + `
tls:
  cert-source: acme
`,
			want: "tls.cert-source",
		},
		{
			name: "unimplemented tls edge rpc source",
			body: `
identity:
  private-key: ` + edgeKey.Private.String() + `
upstream:
  endpoint: server-a.example.com:9820
  public-key: ` + upstreamKey.Public.String() + `
tls:
  cert-source: edge-rpc
`,
			want: "not implemented",
		},
		{
			name: "unimplemented tls file source",
			body: `
identity:
  private-key: ` + edgeKey.Private.String() + `
upstream:
  endpoint: server-a.example.com:9820
  public-key: ` + upstreamKey.Public.String() + `
tls:
  cert-source: file
`,
			want: "not implemented",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, tc.body)
			if _, err := PrepareWorkspaceConfig(dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("PrepareWorkspaceConfig err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestServeContextForwardsToUpstreamGizHTTP(t *testing.T) {
	edgeKey := testKeyPair(t, 0x77)
	upstreamKey := testKeyPair(t, 0x78)
	upstreamListener, err := (&gizwebrtc.ListenConfig{
		SecurityPolicy: edgeTestSecurityPolicy{
			allowService: func(_ giznet.PublicKey, service uint64) bool {
				return service == gizclaw.ServiceEdgeHTTP
			},
		},
	}).Listen(upstreamKey)
	if err != nil {
		t.Fatalf("Listen upstream: %v", err)
	}
	defer upstreamListener.Close()
	signaling := newTestUpstreamSignalingServer(upstreamListener.SignalingHandler())
	defer signaling.Close()

	accepted := make(chan error, 1)
	go func() {
		conn, err := upstreamListener.Accept()
		if err != nil {
			accepted <- err
			return
		}
		server := gizhttp.NewServer(conn, gizclaw.ServiceEdgeHTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/server-info" {
				t.Errorf("path = %q", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		accepted <- server.Serve()
	}()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("newLocalListener: %v", err)
	}
	listenAddr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}

	dir := t.TempDir()
	writeConfig(t, dir, `
identity:
  private-key: `+edgeKey.Private.String()+`
listen: `+listenAddr+`
upstream:
  endpoint: `+signaling.URL+`
  public-key: `+upstreamKey.Public.String()+`
`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeContext(ctx, dir)
	}()
	var lastErr error
	for i := 0; i < 100; i++ {
		resp, err := http.Get("http://" + listenAddr + "/server-info")
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			cancel()
			if serveErr := <-errCh; serveErr != nil {
				t.Fatalf("ServeContext shutdown error = %v", serveErr)
			}
			if upstreamErr := <-accepted; upstreamErr != nil {
				t.Fatalf("upstream gizhttp server error = %v", upstreamErr)
			}
			return
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("edge did not serve request: %v", lastErr)
}

func TestUpstreamTransportReconnectsAfterClosedConn(t *testing.T) {
	edgeKey := testKeyPair(t, 0x79)
	upstreamKey := testKeyPair(t, 0x7a)
	var mu sync.Mutex
	var signalingHandler http.Handler
	signaling := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"edge-session","token_type":"Bearer","expires_at":4102444800000}`))
			return
		}
		mu.Lock()
		handler := signalingHandler
		mu.Unlock()
		if handler == nil {
			http.Error(w, "no signaling handler", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer signaling.Close()

	cfg := Config{
		KeyPair: edgeKey,
		Upstream: UpstreamConfig{
			Endpoint:  signaling.URL,
			PublicKey: upstreamKey.Public,
		},
	}
	upstreamURL, err := cfg.UpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	type testUpstream struct {
		listener giznet.Listener
		connCh   chan giznet.Conn
		errCh    chan error
	}
	startUpstream := func(body string) testUpstream {
		t.Helper()
		upstreamListener, err := (&gizwebrtc.ListenConfig{
			SecurityPolicy: edgeTestSecurityPolicy{
				allowService: func(_ giznet.PublicKey, service uint64) bool {
					return service == gizclaw.ServiceEdgeHTTP
				},
			},
		}).Listen(upstreamKey)
		if err != nil {
			t.Fatalf("Listen upstream: %v", err)
		}
		mu.Lock()
		signalingHandler = upstreamListener.SignalingHandler()
		mu.Unlock()

		connCh := make(chan giznet.Conn, 1)
		errCh := make(chan error, 1)
		go func() {
			conn, err := upstreamListener.Accept()
			if err != nil {
				errCh <- err
				return
			}
			connCh <- conn
			server := gizhttp.NewServer(conn, gizclaw.ServiceEdgeHTTP, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			errCh <- server.Serve()
		}()
		return testUpstream{listener: upstreamListener, connCh: connCh, errCh: errCh}
	}

	first := startUpstream("first")
	defer first.listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport, err := newUpstreamTransport(ctx, cfg, upstreamURL)
	if err != nil {
		t.Fatalf("newUpstreamTransport error = %v", err)
	}
	defer transport.Close()

	var firstConn giznet.Conn
	select {
	case firstConn = <-first.connCh:
	case err := <-first.errCh:
		t.Fatalf("first upstream accept error = %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("first upstream accept timed out")
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	if got := edgeHTTPGetBody(t, client); got != "first" {
		t.Fatalf("first body = %q", got)
	}
	_ = firstConn.Close()
	_ = first.listener.Close()

	second := startUpstream("second")
	defer second.listener.Close()
	if got := edgeHTTPGetBody(t, client); got != "second" {
		t.Fatalf("second body = %q", got)
	}
	select {
	case secondConn := <-second.connCh:
		_ = secondConn.Close()
	case err := <-second.errCh:
		t.Fatalf("second upstream accept error = %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("second upstream accept timed out")
	}
}

func TestUpstreamTransportDoesNotResetCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gizclaw/server-info", nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext error = %v", err)
	}
	conn := &failingGiznetConn{dialErr: context.Canceled}
	transport := &upstreamTransport{conn: conn}

	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("RoundTrip error = nil, want canceled request error")
	}
	if conn.closed {
		t.Fatal("canceled request reset shared upstream conn")
	}
	if transport.conn == nil {
		t.Fatal("canceled request cleared shared upstream conn")
	}
}

func edgeHTTPGetBody(t *testing.T, client *http.Client) string {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://gizclaw/server-info", nil)
	if err != nil {
		t.Fatalf("http.NewRequest error = %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do error = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.StatusCode, string(body))
	}
	return string(body)
}

type failingGiznetConn struct {
	dialErr error
	closed  bool
}

func (c *failingGiznetConn) Dial(uint64) (net.Conn, error) {
	return nil, c.dialErr
}

func (c *failingGiznetConn) ListenService(uint64) giznet.ServiceListener {
	return nil
}

func (c *failingGiznetConn) CloseService(uint64) error       { return nil }
func (c *failingGiznetConn) Read([]byte) (byte, int, error)  { return 0, 0, nil }
func (c *failingGiznetConn) Write(byte, []byte) (int, error) { return 0, nil }
func (c *failingGiznetConn) PublicKey() giznet.PublicKey     { return giznet.PublicKey{} }
func (c *failingGiznetConn) PeerInfo() *giznet.PeerInfo      { return nil }

func (c *failingGiznetConn) Close() error {
	if c.closed {
		return errors.New("already closed")
	}
	c.closed = true
	return nil
}

func TestEdgeCORSHandlerHandlesBrowserPreflight(t *testing.T) {
	called := false
	handler := edgeCORSHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodOptions, "/webrtc/v1/offer", nil)
	req.Header.Set("Origin", "wails://wails.localhost")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-public-key,x-giznet-nonce,x-giznet-public-key,x-giznet-timestamp")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("preflight should not reach upstream proxy")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Public-Key") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want X-Public-Key", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPut) {
		t.Fatalf("Access-Control-Allow-Methods = %q, want PUT", got)
	}
}

func TestEdgeCORSHandlerAddsHeadersToForwardedRequests(t *testing.T) {
	handler := edgeCORSHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

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

func TestUpstreamSignalingURLDefaultsWebRTCPath(t *testing.T) {
	upstreamURL, err := (&Config{Upstream: UpstreamConfig{Endpoint: "http://server:9822"}}).UpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	if got := upstreamSignalingURL(upstreamURL); got != "http://server:9822/webrtc/v1/offer" {
		t.Fatalf("upstreamSignalingURL = %q", got)
	}
}

func TestUpstreamSignalingURLPreservesConfiguredPath(t *testing.T) {
	upstreamURL, err := (&Config{Upstream: UpstreamConfig{Endpoint: "http://server:9822/custom-offer"}}).UpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	if got := upstreamSignalingURL(upstreamURL); got != "http://server:9822/custom-offer" {
		t.Fatalf("upstreamSignalingURL = %q", got)
	}
}

func TestPrivateIngressHTTPClientAddsEdgeSessionHeaders(t *testing.T) {
	edgeKey := testKeyPair(t, 0x99)
	upstreamKey := testKeyPair(t, 0x9a)
	loginCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			loginCount++
			if got := r.Header.Get(publiclogin.PublicKeyHeader); got != edgeKey.Public.String() {
				t.Fatalf("login %s = %q, want edge public key", publiclogin.PublicKeyHeader, got)
			}
			if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") || strings.Contains(got, "edge-session") {
				t.Fatalf("login Authorization = %q, want bearer assertion", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"edge-session","token_type":"Bearer","expires_at":4102444800000}`))
		case gizwebrtc.SignalingPath:
			if got := r.Header.Get(publiclogin.PublicKeyHeader); got != edgeKey.Public.String() {
				t.Fatalf("signaling %s = %q, want edge public key", publiclogin.PublicKeyHeader, got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer edge-session" {
				t.Fatalf("signaling Authorization = %q, want bearer session", got)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	upstreamURL, err := (&Config{Upstream: UpstreamConfig{Endpoint: upstream.URL}}).UpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	client := newPrivateIngressHTTPClient(Config{
		KeyPair: edgeKey,
		Upstream: UpstreamConfig{
			PublicKey: upstreamKey.Public,
		},
	}, upstreamURL)

	for i := 0; i < 2; i++ {
		resp, err := client.Post(upstream.URL+gizwebrtc.SignalingPath, "application/octet-stream", nil)
		if err != nil {
			t.Fatalf("Post signaling error = %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("signaling status = %d", resp.StatusCode)
		}
	}
	if loginCount != 1 {
		t.Fatalf("loginCount = %d, want cached session", loginCount)
	}
}

func TestPrivateIngressHTTPClientRefreshesSessionOnUnauthorized(t *testing.T) {
	edgeKey := testKeyPair(t, 0x9b)
	upstreamKey := testKeyPair(t, 0x9c)
	loginCount := 0
	signalingCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			loginCount++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"access_token":"edge-session-%d","token_type":"Bearer","expires_at":4102444800000}`, loginCount)))
		case gizwebrtc.SignalingPath:
			signalingCount++
			if signalingCount == 1 {
				if got := r.Header.Get("Authorization"); got != "Bearer edge-session-1" {
					t.Fatalf("first signaling Authorization = %q, want stale cached token", got)
				}
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer edge-session-2" {
				t.Fatalf("retry signaling Authorization = %q, want refreshed token", got)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	upstreamURL, err := (&Config{Upstream: UpstreamConfig{Endpoint: upstream.URL}}).UpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	client := newPrivateIngressHTTPClient(Config{
		KeyPair: edgeKey,
		Upstream: UpstreamConfig{
			PublicKey: upstreamKey.Public,
		},
	}, upstreamURL)

	resp, err := client.Post(upstream.URL+gizwebrtc.SignalingPath, "application/octet-stream", strings.NewReader("offer"))
	if err != nil {
		t.Fatalf("Post signaling error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("signaling status = %d", resp.StatusCode)
	}
	if loginCount != 2 {
		t.Fatalf("loginCount = %d, want refreshed session", loginCount)
	}
	if signalingCount != 2 {
		t.Fatalf("signalingCount = %d, want retry", signalingCount)
	}
}

func newTestUpstreamSignalingServer(handler http.Handler) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"edge-session","token_type":"Bearer","expires_at":4102444800000}`))
			return
		}
		handler.ServeHTTP(w, r)
	}))
}

func TestE2EEdgeWorkspaceTemplateParses(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "gizclaw-e2e", "testdata", "edge-workspace", "config.yaml.template"))
	if err != nil {
		t.Fatalf("ReadFile edge template: %v", err)
	}
	body := strings.NewReplacer(
		"${GIZCLAW_E2E_SERVER_ENDPOINT}", "127.0.0.1:9821",
		"${GIZCLAW_E2E_EDGE_UPSTREAM_ENDPOINT}", "http://server:9822",
		"${GIZCLAW_E2E_EDGE_UPSTREAM_PUBLIC_KEY}", testKeyPair(t, 0x88).Public.String(),
		"${GIZCLAW_E2E_TURN_ENDPOINT}", "127.0.0.1:3478",
		"${GIZCLAW_E2E_TURN_RELAY_ADDRESS}", "127.0.0.1",
		"${GIZCLAW_E2E_TURN_REALM}", "gizclaw-e2e",
		"${GIZCLAW_E2E_TURN_USERNAME}", "user",
		"${GIZCLAW_E2E_TURN_CREDENTIAL}", "pass",
		"${GIZCLAW_E2E_TURN_RELAY_MIN_PORT}", "36000",
		"${GIZCLAW_E2E_TURN_RELAY_MAX_PORT}", "36019",
	).Replace(string(data))
	fileCfg, err := parseConfigData([]byte(body))
	if err != nil {
		t.Fatalf("parseConfigData edge template: %v", err)
	}
	if _, err := prepareConfig(Config{}, fileCfg); err != nil {
		t.Fatalf("prepareConfig edge template: %v", err)
	}
}

func writeConfig(t *testing.T, dir string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, workspaceConfigFile), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func testKeyPair(t *testing.T, seed byte) *giznet.KeyPair {
	t.Helper()
	var key giznet.Key
	for i := range key {
		key[i] = seed
	}
	keyPair, err := giznet.NewKeyPair(key)
	if err != nil {
		t.Fatalf("NewKeyPair error = %v", err)
	}
	return keyPair
}

type edgeTestSecurityPolicy struct {
	allowService func(giznet.PublicKey, uint64) bool
}

func (p edgeTestSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (p edgeTestSecurityPolicy) AllowService(publicKey giznet.PublicKey, service uint64) bool {
	return p.allowService == nil || p.allowService(publicKey, service)
}
