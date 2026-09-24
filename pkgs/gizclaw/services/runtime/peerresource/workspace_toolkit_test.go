package peerresource

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/google/jsonschema-go/jsonschema"
	"google.golang.org/protobuf/proto"
)

func TestWorkspaceRPCToolkitResolution(t *testing.T) {
	for _, test := range []struct {
		name, policy string
		want         []string
	}{
		{"absent", ``, []string{"giztest_echo", "other"}},
		{"absent_list", `,"toolkit":{}`, []string{"giztest_echo", "other"}},
		{"empty", `,"toolkit":{"tool_names":[]}`, []string{}},
		{"alias", `,"toolkit":{"tool_names":["echo-alias"]}`, []string{"giztest_echo"}},
		{"invoke_name", `,"toolkit":{"tool_names":["giztest_echo"]}`, []string{"giztest_echo"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			server := newWorkspaceToolkitTestServer(t)
			bindings := *server.RuntimeProfile().Spec.Resources.Tools
			var body rpcapi.WorkspaceCreateBody
			if err := json.Unmarshal([]byte(`{"name":"tool-selection","workflow_name":"journey"`+test.policy+`}`), &body); err != nil {
				t.Fatal(err)
			}
			var payload rpcapi.RPCPayload
			if err := payload.FromWorkspaceCreateRequest(body); err != nil {
				t.Fatal(err)
			}
			request := &rpcapi.RPCRequest{Id: "tool-selection", Method: rpcapi.RPCMethodServerWorkspaceCreate, Params: &payload}
			response := dispatchToolkitRPC(t, server, request)
			if response.Error != nil {
				t.Fatalf("create: %+v", response.Error)
			}
			ws, err := server.getWorkspaceByName(server.ownerContext(ctx), body.Name)
			if err != nil {
				t.Fatal(err)
			}
			projected, err := response.Result.AsWorkspaceCreateResponse()
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "absent" || test.name == "absent_list" {
				if ws.Toolkit != nil || projected.Toolkit != nil {
					t.Fatalf("inherit must omit policy: stored=%+v projected=%+v", ws.Toolkit, projected.Toolkit)
				}
			}
			if test.name == "empty" || test.name == "alias" || test.name == "invoke_name" {
				wantIDs, wantNames := []string{}, []string{}
				if test.name != "empty" {
					wantIDs = []string{"echo-id"}
					wantNames = []string{"echo-alias"}
				}
				if ws.Toolkit == nil || ws.Toolkit.ToolIds == nil || !reflect.DeepEqual(*ws.Toolkit.ToolIds, wantIDs) {
					t.Fatalf("persisted policy = %+v, want %v", ws.Toolkit, wantIDs)
				}
				if projected.Toolkit == nil || projected.Toolkit.ToolNames == nil || !reflect.DeepEqual(*projected.Toolkit.ToolNames, wantNames) {
					t.Fatalf("projected policy = %+v, want %v", projected.Toolkit, wantNames)
				}
			}
			resolver := agenthost.ServiceResolver{Workspaces: server.Workspaces, Workflows: server.Workflows, ToolBuilder: &toolkit.Builder{Tools: server.Tools}}
			spec, err := resolver.ResolveByID(ctx, ws.Id)
			if err != nil {
				t.Fatal(err)
			}
			toolCtx, err := agenthost.WithToolExecution(ctx, &bindings)
			if err != nil {
				t.Fatal(err)
			}
			definitions, err := spec.ToolInvoker.ResolveTools(toolCtx)
			if err != nil {
				t.Fatal(err)
			}
			names := make([]string, 0, len(definitions))
			for _, definition := range definitions {
				names = append(names, definition.Name)
			}
			if !reflect.DeepEqual(names, test.want) {
				t.Fatalf("resolved toolkit = %v, want %v", names, test.want)
			}
		})
	}
}

func newWorkspaceToolkitTestServer(t *testing.T) *Server {
	t.Helper()
	ctx := t.Context()
	server := newWorkspaceInputTestServer(t, ctx)
	server.Tools = toolkittest.New(t)
	bindings := map[string]apitypes.RuntimeProfileBinding{}
	for _, entry := range []struct{ id, alias, invoke string }{{"echo-id", "echo-alias", "giztest_echo"}, {"other-id", "other-alias", "other"}} {
		_, err := server.Tools.CreateTool(ctx, toolkit.Tool{ID: entry.id, InvokeName: entry.invoke, Type: toolkit.ToolTypeHTTPRequest, Enabled: true, HTTP: &toolkit.HTTPRequest{URL: "https://example.com/tool", Method: "GET", Auth: toolkit.HTTPAuth{Method: "none"}, Timeout: time.Second, MaxResponseBytes: 1024}, InputSchema: jsonschema.Schema{Type: "object"}})
		if err != nil {
			t.Fatal(err)
		}
		bindings[entry.alias] = apitypes.RuntimeProfileBinding{ResourceId: entry.id}
	}
	server.RuntimeProfile().Spec.Resources.Tools = &bindings
	return server
}

func dispatchToolkitRPC(t *testing.T, server *Server, request *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	t.Helper()
	wire, err := rpcapi.EncodeRPCRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := proto.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	decodedWire := &rpcpb.RpcRequest{}
	if err := proto.Unmarshal(data, decodedWire); err != nil {
		t.Fatal(err)
	}
	decoded, err := rpcapi.DecodeRPCRequest(decodedWire)
	if err != nil {
		t.Fatal(err)
	}
	response, handled, err := server.Dispatch(t.Context(), decoded)
	if err != nil || !handled || response == nil {
		t.Fatalf("Dispatch: %v, %+v", err, response)
	}
	responseWire, err := rpcapi.EncodeRPCResponseForMethod(request.Method, response)
	if err != nil {
		t.Fatal(err)
	}
	data, err = proto.Marshal(responseWire)
	if err != nil {
		t.Fatal(err)
	}
	decodedResponse := &rpcpb.RpcResponse{}
	if err := proto.Unmarshal(data, decodedResponse); err != nil {
		t.Fatal(err)
	}
	response, err = rpcapi.DecodeRPCResponseForMethod(request.Method, decodedResponse)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestWorkspaceRPCToolkitPutAndWorkflowIntersection(t *testing.T) {
	server := newWorkspaceToolkitTestServer(t)
	ctx := t.Context()
	callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{Name: "put-tools", WorkflowName: "journey"})
	workflowResponse, err := server.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Id: "canonical-workflow"})
	if err != nil {
		t.Fatal(err)
	}
	wf := apitypes.Workflow(workflowResponse.(adminhttp.GetWorkflow200JSONResponse))
	workflowIDs := []string{"other-id"}
	wf.Spec.Toolkit = &apitypes.ToolkitPolicy{ToolIds: &workflowIDs}
	putResponse, err := server.Workflows.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Id: wf.Id, Body: &adminhttp.WorkflowUpsert{Id: wf.Id, Spec: wf.Spec}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := putResponse.(adminhttp.PutWorkflow200JSONResponse); !ok {
		t.Fatalf("put workflow: %+v", putResponse)
	}
	for _, test := range []struct {
		name   string
		policy *rpcapi.ToolkitPolicy
		want   []string
	}{
		{"subset", rpcToolkitPolicy("echo-alias"), []string{}},
		{"empty", rpcToolkitPolicy(), []string{}},
		{"omit_keeps_empty", nil, []string{}},
		{"inherit", &rpcapi.ToolkitPolicy{}, []string{"other"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var payload rpcapi.RPCPayload
			if err := payload.FromWorkspacePutRequest(rpcapi.WorkspacePutRequest{Name: "put-tools", Body: rpcapi.WorkspacePutBody{Toolkit: test.policy}}); err != nil {
				t.Fatal(err)
			}
			response := dispatchToolkitRPC(t, server, &rpcapi.RPCRequest{Id: "put", Method: rpcapi.RPCMethodServerWorkspacePut, Params: &payload})
			if response.Error != nil {
				t.Fatalf("put: %+v", response.Error)
			}
			ws, err := server.getWorkspaceByName(server.ownerContext(ctx), "put-tools")
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "inherit" {
				projected, err := response.Result.AsWorkspacePutResponse()
				if err != nil {
					t.Fatal(err)
				}
				if ws.Toolkit != nil || projected.Toolkit != nil {
					t.Fatalf("put inherit stored=%+v projected=%+v", ws.Toolkit, projected.Toolkit)
				}
			}

			resolver := agenthost.ServiceResolver{Workspaces: server.Workspaces, Workflows: server.Workflows, ToolBuilder: &toolkit.Builder{Tools: server.Tools}}
			spec, err := resolver.ResolveByID(ctx, ws.Id)
			if err != nil {
				t.Fatal(err)
			}
			toolCtx, err := agenthost.WithToolExecution(ctx, server.RuntimeProfile().Spec.Resources.Tools)
			if err != nil {
				t.Fatal(err)
			}
			definitions, err := spec.ToolInvoker.ResolveTools(toolCtx)
			if err != nil {
				t.Fatal(err)
			}
			names := make([]string, 0, len(definitions))
			for _, definition := range definitions {
				names = append(names, definition.Name)
			}
			if !reflect.DeepEqual(names, test.want) {
				t.Fatalf("resolved toolkit = %v, want %v", names, test.want)
			}
		})
	}
}

func TestWorkspaceRPCToolkitRejectsUnavailableNames(t *testing.T) {
	server := newWorkspaceToolkitTestServer(t)
	delete(*server.RuntimeProfile().Spec.Resources.Tools, "other-alias")
	for _, name := range []string{"unknown", "other", "echo-id", ""} {
		t.Run(name, func(t *testing.T) {
			var payload rpcapi.RPCPayload
			if err := payload.FromWorkspaceCreateRequest(rpcapi.WorkspaceCreateBody{Name: "rejected", WorkflowName: "journey", Toolkit: rpcToolkitPolicy(name)}); err != nil {
				t.Fatal(err)
			}
			response := dispatchToolkitRPC(t, server, &rpcapi.RPCRequest{Id: "invalid", Method: rpcapi.RPCMethodServerWorkspaceCreate, Params: &payload})
			want := rpcapi.StatusCodeNotFound
			if name == "" {
				want = rpcapi.StatusCodeInvalidArgument
			}
			if response.Error == nil || response.Error.Code != want {
				t.Fatalf("response = %+v, want %s", response, want)
			}
			if _, err := server.getWorkspaceByName(server.ownerContext(t.Context()), "rejected"); err == nil {
				t.Fatal("invalid selection created workspace")
			}
		})
	}
}

func rpcToolkitPolicy(names ...string) *rpcapi.ToolkitPolicy {
	values := make([]string, len(names))
	copy(values, names)
	return &rpcapi.ToolkitPolicy{ToolNames: &values}
}

func TestWorkspaceToolkitProjectionAfterProfileChange(t *testing.T) {
	server := newWorkspaceToolkitTestServer(t)
	ctx := t.Context()
	callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{Name: "profile-change", WorkflowName: "journey", Toolkit: rpcToolkitPolicy("echo-alias", "other-alias")})
	bindings := server.RuntimeProfile().Spec.Resources.Tools
	for _, removed := range []string{"echo-alias", "other-alias"} {
		delete(*bindings, removed)
		var payload rpcapi.RPCPayload
		if err := payload.FromWorkspaceGetRequest(rpcapi.WorkspaceGetRequest{Name: "profile-change"}); err != nil {
			t.Fatal(err)
		}
		response := dispatchToolkitRPC(t, server, &rpcapi.RPCRequest{Id: "get", Method: rpcapi.RPCMethodServerWorkspaceGet, Params: &payload})
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		got, err := response.Result.AsWorkspaceGetResponse()
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"giztest_echo", "other-alias"}
		if removed == "other-alias" {
			want[1] = "other"
		}
		if got.Value.Toolkit == nil || got.Value.Toolkit.ToolNames == nil || !reflect.DeepEqual(*got.Value.Toolkit.ToolNames, want) {
			t.Fatalf("projection=%+v, want %v", got.Value.Toolkit, want)
		}
		ws, err := server.getWorkspaceByName(server.ownerContext(ctx), "profile-change")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(*ws.Toolkit.ToolIds, []string{"echo-id", "other-id"}) {
			t.Fatalf("stored IDs changed: %+v", ws.Toolkit)
		}
		resolver := agenthost.ServiceResolver{Workspaces: server.Workspaces, Workflows: server.Workflows, ToolBuilder: &toolkit.Builder{Tools: server.Tools}}
		spec, err := resolver.ResolveByID(ctx, ws.Id)
		if err != nil {
			t.Fatal(err)
		}
		toolCtx, err := agenthost.WithToolExecution(ctx, bindings)
		if err != nil {
			t.Fatal(err)
		}
		defs, err := spec.ToolInvoker.ResolveTools(toolCtx)
		if err != nil {
			t.Fatal(err)
		}
		if len(defs) != len(*bindings) {
			t.Fatalf("effective tools=%v, bindings=%v", defs, *bindings)
		}
	}
	// A fallback must not accidentally name a different Tool after alias rebinding.
	(*bindings)["giztest_echo"] = apitypes.RuntimeProfileBinding{ResourceId: "other-id"}
	collisionWorkspace, err := server.getWorkspaceByName(server.ownerContext(ctx), "profile-change")
	if err != nil {
		t.Fatal(err)
	}
	projected := server.projectWorkspaceToolkit(ctx, collisionWorkspace.Toolkit, server.RuntimeProfile())
	if !reflect.DeepEqual(*projected.ToolNames, []string{"giztest_echo"}) {
		t.Fatalf("collision projection = %v", *projected.ToolNames)
	}
	delete(*bindings, "giztest_echo")
	// Missing entries are omitted without altering the stored policy.
	if err := server.Tools.DeleteTool(ctx, "echo-id"); err != nil {
		t.Fatal(err)
	}
	ws, err := server.getWorkspaceByName(server.ownerContext(ctx), "profile-change")
	if err != nil {
		t.Fatal(err)
	}
	projected = server.projectWorkspaceToolkit(ctx, ws.Toolkit, server.RuntimeProfile())
	if !reflect.DeepEqual(*projected.ToolNames, []string{"other"}) {
		t.Fatalf("missing projection = %v", *projected.ToolNames)
	}
}

func TestWorkspaceRPCStaleToolkitDoesNotFailReadsOrMutations(t *testing.T) {
	server := newWorkspaceToolkitTestServer(t)
	ctx := t.Context()
	callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{Name: "stale-workspace", WorkflowName: "journey", Toolkit: rpcToolkitPolicy("echo-alias")})
	callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{Name: "healthy-workspace", WorkflowName: "journey"})
	delete(*server.RuntimeProfile().Spec.Resources.Tools, "echo-alias")
	if err := server.Tools.DeleteTool(ctx, "echo-id"); err != nil {
		t.Fatal(err)
	}
	page := callWorkspaceList(t, ctx, server, "story-teller")
	if len(page.Items) != 2 {
		t.Fatalf("list returned %d items", len(page.Items))
	}
	for _, item := range page.Items {
		if item.Name == "stale-workspace" && (item.Toolkit == nil || item.Toolkit.ToolNames == nil || len(*item.Toolkit.ToolNames) != 0) {
			t.Fatalf("stale projection = %+v", item.Toolkit)
		}
	}
	var payload rpcapi.RPCPayload
	var parameters rpcapi.WorkspaceParameters
	putInput := rpcapi.WorkspaceInputModeRealtime
	if err := parameters.FromFlowcraftWorkspaceParameters(rpcapi.FlowcraftWorkspaceParameters{AgentType: rpcapi.FlowcraftWorkspaceParametersAgentTypeFlowcraft, Input: &putInput}); err != nil {
		t.Fatal(err)
	}
	if err := payload.FromWorkspacePutRequest(rpcapi.WorkspacePutRequest{Name: "stale-workspace", Body: rpcapi.WorkspacePutBody{Parameters: &parameters}}); err != nil {
		t.Fatal(err)
	}
	response := dispatchToolkitRPC(t, server, &rpcapi.RPCRequest{Id: "put-stale", Method: rpcapi.RPCMethodServerWorkspacePut, Params: &payload})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	put, err := response.Result.AsWorkspacePutResponse()
	if err != nil {
		t.Fatal(err)
	}
	if put.Toolkit == nil || put.Toolkit.ToolNames == nil || len(*put.Toolkit.ToolNames) != 0 {
		t.Fatalf("put projection = %+v", put.Toolkit)
	}
	putParams, err := put.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil || putParams.Input == nil || *putParams.Input != putInput {
		t.Fatalf("put input not updated: %+v %v", putParams, err)
	}
	input := rpcapi.WorkspaceInputModePushToTalk
	response = callWorkspaceParametersSet(t, ctx, server, rpcapi.WorkspaceParametersSetRequest{Name: "stale-workspace", Parameters: rpcapi.WorkspaceParametersPatch{Input: &input}})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	updated, err := response.Result.AsWorkspaceParametersSetResponse()
	if err != nil {
		t.Fatal(err)
	}
	if updated.Toolkit == nil || updated.Toolkit.ToolNames == nil || len(*updated.Toolkit.ToolNames) != 0 {
		t.Fatalf("parameters projection = %+v", updated.Toolkit)
	}
	params, err := updated.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil || params.Input == nil || *params.Input != input {
		t.Fatalf("input not updated: %+v %v", params, err)
	}
	stored, err := server.getWorkspaceByName(server.ownerContext(ctx), "stale-workspace")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*stored.Toolkit.ToolIds, []string{"echo-id"}) {
		t.Fatalf("stored selection changed: %+v", stored.Toolkit)
	}
}

func TestWorkspaceRPCDeleteProjectsStaleToolkit(t *testing.T) {
	for _, test := range []struct {
		name      string
		selection []string
		wantNames []string
	}{
		{name: "all stale", selection: []string{"echo-alias"}, wantNames: []string{}},
		{name: "partially stale", selection: []string{"echo-alias", "other"}, wantNames: []string{"other-alias"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newWorkspaceToolkitTestServer(t)
			ctx := t.Context()
			callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
				Name: "delete-stale", WorkflowName: "journey",
				Toolkit: rpcToolkitPolicy(test.selection...),
			})
			delete(*server.RuntimeProfile().Spec.Resources.Tools, "echo-alias")
			if err := server.Tools.DeleteTool(ctx, "echo-id"); err != nil {
				t.Fatal(err)
			}
			var payload rpcapi.RPCPayload
			if err := payload.FromWorkspaceDeleteRequest(rpcapi.WorkspaceDeleteRequest{Name: "delete-stale"}); err != nil {
				t.Fatal(err)
			}
			response := dispatchToolkitRPC(t, server, &rpcapi.RPCRequest{
				Id: "delete-stale", Method: rpcapi.RPCMethodServerWorkspaceDelete, Params: &payload,
			})
			if response.Error != nil || response.Result == nil {
				t.Fatalf("delete response = %+v", response)
			}
			deleted, err := response.Result.AsWorkspaceDeleteResponse()
			if err != nil {
				t.Fatal(err)
			}
			if deleted.Name != "delete-stale" || deleted.Toolkit == nil || deleted.Toolkit.ToolNames == nil || !reflect.DeepEqual(*deleted.Toolkit.ToolNames, test.wantNames) {
				t.Fatalf("delete projection = %+v, toolkit = %+v, want names %v", deleted, deleted.Toolkit, test.wantNames)
			}
			if page := callWorkspaceList(t, ctx, server, "story-teller"); len(page.Items) != 0 {
				t.Fatalf("deleted Workspace remains listed: %+v", page.Items)
			}
		})
	}
}
