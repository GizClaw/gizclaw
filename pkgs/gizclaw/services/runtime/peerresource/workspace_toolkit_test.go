package peerresource

import (
	"context"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolcatalog"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/google/jsonschema-go/jsonschema"
	"google.golang.org/protobuf/proto"
	"reflect"
	"slices"
	"testing"
	"time"
)

func newWorkspaceToolkitTestServer(t *testing.T) *Server {
	t.Helper()
	ctx := t.Context()
	server := newWorkspaceInputTestServer(t, ctx)
	server.Tools = toolkittest.New(t)
	bindings := map[string]apitypes.RuntimeProfileToolBinding{}
	for _, entry := range []struct{ id, alias, invoke string }{{"echo-id", "echo-alias", "giztest_echo"}, {"other-id", "other-alias", "other"}} {
		_, err := server.Tools.CreateTool(ctx, toolkit.Tool{ID: entry.id, InvokeName: entry.invoke, Type: toolkit.ToolTypeHTTPRequest, Enabled: true, HTTP: &toolkit.HTTPRequest{URL: "https://example.com/tool", Method: "GET", Auth: toolkit.HTTPAuth{Method: "none"}, Timeout: time.Second, MaxResponseBytes: 1024}, InputSchema: jsonschema.Schema{Type: "object"}})
		if err != nil {
			t.Fatal(err)
		}
		bindings[entry.alias] = apitypes.RuntimeProfileToolBinding{ResourceId: entry.id}
	}
	server.RuntimeProfile().Spec.Resources.Tools = &bindings
	workflowIDs := []string{"echo-id", "other-id"}
	setWorkflowToolkitForTest(t, server, &apitypes.ToolkitPolicy{ToolIds: &workflowIDs})
	return server
}

// setWorkflowToolkitForTest replaces the toolkit policy of the Workflow behind
// the "journey" alias.
func setWorkflowToolkitForTest(t *testing.T, server *Server, policy *apitypes.ToolkitPolicy) {
	t.Helper()
	binding := server.RuntimeProfile().Spec.Workflows["journey"]
	binding.Toolkit = nil
	if policy != nil {
		names := []string{}
		if policy.ToolNames != nil {
			names = append(names, (*policy.ToolNames)...)
		} else if policy.ToolIds != nil {
			for _, alias := range sortedBindingAliases(*server.RuntimeProfile().Spec.Resources.Tools) {
				tool := (*server.RuntimeProfile().Spec.Resources.Tools)[alias]
				if slices.Contains(*policy.ToolIds, tool.ResourceId) {
					names = append(names, alias)
				}
			}
		}
		binding.Toolkit = &apitypes.RuntimeProfileToolSelection{ToolNames: &names}
	}
	server.RuntimeProfile().Spec.Workflows["journey"] = binding
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

func rpcToolkitPolicy(names ...string) *rpcapi.ToolkitPolicy {
	return &rpcapi.ToolkitPolicy{ToolNames: &names}
}

func workspaceToolNames(t *testing.T, server *Server, name string) []string {
	t.Helper()
	ws, err := server.getWorkspaceByName(server.ownerContext(t.Context()), name)
	if err != nil {
		t.Fatal(err)
	}
	resolver := agenthost.ServiceResolver{Workspaces: server.Workspaces, Workflows: server.Workflows, ToolCatalog: &toolcatalog.Catalog{Tools: server.Tools}, RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) { return *server.RuntimeProfile(), nil }}
	spec, err := resolver.ResolveByID(t.Context(), ws.Id)
	if err != nil {
		t.Fatal(err)
	}
	defs, err := spec.ToolInvoker.ResolveTools(agenthost.WithResourceAccess(t.Context(), server.Caller.String(), nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return names
}

func TestWorkspaceRPCToolkitResolution(t *testing.T) {
	for _, tc := range []struct {
		name     string
		policy   *rpcapi.ToolkitPolicy
		selected []string
		want     []string
	}{
		{"omitted", nil, []string{"echo-alias", "other-alias"}, []string{"echo-alias", "other-alias"}},
		{"empty", rpcToolkitPolicy(), []string{"echo-alias", "other-alias"}, []string{}},
		{"alias", rpcToolkitPolicy("echo-alias"), []string{"echo-alias", "other-alias"}, []string{"echo-alias"}},
		{"cannot-widen", rpcToolkitPolicy("echo-alias", "other-alias"), []string{"other-alias"}, []string{"other-alias"}},
		{"not-opted-in", rpcToolkitPolicy("echo-alias"), nil, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newWorkspaceToolkitTestServer(t)
			setWorkflowToolkitForTest(t, server, &apitypes.ToolkitPolicy{ToolNames: &tc.selected})
			created := callWorkspaceCreate(t, t.Context(), server, rpcapi.WorkspaceCreateBody{Name: "selection", WorkflowName: "journey", Toolkit: tc.policy})
			stored, err := server.getWorkspaceByName(server.ownerContext(t.Context()), "selection")
			if err != nil {
				t.Fatal(err)
			}
			if tc.policy == nil {
				if stored.Toolkit != nil || created.Toolkit != nil {
					t.Fatal("omitted policy was materialized")
				}
			} else {
				if stored.Toolkit == nil || stored.Toolkit.ToolNames == nil || stored.Toolkit.ToolIds != nil || !slices.Equal(*stored.Toolkit.ToolNames, *tc.policy.ToolNames) {
					t.Fatalf("stored policy=%+v", stored.Toolkit)
				}
			}
			got := workspaceToolNames(t, server, "selection")
			if !slices.Equal(got, tc.want) {
				t.Fatalf("names=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestWorkspaceRejectsHTTPInvocationIdentity(t *testing.T) {
	server := newWorkspaceToolkitTestServer(t)
	for _, name := range []string{"giztest_echo", "echo-id", "unknown", "echo-alias "} {
		if _, err := server.resolveWorkspaceToolkit(t.Context(), rpcToolkitPolicy(name), server.RuntimeProfile()); err == nil {
			t.Fatalf("accepted non-alias %q", name)
		}
	}
}

func TestWorkspaceAliasSelectionSurvivesRebindAndRemoval(t *testing.T) {
	server := newWorkspaceToolkitTestServer(t)
	callWorkspaceCreate(t, t.Context(), server, rpcapi.WorkspaceCreateBody{Name: "persistent", WorkflowName: "journey", Toolkit: rpcToolkitPolicy("echo-alias")})
	binding := (*server.RuntimeProfile().Spec.Resources.Tools)["echo-alias"]
	binding.ResourceId = "other-id"
	(*server.RuntimeProfile().Spec.Resources.Tools)["echo-alias"] = binding
	if names := workspaceToolNames(t, server, "persistent"); !slices.Equal(names, []string{"echo-alias"}) {
		t.Fatal(names)
	}
	delete(*server.RuntimeProfile().Spec.Resources.Tools, "echo-alias")
	if names := workspaceToolNames(t, server, "persistent"); len(names) != 0 {
		t.Fatalf("removed alias grants %v", names)
	}
	stored, err := server.getWorkspaceByName(server.ownerContext(t.Context()), "persistent")
	if err != nil {
		t.Fatal(err)
	}
	projected := server.projectWorkspaceToolkit(t.Context(), stored.Toolkit, server.RuntimeProfile())
	if !reflect.DeepEqual(*projected.ToolNames, []string{"echo-alias"}) {
		t.Fatalf("stale selection rewritten: %v", projected)
	}
	// A canonical ID or HTTP invocation name cannot revive the stale selection.
	(*server.RuntimeProfile().Spec.Resources.Tools)["other"] = apitypes.RuntimeProfileToolBinding{ResourceId: "other-id"}
	if names := workspaceToolNames(t, server, "persistent"); len(names) != 0 {
		t.Fatalf("fallback grants %v", names)
	}
}

func TestCatalogWorkflowAndWorkspaceSubsetsUseSameResolver(t *testing.T) {
	server := newWorkspaceToolkitTestServer(t)
	callWorkspaceCreate(t, t.Context(), server, rpcapi.WorkspaceCreateBody{Name: "narrowed-workspace", WorkflowName: "journey", Toolkit: rpcToolkitPolicy("other-alias")})
	name := "narrowed-workspace"
	selection, err := server.toolSelection(t.Context(), *server.RuntimeProfile(), nil, &name)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := server.catalog().Resolve(t.Context(), server.Caller.String(), *server.RuntimeProfile(), selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Alias != "other-alias" || !tools[0].Available {
		t.Fatalf("catalog=%+v", tools)
	}
	if !slices.Equal(workspaceToolNames(t, server, name), []string{tools[0].FunctionName}) {
		t.Fatal("model and discovery differ")
	}
}
