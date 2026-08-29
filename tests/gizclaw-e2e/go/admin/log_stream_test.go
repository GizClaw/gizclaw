//go:build gizclaw_e2e

package admin_test

import (
	"context"
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
	var userEntry, assistantEntry *struct {
		Fields  map[string]string `json:"fields"`
		Message string            `json:"message"`
	}
	lifecycleStages := make(map[string]bool)
	for time.Now().Before(deadline) {
		now := time.Now().UTC()
		filter := `content_role:user AND content:"` + observabilityConversationPrompt + `"`
		resp, err := h.api.StreamServerLogsWithResponse(h.ctx, &adminhttp.StreamServerLogsParams{
			Filter: &filter, StartTimeMs: ptr(now.Add(-5 * time.Minute).UnixMilli()),
			EndTimeMs: ptr(now.Add(time.Minute).UnixMilli()), Limit: ptr(int32(10)),
		})
		if err != nil {
			t.Fatalf("query user conversation log: %v", err)
		}
		if resp.StatusCode() != http.StatusOK {
			t.Fatalf("query user conversation log status = %d body=%s", resp.StatusCode(), resp.Body)
		}
		entries := decodeLogStreamEntries(t, string(resp.Body))
		if len(entries) == 0 {
			time.Sleep(time.Second)
			continue
		}
		userEntry = &entries[0]
		peerKey := userEntry.Fields["peer_public_key"]
		turnIndex := userEntry.Fields["turn_index"]
		tunnelSessionID := userEntry.Fields["tunnel_session_id"]
		if peerKey == "" || turnIndex == "" || tunnelSessionID == "" ||
			userEntry.Fields["content_source"] != "agent_input" ||
			userEntry.Message != "gizclaw: AI conversation content" {
			t.Fatalf("invalid user conversation log: %+v", *userEntry)
		}
		assistantFilter := "content_role:assistant AND peer_public_key:" + peerKey +
			" AND turn_index:" + turnIndex + " AND tunnel_session_id:" + tunnelSessionID
		assistantResp, err := h.api.StreamServerLogsWithResponse(h.ctx, &adminhttp.StreamServerLogsParams{
			Filter: &assistantFilter, StartTimeMs: ptr(now.Add(-5 * time.Minute).UnixMilli()),
			EndTimeMs: ptr(now.Add(time.Minute).UnixMilli()), Limit: ptr(int32(100)),
		})
		if err != nil {
			t.Fatalf("query assistant conversation log: %v", err)
		}
		if assistantResp.StatusCode() != http.StatusOK {
			t.Fatalf("query assistant conversation log status = %d body=%s", assistantResp.StatusCode(), assistantResp.Body)
		}
		assistantEntries := decodeLogStreamEntries(t, string(assistantResp.Body))
		for index := range assistantEntries {
			entry := &assistantEntries[index]
			if entry.Message == "gizclaw: AI conversation content" &&
				entry.Fields["content_source"] == "peer_delivery" && entry.Fields["content"] != "" {
				assistantEntry = entry
				break
			}
		}
		lifecycleFilter := "stage:* AND peer_public_key:" + peerKey +
			" AND turn_index:" + turnIndex + " AND tunnel_session_id:" + tunnelSessionID
		lifecycleResp, err := h.api.StreamServerLogsWithResponse(h.ctx, &adminhttp.StreamServerLogsParams{
			Filter: &lifecycleFilter, StartTimeMs: ptr(now.Add(-5 * time.Minute).UnixMilli()),
			EndTimeMs: ptr(now.Add(time.Minute).UnixMilli()), Limit: ptr(int32(100)),
		})
		if err != nil {
			t.Fatalf("query peer lifecycle logs: %v", err)
		}
		if lifecycleResp.StatusCode() != http.StatusOK {
			t.Fatalf("query peer lifecycle logs status = %d body=%s", lifecycleResp.StatusCode(), lifecycleResp.Body)
		}
		for _, entry := range decodeLogStreamEntries(t, string(lifecycleResp.Body)) {
			if entry.Message == "gizclaw: peer stream lifecycle" {
				lifecycleStages[entry.Fields["stage"]] = true
			}
		}
		if assistantEntry != nil && lifecycleStages["turn_started"] && lifecycleStages["agent_input_first_push"] &&
			lifecycleStages["output_first_event"] && lifecycleStages["turn_terminal"] {
			break
		}
		time.Sleep(time.Second)
	}
	if userEntry == nil {
		t.Fatal("persisted user conversation content log was not found")
	}
	if assistantEntry == nil {
		t.Fatalf("persisted assistant reply content log was not found for peer=%s turn=%s tunnel=%s",
			userEntry.Fields["peer_public_key"], userEntry.Fields["turn_index"], userEntry.Fields["tunnel_session_id"])
	}
	for _, stage := range []string{"turn_started", "agent_input_first_push", "output_first_event", "turn_terminal"} {
		if !lifecycleStages[stage] {
			t.Errorf("persisted lifecycle stage %q was not found; got stages=%v", stage, lifecycleStages)
		}
	}
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
