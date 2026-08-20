//go:build gizclaw_e2e

package edge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/pion/webrtc/v4"
)

const coturnProductMetricScript = `
set -euo pipefail
exec 3<>/dev/tcp/127.0.0.1/9641
printf 'GET /metrics HTTP/1.0\r\nHost: localhost\r\n\r\n' >&3
cat <&3
`

type coturnProductMetrics struct {
	allocations   float64
	receivedBytes float64
	sentBytes     float64
}

func TestCoturnRelay(t *testing.T) {
	info := fetchCoturnProductServerInfo(t)
	iceServer := requireCoturnProductServerInfo(t, info)

	t.Run("authenticated_ping_and_cleanup", func(t *testing.T) {
		baseline := readCoturnProductMetrics(t)
		client := newCoturnProductClient(t, info, iceServer)
		closed := false
		defer func() {
			if !closed {
				_ = client.Close()
			}
		}()
		if err := client.Dial(coturnProductServerKey(t, info), requiredCoturnProductEnv(t, "GIZCLAW_E2E_SERVER_ENDPOINT")); err != nil {
			t.Fatalf("relay-only authoritative Server Dial: %v", err)
		}
		serveDone := make(chan error, 1)
		go func() { serveDone <- client.Serve() }()
		waitCoturnProductMetrics(t, 15*time.Second, func(metrics coturnProductMetrics) bool {
			return metrics.allocations >= baseline.allocations+2
		}, "at least two live authenticated allocations")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := client.Ping(ctx, "coturn-relay-ping"); err != nil {
			t.Fatalf("Ping through Coturn: %v", err)
		}
		if err := client.Close(); err != nil {
			t.Fatalf("close Coturn product client: %v", err)
		}
		closed = true
		select {
		case err := <-serveDone:
			if err != nil {
				t.Fatalf("serve Coturn product client: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Coturn product client Serve did not stop")
		}
		waitCoturnProductMetrics(t, 15*time.Second, func(metrics coturnProductMetrics) bool {
			return metrics.allocations == baseline.allocations &&
				metrics.receivedBytes > baseline.receivedBytes &&
				metrics.sentBytes > baseline.sentBytes
		}, "allocation cleanup and bidirectional finished-session traffic")
	})

	t.Run("corrupted_temporary_credential_fails_closed", func(t *testing.T) {
		baseline := readCoturnProductMetrics(t)
		invalid := iceServer
		invalid.Credential += "-invalid"
		client := newCoturnProductClient(t, info, invalid)
		err := client.Dial(coturnProductServerKey(t, info), requiredCoturnProductEnv(t, "GIZCLAW_E2E_SERVER_ENDPOINT"))
		_ = client.Close()
		if err == nil {
			t.Fatal("relay-only authoritative Server Dial with corrupted credential succeeded")
		}
		metrics := readCoturnProductMetrics(t)
		// The authoritative Server creates its own valid relay allocation when it
		// answers signaling. A corrupted client credential therefore permits at
		// most that one-sided allocation, never the connected allocation pair.
		if metrics.allocations > baseline.allocations+1 {
			t.Fatalf("corrupted client credential created a connected allocation pair: baseline=%s after=%s", baseline, metrics)
		}
	})
}

func fetchCoturnProductServerInfo(t *testing.T) apitypes.ServerInfo {
	t.Helper()
	endpoint := requiredCoturnProductEnv(t, "GIZCLAW_E2E_SERVER_ENDPOINT")
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("http://" + endpoint + "/server-info")
	if err != nil {
		t.Fatalf("GET authoritative ServerInfo: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET authoritative ServerInfo status = %s", response.Status)
	}
	var info apitypes.ServerInfo
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		t.Fatalf("decode authoritative ServerInfo: %v", err)
	}
	return info
}

func requireCoturnProductServerInfo(t *testing.T, info apitypes.ServerInfo) gizwebrtc.ICEServer {
	t.Helper()
	if info.Protocol != "gizclaw-webrtc" || info.Transport != nil {
		t.Fatalf("authoritative ServerInfo protocol/transport = %q/%#v", info.Protocol, info.Transport)
	}
	if info.IceServers == nil || len(*info.IceServers) != 1 {
		t.Fatalf("authoritative ServerInfo ICE servers = %#v, want exactly one", info.IceServers)
	}
	server := (*info.IceServers)[0]
	if len(server.Urls) != 1 || !strings.HasPrefix(server.Urls[0], "turn:") || !strings.HasSuffix(server.Urls[0], "?transport=udp") {
		t.Fatalf("authoritative ServerInfo TURN URLs = %v", server.Urls)
	}
	if server.Username == nil || server.Credential == nil || *server.Username == "" || *server.Credential == "" {
		t.Fatal("authoritative ServerInfo omitted temporary TURN REST credentials")
	}
	keyID := requiredCoturnProductEnv(t, "GIZCLAW_E2E_TURN_USERNAME")
	expiresText, suffix, found := strings.Cut(*server.Username, ":")
	expires, err := strconv.ParseInt(expiresText, 10, 64)
	if err != nil || !found || suffix != keyID || expires <= time.Now().Unix() {
		t.Fatalf("authoritative ServerInfo TURN REST username has invalid expiry or key ID")
	}
	return gizwebrtc.ICEServer{
		URLs:       append([]string(nil), server.Urls...),
		Username:   *server.Username,
		Credential: *server.Credential,
	}
}

func newCoturnProductClient(
	t *testing.T,
	info apitypes.ServerInfo,
	iceServer gizwebrtc.ICEServer,
) *gizcli.Client {
	t.Helper()
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	signalingPath := info.SignalingPath
	if signalingPath == "" {
		signalingPath = gizwebrtc.SignalingPath
	}
	return &gizcli.Client{
		KeyPair: key,
		DialTransport: func(key *giznet.KeyPair, serverKey giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			return gizwebrtc.Dial(ctx, key, serverKey, gizwebrtc.DialConfig{
				SignalingURL:       "http://" + requiredCoturnProductEnv(t, "GIZCLAW_E2E_SERVER_ENDPOINT") + signalingPath,
				ICEServers:         []gizwebrtc.ICEServer{iceServer},
				ICETransportPolicy: webrtc.ICETransportPolicyRelay,
				SecurityPolicy:     policy,
			})
		},
	}
}

func coturnProductServerKey(t *testing.T, info apitypes.ServerInfo) giznet.PublicKey {
	t.Helper()
	var serverKey giznet.PublicKey
	if err := serverKey.UnmarshalText([]byte(info.PublicKey)); err != nil {
		t.Fatalf("authoritative ServerInfo public key: %v", err)
	}
	return serverKey
}

func readCoturnProductMetrics(t *testing.T) coturnProductMetrics {
	t.Helper()
	project := requiredCoturnProductEnv(t, "GIZCLAW_E2E_DOCKER_PROJECT")
	composeFile, err := filepath.Abs(requiredCoturnProductEnv(t, "GIZCLAW_E2E_DOCKER_COMPOSE_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(
		ctx,
		"docker", "compose", "-p", project, "-f", composeFile,
		"exec", "-T", "turn", "bash", "-lc", coturnProductMetricScript,
	).Output()
	if err != nil {
		t.Fatalf("read product Coturn metrics: %v", err)
	}
	values := make(map[string]float64)
	for line := range strings.Lines(string(output)) {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		name, _, _ := strings.Cut(fields[0], "{")
		value, parseErr := strconv.ParseFloat(fields[1], 64)
		if parseErr == nil {
			values[name] += value
		}
	}
	return coturnProductMetrics{
		allocations:   values["turn_total_allocations"],
		receivedBytes: values["turn_total_traffic_rcvb"],
		sentBytes:     values["turn_total_traffic_sentb"],
	}
}

func waitCoturnProductMetrics(
	t *testing.T,
	timeout time.Duration,
	ready func(coturnProductMetrics) bool,
	description string,
) coturnProductMetrics {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var metrics coturnProductMetrics
	for time.Now().Before(deadline) {
		metrics = readCoturnProductMetrics(t)
		if ready(metrics) {
			return metrics
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("product Coturn metrics = %+v, want %s", metrics, description)
	return coturnProductMetrics{}
}

func requiredCoturnProductEnv(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required; use tests/gizclaw-e2e/run_turn_relay_tests.sh", name)
	}
	return value
}

func (m coturnProductMetrics) String() string {
	return fmt.Sprintf("allocations=%.0f received=%.0f sent=%.0f", m.allocations, m.receivedBytes, m.sentBytes)
}
