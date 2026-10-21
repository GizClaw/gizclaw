package gizclaw

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/infraflow"
	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
)

// BenchmarkHTTPActivitySampling compares the actual completion middleware with
// identical business/logging work. It excludes network and Provider latency.
func BenchmarkHTTPActivitySampling(b *testing.B) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer slog.SetDefault(previous)
	c, err := infraflow.NewCounters(infraapi.E2e, infraflow.CaptureConfig{Node: "server", Routes: []infraflow.CaptureRoute{{Route: infraflow.Route{Source: "edge", Target: "server"}, EdgePublicKey: "edge-key"}}})
	if err != nil {
		b.Fatal(err)
	}
	observer := c.EdgeObserver("edge-key")
	for range infraflow.MaxSamplesPerWindow {
		observer.Begin()
	}
	payload := make([]byte, 128)
	business := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) })
	for _, scenario := range []struct {
		name     string
		counters *infraflow.Counters
	}{{"disabled", nil}, {"enabled", c}} {
		b.Run(scenario.name, func(b *testing.B) {
			handler := observeHTTPHandler(business, httpObservationOptions{surface: observability.SurfaceEdgeHTTP, trustedEdge: true, peerPublicKey: "edge-key", infraFlows: scenario.counters})
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				request := httptest.NewRequest(http.MethodGet, "http://server/server-info", nil)
				request.Header.Set(requestIDHeader, "0123456789abcdef0123456789abcdef")
				for pb.Next() {
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, request)
					if response.Code != http.StatusOK || response.Body.Len() != 128 {
						b.Error("business response changed")
						return
					}
				}
			})
		})
	}
}
