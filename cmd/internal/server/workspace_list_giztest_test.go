package server

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestWorkspaceListGiztestGo(t *testing.T) {
	runWorkspaceListGiztest(t, func(ctx context.Context, file, report string) ([]byte, error) {
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", file, "--parallel", "1", "--report", report})
		err := command.ExecuteContext(ctx)
		return output.Bytes(), err
	})
}

// runWorkspaceListGiztest uses real Server/Edge transports and SQLite storage.
// SFU bindings are real domain records; no Agent or media connection starts.
func runWorkspaceListGiztest(t *testing.T, run func(context.Context, string, string) ([]byte, error)) {
	t.Helper()
	withPeerControlServerTimeout(t, 8, 120*time.Second, func(ctx context.Context, _ *gizclaw.Server, admin *gizcli.Client, _, endpoint string) {
		const graph = `{"graph":{
			"name":"workspace-list","compile":{"node_trigger_mode":"any_predecessor"},
			"state":{"fields":[{"name":"answer","type":"string","merge":"replace"}]},
			"nodes":[{"id":"answer","type":"script","language":"starlark","entrypoint":"run",
				"source":"def run(input):\n    return {\"answer\": \"Unused.\"}\n",
				"outputs":{"answer":"answer"},
				"limits":{"max_execution_steps":1000,"timeout":"1s","max_input_bytes":1024,"max_output_bytes":1024}}],
			"edges":[{"from":"start","to":"answer"},{"from":"answer","to":"end"}],"branches":[],
			"outputs":[{"node":"answer","field":"answer","name":"answer","mime_type":"text/plain","primary":true}]
		}}`
		var spec apitypes.EinoWorkflowSpec
		if err := json.Unmarshal([]byte(graph), &spec); err != nil {
			t.Fatal(err)
		}
		if _, err := adminapi.CreateWorkflow(ctx, admin, apitypes.Workflow{Id: "workspace-list", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &spec}}); err != nil {
			t.Fatal(err)
		}
		profile := adminhttp.RuntimeProfileUpsert{Id: "peer-sync-giztest", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
			"eino-chat-assistant": {ResourceId: "workspace-list", I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Workspace List"}, "zh-CN": {DisplayName: "Workspace List"}}},
		}}}
		if _, err := adminapi.PutRuntimeProfile(ctx, admin, profile.Id, profile); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GIZCLAW_TEST_ENDPOINT", endpoint)
		t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "local-peer-sync-test-token")
		file, err := filepath.Abs(filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "giztest", "server.workspace.list.giztest.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		reportPath := filepath.Join(t.TempDir(), "workspace-list.report.json")
		output, runErr := run(ctx, file, reportPath)
		data, readErr := os.ReadFile(reportPath)
		if runErr != nil || readErr != nil {
			t.Fatalf("Workspace list Giztest: %v, report: %v\n%s\n%s", runErr, readErr, output, data)
		}
		type stepResult struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		var report struct {
			Status string `json:"status"`
			Tasks  []struct {
				Steps   []stepResult `json:"steps"`
				Cleanup []stepResult `json:"cleanup"`
			} `json:"tasks"`
		}
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		if report.Status != "passed" || len(report.Tasks) != 1 || len(report.Tasks[0].Steps) != 36 || len(report.Tasks[0].Cleanup) != 3 {
			t.Fatalf("Workspace list must execute every step: %s", data)
		}
		for _, step := range append(report.Tasks[0].Steps, report.Tasks[0].Cleanup...) {
			if step.Status != "passed" {
				t.Fatalf("step %s: %s", step.ID, step.Status)
			}
		}
		t.Logf("Workspace list: 36 steps and 3 cleanup steps passed\n%s", output)
	}, func(cfg *Config) {
		root := t.TempDir()
		keyFile, secretFile := filepath.Join(root, "sfu-key"), filepath.Join(root, "sfu-secret")
		for _, path := range []string{keyFile, secretFile} {
			if err := os.WriteFile(path, []byte("local-workspace-list-test"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		cfg.Services.SFU = &SFUConfig{URL: "wss://sfu.invalid", APIKeyFile: keyFile, APISecretFile: secretFile}
	})
}
