package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestFixedModelHeaderBudgets(t *testing.T) {
	for _, row := range []struct {
		name        string
		count, size int
		valid       bool
	}{
		{"within budget", 8, 1000, true},
		{"individual overflow", 1, 1025, false},
		{"aggregate overflow", 9, 1000, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			server := &Server{DB: newTestDB(t)}
			body := modelUpsert("fixed", "openai-tenant", "main")
			bindings := make(apitypes.ModelRequestBindings, row.count)
			for i := range bindings {
				raw, err := json.Marshal(map[string]any{"target": map[string]any{"kind": "http_header", "name": fmt.Sprintf("X-Fixed-%d", i)}, "source": map[string]any{"constant": strings.Repeat("v", row.size)}})
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(raw, &bindings[i]); err != nil {
					t.Fatal(err)
				}
			}
			setModelBindings(t, &body, &bindings)
			response, err := server.CreateModel(t.Context(), adminhttp.CreateModelRequestObject{Body: &body})
			if err != nil {
				t.Fatal(err)
			}
			_, ok := response.(adminhttp.CreateModel200JSONResponse)
			if ok != row.valid {
				t.Fatalf("fixed header budget response = %T", response)
			}
		})
	}
}
