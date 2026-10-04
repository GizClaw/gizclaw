package workflow

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
)

func TestRetiredWorkflowReadDeleteAndReplacement(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	const archived = `{"graph":{"name":"retained","entry":"answer","nodes":[{"id":"answer","type":"passthrough"}]}}`
	if _, err := s.DB.Exec(`INSERT INTO workflows(id,driver,config_json) VALUES(?,?,?)`, "retired", "flowcraft", archived); err != nil {
		t.Fatal(err)
	}
	read, err := s.GetWorkflow(t.Context(), adminhttp.GetWorkflowRequestObject{Id: "retired"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := read.(adminhttp.GetWorkflow500JSONResponse); !ok {
		t.Fatalf("retired read = %#v", read)
	}
	deleted, err := s.DeleteWorkflow(t.Context(), adminhttp.DeleteWorkflowRequestObject{Id: "retired"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := deleted.(adminhttp.DeleteWorkflow500JSONResponse); !ok {
		t.Fatalf("retired delete = %#v", deleted)
	}
	var retained string
	if err := s.DB.QueryRow(`SELECT config_json FROM workflows WHERE id=?`, "retired").Scan(&retained); err != nil || retained != archived {
		t.Fatalf("failed deletion changed archived config = %q, %v", retained, err)
	}
	body := mustDocument(t, `{"id":"retired","spec":{"driver":"eino","eino":{"graph":{"name":"replacement","compile":{"node_trigger_mode":"any_predecessor"},"state":{"fields":[{"name":"answer","type":"string","merge":"replace"}]},"nodes":[{"id":"echo","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"answer"}}],"edges":[{"from":"start","to":"echo"},{"from":"echo","to":"end"}],"branches":[],"outputs":[{"node":"echo","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]}}}}`)
	replaced, err := s.PutWorkflow(t.Context(), adminhttp.PutWorkflowRequestObject{Id: "retired", Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := replaced.(adminhttp.PutWorkflow200JSONResponse); !ok {
		t.Fatalf("explicit replacement = %#v", replaced)
	}
	deleted, err = s.DeleteWorkflow(t.Context(), adminhttp.DeleteWorkflowRequestObject{Id: "retired"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := deleted.(adminhttp.DeleteWorkflow200JSONResponse); !ok {
		t.Fatalf("replacement deletion = %#v", deleted)
	}
}
