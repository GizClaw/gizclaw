package gizedge

import "testing"

func TestInstallEdgeMetricsRequiresCompletePrometheusEndpoints(t *testing.T) {
	for _, cfg := range []MetricsConfig{
		{RemoteWriteURL: "https://prometheus.example.test/api/v1/write"},
		{QueryURL: "https://prometheus.example.test"},
		{BearerToken: "token"},
	} {
		if shutdown, store, err := installEdgeMetrics(cfg); err == nil || shutdown != nil || store != nil {
			t.Fatalf("installEdgeMetrics(%#v): shutdown_nil=%t store=%v err=%v, want validation error", cfg, shutdown == nil, store, err)
		}
	}
}
