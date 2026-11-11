package server

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
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
	withPeerControlServerTimeout(t, 8, 180*time.Second, func(ctx context.Context, _ *gizclaw.Server, admin *gizcli.Client, _, edgeURL string) {
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
		command.SetArgs([]string{"run", filepath.Join(root, "music.giztest.yaml"), "--parallel", "1", "--report", reportPath})
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
		if report.Status != "passed" || len(report.Tasks) != 1 || len(report.Tasks[0].Steps) != 14 || len(report.Tasks[0].Cleanup) != 3 {
			t.Fatalf("unexpected live report: %s", data)
		}
		for _, step := range append(report.Tasks[0].Steps, report.Tasks[0].Cleanup...) {
			if step.Status != "passed" {
				t.Fatalf("step %s: %s", step.ID, step.Error)
			}
		}
		t.Logf("real LiteLink + Seed 2.1 Lite: %d steps and %d cleanup steps passed; report=%s", len(report.Tasks[0].Steps), len(report.Tasks[0].Cleanup), reportPath)
	})
}
