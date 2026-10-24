//go:build gizclaw_e2e

package clitest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn"
	"github.com/goccy/go-yaml"
)

func TestSetupReadinessUsesAuthenticatedGoSDK(t *testing.T) {
	endpoint := os.Getenv("GIZCLAW_E2E_SERVER_ENDPOINT")
	if endpoint == "" {
		t.Fatal("local E2E Server endpoint required")
	}
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := filepath.Join(home, "gizclaw", "probe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(map[string]any{"identity": map[string]any{"private-key": key.Private.String()}, "server": map[string]any{"endpoint": endpoint}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := contextconn.Connect(ctx, contextconn.Options{ConfigDir: filepath.Join(home, "gizclaw"), Context: "probe"})
		if err != nil {
			t.Errorf("cleanup readiness Peer connection: %v", err)
			return
		}
		defer client.Close()
		if _, err := client.DeletePeer(ctx, "cleanup-readiness", rpcapi.ServerPeerDeleteRequest{}); err != nil {
			t.Errorf("cleanup readiness Peer: %v", err)
		}
	})
	h := NewHarness(t, "setup-readiness")
	h.BinaryPath = filepath.Join(t.TempDir(), "unavailable-cli")
	t.Run("live-server", func(t *testing.T) {
		if err := h.probeSetupServer(home, "probe", 5*time.Second); err != nil {
			t.Fatalf("authenticated Server readiness depended on a CLI process: %v", err)
		}
	})
	t.Run("missing-context", func(t *testing.T) {
		if err := h.probeSetupServer(home, "absent", time.Second); err == nil {
			t.Fatal("missing context passed readiness")
		}
	})
}
