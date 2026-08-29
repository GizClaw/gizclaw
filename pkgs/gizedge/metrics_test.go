package gizedge

import (
	"context"
	"testing"
)

func TestInstallEdgeMetricsDisabled(t *testing.T) {
	shutdown, store, err := installEdgeMetrics(MetricsConfig{})
	if err != nil || shutdown != nil || store != nil {
		t.Fatalf("installEdgeMetrics(disabled): shutdown_nil=%t store=%v err=%v, want no-op", shutdown == nil, store, err)
	}
}

func TestInstallEdgeMetricsLifecycle(t *testing.T) {
	shutdown, store, err := installEdgeMetrics(MetricsConfig{
		RemoteWriteURL: "https://prometheus.example.test/api/v1/write",
		QueryURL:       "https://prometheus.example.test",
	})
	if err != nil {
		t.Fatalf("installEdgeMetrics(valid) error = %v", err)
	}
	if shutdown == nil || store == nil {
		t.Fatalf("installEdgeMetrics(valid): shutdown_nil=%t store=%v, want installed recorder", shutdown == nil, store)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}
}

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
