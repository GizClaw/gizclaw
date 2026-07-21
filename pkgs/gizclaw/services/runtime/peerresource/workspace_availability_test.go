package peerresource

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestWorkspaceRemainsVisibleWhenRuntimeAliasDisappears(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	workflows := &workflow.Server{Store: store}
	createWorkflowForCollectionTest(t, ctx, workflows, "canonical-workflow")
	workspaces := &workspace.Server{Store: store, WorkflowStore: store}
	profile := runtimeProfileWithWorkspaceAlias("r1")
	server := &Server{
		Caller:     giznet.PublicKey{1},
		Workspaces: workspaces,
		Workflows:  workflows,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}

	created := callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
		Name: "journey-1", Collection: "story-teller", WorkflowAlias: "journey",
	})
	if !created.Available {
		t.Fatalf("created Workspace = %#v, want available", created)
	}

	profile.Spec.Workflows.Collections["story-teller"] = map[string]apitypes.RuntimeProfileBinding{}
	profile.Revision = "r2"
	listed := callWorkspaceList(t, ctx, server, "story-teller")
	if len(listed.Items) != 1 || listed.Items[0].Name != "journey-1" || listed.Items[0].Available {
		t.Fatalf("list after alias removal = %#v", listed)
	}
	got := callWorkspaceGet(t, ctx, server, "journey-1")
	if got.Value.Available {
		t.Fatalf("get after alias removal = %#v, want unavailable", got)
	}
	if _, rpcErr := server.ValidateRunWorkspaceSelection(ctx, "journey-1"); rpcErr == nil || rpcErr.Code != rpcapi.RPCErrorCodeNotFound {
		t.Fatalf("ValidateRunWorkspaceSelection() error = %#v, want NOT_FOUND", rpcErr)
	}

	profile = runtimeProfileWithWorkspaceAlias("r3")
	listed = callWorkspaceList(t, ctx, server, "story-teller")
	if len(listed.Items) != 1 || !listed.Items[0].Available {
		t.Fatalf("list after alias restoration = %#v", listed)
	}
	if name, rpcErr := server.ValidateRunWorkspaceSelection(ctx, "journey-1"); rpcErr != nil || name != "journey-1" {
		t.Fatalf("ValidateRunWorkspaceSelection() = %q, %#v", name, rpcErr)
	}
}

func TestWorkspaceListRejectsUnknownRuntimeCollection(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	profile := runtimeProfileWithWorkspaceAlias("r1")
	server := &Server{
		Caller:     giznet.PublicKey{1},
		Workspaces: &workspace.Server{Store: store, WorkflowStore: store},
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceListRequest(rpcapi.WorkspaceListRequest{Collection: "missing"}); err != nil {
		t.Fatal(err)
	}
	response := server.handleWorkspaceList(ctx, &rpcapi.RPCRequest{Id: "list", Params: &payload})
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeNotFound || response.Result != nil {
		t.Fatalf("workspace list response = %#v, want NOT_FOUND", response)
	}
}

func TestSystemWorkspaceAvailabilityDoesNotRequireCollectionLabel(t *testing.T) {
	system := true
	profile := runtimeProfileWithWorkspaceAlias("r1")
	if !workspaceAvailable(&profile, apitypes.Workspace{
		Name: "pet-1", WorkflowName: "pet-care", System: &system,
	}) {
		t.Fatal("system Workspace without labels is unavailable")
	}
	if workspaceAvailable(nil, apitypes.Workspace{
		Name: "pet-1", WorkflowName: "pet-care", System: &system,
	}) {
		t.Fatal("system Workspace without a RuntimeProfile is available")
	}
	if workspaceAvailable(&profile, apitypes.Workspace{
		Name: "legacy", WorkflowName: "pet-care",
	}) {
		t.Fatal("ordinary unlabeled Workspace is available")
	}
}

func runtimeProfileWithWorkspaceAlias(revision string) apitypes.RuntimeProfile {
	return apitypes.RuntimeProfile{
		Name: "default", Revision: revision,
		Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
			Collections: apitypes.RuntimeProfileWorkflowCollections{
				"story-teller": {"journey": collectionTestBinding("canonical-workflow", "Journey")},
			},
		}},
	}
}

func callWorkspaceCreate(t *testing.T, ctx context.Context, server *Server, body rpcapi.WorkspaceCreateBody) rpcapi.Workspace {
	t.Helper()
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceCreateRequest(body); err != nil {
		t.Fatal(err)
	}
	response, handled, err := server.Dispatch(ctx, &rpcapi.RPCRequest{Id: "create", Method: rpcapi.RPCMethodServerWorkspaceCreate, Params: &payload})
	if err != nil || !handled || response.Error != nil || response.Result == nil {
		var rpcErr any
		if response != nil {
			rpcErr = response.Error
		}
		t.Fatalf("workspace create response error = %#v, handled=%v error=%v", rpcErr, handled, err)
	}
	decoded, err := response.Result.AsWorkspaceCreateResponse()
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func callWorkspaceList(t *testing.T, ctx context.Context, server *Server, collection string) rpcapi.WorkspaceListResponse {
	t.Helper()
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceListRequest(rpcapi.WorkspaceListRequest{Collection: collection}); err != nil {
		t.Fatal(err)
	}
	response := server.handleWorkspaceList(ctx, &rpcapi.RPCRequest{Id: "list", Params: &payload})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("workspace list response = %#v", response)
	}
	decoded, err := response.Result.AsWorkspaceListResponse()
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func callWorkspaceGet(t *testing.T, ctx context.Context, server *Server, name string) rpcapi.WorkspaceGetResponse {
	t.Helper()
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceGetRequest(rpcapi.WorkspaceGetRequest{Name: name}); err != nil {
		t.Fatal(err)
	}
	response := server.handleWorkspaceGet(ctx, &rpcapi.RPCRequest{Id: "get", Params: &payload})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("workspace get response = %#v", response)
	}
	decoded, err := response.Result.AsWorkspaceGetResponse()
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
