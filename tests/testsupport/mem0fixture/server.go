// Package mem0fixture provides an isolated scoped HTTP fixture for memory lifecycle tests.
package mem0fixture

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// NewServer returns an isolated HTTP endpoint. It is a scoped HTTP fixture for Registry ownership tests. Real
// extraction, vectors, authentication and reconciliation use the Mem0 E2E stack.
func NewServer(t testing.TB) string {
	t.Helper()
	server := httptest.NewServer(&scopedMemoryHandler{t: t, records: make(map[string]map[string]any)})
	t.Cleanup(server.Close)
	return server.URL
}

type scopedMemoryHandler struct {
	t       testing.TB
	mu      sync.Mutex
	records map[string]map[string]any
	nextID  int
}

func (h *scopedMemoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var body struct {
		ObservationID     string         `json:"observation_id"`
		ObservationDigest string         `json:"observation_digest"`
		UserID            string         `json:"user_id"`
		Metadata          map[string]any `json:"metadata"`
		Filters           map[string]any `json:"filters"`
		Messages          []struct {
			Content  string         `json:"content"`
			Metadata map[string]any `json:"metadata"`
		} `json:"messages"`
	}
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	userID := r.URL.Query().Get("user_id")
	if r.Method == http.MethodPost {
		userID = body.UserID
		if userID == "" {
			userID, _ = body.Filters["user_id"].(string)
		}
	}
	var response any
	status := http.StatusOK
	h.mu.Lock()
	switch r.Method + " " + r.URL.Path {
	case "POST /memories":
		if len(body.Messages) != 1 || userID == "" {
			status = http.StatusBadRequest
			break
		}
		h.nextID++
		id := fmt.Sprintf("fact-%d", h.nextID)
		record := map[string]any{"id": id, "memory": body.Messages[0].Content, "user_id": userID,
			"metadata": directTestMetadata(body.Messages[0].Metadata, body.Metadata, body.ObservationID, body.ObservationDigest), "score": 1.0, "created_at": "2026-10-04T00:00:00Z"}
		h.records[id] = record
		response = map[string]any{"results": []any{record}}
	case "GET /memories", "POST /search":
		selected := make([]any, 0)
		for _, record := range h.records {
			if record["user_id"] == userID {
				selected = append(selected, record)
			}
		}
		response = map[string]any{"results": selected}
	case "DELETE /memories":
		for id, record := range h.records {
			if record["user_id"] == userID {
				delete(h.records, id)
			}
		}
		response = map[string]any{"message": "deleted"}
	default:
		id := strings.TrimPrefix(r.URL.Path, "/memories/")
		if r.Method == http.MethodGet && id != r.URL.Path {
			record, ok := h.records[id]
			if !ok {
				status = http.StatusNotFound
			} else {
				response = record
			}
		} else {
			status = http.StatusNotFound
		}
	}
	h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.t.Error(err)
	}
}

func directTestMetadata(candidate, extraction map[string]any, id, digest string) map[string]any {
	if id == "" {
		return extraction
	}
	metadata := make(map[string]any, len(candidate)+3)
	maps.Copy(metadata, candidate)
	metadata["gizclaw.observation_id"] = id
	metadata["gizclaw.observation_digest"] = digest
	metadata["gizclaw.fact_index"] = 0
	return metadata
}
