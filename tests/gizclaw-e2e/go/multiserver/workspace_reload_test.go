//go:build gizclaw_e2e

package multiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// TestWorkspaceReloadWithOptions uses the same real WebRTC connection for
// configuration, selection and runtime startup, backed by a model-free workflow.
func TestWorkspaceReloadWithOptions(t *testing.T) {
	server := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A"))
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	client := connectAndServe(t, key, server, server.PublicKey, "reload-options")
	defer client.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	registerSocialPeer(t, ctx, client, server, "GIZCLAW_TEST_REGISTRATION_TOKEN_A")
	created, err := client.CreateWorkspace(ctx, "reload-create", rpcapi.WorkspaceCreateRequest{
		Name: "reload-workspace", Collection: "assistants", WorkflowName: "workspace-reload-echo",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = client.StopServerRun(context.Background(), "reload-stop")
		_, _ = client.DeleteWorkspace(context.Background(), "reload-delete", rpcapi.WorkspaceDeleteRequest{Name: created.Name})
	}()
	state, err := client.ReloadServerRunWorkspaceWithOptions(ctx, "reload-options", rpcapi.ServerReloadRunWorkspaceWithOptionsRequest{
		WorkspaceName: &created.Name,
		Parameters:    &rpcapi.WorkspaceParametersPatch{Input: new(rpcapi.WorkspaceInputModeRealtime)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.RuntimeState != rpcapi.PeerRunStatusStateRunning || state.ActiveWorkspaceName == nil || *state.ActiveWorkspaceName != created.Name {
		t.Fatalf("one reload did not start target: %+v", state)
	}
	stored, err := client.GetWorkspace(ctx, "reload-get", rpcapi.WorkspaceGetRequest{Name: created.Name})
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := stored.Value.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil || parameters.Input == nil || *parameters.Input != rpcapi.WorkspaceInputModeRealtime {
		t.Fatalf("parameters not applied: %+v, %v", parameters, err)
	}
	// A missing target fails before changing the currently running Workspace.
	if _, err := client.ReloadServerRunWorkspaceWithOptions(ctx, "reload-missing", rpcapi.ServerReloadRunWorkspaceWithOptionsRequest{WorkspaceName: new("missing")}); err == nil {
		t.Fatal("missing workspace accepted")
	}
	current, err := client.GetServerRunWorkspace(ctx, "reload-current")
	if err != nil || current.ActiveWorkspaceName == nil || *current.ActiveWorkspaceName != created.Name {
		t.Fatalf("missing target changed active workspace: %+v, %v", current, err)
	}
	state, err = client.ReloadServerRunWorkspaceWithOptions(ctx, "reload-current-options", rpcapi.ServerReloadRunWorkspaceWithOptionsRequest{
		Parameters: &rpcapi.WorkspaceParametersPatch{Input: new(rpcapi.WorkspaceInputModePushToTalk)},
	})
	if err != nil || state.RuntimeState != rpcapi.PeerRunStatusStateRunning {
		t.Fatalf("current workspace reload: %+v, %v", state, err)
	}
	state, err = client.ReloadServerRunWorkspace(ctx, "reload-empty")
	if err != nil || state.RuntimeState != rpcapi.PeerRunStatusStateRunning {
		t.Fatalf("empty reload: %+v, %v", state, err)
	}
}
