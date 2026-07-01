//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerRunRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	status, err := env.peer.GetServerRunStatus(env.ctx, "server.run.status")
	if err != nil {
		t.Fatalf("server.run.status: %v", err)
	}
	if !status.State.Valid() {
		t.Fatalf("server.run.status state = %q", status.State)
	}

	workspace, err := env.peer.SetServerRunWorkspace(env.ctx, "server.run.workspace.set", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: sharedChatroomWorkspace})
	if err != nil {
		t.Fatalf("server.run.workspace.set: %v", err)
	}
	if workspace.WorkspaceName != sharedChatroomWorkspace {
		t.Fatalf("server.run.workspace.set = %#v", workspace)
	}
	workspace, err = env.peer.GetServerRunWorkspace(env.ctx, "server.run.workspace.get")
	if err != nil {
		t.Fatalf("server.run.workspace.get: %v", err)
	}
	if workspace.WorkspaceName != sharedChatroomWorkspace {
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
