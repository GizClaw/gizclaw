//go:build giznet_e2e && giznet_coturn_e2e

package webrtc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/pion/webrtc/v4"
)

const (
	coturnDialSamples       = 30
	coturnRTTSamples        = 200
	coturnThroughputSamples = 3
	coturnThroughputBytes   = 32 * 1024 * 1024
	coturnImage             = "coturn/coturn:4.7.0-r0@sha256:99bf5bf6ab1c119862d0c3d2dfb2bbf805a86a131492cab18c148be64ae7d978"
)

type coturnMeasurementPath struct {
	Name       string
	Service    string
	ICEServers []gizwebrtc.ICEServer
	Policy     webrtc.ICETransportPolicy
}

type coturnMeasurementArtifact struct {
	StartedAt       time.Time                `json:"started_at"`
	FinishedAt      time.Time                `json:"finished_at"`
	RepositoryHead  string                   `json:"repository_head"`
	RepositoryDirty bool                     `json:"repository_dirty"`
	GoVersion       string                   `json:"go_version"`
	GOOS            string                   `json:"goos"`
	GOARCH          string                   `json:"goarch"`
	Docker          string                   `json:"docker"`
	CoturnImage     string                   `json:"coturn_image"`
	CoturnVersion   string                   `json:"coturn_version"`
	DialSamples     int                      `json:"dial_samples"`
	RTTSamples      int                      `json:"rtt_samples"`
	ThroughputRuns  int                      `json:"throughput_runs"`
	ThroughputBytes int                      `json:"throughput_bytes"`
	Paths           []coturnPathMeasurements `json:"paths"`
	Comparisons     []coturnPathComparison   `json:"direct_to_coturn_comparisons"`
}

type coturnPathMeasurements struct {
	Name               string                        `json:"name"`
	DialTotalMS        measurementSummary            `json:"dial_total_ms"`
	DialPhasesMS       map[string]measurementSummary `json:"dial_phases_ms"`
	DialSamples        []coturnDialMeasurement       `json:"dial_samples"`
	RTTMS              measurementSummary            `json:"rtt_ms"`
	RTTSamples         []float64                     `json:"rtt_samples_ms"`
	ClientToListener   []float64                     `json:"client_to_listener_mbps"`
	ListenerToClient   []float64                     `json:"listener_to_client_mbps"`
	ClientMedianMbps   float64                       `json:"client_to_listener_median_mbps"`
	ListenerMedianMbps float64                       `json:"listener_to_client_median_mbps"`
	CoturnBefore       coturnMetrics                 `json:"coturn_before"`
	CoturnAfter        coturnMetrics                 `json:"coturn_after"`
}

type coturnPathComparison struct {
	Path                      string  `json:"path"`
	DialP50DeltaMS            float64 `json:"dial_p50_delta_ms"`
	DialP50RelayToDirectRatio float64 `json:"dial_p50_relay_to_direct_ratio"`
	RTTP50DeltaMS             float64 `json:"rtt_p50_delta_ms"`
	RTTP50RelayToDirectRatio  float64 `json:"rtt_p50_relay_to_direct_ratio"`
	ClientToListenerMbpsRatio float64 `json:"client_to_listener_relay_to_direct_ratio"`
	ListenerToClientMbpsRatio float64 `json:"listener_to_client_relay_to_direct_ratio"`
}

type coturnDialMeasurement struct {
	TotalMS                float64 `json:"total_ms"`
	PeerConnectionMS       float64 `json:"peer_connection_ms"`
	OfferMS                float64 `json:"offer_ms"`
	ICEGatheringMS         float64 `json:"ice_gathering_ms"`
	SignalingMS            float64 `json:"signaling_ms"`
	SetRemoteDescriptionMS float64 `json:"set_remote_description_ms"`
	ICEConnectedMS         float64 `json:"ice_connected_ms"`
	DTLSConnectedMS        float64 `json:"dtls_connected_ms"`
	DataChannelReadyMS     float64 `json:"data_channel_ready_ms"`
}

type measurementSummary struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

type coturnMeasurementConnection struct {
	server         *webRTCServer
	clientListener giznet.Listener
	clientConn     giznet.Conn
	serverConn     giznet.Conn
}

func TestCoturnTransportMeasurements(t *testing.T) {
	started := time.Now().UTC()
	paths := []coturnMeasurementPath{{Name: "direct"}}
	for _, fixture := range coturnFixtures(t) {
		paths = append(paths, coturnMeasurementPath{
			Name: fixture.name, Service: fixture.service,
			ICEServers: fixture.iceServers, Policy: webrtc.ICETransportPolicyRelay,
		})
	}
	artifact := coturnMeasurementArtifact{
		StartedAt: started, GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		CoturnImage: coturnImage, CoturnVersion: "4.7.0",
		DialSamples: coturnDialSamples, RTTSamples: coturnRTTSamples,
		ThroughputRuns: coturnThroughputSamples, ThroughputBytes: coturnThroughputBytes,
	}
	artifact.RepositoryHead = commandOutput(t, "git", "rev-parse", "HEAD")
	artifact.RepositoryDirty = commandOutput(t, "git", "status", "--porcelain") != ""
	artifact.Docker = commandOutput(t, "docker", "version", "--format", "{{.Server.Os}}/{{.Server.Arch}} {{.Server.Version}}")

	for _, path := range paths {
		path := path
		t.Run(path.Name, func(t *testing.T) {
			artifact.Paths = append(artifact.Paths, measureCoturnPath(t, path))
		})
	}
	artifact.Comparisons = compareCoturnPaths(t, artifact.Paths)
	artifact.FinishedAt = time.Now().UTC()
	writeCoturnMeasurementArtifact(t, artifact)
}

func measureCoturnPath(t *testing.T, path coturnMeasurementPath) coturnPathMeasurements {
	result := coturnPathMeasurements{Name: path.Name}
	if path.Service != "" {
		result.CoturnBefore = readCoturnMetrics(t, path.Service)
	}
	totals := make([]float64, 0, coturnDialSamples)
	result.DialSamples = make([]coturnDialMeasurement, 0, coturnDialSamples)
	for range coturnDialSamples {
		connection, total, timing := openCoturnMeasurementConnection(t, path)
		result.DialSamples = append(result.DialSamples, coturnDialMeasurement{
			TotalMS:                milliseconds(total),
			PeerConnectionMS:       milliseconds(timing.PeerConnectionConstruction),
			OfferMS:                milliseconds(timing.OfferCreation),
			ICEGatheringMS:         milliseconds(timing.ICEGathering),
			SignalingMS:            milliseconds(timing.HTTPSignaling),
			SetRemoteDescriptionMS: milliseconds(timing.SetRemoteDescription),
			ICEConnectedMS:         milliseconds(timing.ICEConnected),
			DTLSConnectedMS:        milliseconds(timing.DTLSConnected),
			DataChannelReadyMS:     milliseconds(timing.DataChannelReady),
		})
		totals = append(totals, milliseconds(total))
		connection.close(t)
		waitCoturnPathCleanup(t, path)
	}
	result.DialTotalMS = summarizeMeasurements(totals)
	result.DialPhasesMS = summarizeCoturnDialPhases(result.DialSamples)

	connection, _, _ := openCoturnMeasurementConnection(t, path)
	closed := false
	defer func() {
		if !closed {
			connection.close(t)
		}
	}()
	done := serveEchoService(t, connection.serverConn)
	payload := bytes.Repeat([]byte{0x5a}, 64)
	for range coturnRTTSamples {
		started := time.Now()
		got := roundTripStream(t, connection.clientConn, payload)
		if !bytes.Equal(got, payload) {
			t.Fatal("RTT echo payload mismatch")
		}
		result.RTTSamples = append(result.RTTSamples, milliseconds(time.Since(started)))
	}
	connection.serverConn.CloseService(echoService)
	waitServerDone(t, done)
	connection.close(t)
	closed = true
	waitCoturnPathCleanup(t, path)
	result.RTTMS = summarizeMeasurements(result.RTTSamples)

	for range coturnThroughputSamples {
		result.ClientToListener = append(result.ClientToListener, measureCoturnThroughput(t, path, true))
		result.ListenerToClient = append(result.ListenerToClient, measureCoturnThroughput(t, path, false))
	}
	result.ClientMedianMbps = median(result.ClientToListener)
	result.ListenerMedianMbps = median(result.ListenerToClient)
	if path.Service != "" {
		result.CoturnAfter = waitCoturnMetrics(t, path.Service, 15*time.Second, func(metrics coturnMetrics) bool {
			return metrics.Allocations == result.CoturnBefore.Allocations &&
				metrics.ReceivedBytes > result.CoturnBefore.ReceivedBytes &&
				metrics.SentBytes > result.CoturnBefore.SentBytes
		}, "measurement traffic and allocation cleanup")
	}
	return result
}

func openCoturnMeasurementConnection(
	t testing.TB,
	path coturnMeasurementPath,
) (*coturnMeasurementConnection, time.Duration, gizwebrtc.DialTiming) {
	t.Helper()
	serverKey := mustKeyPair(t)
	clientKey := mustKeyPair(t)
	listenConfig := gizwebrtc.ListenConfig{
		CipherMode: gizwebrtc.CipherModePlaintext, SecurityPolicy: allowAllPolicy{},
		ICEServers: path.ICEServers, ICETransportPolicy: path.Policy,
	}
	server := startWebRTCServerWithConfig(t, serverKey, listenConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var timing gizwebrtc.DialTiming
	started := time.Now()
	clientListener, clientConn, err := gizwebrtc.Dial(ctx, clientKey, serverKey.Public, gizwebrtc.DialConfig{
		SignalingURL: server.signalingURL, CipherMode: gizwebrtc.CipherModePlaintext,
		SecurityPolicy: allowAllPolicy{}, ICEServers: path.ICEServers, ICETransportPolicy: path.Policy,
		OnTiming: func(observation gizwebrtc.DialTiming) { timing = observation },
	})
	total := time.Since(started)
	if err != nil {
		server.Close()
		t.Fatalf("%s measurement Dial: %v", path.Name, err)
	}
	returned := false
	defer func() {
		if returned {
			return
		}
		_ = clientConn.Close()
		_ = clientListener.Close()
		server.Close()
	}()
	serverConn := acceptConn(t, server.listener)
	returned = true
	return &coturnMeasurementConnection{
		server: server, clientListener: clientListener,
		clientConn: clientConn, serverConn: serverConn,
	}, total, timing
}

func (c *coturnMeasurementConnection) close(tb testing.TB) {
	tb.Helper()
	for _, closeFn := range []func() error{c.clientConn.Close, c.clientListener.Close, c.serverConn.Close} {
		if err := closeFn(); err != nil {
			tb.Errorf("close measurement peer: %v", err)
		}
	}
	c.server.Close()
}

func waitCoturnPathCleanup(tb testing.TB, path coturnMeasurementPath) {
	tb.Helper()
	if path.Service == "" {
		return
	}
	waitCoturnMetrics(tb, path.Service, 15*time.Second, func(metrics coturnMetrics) bool {
		return metrics.Allocations == 0
	}, "zero live allocations")
}

func measureCoturnThroughput(t *testing.T, path coturnMeasurementPath, clientToListener bool) float64 {
	t.Helper()
	connection, _, _ := openCoturnMeasurementConnection(t, path)
	closed := false
	defer func() {
		if !closed {
			connection.close(t)
		}
	}()
	service := connection.serverConn.ListenService(echoService)
	accepted := make(chan struct {
		stream io.ReadWriteCloser
		err    error
	}, 1)
	go func() {
		stream, err := service.Accept()
		accepted <- struct {
			stream io.ReadWriteCloser
			err    error
		}{stream: stream, err: err}
	}()
	clientStream, err := connection.clientConn.Dial(echoService)
	if err != nil {
		connection.close(t)
		t.Fatalf("%s throughput Dial service: %v", path.Name, err)
	}
	serverResult := <-accepted
	if serverResult.err != nil {
		connection.close(t)
		t.Fatalf("%s throughput Accept service: %v", path.Name, serverResult.err)
	}
	serverStream := serverResult.stream
	payload := bytes.Repeat([]byte{0xa5}, coturnThroughputBytes)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	errCh := make(chan error, 2)
	started := time.Now()
	if clientToListener {
		go copyExactly(ctx, errCh, serverStream, io.Discard, coturnThroughputBytes)
		go writeExactly(ctx, errCh, clientStream, payload)
	} else {
		go copyExactly(ctx, errCh, clientStream, io.Discard, coturnThroughputBytes)
		go writeExactly(ctx, errCh, serverStream, payload)
	}
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("%s throughput transfer: %v", path.Name, err)
		}
	}
	duration := time.Since(started)
	_ = clientStream.Close()
	_ = serverStream.Close()
	_ = service.Close()
	connection.close(t)
	closed = true
	waitCoturnPathCleanup(t, path)
	return float64(coturnThroughputBytes*8) / duration.Seconds() / 1_000_000
}

func summarizeCoturnDialPhases(samples []coturnDialMeasurement) map[string]measurementSummary {
	phases := map[string][]float64{
		"peer_connection":        nil,
		"offer":                  nil,
		"ice_gathering":          nil,
		"http_signaling":         nil,
		"set_remote_description": nil,
		"ice_connected":          nil,
		"dtls_connected":         nil,
		"data_channel_ready":     nil,
	}
	for _, sample := range samples {
		phases["peer_connection"] = append(phases["peer_connection"], sample.PeerConnectionMS)
		phases["offer"] = append(phases["offer"], sample.OfferMS)
		phases["ice_gathering"] = append(phases["ice_gathering"], sample.ICEGatheringMS)
		phases["http_signaling"] = append(phases["http_signaling"], sample.SignalingMS)
		phases["set_remote_description"] = append(phases["set_remote_description"], sample.SetRemoteDescriptionMS)
		phases["ice_connected"] = append(phases["ice_connected"], sample.ICEConnectedMS)
		phases["dtls_connected"] = append(phases["dtls_connected"], sample.DTLSConnectedMS)
		phases["data_channel_ready"] = append(phases["data_channel_ready"], sample.DataChannelReadyMS)
	}
	summaries := make(map[string]measurementSummary, len(phases))
	for name, values := range phases {
		summaries[name] = summarizeMeasurements(values)
	}
	return summaries
}

func compareCoturnPaths(t *testing.T, paths []coturnPathMeasurements) []coturnPathComparison {
	t.Helper()
	if len(paths) < 2 || paths[0].Name != "direct" {
		t.Fatal("Coturn measurements require direct path first")
	}
	direct := paths[0]
	comparisons := make([]coturnPathComparison, 0, len(paths)-1)
	for _, relay := range paths[1:] {
		comparisons = append(comparisons, coturnPathComparison{
			Path:                      relay.Name,
			DialP50DeltaMS:            relay.DialTotalMS.P50 - direct.DialTotalMS.P50,
			DialP50RelayToDirectRatio: measurementRatio(t, relay.DialTotalMS.P50, direct.DialTotalMS.P50, "dial p50"),
			RTTP50DeltaMS:             relay.RTTMS.P50 - direct.RTTMS.P50,
			RTTP50RelayToDirectRatio:  measurementRatio(t, relay.RTTMS.P50, direct.RTTMS.P50, "RTT p50"),
			ClientToListenerMbpsRatio: measurementRatio(t, relay.ClientMedianMbps, direct.ClientMedianMbps, "client-to-listener throughput"),
			ListenerToClientMbpsRatio: measurementRatio(t, relay.ListenerMedianMbps, direct.ListenerMedianMbps, "listener-to-client throughput"),
		})
	}
	return comparisons
}

func measurementRatio(t *testing.T, numerator, denominator float64, name string) float64 {
	t.Helper()
	if numerator <= 0 || denominator <= 0 {
		t.Fatalf("%s ratio inputs must be positive: numerator=%f denominator=%f", name, numerator, denominator)
	}
	return numerator / denominator
}

func copyExactly(ctx context.Context, result chan<- error, source io.Reader, target io.Writer, count int64) {
	done := make(chan error, 1)
	go func() {
		_, err := io.CopyN(target, source, count)
		done <- err
	}()
	select {
	case err := <-done:
		result <- err
	case <-ctx.Done():
		result <- ctx.Err()
	}
}

func writeExactly(ctx context.Context, result chan<- error, target io.Writer, payload []byte) {
	done := make(chan error, 1)
	go func() {
		written, err := io.Copy(target, bytes.NewReader(payload))
		if err == nil && written != int64(len(payload)) {
			err = fmt.Errorf("wrote %d bytes, want %d", written, len(payload))
		}
		done <- err
	}()
	select {
	case err := <-done:
		result <- err
	case <-ctx.Done():
		result <- ctx.Err()
	}
}

func summarizeMeasurements(values []float64) measurementSummary {
	if len(values) == 0 {
		return measurementSummary{}
	}
	sorted := append([]float64(nil), values...)
	slices.Sort(sorted)
	return measurementSummary{
		Count: len(sorted), Min: sorted[0], Max: sorted[len(sorted)-1],
		P50: measurementPercentile(sorted, 0.50),
		P95: measurementPercentile(sorted, 0.95),
		P99: measurementPercentile(sorted, 0.99),
	}
}

func measurementPercentile(sorted []float64, percentile float64) float64 {
	index := int(percentile*float64(len(sorted)-1) + 0.5)
	return sorted[index]
}

func median(values []float64) float64 {
	return summarizeMeasurements(values).P50
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

func commandOutput(tb testing.TB, name string, args ...string) string {
	tb.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		tb.Fatalf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}

func writeCoturnMeasurementArtifact(tb testing.TB, artifact coturnMeasurementArtifact) {
	tb.Helper()
	path := requiredEnv(tb, "GIZNET_COTURN_ARTIFACT")
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		tb.Fatalf("encode Coturn measurement artifact: %v", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		tb.Fatalf("write Coturn measurement artifact: %v", err)
	}
}
