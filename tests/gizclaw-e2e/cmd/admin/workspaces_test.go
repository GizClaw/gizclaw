//go:build gizclaw_e2e

package admin_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminWorkspacesUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "508-admin-workspaces")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)
	h.CreateContext("workspace-peer").MustSucceed(t)
	h.RegisterContext("workspace-peer", "--sn", "workspace-peer-sn").MustSucceed(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin := h.ConnectClientFromContext("admin-a")
	peer := h.ConnectClientFromContext("workspace-peer")
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin API client: %v", err)
	}
	workflowName := "flowcraft-voice-assistant"
	profileID := fmt.Sprintf("e2e-cli-workspaces-%x", time.Now().UnixNano())
	profile, err := clitest.UpsertRuntimeProfile(ctx, api, adminhttp.RuntimeProfileUpsert{
		Id: profileID,
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{},
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{
					FriendChatroom: "chatroom-direct",
					GroupChatroom:  "chatroom-direct",
					Pet:            "pet-chatroom",
				},
				Collections: apitypes.RuntimeProfileWorkflowCollections{
					"assistants": {
						"voice": {ResourceId: workflowName, I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Voice"}}},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create RuntimeProfile: %v", err)
	}
	token, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{
		Id: profileID, Token: profileID, RuntimeProfileId: profile.Id,
	})
	if err != nil || token.JSON200 == nil {
		t.Fatalf("create RegistrationToken: response=%#v error=%v", token, err)
	}
	if _, err := peer.Register(ctx, "admin.workspaces.register", token.JSON200.Token); err != nil {
		t.Fatalf("register workspace Peer: %v", err)
	}
	workspaceName := fmt.Sprintf("workspace-cli-%x", time.Now().UnixNano())
	workspace, err := peer.CreateWorkspace(ctx, "admin.workspaces.create", rpcapi.WorkspaceCreateRequest{
		Name: workspaceName, Collection: "assistants", WorkflowName: "voice",
	})
	if err != nil {
		t.Fatalf("Peer create Workspace: %v", err)
	}
	defer func() {
		_, _ = peer.DeleteWorkspace(context.Background(), "admin.workspaces.cleanup", rpcapi.WorkspaceDeleteRequest{Name: workspace.Name})
		_ = clitest.DeleteRegistrationTokenByID(context.Background(), api, profileID)
		_, _ = api.DeleteRuntimeProfileWithResponse(context.Background(), profile.Id)
		_ = peer.Close()
		_ = admin.Close()
	}()

	list := h.RunCLI("admin", "workspaces", "list", "--context", "admin-a")
	list.MustSucceed(t)
	if !strings.Contains(list.Stdout, `"name":"`+workspaceName+`"`) {
		t.Fatalf("workspaces list missing created item:\n%s", list.Stdout)
	}
	workspaceID := adminResourceID(t, list.Stdout, workspaceName)
	workflows := h.RunCLI("admin", "workflows", "list", "--context", "admin-a")
	workflows.MustSucceed(t)
	voiceWorkflowID := adminResourceID(t, workflows.Stdout, "flowcraft-voice-assistant")

	get := h.RunCLI("admin", "workspaces", "get", workspaceID, "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"workflow_id":"`+voiceWorkflowID+`"`) {
		t.Fatalf("workspaces get missing canonical workflow ID:\n%s", get.Stdout)
	}
}
