package edge

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

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
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

func TestServeContextForwardsToUpstreamHTTP(t *testing.T) {
	edgeKey := testKeyPair(t, 0x77)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/server-info" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

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
  endpoint: `+upstream.URL+`
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
			return
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("edge did not serve request: %v", lastErr)
}

func TestE2EEdgeWorkspaceTemplateParses(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "edge-workspace", "config.yaml.template"))
	if err != nil {
		t.Fatalf("ReadFile edge template: %v", err)
	}
	body := strings.ReplaceAll(string(data), "${GIZCLAW_E2E_SERVER_ENDPOINT}", "127.0.0.1:9821")
	body = strings.ReplaceAll(body, "${GIZCLAW_E2E_EDGE_UPSTREAM_ENDPOINT}", "http://server:9822")
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
