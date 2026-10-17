package gizlog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestCorrelationIdentityWinsAndDoesNotLeakToSibling(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newContextHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{AddSource: true}), nil)).With("request_id", "stale")
	ctx := WithAPIKeyName(WithSessionID(WithRequestID(WithPeerPublicKey(t.Context(), "peer"), "server-id"), "session"), "key-name")
	logger.InfoContext(WithWorkspace(WithStreamID(ctx, "stream"), "animals"), "first", "request_id", "forged")
	logger.InfoContext(WithRequestID(t.Context(), "sibling-id"), "second")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var first, second map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"peer_public_key": "peer", "session_id": "session", "request_id": "server-id", "api_key_name": "key-name", "stream_id": "stream", "workspace": "animals"} {
		if first[key] != value {
			t.Fatalf("%s: %v", key, first)
		}
	}
	if strings.Count(lines[0], `"request_id"`) != 1 || first["source"] == nil {
		t.Fatalf("duplicate identity or missing source: %s", lines[0])
	}
	if second["request_id"] != "sibling-id" || second["peer_public_key"] != nil || second["api_key_name"] != nil || second["workspace"] != nil {
		t.Fatalf("identity leaked: %v", second)
	}
}

func TestNewIDIsRandom128BitHex(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id, err := NewID()
		if err != nil {
			t.Fatal(err)
		}
		if len(id) != 32 || strings.Trim(id, "0123456789abcdef") != "" || seen[id] {
			t.Fatalf("invalid/duplicate id %q", id)
		}
		seen[id] = true
	}
}
