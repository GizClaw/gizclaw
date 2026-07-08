package edge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestServeValidatesConfigThenReturnsExplicitUnimplemented(t *testing.T) {
	edgeKey := testKeyPair(t, 0x77)
	upstreamKey := testKeyPair(t, 0x88)
	dir := t.TempDir()
	writeConfig(t, dir, `
identity:
  private-key: `+edgeKey.Private.String()+`
upstream:
  endpoint: server-a.example.com:9820
  public-key: `+upstreamKey.Public.String()+`
`)
	if err := Serve(dir); err != ErrRuntimeNotImplemented {
		t.Fatalf("Serve error = %v, want %v", err, ErrRuntimeNotImplemented)
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
