package gizedge

import (
	"context"
	"errors"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	storemetrics "github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

const (
	metricEdgeRequests                 = "giz_edge_webrtc_requests_total"
	metricEdgeRequestDuration          = "giz_edge_webrtc_request_duration_seconds"
	metricEdgeAdmissionsPending        = "giz_edge_webrtc_admissions_pending"
	metricEdgeSessionsActive           = "giz_edge_webrtc_sessions_active"
	metricEdgeBurstSCTPActive          = "giz_edge_webrtc_burst_sctp_active"
	metricEdgeAdmissionRejections      = "giz_edge_webrtc_admission_rejections_total"
	metricEdgeSessionEstablishments    = "giz_edge_webrtc_session_establishments_total"
	metricEdgeSessionEstablishDuration = "giz_edge_webrtc_session_establish_duration_seconds"
	metricEdgeBridges                  = "giz_edge_webrtc_bridges_total"
	metricEdgeBridgeDuration           = "giz_edge_webrtc_bridge_duration_seconds"
	metricEdgeUpstreams                = "giz_edge_webrtc_upstreams"
	metricEdgeUpstreamSessions         = "giz_edge_webrtc_upstream_sessions_active"
	metricEdgeTunnelChannels           = "giz_edge_webrtc_tunnel_channels_active"
	metricEdgeCapacityLimit            = "giz_edge_webrtc_capacity_limit"
)

var edgeDurationBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300}

func installEdgeMetrics(cfg MetricsConfig) (func(context.Context) error, storemetrics.Store, error) {
	if cfg.RemoteWriteURL == "" && cfg.QueryURL == "" && cfg.BearerToken == "" {
		return nil, nil, nil
	}
	store, err := storemetrics.NewPrometheusStore(storemetrics.PrometheusConfig{
		RemoteWriteURL: cfg.RemoteWriteURL,
		QueryURL:       cfg.QueryURL,
		BearerToken:    cfg.BearerToken,
	})
	if err != nil {
		return nil, nil, err
	}
	shutdown, err := gizmetrics.InstallStore(store)
	if err != nil {
		return nil, nil, errors.Join(err, store.Close())
	}
	return shutdown, store, nil
}

func recordEdgeDuration(ctx context.Context, name string, started time.Time, labels ...gizmetrics.Label) {
	gizmetrics.ObserveHistogram(ctx, name, time.Since(started).Seconds(), edgeDurationBuckets, labels...)
}
