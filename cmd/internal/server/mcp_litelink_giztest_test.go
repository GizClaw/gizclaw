package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/mcp"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
	"github.com/goccy/go-yaml"
)

// TestLiteLinkMCPMusicGiztest qualifies real LiteLink and Seed 2.1 Lite through
// actual local Server/Edge sockets and a stateful WebRTC Giztest device.
func TestLiteLinkMCPMusicGiztest(t *testing.T) {
	key := os.Getenv("GIZCLAW_MCP_ARK_API_KEY")
	if key == "" {
		t.Skip("set GIZCLAW_MCP_ARK_API_KEY for live LiteLink/Seed qualification")
	}
	withPeerControlServerTimeout(t, 8, 600*time.Second, func(ctx context.Context, _ *gizclaw.Server, admin *gizcli.Client, _, edgeURL string) {
		root := filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "mcp-litelink")
		raw, err := os.ReadFile(filepath.Join(root, "resources.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		data, err := yaml.YAMLToJSON(raw)
		if err != nil {
			t.Fatal(err)
		}
		token := "local-litelink-mcp-test-token"
		encodedKey, _ := json.Marshal(key)
		encodedToken, _ := json.Marshal(token)
		data = bytes.ReplaceAll(data, []byte(`"${GIZCLAW_MCP_ARK_API_KEY}"`), encodedKey)
		data = bytes.ReplaceAll(data, []byte(`"${GIZCLAW_TEST_REGISTRATION_TOKEN}"`), encodedToken)
		var resource apitypes.Resource
		if err := json.Unmarshal(data, &resource); err != nil {
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
		t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", token)
		t.Setenv("GIZCLAW_MCP_ALBUM_REFERENCE", liveLiteLinkAlbumReference(t, ctx))
		directory := os.Getenv("GIZCLAW_MCP_EVIDENCE")
		if directory == "" {
			directory = t.TempDir()
		}
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		reportPath := filepath.Join(directory, "music.report.json")
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", filepath.Join(root, "music.giztest.yaml"), filepath.Join(root, "music-clarification.giztest.yaml"), filepath.Join(root, "album.giztest.yaml"), "--parallel", "1", "--report", reportPath})
		runErr := command.ExecuteContext(ctx)
		if err := os.WriteFile(filepath.Join(directory, "runner.log"), output.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if runErr != nil {
			t.Fatalf("live Giztest failed: %v\n%s", runErr, output.String())
		}
		data, err = os.ReadFile(reportPath)
		if err != nil {
			t.Fatal(err)
		}
		var report giztest.Report
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		expected := map[string]int{"mcp-litelink.seed-lite-music": 14, "mcp-litelink.seed-lite-music-clarification": 17, "mcp-litelink.seed-lite-three-kingdoms-album": 15}
		if report.Status != "passed" || len(report.Tasks) != len(expected) {
			t.Fatalf("unexpected live report: %s", data)
		}
		for _, task := range report.Tasks {
			count, ok := expected[task.Name]
			if !ok || len(task.Steps) != count || len(task.Cleanup) != 3 {
				t.Fatalf("unexpected task coverage: %s", data)
			}
			delete(expected, task.Name)
			for _, step := range append(task.Steps, task.Cleanup...) {
				if step.Status != "passed" {
					t.Fatalf("task %s step %s: %s", task.Name, step.ID, step.Error)
				}
			}
			t.Logf("real LiteLink + Seed 2.1 Lite: %s passed %d steps and %d cleanup steps", task.Name, len(task.Steps), len(task.Cleanup))
		}
		t.Logf("live report=%s", reportPath)
	})
}

func liveLiteLinkAlbumReference(t *testing.T, ctx context.Context) string {
	t.Helper()
	client, err := mcp.Dial(ctx, mcp.Config{Endpoint: "https://litelink.haivivi.com/mcp/v1/public"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.CallTool(ctx, "get_album", json.RawMessage(`{"album_id":"history.three-kingdoms","lang":"zh-CN"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal("LiteLink rejected the independent album reference query")
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := liteLinkAlbumReference(data)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("independent LiteLink album reference: id=%s episodes=%d", reference["album_id"], reference["count"])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/album-reference" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(reference); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// liteLinkAlbumReference derives an independent ordered playback expectation
// from the deployed catalog. The model never receives this reference object.
func liteLinkAlbumReference(data []byte) (map[string]any, error) {
	var album struct {
		ID        string `json:"id"`
		WorkCount int    `json:"work_count"`
		Works     []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Audio struct {
				URL string `json:"url"`
			} `json:"audio"`
		} `json:"works"`
	}
	if err := json.Unmarshal(data, &album); err != nil {
		return nil, err
	}
	if album.ID != "history.three-kingdoms" || len(album.Works) < 2 || len(album.Works) > rpcapi.MaxAudioPlayerItems || album.WorkCount != len(album.Works) {
		return nil, fmt.Errorf("album reference must contain the complete Three Kingdoms album within player capacity")
	}
	items := make([]*rpcpb.AudioPlayerItem, len(album.Works))
	seen := make(map[string]bool, len(items))
	for index, work := range album.Works {
		if work.ID == "" || work.Title == "" || seen[work.ID] {
			return nil, fmt.Errorf("album reference has a missing or repeated episode")
		}
		seen[work.ID] = true
		items[index] = &rpcpb.AudioPlayerItem{Url: work.Audio.URL, Title: new(work.Title), SourceRef: new(work.ID)}
	}
	if err := rpcapi.ValidateAudioPlayerItems(items, false); err != nil {
		return nil, err
	}
	return map[string]any{
		"album_id": album.ID,
		"count":    len(items),
		"playlist": map[string]any{"items": items},
		"readback": map[string]any{"items": items, "playlist_revision": 2},
	}, nil
}
