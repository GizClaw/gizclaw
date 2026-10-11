package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestStoredEinoConversationInitiative(t *testing.T) {
	t.Parallel()
	const config = `{"eino":{"conversation":%s,"graph":{
		"name":"opening","compile":{"node_trigger_mode":"any_predecessor"},
		"state":{"fields":[{"name":"answer","type":"string","merge":"replace"}]},
		"nodes":[{"id":"echo","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"answer"}}],
		"edges":[{"from":"start","to":"echo"},{"from":"echo","to":"end"}],"branches":[],
		"outputs":[{"node":"echo","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]
	}}}`
	for _, test := range []struct {
		name, conversation string
		want               *apitypes.EinoConversationInitiative
	}{
		{"default", `{}`, nil},
		{"agent", `{"initiative":"agent"}`, new(apitypes.EinoConversationInitiativeAgent)},
		{"peer", `{"initiative":"peer"}`, new(apitypes.EinoConversationInitiativePeer)},
		{"stored agent", `{"starts":"agent"}`, new(apitypes.EinoConversationInitiativeAgent)},
		{"stored peer", `{"starts":"peer"}`, new(apitypes.EinoConversationInitiativePeer)},
		{"canonical peer wins", `{"initiative":"peer","starts":"agent"}`, new(apitypes.EinoConversationInitiativePeer)},
		{"canonical agent wins", `{"initiative":"agent","starts":"peer"}`, new(apitypes.EinoConversationInitiativeAgent)},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newTestServer(t)
			stored := fmt.Sprintf(config, test.conversation)
			if _, err := s.DB.Exec(`INSERT INTO workflows(id,driver,config_json) VALUES ('opening','eino',?)`, stored); err != nil {
				t.Fatal(err)
			}
			response, err := s.GetWorkflow(t.Context(), adminhttp.GetWorkflowRequestObject{Id: "opening"})
			if err != nil {
				t.Fatal(err)
			}
			got, ok := response.(adminhttp.GetWorkflow200JSONResponse)
			if !ok {
				t.Fatalf("GetWorkflow() = %#v", response)
			}
			initiative := got.Spec.Eino.Conversation.Initiative
			if (initiative == nil) != (test.want == nil) || initiative != nil && *initiative != *test.want {
				t.Fatalf("initiative = %v, want %v", initiative, test.want)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), `"starts"`) {
				t.Fatalf("GetWorkflow() exposed the retired field: %s", encoded)
			}
			body := adminhttp.WorkflowUpsert{Id: got.Id, Spec: got.Spec}
			updated, err := s.PutWorkflow(t.Context(), adminhttp.PutWorkflowRequestObject{Id: got.Id, Body: &body})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := updated.(adminhttp.PutWorkflow200JSONResponse); !ok {
				t.Fatalf("PutWorkflow() = %#v", updated)
			}
			var saved string
			if err := s.DB.QueryRow(`SELECT config_json FROM workflows WHERE id='opening'`).Scan(&saved); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(saved, `"starts"`) || test.want != nil && !strings.Contains(saved, fmt.Sprintf(`"initiative":%q`, *test.want)) {
				t.Fatalf("PutWorkflow() stored a noncanonical conversation: %s", saved)
			}
		})
	}
}
