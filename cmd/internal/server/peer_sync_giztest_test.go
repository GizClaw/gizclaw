package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizedge"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// TestPeerSyncGiztest runs the committed scenario with real WebRTC devices,
// API keys, authoritative HTTP SSE, and SQLite state without provider credentials.
func TestPeerSyncGiztest(t *testing.T) {
	runPeerSyncGiztest(t, func(ctx context.Context, file, report string) ([]byte, error) {
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", file, "--parallel", "1", "--output", report})
		err := command.ExecuteContext(ctx)
		return output.Bytes(), err
	})
}

func runPeerSyncGiztest(t *testing.T, run func(context.Context, string, string) ([]byte, error)) {
	runPeerControlGiztest(t, "server.peer.sync.giztest.yaml", 23, 2, run)
}

func runPeerControlGiztest(t *testing.T, scenario string, steps, cleanup int, run func(context.Context, string, string) ([]byte, error), setup ...func(*gizclaw.Server, *gizcli.Client)) {
	t.Helper()
	runPeerControlGiztestWithChannels(t, scenario, steps, cleanup, 8, run, setup...)
}

func runPeerControlGiztestWithChannels(t *testing.T, scenario string, steps, cleanup, channelsPerSession int, run func(context.Context, string, string) ([]byte, error), setup ...func(*gizclaw.Server, *gizcli.Client)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	cfg := validLayeredConfig(t.TempDir())
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AdminPublicKey = key.Public
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cfg.EdgeNodes = []giznet.PublicKey{edgeKey.Public}
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(srv)
	srv.PublicEndpoint = strings.TrimPrefix(httpServer.URL, "http://")
	srv.PeerListenerFactories = []gizclaw.PeerListenerFactory{func(opts gizclaw.PeerListenerOptions) (giznet.Listener, error) {
		listener, err := (&gizwebrtc.ListenConfig{SecurityPolicy: opts.SecurityPolicy, PeerEventHandler: opts.PeerEventHandler}).Listen(opts.KeyPair)
		if err == nil {
			srv.WebRTCSignalingHandler = listener.SignalingHandler()
		}
		return listener, err
	}}
	if err := srv.Listen(); err != nil {
		httpServer.Close()
		_ = srv.Close()
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve() }()
	admin := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		return gizwebrtc.Dial(ctx, key, srv.PublicKey(), gizwebrtc.DialConfig{SignalingURL: httpServer.URL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
	}}
	stop := sync.OnceFunc(func() {
		_ = admin.Close()
		httpServer.Close()
		if err := srv.Close(); err != nil {
			t.Error(err)
		}
		select {
		case err := <-served:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Server did not stop")
		}
	})
	t.Cleanup(stop)
	if err := admin.Dial(srv.PublicKey(), httpServer.URL); err != nil {
		t.Fatal(err)
	}
	go func() { _ = admin.Serve() }()
	profile := adminhttp.RuntimeProfileUpsert{Id: "peer-sync-giztest", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{}}}
	if _, err := adminapi.CreateRuntimeProfile(ctx, admin, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateRegistrationToken(ctx, admin, adminhttp.RegistrationTokenUpsert{Id: "peer-sync-token", Token: "local-peer-sync-test-token", RuntimeProfileId: profile.Id}); err != nil {
		t.Fatal(err)
	}
	edgeURL := startPeerSyncGiztestEdge(t, ctx, edgeKey, httpServer.URL, srv.PublicKey(), channelsPerSession)
	t.Setenv("GIZCLAW_TEST_ENDPOINT", edgeURL)
	t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "local-peer-sync-test-token")
	for _, configure := range setup {
		configure(srv.Server, admin)
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "tests", "gizclaw-e2e", "giztest", scenario)
	reportPath := filepath.Join(t.TempDir(), filepath.Base(scenario)+".report.json")
	output, runErr := run(ctx, file, reportPath)
	reportData, readErr := os.ReadFile(reportPath)
	if runErr != nil || readErr != nil {
		t.Fatalf("%s Giztest failed: %v, report: %v\n%s\n%s", scenario, runErr, readErr, output, reportData)
	}
	var report struct {
		Status string `json:"status"`
		Tasks  []struct {
			Steps []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"steps"`
			Cleanup []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"cleanup"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || len(report.Tasks) != 1 || len(report.Tasks[0].Steps) != steps || len(report.Tasks[0].Cleanup) != cleanup {
		t.Fatalf("Giztest must execute every step: %s", reportData)
	}
	for _, step := range append(report.Tasks[0].Steps, report.Tasks[0].Cleanup...) {
		if step.Status != "passed" {
			t.Fatalf("step %s: %s", step.ID, step.Status)
		}
	}
	t.Logf("%s: %d steps and %d cleanup steps passed\n%s", scenario, steps, cleanup, output)
}

func startPeerSyncGiztestEdge(t *testing.T, ctx context.Context, key *giznet.KeyPair, serverURL string, serverKey giznet.PublicKey, channelsPerSession int) string {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	config := fmt.Sprintf("identity:\n  private-key: %s\nwebrtc:\n  listen: %s\n  endpoint: %s\nupstreams:\n  - endpoint: %s\n    public-key: %s\nhttp:\n  listeners:\n    - listen: %s\ngateway:\n  enabled: true\n  max-sessions: 8\n  max-upstreams: 1\n  sessions-per-upstream: 8\n  channels-per-session: %d\n  channels-per-upstream: %d\n  max-pending-handshakes: 8\n  session-buffer-bytes: 1048576\n  idle-timeout: 1m\n  drain-timeout: 1s\n", key.Private.String(), address, address, serverURL, serverKey.String(), address, channelsPerSession, 4*channelsPerSession)
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	edgeCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- gizedge.ServeContext(edgeCtx, root) }()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("Edge did not stop")
		}
	})
	endpoint := "http://" + address
	readyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(readyCtx, http.MethodGet, endpoint+"/server-info", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return endpoint
			}
		}
		select {
		case <-readyCtx.Done():
			t.Fatalf("Edge not ready: %v", readyCtx.Err())
		case <-ticker.C:
		}
	}
}
