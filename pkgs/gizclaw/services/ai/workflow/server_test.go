package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/gofiber/fiber/v2"
)

func TestServerWorkflowsCRUD(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	createDoc := mustDocument(t, `{
		"metadata": {
			"name": "demo-assistant",
			"description": "flowcraft workflow"
		},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {
				"workspace_layout": {},
				"runtime": {},
				"agents": [],
				"entry_agent": ""
			}
		}
	}`)

	createResp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &createDoc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	created, ok := createResp.(adminservice.CreateWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkflow() response = %#v", createResp)
	}
	if got := workflowDriver(t, apitypes.WorkflowDocument(created)); got != "flowcraft" {
		t.Fatalf("CreateWorkflow() driver = %q", got)
	}

	listResp, err := srv.ListWorkflows(ctx, adminservice.ListWorkflowsRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkflows() error = %v", err)
	}
	listed, ok := listResp.(adminservice.ListWorkflows200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkflows() response = %#v", listResp)
	}
	if len(listed.Items) != 1 || listed.HasNext {
		t.Fatalf("ListWorkflows() = %#v", listed)
	}

	getResp, err := srv.GetWorkflow(ctx, adminservice.GetWorkflowRequestObject{Name: "demo-assistant"})
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	gotDoc, ok := getResp.(adminservice.GetWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("GetWorkflow() response = %#v", getResp)
	}
	gotSingle := mustSingle(t, apitypes.WorkflowDocument(gotDoc))
	if gotSingle.Metadata.Name != "demo-assistant" {
		t.Fatalf("GetWorkflow() name = %q", gotSingle.Metadata.Name)
	}

	updateDoc := mustDocument(t, `{
		"metadata": {
			"name": "demo-assistant",
			"description": "updated description"
		},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {
				"runtime": {
					"executor_ref": "local"
				}
			}
		}
	}`)
	putResp, err := srv.PutWorkflow(ctx, adminservice.PutWorkflowRequestObject{
		Name: "demo-assistant",
		Body: &updateDoc,
	})
	if err != nil {
		t.Fatalf("PutWorkflow() error = %v", err)
	}
	putDoc, ok := putResp.(adminservice.PutWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("PutWorkflow() response = %#v", putResp)
	}
	putSingle := mustSingle(t, apitypes.WorkflowDocument(putDoc))
	if putSingle.Metadata.Description == nil || *putSingle.Metadata.Description != "updated description" {
		t.Fatalf("PutWorkflow() description = %#v", putSingle.Metadata.Description)
	}

	deleteResp, err := srv.DeleteWorkflow(ctx, adminservice.DeleteWorkflowRequestObject{Name: "demo-assistant"})
	if err != nil {
		t.Fatalf("DeleteWorkflow() error = %v", err)
	}
	if _, ok := deleteResp.(adminservice.DeleteWorkflow200JSONResponse); !ok {
		t.Fatalf("DeleteWorkflow() response = %#v", deleteResp)
	}

	getAfterDelete, err := srv.GetWorkflow(ctx, adminservice.GetWorkflowRequestObject{Name: "demo-assistant"})
	if err != nil {
		t.Fatalf("GetWorkflow() after delete error = %v", err)
	}
	if _, ok := getAfterDelete.(adminservice.GetWorkflow404JSONResponse); !ok {
		t.Fatalf("GetWorkflow() after delete response = %#v", getAfterDelete)
	}
}

func TestServerRejectsUnknownWorkflowDriver(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{
		"metadata": {
			"name": "bad-workflow"
		},
		"spec": {
			"driver": "bad-driver"
		}
	}`)

	resp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminservice.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
}

func TestServerAcceptsEmptyFlowcraftSpec(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{
		"metadata": {
			"name": "empty-flowcraft"
		},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {}
		}
	}`)

	resp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	created, ok := resp.(adminservice.CreateWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
	flowcraft := apitypes.WorkflowDocument(created)
	if flowcraft.Metadata.Name != "empty-flowcraft" {
		t.Fatalf("CreateWorkflow() name = %q", flowcraft.Metadata.Name)
	}
}

func TestServerAcceptsChatRoomWorkflowSpec(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{
		"metadata": {
			"name": "chatroom"
		},
		"spec": {
			"driver": "chatroom",
			"chatroom": {
				"history": {
					"ttl": "168h"
				},
				"transcript": {
					"enabled": true,
					"asr_model": "e2e-asr"
				}
			}
		}
	}`)

	resp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow(chatroom) error = %v", err)
	}
	created, ok := resp.(adminservice.CreateWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkflow(chatroom) response = %#v", resp)
	}
	got := apitypes.WorkflowDocument(created)
	if got.Spec.Driver != apitypes.WorkflowDriverChatroom || got.Spec.Chatroom == nil {
		t.Fatalf("CreateWorkflow(chatroom) spec = %#v", got.Spec)
	}
	if got.Spec.Chatroom.History.Ttl == nil || *got.Spec.Chatroom.History.Ttl != "168h" {
		t.Fatalf("CreateWorkflow(chatroom) history = %#v", got.Spec.Chatroom.History)
	}
}

func TestServerRejectsInvalidChatRoomWorkflowSpec(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	cases := map[string]apitypes.WorkflowDocument{
		"missing chatroom": mustDocument(t, `{
			"metadata": {"name": "missing-chatroom"},
			"spec": {
				"driver": "chatroom"
			}
		}`),
	}
	for name, doc := range cases {
		resp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc})
		if err != nil {
			t.Fatalf("CreateWorkflow(%s) error = %v", name, err)
		}
		if _, ok := resp.(adminservice.CreateWorkflow400JSONResponse); !ok {
			t.Fatalf("CreateWorkflow(%s) response = %#v", name, resp)
		}
	}
}

func TestServerCreateWorkflowRequiresName(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{
		"metadata": {},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {}
		}
	}`)

	resp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminservice.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
}

func TestServerPutRejectsPathNameMismatch(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{
		"metadata": {
			"name": "other-name"
		},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {}
		}
	}`)

	resp, err := srv.PutWorkflow(ctx, adminservice.PutWorkflowRequestObject{
		Name: "expected-name",
		Body: &doc,
	})
	if err != nil {
		t.Fatalf("PutWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminservice.PutWorkflow400JSONResponse); !ok {
		t.Fatalf("PutWorkflow() response = %#v", resp)
	}

	nilCreateResp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{})
	if err != nil {
		t.Fatalf("CreateWorkflow(nil body) error = %v", err)
	}
	if _, ok := nilCreateResp.(adminservice.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow(nil body) response = %#v", nilCreateResp)
	}

	nilPutResp, err := srv.PutWorkflow(ctx, adminservice.PutWorkflowRequestObject{Name: "expected-name"})
	if err != nil {
		t.Fatalf("PutWorkflow(nil body) error = %v", err)
	}
	if _, ok := nilPutResp.(adminservice.PutWorkflow400JSONResponse); !ok {
		t.Fatalf("PutWorkflow(nil body) response = %#v", nilPutResp)
	}
}

func TestServerRejectsNonCanonicalWorkflowName(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{
		"metadata": {
			"name": " padded-workflow "
		},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {}
		}
	}`)

	resp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminservice.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
}

func TestServerListWorkflowsPagination(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	for _, name := range []string{"alpha001", "beta0001", "gamma001"} {
		doc := mustDocument(t, fmt.Sprintf(`{
			"metadata": {
				"name": %q
			},
			"spec": {
				"driver": "flowcraft",
				"flowcraft": {}
			}
		}`, name))
		if _, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc}); err != nil {
			t.Fatalf("CreateWorkflow(%q) error = %v", name, err)
		}
	}

	limit := int32(1)
	firstResp, err := srv.ListWorkflows(ctx, adminservice.ListWorkflowsRequestObject{
		Params: adminservice.ListWorkflowsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListWorkflows(first page) error = %v", err)
	}
	first, ok := firstResp.(adminservice.ListWorkflows200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkflows(first page) response = %#v", firstResp)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("ListWorkflows(first page) = %#v", first)
	}

	cursor := string(*first.NextCursor)
	secondResp, err := srv.ListWorkflows(ctx, adminservice.ListWorkflowsRequestObject{
		Params: adminservice.ListWorkflowsParams{
			Cursor: &cursor,
			Limit:  &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListWorkflows(second page) error = %v", err)
	}
	second, ok := secondResp.(adminservice.ListWorkflows200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkflows(second page) response = %#v", secondResp)
	}
	if len(second.Items) != 1 {
		t.Fatalf("ListWorkflows(second page) = %#v", second)
	}
}

func TestServerWorkflowConflictAndMissingDelete(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{
		"metadata": {"name": "duplicate"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {}
		}
	}`)
	if _, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc}); err != nil {
		t.Fatalf("CreateWorkflow(seed) error = %v", err)
	}
	duplicateResp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow(duplicate) error = %v", err)
	}
	if _, ok := duplicateResp.(adminservice.CreateWorkflow409JSONResponse); !ok {
		t.Fatalf("CreateWorkflow(duplicate) response = %#v", duplicateResp)
	}

	deleteResp, err := srv.DeleteWorkflow(ctx, adminservice.DeleteWorkflowRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("DeleteWorkflow(missing) error = %v", err)
	}
	if _, ok := deleteResp.(adminservice.DeleteWorkflow404JSONResponse); !ok {
		t.Fatalf("DeleteWorkflow(missing) response = %#v", deleteResp)
	}
}

func TestServerWorkflowStoreNotConfigured(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	ctx := context.Background()
	doc := mustDocument(t, `{
		"metadata": {"name": "missing-store"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {}
		}
	}`)

	listResp, err := srv.ListWorkflows(ctx, adminservice.ListWorkflowsRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkflows() error = %v", err)
	}
	if _, ok := listResp.(adminservice.ListWorkflows500JSONResponse); !ok {
		t.Fatalf("ListWorkflows() response = %#v", listResp)
	}
	createResp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := createResp.(adminservice.CreateWorkflow500JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", createResp)
	}
	getResp, err := srv.GetWorkflow(ctx, adminservice.GetWorkflowRequestObject{Name: "missing-store"})
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if _, ok := getResp.(adminservice.GetWorkflow500JSONResponse); !ok {
		t.Fatalf("GetWorkflow() response = %#v", getResp)
	}
}

func TestServerRejectsMissingWorkflowRequiredFields(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	for name, raw := range map[string]string{
		"name":   `{"metadata":{},"spec":{"driver":"flowcraft","flowcraft":{}}}`,
		"driver": `{"metadata":{"name":"bad"},"spec":{"flowcraft":{}}}`,
		"spec":   `{"metadata":{"name":"bad"}}`,
	} {
		doc := mustDocument(t, raw)
		resp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc})
		if err != nil {
			t.Fatalf("CreateWorkflow(%s) error = %v", name, err)
		}
		if _, ok := resp.(adminservice.CreateWorkflow400JSONResponse); !ok {
			t.Fatalf("CreateWorkflow(%s) response = %#v", name, resp)
		}
	}
}

func TestServerRejectsUnsupportedWorkflowDriver(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{
		"metadata": {"name": "bad-version"},
		"spec": {"driver": "example-invalid"}
	}`)
	resp, err := srv.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow(bad driver) error = %v", err)
	}
	if _, ok := resp.(adminservice.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow(bad driver) response = %#v", resp)
	}
}

func TestWorkflowResponseVisitors(t *testing.T) {
	t.Parallel()

	doc := mustDocument(t, `{
		"metadata": {"name": "visitor"},
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {}
		}
	}`)
	cases := map[string]func(*fiber.Ctx) error{
		"create": createWorkflow200Response{doc: doc}.VisitCreateWorkflowResponse,
		"get":    getWorkflow200Response{doc: doc}.VisitGetWorkflowResponse,
		"put":    putWorkflow200Response{doc: doc}.VisitPutWorkflowResponse,
		"delete": deleteWorkflow200Response{doc: doc}.VisitDeleteWorkflowResponse,
	}
	for name, visit := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			app.Get("/", visit)
			resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
			if err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if got := resp.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("content-type = %q, want application/json", got)
			}
		})
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatalf("NewBadgerInMemory() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &Server{Store: store}
}

func mustDocument(t *testing.T, raw string) apitypes.WorkflowDocument {
	t.Helper()

	var doc apitypes.WorkflowDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return doc
}

func workflowDriver(t *testing.T, doc apitypes.WorkflowDocument) string {
	t.Helper()

	return string(doc.Spec.Driver)
}

func mustSingle(t *testing.T, doc apitypes.WorkflowDocument) apitypes.WorkflowDocument {
	t.Helper()

	return doc
}
