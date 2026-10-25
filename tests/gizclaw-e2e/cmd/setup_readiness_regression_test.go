//go:build gizclaw_e2e

package clitest

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
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

type stalledSetupTransport struct {
	giznet.Conn
	started chan struct{}
	release chan struct{}
}

func (c *stalledSetupTransport) Dial(service uint64) (net.Conn, error) {
	return c.DialContext(context.Background(), service)
}
func (c *stalledSetupTransport) DialContext(ctx context.Context, _ uint64) (net.Conn, error) {
	close(c.started)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.release:
		return nil, io.ErrClosedPipe
	}
}

func TestSetupProbeServiceOpenHonorsCancellation(t *testing.T) {
	for _, service := range []struct {
		name string
		id   uint64
	}{{"mandatory-event", gizcli.EventStreamAgent}, {"http", gizcli.ServicePeerHTTP}, {"rpc", gizcli.ServicePeerRPC}} {
		for _, deadline := range []bool{false, true} {
			name := service.name + "/cancel"
			if deadline {
				name = service.name + "/deadline"
			}
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(t.Context(), 30*time.Millisecond)
				}
				defer cancel()
				stalled := &stalledSetupTransport{started: make(chan struct{}), release: make(chan struct{})}
				defer close(stalled.release)
				conn := &setupProbeConn{Conn: stalled, ctx: ctx}
				result := make(chan error, 1)
				go func() { _, err := conn.Dial(service.id); result <- err }()
				select {
				case <-stalled.started:
				case <-time.After(time.Second):
					t.Fatal("service open did not start")
				}
				expected := context.DeadlineExceeded
				if !deadline {
					cancel()
					expected = context.Canceled
				}
				select {
				case err := <-result:
					if !errors.Is(err, expected) {
						t.Fatalf("stalled service open error=%v, want %v", err, expected)
					}
				case <-time.After(time.Second):
					t.Fatal("stalled service open outlived the probe context")
				}
			})
		}
	}
}
