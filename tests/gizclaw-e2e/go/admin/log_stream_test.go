//go:build gizclaw_e2e

package admin_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
)

func TestAdminLogStreamUnconfiguredBackend(t *testing.T) {
	h := newAdminAPIHarness(t)
	resp, err := h.api.StreamServerLogsWithResponse(h.ctx, &adminhttp.StreamServerLogsParams{
		StartTimeMs: ptr(int64(1783400000000)),
		EndTimeMs:   ptr(int64(1783403600000)),
	})
	if err != nil {
		t.Fatalf("StreamServerLogs error: %v", err)
	}
	if resp.StatusCode() == http.StatusOK {
		t.Skip("server has a configured system_log.query_store")
	}
	if resp.StatusCode() != http.StatusNotImplemented {
		t.Fatalf("status = %d body=%s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON501 == nil || resp.JSON501.Error.Code != "LOG_QUERY_NOT_CONFIGURED" {
		t.Fatalf("JSON501 = %#v body=%s", resp.JSON501, string(resp.Body))
	}
}

func TestAdminLogStreamVolcSmoke(t *testing.T) {
	h := newAdminAPIHarness(t)
	deadline := time.Now().Add(30 * time.Second)
	var lastBody string
	for time.Now().Before(deadline) {
		now := time.Now().UTC()
		resp, err := h.api.StreamServerLogsWithResponse(h.ctx, &adminhttp.StreamServerLogsParams{
			Filter: ptr("*"), StartTimeMs: ptr(now.Add(-5 * time.Minute).UnixMilli()),
			EndTimeMs: ptr(now.Add(time.Minute).UnixMilli()), Limit: ptr(int32(10)),
		})
		if err != nil {
			t.Fatalf("StreamServerLogs error: %v", err)
		}
		if resp.StatusCode() == http.StatusNotImplemented && resp.JSON501 != nil && resp.JSON501.Error.Code == "LOG_QUERY_NOT_CONFIGURED" {
			t.Skip("server has no system_log.query_store; enable GIZCLAW_E2E_VOLC_LOG_ENABLED with a provisioned compatible topic")
		}
		lastBody = string(resp.Body)
		if resp.StatusCode() != http.StatusOK || !strings.Contains(lastBody, "event: end") {
			t.Fatalf("status = %d body=%s", resp.StatusCode(), lastBody)
		}
		if strings.Contains(lastBody, "event: log") {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("Volc LogStore returned no persisted system log before timeout; last body=%s", lastBody)
}
