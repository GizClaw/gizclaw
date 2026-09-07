package peerresource

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workflowtest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workspacetest"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestWorkspaceRemainsVisibleWhenRuntimeAliasDisappears(t *testing.T) {
	ctx := context.Background()
	store := workspacetest.New(t).DB
	workflows := workflowtest.New(t)
	createWorkflowForCollectionTest(t, ctx, workflows, "canonical-workflow")
	workspaces := &workspace.Server{DB: store, Workflows: workflows}
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
		Name: "journey-1", Collection: "story-teller", WorkflowName: "journey",
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
	if _, rpcErr := server.ValidateRunWorkspaceSelection(ctx, "journey-1"); rpcErr == nil || rpcErr.Code != rpcapi.StatusCodeNotFound {
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
	store := workspacetest.New(t).DB
	profile := runtimeProfileWithWorkspaceAlias("r1")
	workflows := workflowtest.New(t)
	server := &Server{
		Caller:     giznet.PublicKey{1},
		Workspaces: &workspace.Server{DB: store, Workflows: workflows},
		Workflows:  workflows,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceListRequest(rpcapi.WorkspaceListRequest{Collection: "missing"}); err != nil {
		t.Fatal(err)
	}
	response := server.handleWorkspaceList(ctx, &rpcapi.RPCRequest{Id: "list", Params: &payload})
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound || response.Result != nil {
		t.Fatalf("workspace list response = %#v, want NOT_FOUND", response)
	}
}

func TestWorkspaceCreatePreservesNotFoundForUnknownWorkflowAlias(t *testing.T) {
	ctx := context.Background()
	store := workspacetest.New(t).DB
	profile := runtimeProfileWithWorkspaceAlias("r1")
	workflows := workflowtest.New(t)
	server := &Server{
		Caller:     giznet.PublicKey{1},
		Workspaces: &workspace.Server{DB: store, Workflows: workflows},
		Workflows:  workflows,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceCreateRequest(rpcapi.WorkspaceCreateRequest{
		Name: "missing", Collection: "story-teller", WorkflowName: "missing",
	}); err != nil {
		t.Fatal(err)
	}
	response, handled, err := server.Dispatch(ctx, &rpcapi.RPCRequest{Id: "create", Method: rpcapi.RPCMethodServerWorkspaceCreate, Params: &payload})
	if err != nil || !handled || response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound || response.Result != nil {
		t.Fatalf("workspace create response = %#v, handled=%v error=%v, want NOT_FOUND", response, handled, err)
	}
}

func TestWorkspaceCreateProjectsResolvedRuntimeProfileSnapshot(t *testing.T) {
	ctx := context.Background()
	store := workspacetest.New(t).DB
	resolved := runtimeProfileWithWorkspaceAlias("r1")
	workflows := workflowtest.New(t)
	createWorkflowForCollectionTest(t, ctx, workflows, "canonical-workflow")
	calls := 0
	workspaces := &profileMutatingWorkspaceService{
		Server: &workspace.Server{DB: store, Workflows: workflows},
		afterCreate: func() {
			resolved.Revision = "r2"
			resolved.Spec.Workflows.Collections["story-teller"] = map[string]apitypes.RuntimeProfileBinding{}
		},
	}
	server := &Server{
		Caller:     giznet.PublicKey{1},
		Workspaces: workspaces,
		Workflows:  workflows,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			calls++
			return &resolved
		},
	}

	created := callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
		Name: "snapshot", Collection: "story-teller", WorkflowName: "journey",
	})
	if calls != 1 {
		t.Fatalf("RuntimeProfile calls = %d, want 1", calls)
	}
	if created.WorkflowName != "journey" || !created.Available {
		t.Fatalf("created Workspace = %#v, want projection from r1", created)
	}
}

type profileMutatingWorkspaceService struct {
	*workspace.Server
	afterCreate func()
}

func (s *profileMutatingWorkspaceService) CreatePeerWorkspace(
	ctx context.Context,
	request workspace.PeerWorkspaceCreateRequest,
) (apitypes.Workspace, error) {
	created, err := s.Server.CreatePeerWorkspace(ctx, request)
	if err == nil && s.afterCreate != nil {
		s.afterCreate()
	}
	return created, err
}

func TestSystemWorkspaceAvailabilityRequiresSocialOrCollectionBinding(t *testing.T) {
	system := true
	profile := runtimeProfileWithWorkspaceAlias("r1")
	if workspaceAvailable(&profile, apitypes.Workspace{
		Name: "system-1", WorkflowId: "system-workflow", System: &system,
	}) {
		t.Fatal("non-Social system Workspace without a collection binding is available")
	}
	if workspaceAvailable(nil, apitypes.Workspace{
		Name: "system-1", WorkflowId: "system-workflow", System: &system,
	}) {
		t.Fatal("system Workspace without a RuntimeProfile is available")
	}
	if workspaceAvailable(&profile, apitypes.Workspace{
		Name: "legacy", WorkflowId: "system-workflow",
	}) {
		t.Fatal("ordinary unlabeled Workspace is available")
	}
}

func TestSFUWorkspaceProjectionWithoutRuntimeProfile(t *testing.T) {
	system, ordinary := true, false
	for _, tc := range []struct {
		name      string
		workspace apitypes.Workspace
		available bool
	}{
		{"sfu", apitypes.Workspace{WorkflowId: socialutil.SFUWorkflowID, System: &system}, true},
		{"ordinary sfu id", apitypes.Workspace{WorkflowId: socialutil.SFUWorkflowID, System: &ordinary}, false},
		{"unmarked sfu id", apitypes.Workspace{WorkflowId: socialutil.SFUWorkflowID}, false},
		{"system non-sfu", apitypes.Workspace{WorkflowId: "system-workflow", System: &system}, false},
		{"ordinary workflow", apitypes.Workspace{WorkflowId: "canonical-workflow"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projected, err := workspaceRPCProjection(tc.workspace, nil)
			if err != nil {
				t.Fatal(err)
			}
			if projected.Available != tc.available {
				t.Fatalf("Available = %v, want %v", projected.Available, tc.available)
			}
			if tc.available && projected.WorkflowName != "sfu" {
				t.Fatalf("WorkflowName = %q, want sfu", projected.WorkflowName)
			}
		})
	}
}

func runtimeProfileWithWorkspaceAlias(revision string) apitypes.RuntimeProfile {
	return apitypes.RuntimeProfile{
		Id: "default", Revision: revision,
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{
			Models: &map[string]apitypes.RuntimeProfileBinding{
				"llm": collectionTestBinding("chat-model", "Chat"),
			},
		}, Workflows: apitypes.RuntimeProfileWorkflows{
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
