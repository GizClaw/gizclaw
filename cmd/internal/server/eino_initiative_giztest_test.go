package server

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// Real Server/Edge/WebRTC execution proves that the public Workflow default
// reaches the agent and that Workspace initiative overrides it in both directions.
func TestEinoInitiativeGiztest(t *testing.T) {
	runPeerControlGiztest(t, "eino.initiative.giztest.yaml", 21, 6, func(ctx context.Context, file, report string) ([]byte, error) {
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", file, "--parallel", "1", "--output", report})
		err := command.ExecuteContext(ctx)
		return output.Bytes(), err
	}, func(_ *gizclaw.Server, admin *gizcli.Client) {
		const graph = `{"graph":{
			"name":"opening","compile":{"node_trigger_mode":"any_predecessor"},
			"state":{"fields":[{"name":"answer","type":"string","merge":"replace"}]},
			"nodes":[{"id":"opening","type":"script","language":"starlark","entrypoint":"run",
				"source":"def run(input):\n    return {\"answer\": \"Opening.\"}\n",
				"outputs":{"answer":"answer"},
				"limits":{"max_execution_steps":1000,"timeout":"1s","max_input_bytes":1024,"max_output_bytes":1024}}],
			"edges":[{"from":"start","to":"opening"},{"from":"opening","to":"end"}],"branches":[],
			"outputs":[{"node":"opening","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]
		}}`
		workflows := apitypes.RuntimeProfileWorkflows{}
		for _, value := range []string{"agent", "peer", "default"} {
			var spec apitypes.EinoWorkflowSpec
			if err := json.Unmarshal([]byte(graph), &spec); err != nil {
				t.Fatal(err)
			}
			if value != "default" {
				spec.Conversation = &apitypes.EinoConversation{Initiative: new(apitypes.EinoConversationInitiative(value))}
			}
			id := "initiative-" + value
			if _, err := adminapi.CreateWorkflow(t.Context(), admin, apitypes.Workflow{Id: id, Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &spec}}); err != nil {
				t.Fatal(err)
			}
			workflows[value] = apitypes.RuntimeProfileWorkflowBinding{ResourceId: id, I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Opening"}, "zh-CN": {DisplayName: "开场"}}}
		}
		profile := adminhttp.RuntimeProfileUpsert{Id: "peer-sync-giztest", Spec: apitypes.RuntimeProfileSpec{Workflows: workflows}}
		if _, err := adminapi.PutRuntimeProfile(t.Context(), admin, profile.Id, profile); err != nil {
			t.Fatal(err)
		}
	})
}
