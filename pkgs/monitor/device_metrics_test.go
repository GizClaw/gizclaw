package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	monitorapi "github.com/GizClaw/gizclaw-go/pkgs/monitor/api"
)

func TestDeviceMetricsUseLiveMemoryAndKeepAuthorization(t *testing.T) {
	token := "gizclaw_mk_" + strings.Repeat("x", 32)
	var online atomic.Int64
	var reads atomic.Int64
	callback := func() int64 { reads.Add(1); return online.Load() }
	handler := Handler(Config{Token: token}, Node{Role: "server", OnlineDevices: callback}, http.NotFoundHandler())
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest("GET", "/monitor/api/node", nil))
	if unauthorized.Code != 401 || reads.Load() != 0 {
		t.Fatal("unauthorized request reached device metrics")
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client, err := monitorapi.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int64{0, 12, 3} {
		online.Store(count)
		response, err := client.GetNodeMonitorWithResponse(t.Context(), func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if response.JSON200 == nil || response.JSON200.DeviceMetrics == nil {
			t.Fatal("device metrics missing")
		}
		metrics := response.JSON200.DeviceMetrics
		if metrics.OnlineDevices == nil || *metrics.OnlineDevices != uint64(count) || len(metrics.Transports) != 4 {
			t.Fatalf("device metrics = %+v, want count %d and four peer kinds", metrics, count)
		}
		for _, item := range metrics.Transports {
			if !item.PeerKind.Valid() {
				t.Fatalf("invalid peer kind %q", item.PeerKind)
			}
		}
	}
	if reads.Load() != 3 {
		t.Fatalf("count source read %d times", reads.Load())
	}
}

func TestMissingDeviceCountRemainsUnknown(t *testing.T) {
	for _, node := range []Node{{Role: "edge"}, {Role: "server", OnlineDevices: func() int64 { return -1 }}} {
		result := (&nodeServer{node: node}).deviceMetrics()
		if result.OnlineDevices != nil || len(result.Transports) != 4 {
			t.Fatal("unavailable device count was presented as zero")
		}
	}
}
