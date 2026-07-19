package peerresource

import (
	"context"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestListWorkflowsForSourceUsesRuntimeAliasesAndOwnedNames(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	workflows := &workflow.Server{Store: store}
	createWorkflowForSourceTest(t, ctx, workflows, "runtime-chat")
	createWorkflowForSourceTest(t, ctx, workflows, "runtime-translate")

	caller := giznet.PublicKey{1}
	ownerCtx := ownership.WithOwner(ctx, caller.String())
	createWorkflowForSourceTest(t, ownerCtx, workflows, "owned-zeta")
	createWorkflowForSourceTest(t, ownerCtx, workflows, "owned-alpha")

	bindings := map[string]string{
		"translate": "runtime-translate",
		"chat":      "runtime-chat",
		"missing":   "deleted-workflow",
	}
	profile := apitypes.RuntimeProfile{
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{Workflows: &bindings},
		},
	}
	server := &Server{
		Caller:    caller,
		Workflows: workflows,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}

	runtimeItems, err := server.listWorkflowsForSource(ctx, rpcapi.ResourceSourceRuntime)
	if err != nil {
		t.Fatalf("listWorkflowsForSource(runtime) error = %v", err)
	}
	if got := workflowSourceTestNames(runtimeItems); !reflect.DeepEqual(got, []string{"chat", "translate"}) {
		t.Fatalf("runtime names = %#v", got)
	}
	for _, item := range runtimeItems {
		if item.OwnerPublicKey != nil {
			t.Fatalf("runtime item %q exposed owner %q", item.Name, *item.OwnerPublicKey)
		}
	}

	ownedItems, err := server.listWorkflowsForSource(ctx, rpcapi.ResourceSourceOwned)
	if err != nil {
		t.Fatalf("listWorkflowsForSource(owned) error = %v", err)
	}
	if got := workflowSourceTestNames(ownedItems); !reflect.DeepEqual(got, []string{"owned-alpha", "owned-zeta"}) {
		t.Fatalf("owned names = %#v", got)
	}
	for _, item := range ownedItems {
		if item.OwnerPublicKey == nil || *item.OwnerPublicKey != caller.String() {
			t.Fatalf("owned item %q owner = %#v", item.Name, item.OwnerPublicKey)
		}
	}
}

func TestResolveWorkspaceWorkflowRequiresWorkflowService(t *testing.T) {
	t.Parallel()

	source := rpcapi.ResourceSourceRuntime
	_, response := (&Server{}).resolveWorkspaceWorkflow(context.Background(), "request", &source, "chat")
	if response == nil || response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeInternalError {
		t.Fatalf("resolveWorkspaceWorkflow() response = %#v", response)
	}
}

func createWorkflowForSourceTest(t *testing.T, ctx context.Context, server *workflow.Server, name string) {
	t.Helper()
	document := apitypes.Workflow{
		Name: name,
		Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft},
	}
	response, err := server.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &document})
	if err != nil {
		t.Fatalf("CreateWorkflow(%q) error = %v", name, err)
	}
	if _, ok := response.(adminhttp.CreateWorkflow200JSONResponse); !ok {
		t.Fatalf("CreateWorkflow(%q) response = %#v", name, response)
	}
}

func workflowSourceTestNames(items []rpcapi.Workflow) []string {
	names := make([]string, len(items))
	for i := range items {
		names[i] = items[i].Name
	}
	return names
}
