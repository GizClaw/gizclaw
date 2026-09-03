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

// Workspace deletion only marks the record pending until the asynchronous
// cleanup finalizes it. The Workspace listing must skip that record instead of
// failing, so the caller keeps seeing the collection's remaining Workspaces
// while the deletion is still in flight.
func TestWorkspaceListSkipsWorkspacePendingDeletion(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	workflows := &workflow.Server{Store: store}
	createWorkflowForCollectionTest(t, ctx, workflows, "canonical-workflow")
	profile := runtimeProfileWithWorkspaceAlias("r1")
	server := &Server{
		Caller:     giznet.PublicKey{1},
		Workspaces: &workspace.Server{Store: store, Workflows: workflows},
		Workflows:  workflows,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}
	for _, name := range []string{"journey-1", "journey-2"} {
		callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
			Name: name, Collection: "story-teller", WorkflowName: "journey",
		})
	}

	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceDeleteRequest(rpcapi.WorkspaceDeleteRequest{Name: "journey-1"}); err != nil {
		t.Fatal(err)
	}
	response, handled, err := server.Dispatch(ctx, &rpcapi.RPCRequest{
		Id: "delete", Method: rpcapi.RPCMethodServerWorkspaceDelete, Params: &payload,
	})
	if err != nil || !handled || response.Error != nil || response.Result == nil {
		t.Fatalf("workspace delete response = %#v, handled=%v error=%v", response, handled, err)
	}

	listed := callWorkspaceList(t, ctx, server, "story-teller")
	if len(listed.Items) != 1 || listed.Items[0].Name != "journey-2" {
		t.Fatalf("workspace list while deletion is pending = %#v, want only journey-2", listed)
	}
}
