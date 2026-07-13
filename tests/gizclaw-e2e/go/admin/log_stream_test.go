//go:build gizclaw_e2e

package admin_test

import (
	"net/http"
	"testing"

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
	if resp.StatusCode() != http.StatusNotImplemented {
		t.Fatalf("status = %d body=%s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON501 == nil || resp.JSON501.Error.Code != "LOG_QUERY_NOT_CONFIGURED" {
		t.Fatalf("JSON501 = %#v body=%s", resp.JSON501, string(resp.Body))
	}
}
