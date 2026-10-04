package mem0fixture

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetMemoryRecord(t *testing.T) {
	handler := &scopedMemoryHandler{
		t: t,
		records: map[string]map[string]any{
			"known": {"id": "known", "memory": "remembered"},
		},
	}
	for _, test := range []struct {
		id     string
		status int
	}{
		{id: "known", status: http.StatusOK},
		{id: "missing", status: http.StatusNotFound},
	} {
		t.Run(test.id, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/memories/"+test.id, nil))
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if response.Code == http.StatusOK {
				var record map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &record); err != nil {
					t.Fatal(err)
				}
				if record["id"] != test.id || record["memory"] != "remembered" {
					t.Fatalf("unexpected memory record: %v", record)
				}
			}
		})
	}
}
