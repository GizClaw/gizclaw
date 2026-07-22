//go:build gizclaw_e2e

package chat_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	cgointernal "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cgo/internal"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
	gochat "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/go/chat"
)

func TestCSDKChatWorkspaceRPC(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-chat")
	identityDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_PEER_IDENTITY", "peer")
	cgointernal.AssertServerAvailable(t, identityDir)
	registrationToken := createCSDKChatRegistrationToken(t, h, "workspace")
	cgointernal.CSDKChatWorkspace(t, identityDir, registrationToken)
}

func TestCSDKChatRoundtrip(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-chat-roundtrip")
	identityDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_PEER_IDENTITY", "peer")
	cgointernal.AssertServerAvailable(t, identityDir)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	configPath := filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "workspaces", "doubao-realtime.json")
	contextConfigPath := filepath.Join(identityDir, "config.yaml")
	registrationToken := createCSDKChatRegistrationToken(t, h, "roundtrip")
	workspaceName, err := gochat.PrepareCgoPushToTalkWorkspace(ctx, configPath, contextConfigPath, "realtime-workflow", registrationToken)
	if err != nil {
		t.Fatalf("prepare cgo chat workspace: %v", err)
	}
	fixture := filepath.Join(h.RepoRoot, "tests", "genx-e2e", "transformer", "testdata", "doubao_realtime_duplex_prompt.ogg")
	cgointernal.CSDKChatRoundtrip(t, identityDir, registrationToken, workspaceName, fixture)
}

func createCSDKChatRegistrationToken(t *testing.T, h *clitest.Harness, scenario string) string {
	t.Helper()
	adminDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_ADMIN_IDENTITY", "admin")
	h.SetContextDirAlias("admin-a", adminDir)
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	workflowResources := map[string]string{
		"chatroom":          "chatroom-direct",
		"realtime-workflow": "doubao-realtime-conversation",
	}
	modelResources := map[string]string{
		"llm":      "doubao-lite-chat",
		"tts":      "volc-bigtts",
		"asr":      "volc-bigasr-sauc",
		"realtime": "doubao-realtime-dialog",
	}
	voiceResources := map[string]string{
		"doubao-assistant": "volc-tenant:volc-main:zh_female_vv_jupiter_bigtts",
	}
	profileName := "cgo-chat"
	profileResp, err := api.PutRuntimeProfileWithResponse(ctx, profileName, adminhttp.RuntimeProfileUpsert{
		Name: profileName,
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{
			Models: ptr(runtimeBindings(modelResources)),
			Voices: ptr(runtimeBindings(voiceResources)),
		}, Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{
			"assistants": runtimeBindings(workflowResources),
		}}},
	})
	if err != nil {
		t.Fatalf("put C SDK chat RuntimeProfile: %v", err)
	}
	if profileResp.JSON200 == nil {
		t.Fatalf("put C SDK chat RuntimeProfile status %d: %s", profileResp.StatusCode(), strings.TrimSpace(string(profileResp.Body)))
	}
	tokenName := "cgo-chat-" + scenario
	_, _ = api.DeleteRegistrationTokenWithResponse(ctx, tokenName)
	tokenResp, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{
		Name:               tokenName,
		RuntimeProfileName: profileName,
	})
	if err != nil {
		t.Fatalf("create C SDK chat RegistrationToken: %v", err)
	}
	if tokenResp.JSON200 == nil || tokenResp.JSON200.Token == "" {
		t.Fatalf("create C SDK chat RegistrationToken status %d: %s", tokenResp.StatusCode(), strings.TrimSpace(string(tokenResp.Body)))
	}
	return tokenResp.JSON200.Token
}

func runtimeBindings(resources map[string]string) map[string]apitypes.RuntimeProfileBinding {
	bindings := make(map[string]apitypes.RuntimeProfileBinding, len(resources))
	for alias, resourceID := range resources {
		bindings[alias] = apitypes.RuntimeProfileBinding{
			ResourceId: resourceID,
			I18n: map[string]apitypes.RuntimeProfileI18nText{
				"en":    {DisplayName: alias},
				"zh-CN": {DisplayName: alias},
			},
		}
	}
	return bindings
}

func ptr[T any](value T) *T { return &value }
