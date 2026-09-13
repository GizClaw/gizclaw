package peerresource

import (
	"encoding/json"
	"reflect"
	"testing"

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
			if err := json.Unmarshal([]byte(`{"name":"tool-selection","collection":"story-teller","workflow_name":"journey"`+test.policy+`}`), &body); err != nil {
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
			toolCtx, err := agenthost.WithToolExecution(ctx, &bindings, nil)
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
		_, err := server.Tools.CreateTool(ctx, toolkit.Tool{ID: entry.id, InvokeName: entry.invoke, Type: toolkit.ToolTypeClientRPC, Enabled: true, InputSchema: jsonschema.Schema{Type: "object"}})
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
	callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{Name: "put-tools", Collection: "story-teller", WorkflowName: "journey"})
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
		{"subset", &rpcapi.ToolkitPolicy{ToolNames: new([]string{"echo-alias"})}, []string{}},
		{"empty", &rpcapi.ToolkitPolicy{ToolNames: new([]string{})}, []string{}},
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
			resolver := agenthost.ServiceResolver{Workspaces: server.Workspaces, Workflows: server.Workflows, ToolBuilder: &toolkit.Builder{Tools: server.Tools}}
			spec, err := resolver.ResolveByID(ctx, ws.Id)
			if err != nil {
				t.Fatal(err)
			}
			toolCtx, err := agenthost.WithToolExecution(ctx, server.RuntimeProfile().Spec.Resources.Tools, nil)
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
			if err := payload.FromWorkspaceCreateRequest(rpcapi.WorkspaceCreateBody{Name: "rejected", Collection: "story-teller", WorkflowName: "journey", Toolkit: &rpcapi.ToolkitPolicy{ToolNames: new([]string{name})}}); err != nil {
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
