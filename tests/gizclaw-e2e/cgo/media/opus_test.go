//go:build gizclaw_e2e

package media_test

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

func TestCSDKBidirectionalOpusRTP(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-opus-media")
	identityDir := cgointernal.SharedIdentityDir(
		t, h, "GIZCLAW_E2E_PEER_IDENTITY", "peer",
	)
	cgointernal.AssertServerAvailable(t, identityDir)
	registrationToken := createMediaRegistrationToken(t, h)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	workspaceName, err := gochat.PrepareCgoPushToTalkWorkspace(
		ctx,
		filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "workspaces", "doubao-realtime.json"),
		filepath.Join(identityDir, "config.yaml"),
		"realtime-workflow",
		registrationToken,
	)
	if err != nil {
		t.Fatalf("prepare C SDK Opus workspace: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cleanupCancel()
		if err := gochat.CleanupCgoPushToTalkWorkspaces(
			cleanupCtx,
			filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "workspaces", "doubao-realtime.json"),
			filepath.Join(identityDir, "config.yaml"),
			registrationToken,
			workspaceName,
		); err != nil {
			t.Errorf("cleanup C SDK Opus Workspace: %v", err)
		}
	})
	fixture := filepath.Join(
		h.RepoRoot,
		"tests", "genx-e2e", "transformer", "testdata",
		"doubao_realtime_duplex_prompt.ogg",
	)
	// CSDKChatRoundtrip asserts that uplink Opus increments only the RTP
	// counter, never the packet-DataChannel counter, and that remote RTP is
	// returned through gzc_client_read_packet as protocol 0x10.
	cgointernal.CSDKChatRoundtrip(
		t, identityDir, registrationToken, workspaceName, fixture,
	)
}

func createMediaRegistrationToken(t *testing.T, h *clitest.Harness) string {
	t.Helper()
	adminDir := cgointernal.SharedIdentityDir(
		t, h, "GIZCLAW_E2E_ADMIN_IDENTITY", "admin",
	)
	h.SetContextDirAlias("admin-a", adminDir)
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	profileName := "cgo-opus-media"
	profile, err := clitest.UpsertRuntimeProfileByName(
		ctx,
		api,
		adminhttp.RuntimeProfileUpsert{
			Name: profileName,
			Spec: apitypes.RuntimeProfileSpec{
				Resources: apitypes.RuntimeProfileResources{
					Models: ptr(runtimeBindings(map[string]string{
						"llm":      "doubao-lite-chat",
						"tts":      "volc-bigtts",
						"asr":      "volc-bigasr-sauc",
						"realtime": "doubao-realtime-dialog",
					})),
					Voices: ptr(runtimeBindings(map[string]string{
						"doubao-assistant": "volc-tenant:volc-main:zh_female_vv_jupiter_bigtts",
					})),
				},
				Workflows: apitypes.RuntimeProfileWorkflows{
					System: apitypes.RuntimeProfileSystemWorkflows{
						FriendChatroom: "chatroom-direct",
						GroupChatroom:  "chatroom-direct",
						Pet:            "pet-chatroom",
					},
					Collections: apitypes.RuntimeProfileWorkflowCollections{
						"assistants": runtimeBindings(map[string]string{
							"chatroom":          "chatroom-direct",
							"realtime-workflow": "doubao-realtime-conversation",
						}),
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("put C SDK media RuntimeProfile: %v", err)
	}
	tokenName := "cgo-opus-media"
	if err := clitest.DeleteRegistrationTokenByName(ctx, api, tokenName); err != nil {
		t.Fatalf("retire C SDK media RegistrationToken: %v", err)
	}
	tokenResp, err := api.CreateRegistrationTokenWithResponse(
		ctx,
		adminhttp.RegistrationTokenUpsert{
			Name:               tokenName,
			Token:              tokenName,
			RuntimeProfileId: profile.Id,
		},
	)
	if err != nil {
		t.Fatalf("create C SDK media RegistrationToken: %v", err)
	}
	if tokenResp.JSON200 == nil || tokenResp.JSON200.Token == "" {
		t.Fatalf(
			"create C SDK media RegistrationToken status %d: %s",
			tokenResp.StatusCode(),
			strings.TrimSpace(string(tokenResp.Body)),
		)
	}
	return tokenResp.JSON200.Token
}

func runtimeBindings(
	resources map[string]string,
) map[string]apitypes.RuntimeProfileBinding {
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
