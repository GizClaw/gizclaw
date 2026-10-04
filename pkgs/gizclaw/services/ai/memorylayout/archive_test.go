package memorylayout

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
)

func TestPolicyReplacementPreservesArchivedMetadata(t *testing.T) {
	server := newTestServer(t)
	layout := testLayout(t, "archive-layout")
	response, err := server.CreateMemoryLayout(t.Context(), adminhttp.CreateMemoryLayoutRequestObject{Body: &layout})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateMemoryLayout200JSONResponse); !ok {
		t.Fatalf("create = %T", response)
	}
	const archived = `{"old":"policy bytes retained"}`
	if _, err := server.DB.Exec(`UPDATE memory_layouts SET flowcraft_json=? WHERE id=?`, archived, layout.Id); err != nil {
		t.Fatal(err)
	}
	layout.Spec.Mem0.CustomInstructions = new("replacement")
	updated, err := server.PutMemoryLayout(t.Context(), adminhttp.PutMemoryLayoutRequestObject{Id: layout.Id, Body: &layout})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updated.(adminhttp.PutMemoryLayout200JSONResponse); !ok {
		t.Fatalf("replace = %T", updated)
	}
	var retained string
	if err := server.DB.QueryRow(`SELECT flowcraft_json FROM memory_layouts WHERE id=?`, layout.Id).Scan(&retained); err != nil || retained != archived {
		t.Fatalf("archived metadata = %q, %v", retained, err)
	}
}
