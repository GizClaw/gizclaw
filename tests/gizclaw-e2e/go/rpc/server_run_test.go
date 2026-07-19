//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerRunRPC(t *testing.T) {
	env := newServerResourceHarness(t)
	workspaceName := "run-rpc-workspace"
	_, _ = env.peer.DeleteWorkspace(env.ctx, "server.run.workspace.delete.preclean", rpcapi.WorkspaceDeleteRequest{Name: workspaceName})
	if _, err := env.peer.CreateWorkspace(env.ctx, "server.run.workspace.create", rpcapi.WorkspaceCreateRequest{
		Name:           workspaceName,
		WorkflowName:   "chatroom",
		WorkflowSource: runtimeSourcePtr(),
		Parameters:     rpcChatroomWorkspaceParameters(t),
	}); err != nil {
		t.Fatalf("server.run workspace.create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.peer.StopServerRun(env.ctx, "server.run.stop.cleanup")
		_, _ = env.peer.DeleteWorkspace(env.ctx, "server.run.workspace.delete.cleanup", rpcapi.WorkspaceDeleteRequest{Name: workspaceName})
	})

	status, err := env.peer.GetServerRunStatus(env.ctx, "server.run.status")
	if err != nil {
		t.Fatalf("server.run.status: %v", err)
	}
	if !status.State.Valid() {
		t.Fatalf("server.run.status state = %q", status.State)
	}

	workspace, err := env.peer.SetServerRunWorkspace(env.ctx, "server.run.workspace.set", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspaceName})
	if err != nil {
		t.Fatalf("server.run.workspace.set: %v", err)
	}
	if workspace.WorkspaceName != workspaceName {
		t.Fatalf("server.run.workspace.set = %#v", workspace)
	}
	workspace, err = env.peer.GetServerRunWorkspace(env.ctx, "server.run.workspace.get")
	if err != nil {
		t.Fatalf("server.run.workspace.get: %v", err)
	}
	if workspace.WorkspaceName != workspaceName {
		t.Fatalf("server.run.workspace.get = %#v", workspace)
	}

	reloaded, err := env.peer.ReloadServerRun(env.ctx, "server.run.reload")
	if err != nil {
		t.Fatalf("server.run.reload: %v", err)
	}
	if !reloaded.State.Valid() {
		t.Fatalf("server.run.reload state = %q", reloaded.State)
	}
	stopped, err := env.peer.StopServerRun(env.ctx, "server.run.stop")
	if err != nil {
		t.Fatalf("server.run.stop: %v", err)
	}
	if !stopped.State.Valid() {
		t.Fatalf("server.run.stop state = %q", stopped.State)
	}
}
