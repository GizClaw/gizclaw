package server

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
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
		command.SetArgs([]string{"run", file, "--parallel", "1", "--report", report})
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
		api, err := admin.ServerAdminClient()
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{"agent", "peer", "default"} {
			var spec apitypes.EinoWorkflowSpec
			if err := json.Unmarshal([]byte(graph), &spec); err != nil {
				t.Fatal(err)
			}
			if value != "default" {
				spec.Conversation = &apitypes.EinoConversation{Initiative: new(apitypes.EinoConversationInitiative(value))}
			}
			id := "initiative-" + value
			body, err := json.Marshal(apitypes.Workflow{Id: id, Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &spec}})
			if err != nil {
				t.Fatal(err)
			}
			if value == "default" {
				// Send the obsolete field over HTTP without the client's JSON
				// adapter dropping it before it reaches the Server.
				body = []byte(strings.Replace(string(body), `"eino":{`, `"eino":{"conversation":{"starts":"agent"},`, 1))
			}
			created, err := api.CreateWorkflowWithBodyWithResponse(t.Context(), "application/json", bytes.NewReader(body))
			if err != nil || created == nil || created.JSON200 == nil {
				t.Fatalf("CreateWorkflow() = %+v, %v", created, err)
			}
			if bytes.Contains(created.Body, []byte(`"starts"`)) {
				t.Fatalf("Server returned the ignored starts field: %s", created.Body)
			}
			if value == "default" && (created.JSON200.Spec.Eino.Conversation == nil || created.JSON200.Spec.Eino.Conversation.Initiative != nil || created.JSON200.Spec.Eino.Conversation.Starts != nil) {
				t.Fatalf("Server retained or promoted starts: %+v", created.JSON200.Spec.Eino.Conversation)
			}
			workflows["initiative-"+value] = apitypes.RuntimeProfileWorkflowBinding{ResourceId: id, I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Opening"}, "zh-CN": {DisplayName: "开场"}}}
		}
		profile := adminhttp.RuntimeProfileUpsert{Id: "peer-sync-giztest", Spec: apitypes.RuntimeProfileSpec{Workflows: workflows}}
		if _, err := adminapi.PutRuntimeProfile(t.Context(), admin, profile.Id, profile); err != nil {
			t.Fatal(err)
		}
	})
}
