package contextstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestStoreCreateLoadListEndpointContext(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{Root: t.TempDir()}
	if err := store.CreateWithOptions("local", "127.0.0.1:9820", CreateOptions{
		Description:     "Local dev",
		ServerPublicKey: serverKey.Public.String(),
	}); err != nil {
		t.Fatalf("CreateWithOptions() error = %v", err)
	}
	ctx, err := store.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if ctx == nil || ctx.Name != "local" {
		t.Fatalf("Current() = %#v", ctx)
	}
	if ctx.Config.Description != "Local dev" || ctx.Config.Server.Endpoint != "127.0.0.1:9820" {
		t.Fatalf("config = %#v", ctx.Config)
	}
	if ctx.Config.Server.SignalingURL() != "http://127.0.0.1:9820/webrtc/v1/offer" {
		t.Fatalf("SignalingURL() = %q", ctx.Config.Server.SignalingURL())
	}
	summaries, err := store.ListSummaries()
	if err != nil {
		t.Fatalf("ListSummaries() error = %v", err)
	}
	if len(summaries) != 1 || !summaries[0].Current || summaries[0].LocalPublicKey.IsZero() {
		t.Fatalf("summaries = %#v", summaries)
	}
}

func TestLoadRejectsOldNoiseFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`
server:
  host: 127.0.0.1
  public-api-port: 9820
  public-key: 11111111111111111111111111111111
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(dir); err == nil || !strings.Contains(err.Error(), "server.host is not supported") {
		t.Fatalf("LoadConfig() error = %v", err)
	}
}

func TestCreateRejectsEndpointURL(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{Root: t.TempDir()}
	err = store.Create("bad", "http://127.0.0.1:9820", serverKey.Public.String())
	if err == nil || !strings.Contains(err.Error(), "host:port") {
		t.Fatalf("Create() error = %v", err)
	}
}
