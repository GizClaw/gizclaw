//go:build gizclaw_e2e

package openai_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type openAIHarness struct {
	ctx    context.Context
	h      *clitest.Harness
	peer   *gizcli.Client
	client openai.Client
	other  openai.Client
	http   *http.Client
}

func newOpenAIHarness(t *testing.T) *openAIHarness {
	t.Helper()
	h := clitest.NewSetupHarness(t, "openai-conversations")
	identitiesHome := envOr("GIZCLAW_E2E_IDENTITIES_HOME", filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "identities"))
	h.SetContextDirAlias("admin", filepath.Join(identitiesHome, envOr("GIZCLAW_E2E_ADMIN_IDENTITY", "admin")))
	h.CreateContext("openai-peer").MustSucceed(t)
	h.CreateContext("openai-other").MustSucceed(t)
	h.RegisterContext("openai-peer", "--sn", "openai-conversations-e2e").MustSucceed(t)
	peer := h.ConnectClientFromContext("openai-peer")
	t.Cleanup(func() { peer.Close() })
	otherPeer := h.ConnectClientFromContext("openai-other")
	t.Cleanup(func() { otherPeer.Close() })

	admin := h.ConnectClientFromContext("admin")
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	profileID := "e2e-openai-conversations"
	profile, err := clitest.UpsertRuntimeProfile(ctx, api, adminhttp.RuntimeProfileUpsert{Id: profileID, Spec: openAIRuntimeProfile(t)})
	if err != nil {
		t.Fatalf("put RuntimeProfile: %v", err)
	}
	tokenID := "e2e-openai-conversations-token"
	if err := clitest.DeleteRegistrationTokenByID(ctx, api, tokenID); err != nil {
		t.Fatal(err)
	}
	firmware, found, err := clitest.FirmwareByID(ctx, api, "devkit-firmware-main")
	if err != nil || !found {
		t.Fatalf("resolve Firmware: found=%v err=%v", found, err)
	}
	firmwareID := firmware.Id
	tokenResponse, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{Id: tokenID, Token: tokenID, RuntimeProfileId: profile.Id, FirmwareId: &firmwareID})
	if err != nil || tokenResponse.JSON200 == nil {
		t.Fatalf("create RegistrationToken: response=%#v err=%v", tokenResponse, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = clitest.DeleteRegistrationTokenByID(cleanupCtx, api, tokenID)
		_, _ = api.DeleteRuntimeProfileWithResponse(cleanupCtx, profile.Id)
		_ = admin.Close()
	})
	if _, err := peer.Register(ctx, "openai.e2e.register", tokenResponse.JSON200.Token); err != nil {
		t.Fatalf("register Peer: %v", err)
	}
	if _, err := otherPeer.Register(ctx, "openai.e2e.register.other", tokenResponse.JSON200.Token); err != nil {
		t.Fatalf("register other Peer: %v", err)
	}
	httpClient := peer.HTTPClient(gizcli.ServicePeerOpenAI)
	httpClient.Timeout = 3 * time.Minute
	client := openai.NewClient(option.WithAPIKey("gizclaw-peer"), option.WithBaseURL("http://gizclaw/v1"), option.WithHTTPClient(httpClient))
	otherHTTP := otherPeer.HTTPClient(gizcli.ServicePeerOpenAI)
	otherHTTP.Timeout = 3 * time.Minute
	other := openai.NewClient(option.WithAPIKey("gizclaw-peer"), option.WithBaseURL("http://gizclaw/v1"), option.WithHTTPClient(otherHTTP))
	return &openAIHarness{ctx: ctx, h: h, peer: peer, client: client, other: other, http: httpClient}
}

func openAIRuntimeProfile(t *testing.T) apitypes.RuntimeProfileSpec {
	t.Helper()
	workflows := apitypes.RuntimeProfileWorkflowCollections{"assistants": {"shared": binding("flowcraft-chat-assistant")}}
	models := map[string]apitypes.RuntimeProfileBinding{"llm": binding("fake-openai-chat-000"), "asr": binding("volc-bigasr-sauc")}
	voices := map[string]apitypes.RuntimeProfileBinding{"narrator": binding("volc-tenant:volc-main:zh_female_vv_jupiter_bigtts")}
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftBBHConnection(apitypes.RuntimeProfileFlowcraftBBHConnection{Type: apitypes.RuntimeProfileFlowcraftBBHConnectionTypeFlowcraftBbh}); err != nil {
		t.Fatal(err)
	}
	memories := map[string]apitypes.RuntimeProfileMemoryBinding{"chat-memory": {LayoutId: "chat-memory", Driver: apitypes.RuntimeProfileMemoryDriverFlowcraft, Connection: connection}}
	return apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{
			System:      apitypes.RuntimeProfileSystemWorkflows{FriendChatroom: "chatroom-direct", GroupChatroom: "chatroom-direct", Pet: "pet-chatroom"},
			Collections: workflows,
		},
		Resources: apitypes.RuntimeProfileResources{Models: &models, Voices: &voices, Memories: &memories},
	}
}

func binding(id string) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{ResourceId: id, I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: id}}}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
