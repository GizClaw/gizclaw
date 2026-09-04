package peerresource

import (
	"context"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func newWorkspaceInputTestServer(t *testing.T, ctx context.Context) *Server {
	t.Helper()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	workflows := &workflow.Server{Store: kv.Prefixed(store, kv.Key{"workflows"})}
	createWorkflowForCollectionTest(t, ctx, workflows, "canonical-workflow")
	profile := runtimeProfileWithWorkspaceAlias("r1")
	return &Server{
		Caller:     giznet.PublicKey{1},
		Workspaces: &workspace.Server{Store: kv.Prefixed(store, kv.Key{"workspaces"}), Workflows: workflows},
		Workflows:  workflows,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}
}

func callWorkspaceInputPut(
	t *testing.T,
	ctx context.Context,
	server *Server,
	request rpcapi.WorkspaceInputPutRequest,
) *rpcapi.RPCResponse {
	t.Helper()
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceInputPutRequest(request); err != nil {
		t.Fatal(err)
	}
	response, handled, err := server.Dispatch(ctx, &rpcapi.RPCRequest{
		Id: "input-put", Method: rpcapi.RPCMethodServerWorkspaceInputPut, Params: &payload,
	})
	if err != nil || !handled {
		t.Fatalf("workspace input put handled=%v error=%v", handled, err)
	}
	return response
}

func callWorkspaceParametersSet(
	t *testing.T,
	ctx context.Context,
	server *Server,
	request rpcapi.WorkspaceParametersSetRequest,
) *rpcapi.RPCResponse {
	t.Helper()
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceParametersSetRequest(request); err != nil {
		t.Fatal(err)
	}
	response, handled, err := server.Dispatch(ctx, &rpcapi.RPCRequest{
		Id: "parameters-set", Method: rpcapi.RPCMethodServerWorkspaceParametersSet, Params: &payload,
	})
	if err != nil || !handled {
		t.Fatalf("workspace parameters set handled=%v error=%v", handled, err)
	}
	return response
}

func TestWorkspaceParametersSetDerivesAgentTypeAndMergesFields(t *testing.T) {
	ctx := context.Background()
	server := newWorkspaceInputTestServer(t, ctx)
	created := callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
		Name: "journey-parameters", Collection: "story-teller", WorkflowName: "journey",
	})
	if created.Parameters != nil {
		t.Fatalf("created parameters = %#v, want inherited", created.Parameters)
	}
	realtime := rpcapi.WorkspaceInputModeRealtime
	agent := rpcapi.ConversationParametersInitiativeAgent
	policy := rpcapi.ConversationParametersAgentInitiativePolicyOnReload
	response := callWorkspaceParametersSet(t, ctx, server, rpcapi.WorkspaceParametersSetRequest{
		Name: "journey-parameters",
		Parameters: rpcapi.WorkspaceParametersPatch{
			Input: &realtime,
			Conversation: &rpcapi.ConversationParameters{
				Initiative: &agent, AgentInitiativePolicy: &policy,
			},
		},
	})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("workspace parameters set response = %#v", response)
	}
	updated, err := response.Result.AsWorkspaceParametersSetResponse()
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := updated.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if parameters.AgentType != rpcapi.FlowcraftWorkspaceParametersAgentTypeFlowcraft || parameters.Input == nil || *parameters.Input != realtime {
		t.Fatalf("parameters = %+v", parameters)
	}
	if parameters.Conversation == nil || parameters.Conversation.Initiative == nil || *parameters.Conversation.Initiative != agent ||
		parameters.Conversation.AgentInitiativePolicy == nil || *parameters.Conversation.AgentInitiativePolicy != policy {
		t.Fatalf("conversation = %+v", parameters.Conversation)
	}
}

func TestWorkspaceParametersSetRejectsEmptyPatch(t *testing.T) {
	ctx := context.Background()
	server := newWorkspaceInputTestServer(t, ctx)
	callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
		Name: "journey-parameters-empty", Collection: "story-teller", WorkflowName: "journey",
	})

	response := callWorkspaceParametersSet(t, ctx, server, rpcapi.WorkspaceParametersSetRequest{
		Name: "journey-parameters-empty",
	})
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("workspace parameters set response = %#v, want BAD_REQUEST", response)
	}
}

func TestWorkspaceInputPutKeepsParametersAndToolkit(t *testing.T) {
	ctx := context.Background()
	server := newWorkspaceInputTestServer(t, ctx)

	pushToTalk := rpcapi.WorkspaceInputModePushToTalk
	initiative := rpcapi.ConversationParametersInitiativeAgent
	var parameters rpcapi.WorkspaceParameters
	if err := parameters.FromFlowcraftWorkspaceParameters(rpcapi.FlowcraftWorkspaceParameters{
		AgentType:    rpcapi.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		Conversation: &rpcapi.ConversationParameters{Initiative: &initiative},
		Input:        &pushToTalk,
	}); err != nil {
		t.Fatalf("build RPC parameters: %v", err)
	}
	toolNames := []string{"tool-a"}
	created := callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
		Name: "journey-1", Collection: "story-teller", WorkflowName: "journey",
		Parameters: &parameters,
		Toolkit:    &rpcapi.ToolkitPolicy{ToolNames: &toolNames},
	})
	if created.Name != "journey-1" {
		t.Fatalf("created Workspace = %#v", created)
	}

	response := callWorkspaceInputPut(t, ctx, server, rpcapi.WorkspaceInputPutRequest{
		Name: "journey-1", Input: rpcapi.WorkspaceInputModeRealtime,
	})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("workspace input put response = %#v", response)
	}
	updated, err := response.Result.AsWorkspaceInputPutResponse()
	if err != nil {
		t.Fatal(err)
	}
	if updated.Parameters == nil {
		t.Fatalf("updated Workspace parameters = nil")
	}
	flowcraft, err := updated.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if flowcraft.Input == nil || *flowcraft.Input != rpcapi.WorkspaceInputModeRealtime {
		t.Fatalf("input = %+v, want realtime", flowcraft.Input)
	}
	if flowcraft.Conversation == nil || flowcraft.Conversation.Initiative == nil ||
		*flowcraft.Conversation.Initiative != initiative {
		t.Fatalf("conversation = %+v, want initiative preserved", flowcraft.Conversation)
	}
	if !reflect.DeepEqual(updated.Toolkit, created.Toolkit) {
		t.Fatalf("toolkit = %+v, want %+v", updated.Toolkit, created.Toolkit)
	}

	back := callWorkspaceInputPut(t, ctx, server, rpcapi.WorkspaceInputPutRequest{
		Name: "journey-1", Input: rpcapi.WorkspaceInputModePushToTalk,
	})
	if back.Error != nil || back.Result == nil {
		t.Fatalf("workspace input put (realtime to push-to-talk) response = %#v", back)
	}
	restored, err := back.Result.AsWorkspaceInputPutResponse()
	if err != nil {
		t.Fatal(err)
	}
	flowcraft, err = restored.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if flowcraft.Input == nil || *flowcraft.Input != rpcapi.WorkspaceInputModePushToTalk {
		t.Fatalf("input = %+v, want push-to-talk", flowcraft.Input)
	}
}

func TestWorkspaceInputPutSetsInheritedParameters(t *testing.T) {
	ctx := context.Background()
	server := newWorkspaceInputTestServer(t, ctx)
	created := callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
		Name: "journey-1", Collection: "story-teller", WorkflowName: "journey",
	})
	if created.Parameters != nil {
		t.Fatalf("created Workspace parameters = %#v, want inherited", created.Parameters)
	}

	response := callWorkspaceInputPut(t, ctx, server, rpcapi.WorkspaceInputPutRequest{
		Name: "journey-1", Input: rpcapi.WorkspaceInputModeRealtime,
	})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("workspace input put response = %#v", response)
	}
	updated, err := response.Result.AsWorkspaceInputPutResponse()
	if err != nil {
		t.Fatal(err)
	}
	flowcraft, err := updated.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatal(err)
	}
	if flowcraft.Input == nil || *flowcraft.Input != rpcapi.WorkspaceInputModeRealtime {
		t.Fatalf("input = %+v, want realtime", flowcraft.Input)
	}
}

func TestWorkspaceInputPutRejectsUnknownWorkspaceAndInput(t *testing.T) {
	ctx := context.Background()
	server := newWorkspaceInputTestServer(t, ctx)
	callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
		Name: "journey-1", Collection: "story-teller", WorkflowName: "journey",
	})

	missing := callWorkspaceInputPut(t, ctx, server, rpcapi.WorkspaceInputPutRequest{
		Name: "missing", Input: rpcapi.WorkspaceInputModeRealtime,
	})
	if missing.Error == nil || missing.Error.Code != rpcapi.RPCErrorCodeNotFound {
		t.Fatalf("workspace input put (missing) error = %#v, want NOT_FOUND", missing.Error)
	}

	invalid := callWorkspaceInputPut(t, ctx, server, rpcapi.WorkspaceInputPutRequest{Name: "journey-1"})
	if invalid.Error == nil || invalid.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("workspace input put (invalid mode) response = %#v, want BAD_REQUEST", invalid)
	}
}

func TestWorkspaceInputPutRejectsAnotherPeersWorkspace(t *testing.T) {
	ctx := context.Background()
	server := newWorkspaceInputTestServer(t, ctx)
	domain, ok := server.Workspaces.(*workspace.Server)
	if !ok {
		t.Fatalf("Workspaces = %T", server.Workspaces)
	}
	workflows, ok := server.Workflows.(*workflow.Server)
	if !ok {
		t.Fatalf("Workflows = %T", server.Workflows)
	}
	// Friend Workspaces run the built-in SFU Workflow, so the Server only has
	// to materialize it; nothing is read from the owner's RuntimeProfile.
	if err := workflows.EnsureBuiltinWorkflows(ctx); err != nil {
		t.Fatalf("EnsureBuiltinWorkflows error: %v", err)
	}

	// A Friend relationship gives the caller a Workspace it can resolve but
	// does not own, which is the only way a foreign Workspace becomes visible.
	friendStore := kv.NewMemory(nil)
	t.Cleanup(func() { _ = friendStore.Close() })
	friends := &friend.Server{
		Friends:    friendStore,
		Workspaces: domain,
		SFUURL:     "ws://sfu.invalid",
	}
	other := giznet.PublicKey{9}
	relation, err := friends.AdminCreateFriend(ctx, other.String(), server.Caller.String())
	if err != nil {
		t.Fatalf("AdminCreateFriend error: %v", err)
	}
	server.Friends = friends
	sharedName := socialutil.StringValue(relation.WorkspaceName)

	response := callWorkspaceInputPut(t, ctx, server, rpcapi.WorkspaceInputPutRequest{
		Name: sharedName, Input: rpcapi.WorkspaceInputModeRealtime,
	})
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeForbidden {
		t.Fatalf("workspace input put (shared foreign Workspace) response = %#v, want FORBIDDEN", response)
	}
}

func TestWorkspaceInputPutHidesUnsharedForeignWorkspace(t *testing.T) {
	ctx := context.Background()
	server := newWorkspaceInputTestServer(t, ctx)
	domain, ok := server.Workspaces.(*workspace.Server)
	if !ok {
		t.Fatalf("Workspaces = %T", server.Workspaces)
	}
	other := giznet.PublicKey{9}
	if _, err := domain.CreatePeerWorkspace(ownership.WithOwner(ctx, other.String()), workspace.PeerWorkspaceCreateRequest{
		Name: "journey-1", WorkflowID: "canonical-workflow",
	}); err != nil {
		t.Fatalf("CreatePeerWorkspace error: %v", err)
	}

	response := callWorkspaceInputPut(t, ctx, server, rpcapi.WorkspaceInputPutRequest{
		Name: "journey-1", Input: rpcapi.WorkspaceInputModeRealtime,
	})
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeNotFound {
		t.Fatalf("workspace input put (unshared foreign Workspace) response = %#v, want NOT_FOUND", response)
	}
}

func TestWorkspaceInputPutRejectsForbiddenSystemWorkspaceUpdate(t *testing.T) {
	ctx := context.Background()
	server := newWorkspaceInputTestServer(t, ctx)
	domain, ok := server.Workspaces.(*workspace.Server)
	if !ok {
		t.Fatalf("Workspaces = %T", server.Workspaces)
	}
	ownerCtx := ownership.WithOwner(ctx, server.Caller.String())
	if _, _, err := domain.CreateSystemWorkspace(ownerCtx, adminhttp.WorkspaceUpsert{
		Id: "system-workspace", Name: "system-1", WorkflowId: "canonical-workflow",
	}); err != nil {
		t.Fatalf("CreateSystemWorkspace error: %v", err)
	}

	response := callWorkspaceInputPut(t, ctx, server, rpcapi.WorkspaceInputPutRequest{
		Name: "system-1", Input: rpcapi.WorkspaceInputModeRealtime,
	})
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeConflict {
		t.Fatalf("workspace input put (system workspace) response = %#v, want CONFLICT", response)
	}
}
