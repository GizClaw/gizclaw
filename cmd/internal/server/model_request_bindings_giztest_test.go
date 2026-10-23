package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
)

// Exercise telemetry, SQL, Eino and device-facing error EOS through the real
// Server, Edge and WebRTC driver. Only the upstream model is a local fixture.
func TestModelRequestBindingsGiztest(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/requests" {
			mu.Lock()
			defer mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"count": len(requests), "requests": requests})
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode provider request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" || body["model"] != "binding-fixture" || body["stream"] != true || r.Header.Get("Authorization") != "Bearer local-binding-fixture" {
			t.Errorf("unexpected provider request: %s %s, model=%v stream=%v", r.Method, r.URL.Path, body["model"], body["stream"])
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Error(err)
		}
		if strings.Contains(string(encoded), "device-secret-") {
			t.Error("binding metadata leaked into the model request body")
		}
		mu.Lock()
		requests = append(requests, map[string]any{"vid": r.Header.Get("X-AI-VID"), "has_vid": len(r.Header.Values("X-AI-VID")) != 0})
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+`{"id":"binding","object":"chat.completion.chunk","model":"binding-fixture","choices":[{"index":0,"delta":{"content":"binding reply"},"finish_reason":null}]}`+"\n\ndata: "+`{"id":"binding","object":"chat.completion.chunk","model":"binding-fixture","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(provider.Close)
	withPeerControlServer(t, 8, func(ctx context.Context, _ *gizclaw.Server, admin *gizcli.Client, _, edgeURL string) {
		root := filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "model-request-bindings")
		data, err := os.ReadFile(filepath.Join(root, "resources.json"))
		if err != nil {
			t.Fatal(err)
		}
		var resource apitypes.Resource
		if err := json.Unmarshal(bytes.ReplaceAll(data, []byte("FIXTURE_ENDPOINT"), []byte(provider.URL)), &resource); err != nil {
			t.Fatal(err)
		}
		api, err := admin.ServerAdminClient()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := adminresource.NewClient(api, nil).ApplyResource(ctx, resource); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GIZCLAW_TEST_ENDPOINT", edgeURL)
		t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "local-peer-sync-test-token")
		t.Setenv("GIZCLAW_BINDING_FIXTURE", provider.URL)
		directory := os.Getenv("GIZCLAW_BINDING_EVIDENCE")
		if directory == "" {
			directory = t.TempDir()
		}
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		reportPath := filepath.Join(directory, "giztest.json")
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", filepath.Join(root, "lifecycle.giztest.yaml"), "--parallel", "1", "--evidence", "full", "--output", filepath.Join(directory, "replay"), "--report", reportPath})
		runErr := command.ExecuteContext(ctx)
		if err := os.WriteFile(filepath.Join(directory, "runner.log"), output.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if runErr != nil {
			t.Fatalf("Giztest: %v\n%s", runErr, output.String())
		}
		data, err = os.ReadFile(reportPath)
		if err != nil {
			t.Fatal(err)
		}
		var report giztest.Report
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		if report.Status != "passed" || len(report.Tasks) != 1 || len(report.Tasks[0].Cleanup) != 4 {
			t.Fatalf("unexpected Giztest report: %s", data)
		}
		for _, step := range append(report.Tasks[0].Steps, report.Tasks[0].Cleanup...) {
			if step.Status != "passed" {
				t.Fatalf("step %s: %s", step.ID, step.Error)
			}
		}
	})
}
