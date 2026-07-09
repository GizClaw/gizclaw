package gizedge

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
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
  ice-endpoint: server-a.example.com:19820
  public-key: `+upstreamKey.Public.String()+`
tls:
  cert-source: edge-rpc
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
	if cfg.Upstream.ICE != "server-a.example.com:19820" {
		t.Fatalf("Upstream.ICE = %q", cfg.Upstream.ICE)
	}
	if !cfg.Upstream.PublicKey.Equal(upstreamKey.Public) {
		t.Fatalf("Upstream.PublicKey = %v, want %v", cfg.Upstream.PublicKey, upstreamKey.Public)
	}
	if cfg.TLS.CertSource != TLSCertSourceEdgeRPC {
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
			name: "invalid upstream ice endpoint",
			body: `
identity:
  private-key: ` + edgeKey.Private.String() + `
upstream:
  endpoint: server-a.example.com:9820
  ice-endpoint: server-a.example.com
  public-key: ` + upstreamKey.Public.String() + `
`,
			want: "upstream.ice-endpoint",
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
				return service == gizclaw.ServicePeerHTTP
			},
		},
	}).Listen(upstreamKey)
	if err != nil {
		t.Fatalf("Listen upstream: %v", err)
	}
	defer upstreamListener.Close()
	signaling := httptest.NewServer(upstreamListener.SignalingHandler())
	defer signaling.Close()

	accepted := make(chan error, 1)
	go func() {
		conn, err := upstreamListener.Accept()
		if err != nil {
			accepted <- err
			return
		}
		server := gizhttp.NewServer(conn, gizclaw.ServicePeerHTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestE2EEdgeWorkspaceTemplateParses(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "gizclaw-e2e", "testdata", "edge-workspace", "config.yaml.template"))
	if err != nil {
		t.Fatalf("ReadFile edge template: %v", err)
	}
	body := strings.ReplaceAll(string(data), "${GIZCLAW_E2E_SERVER_ENDPOINT}", "127.0.0.1:9821")
	body = strings.ReplaceAll(body, "${GIZCLAW_E2E_EDGE_UPSTREAM_ENDPOINT}", "http://server:9822")
	body = strings.ReplaceAll(body, "${GIZCLAW_E2E_EDGE_UPSTREAM_ICE_ENDPOINT}", "server:9820")
	body = strings.ReplaceAll(body, "${GIZCLAW_E2E_EDGE_UPSTREAM_PUBLIC_KEY}", testKeyPair(t, 0x88).Public.String())
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
