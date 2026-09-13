package workflow

import (
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestSQLWorkflowPageFiltersBeforeDecoding(t *testing.T) {
	s := newTestServer(t)
	ctx := t.Context()
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := range 2000 {
		config := `{"sfu":{}}`
		if i < 1000 || i > 1002 {
			config = `invalid`
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflows(id,driver,config_json) VALUES (?,'sfu',?)`, fmt.Sprintf("workflow-%04d", i), config); err != nil {
			t.Fatal(err)
		}
	}
	// The built-in row sorts before the page and must be excluded by SQL as well.
	if _, err := tx.ExecContext(ctx, `INSERT INTO workflows(id,driver,config_json) VALUES ('system-sfu','sfu','invalid')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	response, err := s.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{Params: adminhttp.ListWorkflowsParams{Cursor: new("workflow-0999"), Limit: new(int32(2))}})
	if err != nil {
		t.Fatal(err)
	}
	page, ok := response.(adminhttp.ListWorkflows200JSONResponse)
	if !ok {
		t.Fatalf("page response=%#v", response)
	}
	if len(page.Items) != 2 || page.Items[0].Id != "workflow-1000" || page.Items[1].Id != "workflow-1001" || !page.HasNext || page.NextCursor == nil || *page.NextCursor != "workflow-1001" {
		t.Fatalf("page=%+v", page)
	}
	// Empty visible range still ignores the corrupt built-in record.
	response, err = s.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{Params: adminhttp.ListWorkflowsParams{Cursor: new("workflow-9999")}})
	if err != nil {
		t.Fatal(err)
	}
	page, ok = response.(adminhttp.ListWorkflows200JSONResponse)
	if !ok || len(page.Items) != 0 {
		t.Fatalf("empty page=%#v", response)
	}
}

func TestSQLWorkflowPutCannotResurrectDeletedRow(t *testing.T) {
	s := newTestServer(t)
	ctx := t.Context()
	for i := range 30 {
		id := fmt.Sprintf("race-%03d", i)
		spec := apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverSfu, Sfu: new(apitypes.SFUWorkflowSpec{})}
		body := adminhttp.WorkflowUpsert{Id: id, Spec: spec}
		created, err := s.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &body})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := created.(adminhttp.CreateWorkflow200JSONResponse); !ok {
			t.Fatalf("create=%#v", created)
		}
		start := make(chan struct{})
		updated := make(chan adminhttp.PutWorkflowResponseObject, 1)
		failed := make(chan error, 1)
		go func() {
			<-start
			response, err := s.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Id: id, Body: &body})
			updated <- response
			failed <- err
		}()
		close(start)
		removed, err := s.DeleteWorkflow(ctx, adminhttp.DeleteWorkflowRequestObject{Id: id})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := removed.(adminhttp.DeleteWorkflow200JSONResponse); !ok {
			t.Fatalf("delete=%#v", removed)
		}
		response := <-updated
		if err := <-failed; err != nil {
			t.Fatal(err)
		}
		switch response.(type) {
		case adminhttp.PutWorkflow200JSONResponse, adminhttp.PutWorkflow404JSONResponse:
		default:
			t.Fatalf("put=%#v", response)
		}
		got, err := s.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Id: id})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := got.(adminhttp.GetWorkflow404JSONResponse); !ok {
			t.Fatalf("row recreated after deletion: %#v", got)
		}
	}
}
