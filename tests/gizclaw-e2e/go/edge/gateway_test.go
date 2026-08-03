//go:build gizclaw_e2e

package edge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

type gatewayEndpoint struct {
	endpoint     string
	serverKey    giznet.PublicKey
	transportKey giznet.PublicKey
	signalingURL string
}

func TestGatewayMultiEdgeAPIAndPacketLanes(t *testing.T) {
	first := loadGatewayEndpoint(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_ENDPOINT"))
	second := loadGatewayEndpoint(t, requiredEnv(t, "GIZCLAW_E2E_EDGE2_ENDPOINT"))
	if !first.serverKey.Equal(second.serverKey) {
		t.Fatal("gateway Edges advertise different authoritative Server identities")
	}
	if first.transportKey.Equal(second.transportKey) {
		t.Fatal("gateway Edges share a physical transport identity")
	}

	for index, endpoint := range []gatewayEndpoint{first, second} {
		client := connect(t, endpoint)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if _, err := client.Ping(ctx, fmt.Sprintf("gateway-edge-%d", index)); err != nil {
			cancel()
			client.Close()
			t.Fatalf("ping through edge %d: %v", index, err)
		}
		cancel()
		conn := client.PeerConn()
		if conn == nil {
			client.Close()
			t.Fatalf("edge %d has no peer connection", index)
		}
		if _, err := conn.Write(0x40, []byte{byte(index), 0x42}); err != nil {
			client.Close()
			t.Fatalf("direct packet through edge %d: %v", index, err)
		}
		if _, err := conn.Write(giznet.ProtocolOpusPacket, []byte{0x00, 0xaa, byte(index)}); err != nil {
			client.Close()
			t.Fatalf("Opus packet through edge %d: %v", index, err)
		}
		if !client.ServerPublicKey().Equal(endpoint.serverKey) {
			client.Close()
			t.Fatalf("edge %d replaced authoritative Server identity", index)
		}
		client.Close()
	}
}

func TestGatewayOneEdgeFailureIsIsolated(t *testing.T) {
	first := loadGatewayEndpoint(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_ENDPOINT"))
	second := loadGatewayEndpoint(t, requiredEnv(t, "GIZCLAW_E2E_EDGE2_ENDPOINT"))
	firstClient := connect(t, first)
	defer firstClient.Close()
	secondClient := connect(t, second)
	defer secondClient.Close()
	assertPing(t, firstClient, "stopped-edge-baseline")
	assertPing(t, secondClient, "surviving-edge-baseline")

	serverContainer := composeOutput(t, "ps", "-q", "server")
	secondEdgeContainer := composeOutput(t, "ps", "-q", "edge2")
	compose(t, "stop", "--timeout", "1", "edge")
	if got := composeOutput(t, "ps", "-q", "server"); got != serverContainer {
		t.Fatalf("Server container changed while stopping Edge1: before=%s after=%s", serverContainer, got)
	}
	if got := composeOutput(t, "ps", "-q", "edge2"); got != secondEdgeContainer {
		t.Fatalf("Edge2 container changed while stopping Edge1: before=%s after=%s", secondEdgeContainer, got)
	}
	t.Cleanup(func() {
		compose(t, "up", "--no-deps", "-d", "edge")
		waitHTTP(t, first.endpoint)
	})

	assertPing(t, secondClient, "surviving-edge-immediate")
	deadline := time.Now().Add(20 * time.Second)
	stoppedSessionFailed := false
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := firstClient.Ping(ctx, "stopped-edge")
		cancel()
		if err != nil {
			stoppedSessionFailed = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !stoppedSessionFailed {
		t.Fatal("session pinned to stopped Edge remained usable")
	}
	assertPing(t, secondClient, "surviving-edge")

	newClient := connect(t, second)
	defer newClient.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := newClient.Ping(ctx, "new-session-surviving-edge"); err != nil {
		t.Fatalf("new session through surviving Edge: %v", err)
	}
}

func assertPing(t *testing.T, client *gizcli.Client, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.Ping(ctx, id); err != nil {
		t.Fatalf("%s: %v", id, err)
	}
}

func loadGatewayEndpoint(t *testing.T, endpoint string) gatewayEndpoint {
	t.Helper()
	base := httpBase(t, endpoint)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(base + "/server-info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s/server-info status = %s", base, resp.Status)
	}
	var info apitypes.ServerInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.Transport == nil || info.Transport.Mode != "edge-gateway" {
		t.Fatalf("%s transport = %#v", endpoint, info.Transport)
	}
	if info.IceServers != nil {
		t.Fatalf("%s retained authoritative ICE/TURN servers", endpoint)
	}
	var serverKey, transportKey giznet.PublicKey
	if err := serverKey.UnmarshalText([]byte(info.PublicKey)); err != nil {
		t.Fatal(err)
	}
	if err := transportKey.UnmarshalText([]byte(info.Transport.PublicKey)); err != nil {
		t.Fatal(err)
	}
	return gatewayEndpoint{
		endpoint: endpoint, serverKey: serverKey, transportKey: transportKey,
		signalingURL: httpBase(t, info.Transport.Endpoint) + info.Transport.SignalingPath,
	}
}

func connect(t *testing.T, endpoint gatewayEndpoint) *gizcli.Client {
	t.Helper()
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	client := &gizcli.Client{
		KeyPair: key,
		DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			return gizwebrtc.Dial(ctx, key, endpoint.transportKey, gizwebrtc.DialConfig{
				SignalingURL: endpoint.signalingURL, SecurityPolicy: policy,
			})
		},
	}
	if err := client.Dial(endpoint.serverKey, endpoint.endpoint); err != nil {
		t.Fatal(err)
	}
	go func() { _ = client.Serve() }()
	return client
}

func compose(t *testing.T, args ...string) {
	t.Helper()
	_ = composeOutput(t, args...)
}

func composeOutput(t *testing.T, args ...string) string {
	t.Helper()
	project := requiredEnv(t, "GIZCLAW_E2E_DOCKER_PROJECT")
	composeFile := requiredEnv(t, "GIZCLAW_E2E_DOCKER_COMPOSE_FILE")
	commandArgs := []string{"compose", "-p", project, "-f", composeFile}
	if overlay := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY")); overlay != "" {
		absolute, err := filepath.Abs(overlay)
		if err != nil {
			t.Fatal(err)
		}
		commandArgs = append(commandArgs, "-f", absolute)
	}
	commandArgs = append(commandArgs, args...)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(commandArgs, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func waitHTTP(t *testing.T, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	target := httpBase(t, endpoint) + "/server-info"
	for time.Now().Before(deadline) {
		resp, err := (&http.Client{Timeout: time.Second}).Get(target)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("Edge did not recover at %s", target)
}

func httpBase(t *testing.T, endpoint string) string {
	t.Helper()
	endpoint = strings.TrimSpace(endpoint)
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		t.Fatalf("invalid endpoint %q", endpoint)
	}
	parsed.Path, parsed.RawQuery, parsed.Fragment = "", "", ""
	return strings.TrimSuffix(parsed.String(), "/")
}

func requiredEnv(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required; source tests/gizclaw-e2e/testdata/docker/current.env", name)
	}
	if name == "GIZCLAW_E2E_DOCKER_COMPOSE_FILE" {
		absolute, err := filepath.Abs(value)
		if err != nil {
			t.Fatal(err)
		}
		return absolute
	}
	return value
}
