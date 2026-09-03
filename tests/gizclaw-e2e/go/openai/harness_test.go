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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type openAIHarness struct {
	ctx            context.Context
	h              *clitest.Harness
	peer           *gizcli.Client
	client         openai.Client
	other          openai.Client
	http           *http.Client
	baseURL        string
	apiKey         string
	apiKeyCreateMS int64
	apiKeyAuthMS   int64
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
	apiKeyCreateStarted := time.Now()
	created, err := peer.CreateAPIKey(ctx, "openai.e2e.api-key", rpcapi.APIKeyCreateRequest{DisplayName: "OpenAI compatibility"})
	if err != nil {
		t.Fatalf("create OpenAI API key: %v", err)
	}
	apiKeyCreateMS := max(time.Since(apiKeyCreateStarted).Milliseconds(), 1)
	otherCreated, err := otherPeer.CreateAPIKey(ctx, "openai.e2e.api-key.other", rpcapi.APIKeyCreateRequest{DisplayName: "OpenAI compatibility other"})
	if err != nil {
		t.Fatalf("create other OpenAI API key: %v", err)
	}
	baseURL := strings.TrimRight(h.PublicHTTPURL(), "/") + "/openai/v1"
	httpClient := &http.Client{Timeout: 3 * time.Minute}
	authStarted := time.Now()
	authRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(h.PublicHTTPURL(), "/")+"/gizclaw/v1/api-keys/self", nil)
	if err != nil {
		t.Fatal(err)
	}
	authRequest.Header.Set("Authorization", "Bearer "+created.APIKey)
	authResponse, err := httpClient.Do(authRequest)
	if err != nil {
		t.Fatalf("measure API key auth: %v", err)
	}
	_ = authResponse.Body.Close()
	if authResponse.StatusCode != http.StatusOK {
		t.Fatalf("measure API key auth status = %d", authResponse.StatusCode)
	}
	apiKeyAuthMS := max(time.Since(authStarted).Milliseconds(), 1)
	client := openai.NewClient(option.WithAPIKey(created.APIKey), option.WithBaseURL(baseURL), option.WithHTTPClient(httpClient))
	other := openai.NewClient(option.WithAPIKey(otherCreated.APIKey), option.WithBaseURL(baseURL), option.WithHTTPClient(httpClient))
	return &openAIHarness{
		ctx: ctx, h: h, peer: peer, client: client, other: other, http: httpClient,
		baseURL: baseURL, apiKey: created.APIKey,
		apiKeyCreateMS: apiKeyCreateMS, apiKeyAuthMS: apiKeyAuthMS,
	}
}

func openAIRuntimeProfile(t *testing.T) apitypes.RuntimeProfileSpec {
	t.Helper()
	workflows := apitypes.RuntimeProfileWorkflowCollections{"assistants": {"shared": binding("flowcraft-chat-assistant")}}
	models := map[string]apitypes.RuntimeProfileBinding{"llm": binding("doubao-mini-chat"), "asr": binding("volc-bigasr-sauc")}
	voices := map[string]apitypes.RuntimeProfileBinding{"narrator": binding("volc-tenant:volc-main:zh_female_xiaohe_uranus_bigtts")}
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftRedis8Connection(apitypes.RuntimeProfileFlowcraftRedis8Connection{
		Type: apitypes.RuntimeProfileFlowcraftRedis8ConnectionTypeFlowcraftRedis8, Url: "redis://redis:6379/0",
	}); err != nil {
		t.Fatal(err)
	}
	memories := map[string]apitypes.RuntimeProfileMemoryBinding{"chat-memory": {LayoutId: "chat-memory", Driver: apitypes.RuntimeProfileMemoryDriverFlowcraft, Connection: connection}}
	return apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{
			System:      apitypes.RuntimeProfileSystemWorkflows{Pet: "pet-care"},
			Collections: workflows,
		},
		Resources: apitypes.RuntimeProfileResources{Models: &models, Voices: &voices, Memories: &memories},
	}
}

func binding(id string) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{
		ResourceId: id,
		I18n: map[string]apitypes.RuntimeProfileI18nText{
			"en":    {DisplayName: id},
			"zh-CN": {DisplayName: id},
		},
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
