//go:build gizclaw_e2e

package admin_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
)

const observabilityConversationPrompt = "Hello from an independent Giztest workflow task."

func TestAdminLogStreamVolcSmoke(t *testing.T) {
	h := newAdminAPIHarness(t)
	clientRequestID := "log-store-smoke-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	seed, err := h.api.GetPeerWithResponse(h.ctx, h.peerKey, func(_ context.Context, request *http.Request) error {
		request.Header.Set("X-Request-ID", clientRequestID)
		return nil
	})
	if err != nil {
		t.Fatalf("seed system log: %v", err)
	}
	requireStatusOK(t, seed, seed.Body)
	requestID := seed.HTTPResponse.Header.Get("X-Request-ID")
	decoded, decodeErr := hex.DecodeString(requestID)
	if decodeErr != nil || len(decoded) != 16 || requestID == clientRequestID {
		t.Fatalf("request ID must be a fresh server-generated 128-bit ID: %q", requestID)
	}

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
			t.Fatal("server has no system_log.query_store; start it through run_volc_log_tests.sh")
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

func TestAdminConversationAuditLogs(t *testing.T) {
	if os.Getenv("GIZCLAW_E2E_OBSERVABILITY") != "1" {
		t.Skip("requires Docker E2E --observability mode")
	}
	h := newAdminAPIHarness(t)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		now := time.Now().UTC()
		filter := `boundary:agent_input AND event:text AND content:"` + observabilityConversationPrompt + `"`
		resp, err := h.api.StreamServerLogsWithResponse(h.ctx, &adminhttp.StreamServerLogsParams{
			Filter: &filter, StartTimeMs: ptr(now.Add(-30 * time.Minute).UnixMilli()),
			EndTimeMs: ptr(now.Add(time.Minute).UnixMilli()), Limit: ptr(int32(10)),
		})
		if err != nil {
			t.Fatalf("query input: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		entries := decodeLogStreamEntries(t, string(resp.Body))
		if len(entries) == 0 {
			time.Sleep(time.Second)
			continue
		}
		input := entries[0].Fields
		for _, key := range []string{"peer_public_key", "session_id", "stream_id", "source_file", "source_line"} {
			if input[key] == "" {
				t.Fatalf("input missing %s: %v", key, input)
			}
		}
		filter = "session_id:" + input["session_id"] + " AND event:*"
		resp, err = h.api.StreamServerLogsWithResponse(h.ctx, &adminhttp.StreamServerLogsParams{
			Filter: &filter, StartTimeMs: ptr(now.Add(-30 * time.Minute).UnixMilli()),
			EndTimeMs: ptr(now.Add(time.Minute).UnixMilli()), Limit: ptr(int32(1000)),
		})
		if err != nil {
			t.Fatalf("query conversation: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		found := map[string]bool{}
		routes := map[string]map[string]int{}
		for _, entry := range decodeLogStreamEntries(t, string(resp.Body)) {
			if entry.Message != "genx: stream" {
				continue
			}
			f := entry.Fields
			for _, key := range []string{"peer_public_key", "session_id", "stream_id", "source_file", "source_line", "started_at", "observed_at"} {
				if f[key] == "" {
					t.Fatalf("stream missing %s: %v", key, f)
				}
			}
			route := strings.Join([]string{f["request_id"], f["transformer"], f["boundary"], f["stream_id"], f["role"], f["mime_type"], f["segment_index"]}, "/")
			if routes[route] == nil {
				routes[route] = map[string]int{}
			}
			routes[route][f["event"]]++
			if f["role"] == "assistant" && f["input_stream_id"] == input["stream_id"] {
				if f["event"] == "first_text" || f["event"] == "first_audio" {
					if _, err := strconv.ParseFloat(f["input_elapsed_ms"], 64); err != nil {
						t.Fatalf("invalid input latency: %v", f)
					}
					found[f["boundary"]+"/"+f["event"]] = true
				}
			}
		}
		complete := true
		for _, key := range []string{"model_output/first_text", "model_output/first_audio", "peer_delivery/first_text", "peer_delivery/first_audio"} {
			complete = complete && found[key]
		}
		for route, events := range routes {
			if events["stream_start"] > 1 || events["stream_end"] > 1 || events["first_text"] > 1 || events["first_audio"] > 1 {
				t.Fatalf("duplicate route events %s: %v", route, events)
			}
			complete = complete && events["stream_start"] == 1 && events["stream_end"] == 1
		}
		if complete {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("conversation has missing timing boundaries or dangling streams; inspect persisted session logs")
}

func logStreamContainsRequestID(t *testing.T, body, requestID string) bool {
	t.Helper()
	for _, entry := range decodeLogStreamEntries(t, body) {
		if entry.Fields["request_id"] == requestID {
			return true
		}
	}
	return false
}

func decodeLogStreamEntries(t *testing.T, body string) []struct {
	Fields  map[string]string `json:"fields"`
	Message string            `json:"message"`
} {
	t.Helper()
	var entries []struct {
		Fields  map[string]string `json:"fields"`
		Message string            `json:"message"`
	}
	for _, block := range strings.Split(body, "\n\n") {
		lines := strings.Split(block, "\n")
		if len(lines) < 2 || lines[0] != "event: log" || !strings.HasPrefix(lines[1], "data: ") {
			continue
		}
		var entry struct {
			Fields  map[string]string `json:"fields"`
			Message string            `json:"message"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &entry); err != nil {
			t.Fatalf("decode log SSE payload: %v; block=%s", err, block)
		}
		entries = append(entries, entry)
	}
	return entries
}
