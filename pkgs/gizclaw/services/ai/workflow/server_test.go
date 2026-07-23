package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/gofiber/fiber/v2"
)

func TestServerWorkflowsCRUD(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	createDoc := mustDocument(t, `{
		"name": "demo-assistant",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {
				"agent": {
					"id": "assistant",
					"name": "Assistant",
					"graph": {
						"name": "assistant",
						"entry": "answer",
						"nodes": [{"id": "answer", "type": "llm", "publish": true, "config": {"model": "llm"}}],
						"edges": [{"from": "answer", "to": "__end__"}]
					}
				}
			}
		}
	}`)

	createResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &createDoc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	created, ok := createResp.(adminhttp.CreateWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkflow() response = %#v", createResp)
	}
	if got := workflowDriver(t, apitypes.Workflow(created)); got != "flowcraft" {
		t.Fatalf("CreateWorkflow() driver = %q", got)
	}

	listResp, err := srv.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkflows() error = %v", err)
	}
	listed, ok := listResp.(adminhttp.ListWorkflows200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkflows() response = %#v", listResp)
	}
	if len(listed.Items) != 1 || listed.HasNext {
		t.Fatalf("ListWorkflows() = %#v", listed)
	}

	getResp, err := srv.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: "demo-assistant"})
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	gotDoc, ok := getResp.(adminhttp.GetWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("GetWorkflow() response = %#v", getResp)
	}
	gotSingle := mustSingle(t, apitypes.Workflow(gotDoc))
	if gotSingle.Name != "demo-assistant" {
		t.Fatalf("GetWorkflow() name = %q", gotSingle.Name)
	}

	updateDoc := mustDocument(t, `{
		"name": "demo-assistant",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {
				"agent": {
					"id": "assistant",
					"name": "Updated Assistant",
					"graph": {
						"name": "assistant",
						"entry": "answer",
						"nodes": [{"id": "answer", "type": "llm", "publish": true, "config": {"model": "llm"}}],
						"edges": [{"from": "answer", "to": "__end__"}]
					}
				}
			}
		}
	}`)
	putResp, err := srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{
		Name: "demo-assistant",
		Body: &updateDoc,
	})
	if err != nil {
		t.Fatalf("PutWorkflow() error = %v", err)
	}
	putDoc, ok := putResp.(adminhttp.PutWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("PutWorkflow() response = %#v", putResp)
	}
	putSingle := mustSingle(t, apitypes.Workflow(putDoc))
	if putSingle.Spec.Flowcraft == nil || putSingle.Spec.Flowcraft.Agent.Name != "Updated Assistant" {
		t.Fatalf("PutWorkflow() spec = %#v", putSingle.Spec)
	}

	deleteResp, err := srv.DeleteWorkflow(ctx, adminhttp.DeleteWorkflowRequestObject{Name: "demo-assistant"})
	if err != nil {
		t.Fatalf("DeleteWorkflow() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteWorkflow200JSONResponse); !ok {
		t.Fatalf("DeleteWorkflow() response = %#v", deleteResp)
	}

	getAfterDelete, err := srv.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: "demo-assistant"})
	if err != nil {
		t.Fatalf("GetWorkflow() after delete error = %v", err)
	}
	if _, ok := getAfterDelete.(adminhttp.GetWorkflow404JSONResponse); !ok {
		t.Fatalf("GetWorkflow() after delete response = %#v", getAfterDelete)
	}
}

func TestServerRejectsUnknownWorkflowDriver(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := apitypes.Workflow{
		Name: "bad-workflow",
		Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriver("bad-driver")},
	}

	resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
}

func TestValidateDriverSpecRequiresPetConfig(t *testing.T) {
	if err := validateDriverSpec(apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverPet}); err == nil || !strings.Contains(err.Error(), "spec.pet") {
		t.Fatalf("validateDriverSpec() error = %v", err)
	}
	petSpec := apitypes.PetWorkflowSpec{
		Driver:   apitypes.ReusableWorkflowDriverChatroom,
		Chatroom: &apitypes.ChatRoomWorkflowSpec{},
	}
	if err := validateDriverSpec(apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverPet, Pet: &petSpec}); err != nil {
		t.Fatalf("validateDriverSpec(valid pet) error = %v", err)
	}
	petSpec.Chatroom = nil
	if err := validateDriverSpec(apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverPet, Pet: &petSpec}); err == nil || !strings.Contains(err.Error(), "spec.chatroom is required") {
		t.Fatalf("validateDriverSpec(missing nested payload) error = %v", err)
	}
	petSpec.Chatroom = &apitypes.ChatRoomWorkflowSpec{}
	petSpec.Flowcraft = &apitypes.FlowcraftWorkflowSpec{}
	if err := validateDriverSpec(apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverPet, Pet: &petSpec}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("validateDriverSpec(mismatched nested config) error = %v", err)
	}
	petSpec.Flowcraft = nil
	petSpec.Driver = apitypes.ReusableWorkflowDriver("pet")
	if err := validateDriverSpec(apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverPet, Pet: &petSpec}); err == nil || !strings.Contains(err.Error(), "not a reusable") {
		t.Fatalf("validateDriverSpec(recursive pet) error = %v", err)
	}
}

func TestValidateDriverSpecRejectsDoubaoRealtimeTools(t *testing.T) {
	tools := []apitypes.DoubaoRealtimeFunctionTool{{
		Type: apitypes.DoubaoRealtimeFunctionToolTypeFunction,
		Name: "get_weather",
	}}
	err := validateDriverSpec(apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverDoubaoRealtime,
		DoubaoRealtime: &apitypes.DoubaoRealtimeWorkflowSpec{
			Model: "doubao-realtime",
			Tools: &tools,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "tools are unsupported") {
		t.Fatalf("validateDriverSpec() error = %v", err)
	}
}

func TestValidateDriverSpecRequiresDoubaoRealtimeConfigAndModel(t *testing.T) {
	if err := validateDriverSpec(apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverDoubaoRealtime}); err == nil || !strings.Contains(err.Error(), "spec.doubao_realtime is required") {
		t.Fatalf("validateDriverSpec(missing config) error = %v", err)
	}
	if err := validateDriverSpec(apitypes.WorkflowSpec{
		Driver:         apitypes.WorkflowDriverDoubaoRealtime,
		DoubaoRealtime: &apitypes.DoubaoRealtimeWorkflowSpec{},
	}); err == nil || !strings.Contains(err.Error(), "spec.doubao_realtime.model is required") {
		t.Fatalf("validateDriverSpec(missing model) error = %v", err)
	}
	if err := validateDriverSpec(apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverDoubaoRealtime,
		DoubaoRealtime: &apitypes.DoubaoRealtimeWorkflowSpec{
			Model: "doubao-realtime",
		},
	}); err != nil {
		t.Fatalf("validateDriverSpec(valid config) error = %v", err)
	}
}

func TestServerRejectsEmptyFlowcraftSpec(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	empty := apitypes.FlowcraftWorkflowSpec{}
	doc := apitypes.Workflow{Name: "empty-flowcraft", Spec: apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &empty,
	}}

	resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
}

func TestServerAcceptsChatRoomWorkflowSpec(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{
		"name": "chatroom",
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

	resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow(chatroom) error = %v", err)
	}
	created, ok := resp.(adminhttp.CreateWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkflow(chatroom) response = %#v", resp)
	}
	got := apitypes.Workflow(created)
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
	cases := map[string]apitypes.Workflow{
		"missing chatroom": {
			Name: "missing-chatroom",
			Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverChatroom},
		},
	}
	for name, doc := range cases {
		resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
		if err != nil {
			t.Fatalf("CreateWorkflow(%s) error = %v", name, err)
		}
		if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
			t.Fatalf("CreateWorkflow(%s) response = %#v", name, resp)
		}
	}
}

func TestServerRejectsInvalidToolkitPolicy(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	toolIDs := []string{""}
	doc := apitypes.Workflow{
		Name: "bad-toolkit",
		Spec: apitypes.WorkflowSpec{
			Driver:  apitypes.WorkflowDriverFlowcraft,
			Toolkit: &apitypes.ToolkitPolicy{ToolIds: &toolIDs},
		},
	}

	createResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := createResp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", createResp)
	}

	putResp, err := srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Name: "bad-toolkit", Body: &doc})
	if err != nil {
		t.Fatalf("PutWorkflow() error = %v", err)
	}
	if _, ok := putResp.(adminhttp.PutWorkflow400JSONResponse); !ok {
		t.Fatalf("PutWorkflow() response = %#v", putResp)
	}
}

func TestServerNormalizesNestedPetToolkitPolicy(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	toolIDs := []string{" tool-b ", "tool-a", "tool-a"}
	doc := apitypes.Workflow{
		Name: "pet-care",
		Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet: &apitypes.PetWorkflowSpec{
				Driver:   apitypes.ReusableWorkflowDriverChatroom,
				Chatroom: &apitypes.ChatRoomWorkflowSpec{},
				Toolkit:  &apitypes.ToolkitPolicy{ToolIds: &toolIDs},
			},
		},
	}

	createResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	created, ok := createResp.(adminhttp.CreateWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkflow() response = %#v", createResp)
	}
	if created.Spec.Pet == nil || created.Spec.Pet.Toolkit == nil || created.Spec.Pet.Toolkit.ToolIds == nil {
		t.Fatalf("CreateWorkflow() nested toolkit = %#v", created.Spec.Pet)
	}
	if got := *created.Spec.Pet.Toolkit.ToolIds; len(got) != 2 || got[0] != "tool-a" || got[1] != "tool-b" {
		t.Fatalf("CreateWorkflow() nested tool IDs = %#v", got)
	}

	invalidIDs := []string{" "}
	doc.Spec.Pet.Toolkit = &apitypes.ToolkitPolicy{ToolIds: &invalidIDs}
	invalidResp, err := srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Name: doc.Name, Body: &doc})
	if err != nil {
		t.Fatalf("PutWorkflow() error = %v", err)
	}
	if _, ok := invalidResp.(adminhttp.PutWorkflow400JSONResponse); !ok {
		t.Fatalf("PutWorkflow() response = %#v", invalidResp)
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
			"flowcraft": {"agent":{"id":"assistant","name":"Assistant","graph":{"name":"assistant","entry":"answer","nodes":[{"id":"answer","type":"llm","publish":true,"config":{"model":"llm"}}],"edges":[{"from":"answer","to":"__end__"}]}}}
		}
	}`)

	resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
}

func TestServerPutRejectsPathNameMismatch(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{
		"name": "other-name",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"agent":{"id":"assistant","name":"Assistant","graph":{"name":"assistant","entry":"answer","nodes":[{"id":"answer","type":"llm","publish":true,"config":{"model":"llm"}}],"edges":[{"from":"answer","to":"__end__"}]}}}
		}
	}`)

	resp, err := srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{
		Name: "expected-name",
		Body: &doc,
	})
	if err != nil {
		t.Fatalf("PutWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminhttp.PutWorkflow400JSONResponse); !ok {
		t.Fatalf("PutWorkflow() response = %#v", resp)
	}

	nilCreateResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{})
	if err != nil {
		t.Fatalf("CreateWorkflow(nil body) error = %v", err)
	}
	if _, ok := nilCreateResp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow(nil body) response = %#v", nilCreateResp)
	}

	nilPutResp, err := srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Name: "expected-name"})
	if err != nil {
		t.Fatalf("PutWorkflow(nil body) error = %v", err)
	}
	if _, ok := nilPutResp.(adminhttp.PutWorkflow400JSONResponse); !ok {
		t.Fatalf("PutWorkflow(nil body) response = %#v", nilPutResp)
	}
}

func TestServerRejectsNonCanonicalWorkflowName(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{
		"name": " padded-workflow ",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"agent":{"id":"assistant","name":"Assistant","graph":{"name":"assistant","entry":"answer","nodes":[{"id":"answer","type":"llm","publish":true,"config":{"model":"llm"}}],"edges":[{"from":"answer","to":"__end__"}]}}}
		}
	}`)

	resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
}

func TestServerListWorkflowsPagination(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	for _, name := range []string{"alpha001", "beta0001", "gamma001"} {
		doc := mustDocument(t, fmt.Sprintf(`{
			"name": %q,
			"spec": {
				"driver": "flowcraft",
				"flowcraft": {"agent":{"id":"assistant","name":"Assistant","graph":{"name":"assistant","entry":"answer","nodes":[{"id":"answer","type":"llm","publish":true,"config":{"model":"llm"}}],"edges":[{"from":"answer","to":"__end__"}]}}}
			}
		}`, name))
		if _, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc}); err != nil {
			t.Fatalf("CreateWorkflow(%q) error = %v", name, err)
		}
	}

	limit := int32(1)
	firstResp, err := srv.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{
		Params: adminhttp.ListWorkflowsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListWorkflows(first page) error = %v", err)
	}
	first, ok := firstResp.(adminhttp.ListWorkflows200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkflows(first page) response = %#v", firstResp)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("ListWorkflows(first page) = %#v", first)
	}

	cursor := string(*first.NextCursor)
	secondResp, err := srv.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{
		Params: adminhttp.ListWorkflowsParams{
			Cursor: &cursor,
			Limit:  &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListWorkflows(second page) error = %v", err)
	}
	second, ok := secondResp.(adminhttp.ListWorkflows200JSONResponse)
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
		"name": "duplicate",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"agent":{"id":"assistant","name":"Assistant","graph":{"name":"assistant","entry":"answer","nodes":[{"id":"answer","type":"llm","publish":true,"config":{"model":"llm"}}],"edges":[{"from":"answer","to":"__end__"}]}}}
		}
	}`)
	if _, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc}); err != nil {
		t.Fatalf("CreateWorkflow(seed) error = %v", err)
	}
	duplicateResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow(duplicate) error = %v", err)
	}
	if _, ok := duplicateResp.(adminhttp.CreateWorkflow409JSONResponse); !ok {
		t.Fatalf("CreateWorkflow(duplicate) response = %#v", duplicateResp)
	}

	deleteResp, err := srv.DeleteWorkflow(ctx, adminhttp.DeleteWorkflowRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("DeleteWorkflow(missing) error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteWorkflow404JSONResponse); !ok {
		t.Fatalf("DeleteWorkflow(missing) response = %#v", deleteResp)
	}
}

func TestServerWorkflowStoreNotConfigured(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	ctx := context.Background()
	doc := mustDocument(t, `{
		"name": "missing-store",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"agent":{"id":"assistant","name":"Assistant","graph":{"name":"assistant","entry":"answer","nodes":[{"id":"answer","type":"llm","publish":true,"config":{"model":"llm"}}],"edges":[{"from":"answer","to":"__end__"}]}}}
		}
	}`)

	listResp, err := srv.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkflows() error = %v", err)
	}
	if _, ok := listResp.(adminhttp.ListWorkflows500JSONResponse); !ok {
		t.Fatalf("ListWorkflows() response = %#v", listResp)
	}
	createResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := createResp.(adminhttp.CreateWorkflow500JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", createResp)
	}
	getResp, err := srv.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: "missing-store"})
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if _, ok := getResp.(adminhttp.GetWorkflow500JSONResponse); !ok {
		t.Fatalf("GetWorkflow() response = %#v", getResp)
	}
}

func TestServerRejectsMissingWorkflowRequiredFields(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	for name, doc := range map[string]apitypes.Workflow{
		"name": {
			Spec: apitypes.WorkflowSpec{
				Driver:   apitypes.WorkflowDriverChatroom,
				Chatroom: &apitypes.ChatRoomWorkflowSpec{},
			},
		},
		"driver": {Name: "bad"},
		"spec":   {Name: "bad"},
	} {
		resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
		if err != nil {
			t.Fatalf("CreateWorkflow(%s) error = %v", name, err)
		}
		if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
			t.Fatalf("CreateWorkflow(%s) response = %#v", name, resp)
		}
	}
}

func TestServerRejectsUnsupportedWorkflowDriver(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := apitypes.Workflow{
		Name: "bad-version",
		Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriver("example-invalid")},
	}
	resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow(bad driver) error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow(bad driver) response = %#v", resp)
	}
}

func TestWorkflowResponseVisitors(t *testing.T) {
	t.Parallel()

	doc := mustDocument(t, `{
		"name": "visitor",
		"spec": {
			"driver": "flowcraft",
			"flowcraft": {"agent":{"id":"assistant","name":"Assistant","graph":{"name":"assistant","entry":"answer","nodes":[{"id":"answer","type":"llm","publish":true,"config":{"model":"llm"}}],"edges":[{"from":"answer","to":"__end__"}]}}}
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

func mustDocument(t *testing.T, raw string) apitypes.Workflow {
	t.Helper()

	var doc apitypes.Workflow
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return doc
}

func workflowDriver(t *testing.T, doc apitypes.Workflow) string {
	t.Helper()

	return string(doc.Spec.Driver)
}

func mustSingle(t *testing.T, doc apitypes.Workflow) apitypes.Workflow {
	t.Helper()

	return doc
}
