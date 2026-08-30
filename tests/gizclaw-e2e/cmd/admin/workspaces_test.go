//go:build gizclaw_e2e

package admin_test

import (
	"context"
	"encoding/json"
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
	var memoryConnection apitypes.RuntimeProfileMemoryConnection
	if err := memoryConnection.FromRuntimeProfileFlowcraftBBHConnection(apitypes.RuntimeProfileFlowcraftBBHConnection{
		Type: apitypes.RuntimeProfileFlowcraftBBHConnectionTypeFlowcraftBbh,
	}); err != nil {
		t.Fatalf("build RuntimeProfile memory connection: %v", err)
	}
	memories := map[string]apitypes.RuntimeProfileMemoryBinding{
		"voice-assistant-memory": {
			LayoutId:   "voice-assistant-memory",
			Driver:     apitypes.RuntimeProfileMemoryDriverFlowcraft,
			Connection: memoryConnection,
		},
	}
	models := map[string]apitypes.RuntimeProfileBinding{
		"llm": runtimeProfileBinding("doubao-mini-chat"),
		"asr": runtimeProfileBinding("volc-bigasr-sauc"),
	}
	voices := map[string]apitypes.RuntimeProfileBinding{
		"narrator": runtimeProfileBinding("volc-tenant:volc-main:zh_female_xiaohe_uranus_bigtts"),
	}
	profile, err := clitest.UpsertRuntimeProfile(ctx, api, adminhttp.RuntimeProfileUpsert{
		Id: profileID,
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{Memories: &memories, Models: &models, Voices: &voices},
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{
					FriendChatroom: "chatroom-direct",
					GroupChatroom:  "chatroom-direct",
					Pet:            "pet-chatroom",
				},
				Collections: apitypes.RuntimeProfileWorkflowCollections{
					"assistants": {
						"voice": {ResourceId: workflowName, I18n: map[string]apitypes.RuntimeProfileI18nText{
							"en":    {DisplayName: "Voice"},
							"zh-CN": {DisplayName: "语音"},
						}},
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
	workspaceID := adminWorkspaceIDByName(t, list.Stdout, workspaceName)
	workflows := h.RunCLI("admin", "workflows", "list", "--context", "admin-a")
	workflows.MustSucceed(t)
	voiceWorkflowID := adminResourceID(t, workflows.Stdout, "flowcraft-voice-assistant")

	get := h.RunCLI("admin", "workspaces", "get", workspaceID, "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"workflow_id":"`+voiceWorkflowID+`"`) {
		t.Fatalf("workspaces get missing canonical workflow ID:\n%s", get.Stdout)
	}
}

func runtimeProfileBinding(resourceID string) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{ResourceId: resourceID, I18n: map[string]apitypes.RuntimeProfileI18nText{
		"en":    {DisplayName: resourceID},
		"zh-CN": {DisplayName: resourceID},
	}}
}

func adminWorkspaceIDByName(t *testing.T, output, name string) string {
	t.Helper()
	var workspaces []apitypes.Workspace
	if err := json.Unmarshal([]byte(output), &workspaces); err != nil {
		t.Fatalf("decode admin workspace list: %v\n%s", err, output)
	}
	for _, workspace := range workspaces {
		if workspace.Name == name {
			if workspace.Id == "" {
				t.Fatalf("admin workspace %q has an empty canonical ID", name)
			}
			return workspace.Id
		}
	}
	t.Fatalf("admin workspace %q not found in list:\n%s", name, output)
	return ""
}
