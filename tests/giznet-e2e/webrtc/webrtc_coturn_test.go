//go:build giznet_e2e && giznet_coturn_e2e

package webrtc_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/pion/webrtc/v4"
)

const coturnMetricScript = `
set -euo pipefail
exec 3<>/dev/tcp/127.0.0.1/9641
printf 'GET /metrics HTTP/1.0\r\nHost: localhost\r\n\r\n' >&3
cat <&3
`

type coturnFixture struct {
	name       string
	service    string
	iceServers []gizwebrtc.ICEServer
}

type coturnMetrics struct {
	Allocations   float64 `json:"allocations"`
	ReceivedBytes float64 `json:"received_bytes"`
	SentBytes     float64 `json:"sent_bytes"`
}

func TestWebRTCCoturn(t *testing.T) {
	for _, fixture := range coturnFixtures(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Run("packet_and_service_stream", func(t *testing.T) {
				testCoturnPacketAndServiceStream(t, fixture)
			})
			t.Run("invalid_authentication", func(t *testing.T) {
				testCoturnInvalidAuthentication(t, fixture)
			})
		})
	}
}

func coturnFixtures(tb testing.TB) []coturnFixture {
	tb.Helper()
	staticURL := requiredEnv(tb, "GIZNET_COTURN_STATIC_URL")
	restURL := requiredEnv(tb, "GIZNET_COTURN_REST_URL")
	return []coturnFixture{
		{
			name:    "static",
			service: "coturn-static",
			iceServers: []gizwebrtc.ICEServer{{
				URLs:           []string{staticURL},
				Username:       requiredEnv(tb, "GIZNET_COTURN_STATIC_USERNAME"),
				Credential:     requiredEnv(tb, "GIZNET_COTURN_STATIC_CREDENTIAL"),
				CredentialMode: gizwebrtc.ICECredentialModeStatic,
			}},
		},
		{
			name:    "turn_rest",
			service: "coturn-rest",
			iceServers: []gizwebrtc.ICEServer{{
				URLs:           []string{restURL},
				Username:       requiredEnv(tb, "GIZNET_COTURN_REST_KEY_ID"),
				Credential:     requiredEnv(tb, "GIZNET_COTURN_REST_SECRET"),
				CredentialMode: gizwebrtc.ICECredentialModeTURNREST,
			}},
		},
	}
}

func requiredEnv(tb testing.TB, name string) string {
	tb.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		tb.Fatalf("%s is required; use tests/giznet-e2e/run_coturn_tests.sh", name)
	}
	return value
}

func testCoturnPacketAndServiceStream(t *testing.T, fixture coturnFixture) {
	baseline := readCoturnMetrics(t, fixture.service)
	serverKey := mustKeyPair(t)
	clientKey := mustKeyPair(t)
	server := startWebRTCServerWithConfig(t, serverKey, gizwebrtc.ListenConfig{
		CipherMode:         gizwebrtc.CipherModePlaintext,
		SecurityPolicy:     allowAllPolicy{},
		ICEServers:         fixture.iceServers,
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
	})

	clientListener, clientConn := dialWebRTCWithConfig(t, clientKey, serverKey.Public, server.signalingURL, gizwebrtc.DialConfig{
		CipherMode:         gizwebrtc.CipherModePlaintext,
		SecurityPolicy:     allowAllPolicy{},
		ICEServers:         fixture.iceServers,
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
	})
	serverConn := acceptConn(t, server.listener)
	closed := false
	defer func() {
		if !closed {
			closeCoturnPeers(t, clientListener, clientConn, serverConn, server)
		}
	}()
	waitCoturnMetrics(t, fixture.service, 15*time.Second, func(metrics coturnMetrics) bool {
		return metrics.Allocations >= baseline.Allocations+2
	}, "at least two live allocations")

	roundTripPacket(t, clientConn, serverConn, 0x42, []byte("coturn relay packet"))
	done := serveEchoService(t, serverConn)
	payload := bytes.Repeat([]byte("coturn-relay-stream-"), 4096)
	if got := roundTripStream(t, clientConn, payload); !bytes.Equal(got, payload) {
		t.Fatalf("stream echo len=%d, want %d", len(got), len(payload))
	}
	serverConn.CloseService(echoService)
	waitServerDone(t, done)

	closeCoturnPeers(t, clientListener, clientConn, serverConn, server)
	closed = true
	waitCoturnMetrics(t, fixture.service, 15*time.Second, func(metrics coturnMetrics) bool {
		return metrics.Allocations == baseline.Allocations &&
			metrics.ReceivedBytes > baseline.ReceivedBytes &&
			metrics.SentBytes > baseline.SentBytes
	}, "allocation cleanup and bidirectional traffic counters")
}

func testCoturnInvalidAuthentication(t *testing.T, fixture coturnFixture) {
	baseline := readCoturnMetrics(t, fixture.service)
	serverKey := mustKeyPair(t)
	clientKey := mustKeyPair(t)
	server := startWebRTCServerWithConfig(t, serverKey, gizwebrtc.ListenConfig{
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: allowAllPolicy{},
	})
	defer server.Close()

	invalid := append([]gizwebrtc.ICEServer(nil), fixture.iceServers...)
	invalid[0].Credential += "-invalid"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	listener, conn, err := gizwebrtc.Dial(ctx, clientKey, serverKey.Public, gizwebrtc.DialConfig{
		SignalingURL:       server.signalingURL,
		CipherMode:         gizwebrtc.CipherModePlaintext,
		SecurityPolicy:     allowAllPolicy{},
		ICEServers:         invalid,
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
	})
	if conn != nil {
		_ = conn.Close()
	}
	if listener != nil {
		_ = listener.Close()
	}
	if err == nil {
		t.Fatal("Dial with invalid Coturn credential succeeded")
	}
	if errors.Is(err, context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("Dial returned unrelated deadline error: %v", err)
	}

	waitCoturnMetrics(t, fixture.service, 15*time.Second, func(metrics coturnMetrics) bool {
		return metrics == baseline
	}, "no authenticated allocation or finished traffic")
}

func closeCoturnPeers(
	t *testing.T,
	clientListener giznet.Listener,
	clientConn giznet.Conn,
	serverConn giznet.Conn,
	server *webRTCServer,
) {
	t.Helper()
	closeOperations := []struct {
		name    string
		closeFn func() error
	}{
		{name: "client connection", closeFn: clientConn.Close},
		{name: "client listener", closeFn: clientListener.Close},
		{name: "server connection", closeFn: serverConn.Close},
	}
	for _, operation := range closeOperations {
		name, closeFn := operation.name, operation.closeFn
		if err := closeFn(); err != nil {
			t.Errorf("close %s: %v", name, err)
		}
	}
	server.Close()
}

func readCoturnMetrics(tb testing.TB, service string) coturnMetrics {
	tb.Helper()
	project := requiredEnv(tb, "GIZNET_COTURN_DOCKER_PROJECT")
	composeFile := requiredEnv(tb, "GIZNET_COTURN_COMPOSE_FILE")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(
		ctx,
		"docker", "compose", "-p", project, "-f", composeFile,
		"exec", "-T", service, "bash", "-lc", coturnMetricScript,
	).Output()
	if err != nil {
		tb.Fatalf("read %s Coturn metrics: %v", service, err)
	}
	metrics := make(map[string]float64)
	for line := range strings.Lines(string(output)) {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		name, _, _ := strings.Cut(fields[0], "{")
		value, parseErr := strconv.ParseFloat(fields[1], 64)
		if parseErr == nil {
			metrics[name] += value
		}
	}
	return coturnMetrics{
		Allocations:   metrics["turn_total_allocations"],
		ReceivedBytes: metrics["turn_total_traffic_rcvb"],
		SentBytes:     metrics["turn_total_traffic_sentb"],
	}
}

func waitCoturnMetrics(
	tb testing.TB,
	service string,
	timeout time.Duration,
	ready func(coturnMetrics) bool,
	description string,
) coturnMetrics {
	tb.Helper()
	deadline := time.Now().Add(timeout)
	var metrics coturnMetrics
	for time.Now().Before(deadline) {
		metrics = readCoturnMetrics(tb, service)
		if ready(metrics) {
			return metrics
		}
		time.Sleep(100 * time.Millisecond)
	}
	tb.Fatalf("%s metrics = %+v, want %s", service, metrics, description)
	return coturnMetrics{}
}

func (m coturnMetrics) String() string {
	return fmt.Sprintf("allocations=%.0f received=%.0f sent=%.0f", m.Allocations, m.ReceivedBytes, m.SentBytes)
}
