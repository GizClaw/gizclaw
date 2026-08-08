//go:build gizclaw_e2e

package delete_test

import (
	"net/http"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestWorkspaceDeletionQuiescesRunningRuntime(t *testing.T) {
	env := newDeletionHarness(t)
	peer := env.newPeer(t, "delete-workspace-peer")
	foreign := env.newPeer(t, "delete-workspace-foreign")
	workspaceName := "delete-workspace-active"
	_, stored := env.createWorkspace(t, peer, workspaceName)
	_, foreignStored := env.createWorkspace(t, foreign, "delete-workspace-foreign-kept")

	env.startWorkspace(t, peer, workspaceName)
	status, err := env.runStatus(peer, "delete.workspace.status.before")
	if err != nil || status.State != rpcapi.PeerRunStatusStateRunning || status.WorkspaceName == nil || *status.WorkspaceName != workspaceName {
		t.Fatalf("Workspace was not observably running before delete: status=%#v error=%v", status, err)
	}
	active := env.startActiveTransform(t, peer)

	deleted, err := peer.client.DeleteWorkspace(env.ctx, "delete.workspace.submit", rpcapi.WorkspaceDeleteRequest{Name: workspaceName})
	if err != nil {
		t.Fatalf("delete running Workspace: %v", err)
	}
	if deleted.Name != workspaceName {
		t.Fatalf("Workspace delete response = %#v", deleted)
	}
	if _, err := peer.client.GetWorkspace(env.ctx, "delete.workspace.fenced", rpcapi.WorkspaceGetRequest{Name: workspaceName}); err == nil {
		t.Fatal("Workspace accepted business access after delete response")
	}
	if _, err := peer.client.DeleteWorkspace(env.ctx, "delete.workspace.repeat", rpcapi.WorkspaceDeleteRequest{Name: workspaceName}); err == nil {
		t.Fatal("repeat Workspace delete unexpectedly bypassed the deletion fence")
	}
	env.waitWorkspaceAbsent(t, stored.Id)
	active.requireTerminated(t, env.ctx, "deleted Workspace")
	env.waitRunStopped(t, peer, "delete.workspace.status.after")
	if _, err := peer.client.GetServerInfo(env.ctx, "delete.workspace.peer-still-active"); err != nil {
		t.Fatalf("Workspace deletion stopped the owning Peer connection: %v", err)
	}
	if response, err := env.api.GetWorkspaceWithResponse(env.ctx, foreignStored.Id); err != nil || response.StatusCode() != http.StatusOK {
		t.Fatalf("foreign Workspace was affected: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	assertNoPendingDeletion(t, env, "workspace", "workspace", stored.Id)
}
