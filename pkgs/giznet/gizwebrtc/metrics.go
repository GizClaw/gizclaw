package gizwebrtc

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
)

const (
	metricSignalingRequests      = "giz_webrtc_signaling_requests_total"
	metricSignalingDuration      = "giz_webrtc_signaling_duration_seconds"
	metricSignalingPhaseDuration = "giz_webrtc_signaling_phase_duration_seconds"
	metricDialRequests           = "giz_webrtc_dial_requests_total"
	metricDialAttempts           = "giz_webrtc_dial_attempts_total"
	metricDialDuration           = "giz_webrtc_dial_duration_seconds"
	metricDialPhaseDuration      = "giz_webrtc_dial_phase_duration_seconds"
	metricConnections            = "giz_webrtc_connections_total"
	metricConnectionsActive      = "giz_webrtc_connections_active"
	metricConnectionDuration     = "giz_webrtc_connection_duration_seconds"
	metricServiceRequests        = "giz_webrtc_service_requests_total"
	metricServiceDuration        = "giz_webrtc_service_open_duration_seconds"
	metricServiceStreamsActive   = "giz_webrtc_service_streams_active"
	defaultMetricsNodeRole       = "application"
	metricsNodeRoleEdge          = "edge"
)

var webrtcDurationBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

var activeMetrics = struct {
	sync.Mutex
	connections map[string]int
	services    map[string]int
}{connections: make(map[string]int), services: make(map[string]int)}

func normalizedMetricsNodeRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case metricsNodeRoleEdge:
		return metricsNodeRoleEdge
	default:
		return defaultMetricsNodeRole
	}
}

func recordSignalingRequest(ctx context.Context, nodeRole, result string, started time.Time) {
	labels := []gizmetrics.Label{
		{Name: "node_role", Value: normalizedMetricsNodeRole(nodeRole)},
		{Name: "result", Value: result},
	}
	gizmetrics.AddCounter(ctx, metricSignalingRequests, 1, labels...)
	gizmetrics.ObserveHistogram(ctx, metricSignalingDuration, time.Since(started).Seconds(), webrtcDurationBuckets, labels...)
}

func recordSignalingPhases(ctx context.Context, nodeRole, result string, timing signalingTiming) {
	phases := []struct {
		name     string
		duration time.Duration
	}{
		{name: "peer_connection", duration: timing.peerConnection},
		{name: "set_remote", duration: timing.setRemote},
		{name: "create_answer", duration: timing.createAnswer},
		{name: "set_local", duration: timing.setLocal},
		{name: "ice_gathering", duration: timing.iceGathering},
		{name: "rewrite_sdp", duration: timing.rewriteSDP},
	}
	for _, phase := range phases {
		if phase.duration <= 0 {
			continue
		}
		gizmetrics.ObserveHistogram(ctx, metricSignalingPhaseDuration, phase.duration.Seconds(), webrtcDurationBuckets,
			gizmetrics.Label{Name: "node_role", Value: normalizedMetricsNodeRole(nodeRole)},
			gizmetrics.Label{Name: "result", Value: result},
			gizmetrics.Label{Name: "phase", Value: phase.name},
		)
	}
}

func signalingResult(status int) string {
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return "success"
	case status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable:
		return "over_capacity"
	case status >= http.StatusBadRequest && status < http.StatusInternalServerError:
		return "rejected"
	default:
		return "internal_error"
	}
}

func recordDial(ctx context.Context, nodeRole string, timing DialTiming, err error) {
	result := "success"
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			result = "canceled"
		case errors.Is(err, context.DeadlineExceeded):
			result = "timeout"
		default:
			result = "error"
		}
	}
	labels := []gizmetrics.Label{
		{Name: "node_role", Value: normalizedMetricsNodeRole(nodeRole)},
		{Name: "result", Value: result},
	}
	gizmetrics.AddCounter(ctx, metricDialRequests, 1, labels...)
	if timing.Attempts > 0 {
		gizmetrics.AddCounter(ctx, metricDialAttempts, float64(timing.Attempts), labels...)
	}
	gizmetrics.ObserveHistogram(ctx, metricDialDuration, timing.Total.Seconds(), webrtcDurationBuckets, labels...)
	phases := []struct {
		name     string
		duration time.Duration
	}{
		{name: "peer_connection", duration: timing.PeerConnectionConstruction},
		{name: "offer", duration: timing.OfferCreation},
		{name: "set_local", duration: timing.SetLocalDescription},
		{name: "ice_gathering", duration: timing.ICEGathering},
		{name: "http_signaling", duration: timing.HTTPSignaling},
		{name: "set_remote", duration: timing.SetRemoteDescription},
		{name: "ice_connected", duration: timing.ICEConnected},
		{name: "dtls_connected", duration: timing.DTLSConnected},
		{name: "data_channel_ready", duration: timing.DataChannelReady},
	}
	for _, phase := range phases {
		if phase.duration <= 0 {
			continue
		}
		phaseLabels := append(append([]gizmetrics.Label(nil), labels...), gizmetrics.Label{Name: "phase", Value: phase.name})
		gizmetrics.ObserveHistogram(ctx, metricDialPhaseDuration, phase.duration.Seconds(), webrtcDurationBuckets, phaseLabels...)
	}
}

func serviceResult(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "error"
	}
}

func recordServiceRequest(ctx context.Context, nodeRole, role, direction, result string, started time.Time) {
	labels := []gizmetrics.Label{
		{Name: "node_role", Value: normalizedMetricsNodeRole(nodeRole)},
		{Name: "role", Value: role},
		{Name: "direction", Value: direction},
		{Name: "result", Value: result},
	}
	gizmetrics.AddCounter(ctx, metricServiceRequests, 1, labels...)
	gizmetrics.ObserveHistogram(ctx, metricServiceDuration, time.Since(started).Seconds(), webrtcDurationBuckets, labels...)
}

func adjustActiveMetric(ctx context.Context, name, nodeRole, role string, delta int) {
	key := normalizedMetricsNodeRole(nodeRole) + "\x00" + role
	activeMetrics.Lock()
	values := activeMetrics.connections
	if name == metricServiceStreamsActive {
		values = activeMetrics.services
	}
	values[key] = max(0, values[key]+delta)
	value := values[key]
	activeMetrics.Unlock()
	gizmetrics.SetGauge(ctx, name, float64(value),
		gizmetrics.Label{Name: "node_role", Value: normalizedMetricsNodeRole(nodeRole)},
		gizmetrics.Label{Name: "role", Value: role},
	)
}
