//go:build gizclaw_e2e

package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
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
	requestID := "log-store-smoke-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	limit := int32(1)
	seed, err := h.api.ListPeersWithResponse(h.ctx, &adminhttp.ListPeersParams{Limit: &limit}, func(_ context.Context, request *http.Request) error {
		request.Header.Set("X-Request-ID", requestID)
		return nil
	})
	if err != nil {
		t.Fatalf("seed system log: %v", err)
	}
	requireStatusOK(t, seed, seed.Body)

	deadline := time.Now().Add(30 * time.Second)
	var lastBody string
	for time.Now().Before(deadline) {
		now := time.Now().UTC()
		resp, err := h.api.StreamServerLogsWithResponse(h.ctx, &adminhttp.StreamServerLogsParams{
			Filter: ptr("request_id:" + requestID), StartTimeMs: ptr(now.Add(-5 * time.Minute).UnixMilli()),
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
		if logStreamContainsRequestID(t, lastBody, requestID) {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("Volc LogStore returned no persisted system log before timeout; last body=%s", lastBody)
}

func logStreamContainsRequestID(t *testing.T, body, requestID string) bool {
	t.Helper()
	for _, block := range strings.Split(body, "\n\n") {
		lines := strings.Split(block, "\n")
		if len(lines) < 2 || lines[0] != "event: log" || !strings.HasPrefix(lines[1], "data: ") {
			continue
		}
		var entry struct {
			Fields map[string]string `json:"fields"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &entry); err != nil {
			t.Fatalf("decode log SSE payload: %v; block=%s", err, block)
		}
		if entry.Fields["request_id"] == requestID {
			return true
		}
	}
	return false
}
