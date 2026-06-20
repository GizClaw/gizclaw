package adminvoices_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestAdminVoicesUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "506-admin-voices")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	seedVoices(t, h)

	list := h.RunCLI("admin", "voices", "list", "--context", "admin-a")
	list.MustSucceed(t)
	for _, want := range []string{`"id":"voice-1"`, `"id":"voice-2"`} {
		if !strings.Contains(list.Stdout, want) {
			t.Fatalf("voices list missing %q:\n%s", want, list.Stdout)
		}
	}

	filtered := h.RunCLI("admin", "voices", "list", "--provider-name", "main-cn", "--context", "admin-a")
	filtered.MustSucceed(t)
	if !strings.Contains(filtered.Stdout, `"id":"voice-1"`) || strings.Contains(filtered.Stdout, `"id":"voice-2"`) {
		t.Fatalf("voices filtered list returned unexpected items:\n%s", filtered.Stdout)
	}

	get := h.RunCLI("admin", "voices", "get", "voice-1", "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"name":"Voice One"`) {
		t.Fatalf("voices get missing name:\n%s", get.Stdout)
	}

	showVolcVoice := h.RunCLI("admin", "--context", "admin-a", "show", "Voice", "volc-tenant:gizclaw-dev:ICL_cli_seed_voice")
	showVolcVoice.MustSucceed(t)
	for _, want := range []string{`"kind":"Voice"`, `"name":"volc-tenant:gizclaw-dev:ICL_cli_seed_voice"`, `"resource_id":"seed-tts-2.0"`} {
		if !strings.Contains(showVolcVoice.Stdout, want) {
			t.Fatalf("admin show Volc voice missing %q:\n%s", want, showVolcVoice.Stdout)
		}
	}

	showVolcTenant := h.RunCLI("admin", "--context", "admin-a", "show", "VolcTenant", "gizclaw-dev")
	showVolcTenant.MustSucceed(t)
	for _, want := range []string{`"kind":"VolcTenant"`, `"name":"gizclaw-dev"`, `"app_id":"app-cli"`} {
		if !strings.Contains(showVolcTenant.Stdout, want) {
			t.Fatalf("admin show VolcTenant missing %q:\n%s", want, showVolcTenant.Stdout)
		}
	}

	syncVolcTenant := h.RunCLI("admin", "volc-tenants", "--context", "admin-a", "sync-voices", "gizclaw-dev")
	if syncVolcTenant.Err == nil {
		t.Fatalf("volc sync with incomplete credential should fail:\n%s", syncVolcTenant.Stdout)
	}
	for _, want := range []string{"INVALID_VOLC_TENANT", "missing access_key_id/secret_access_key"} {
		if !strings.Contains(syncVolcTenant.Stderr, want) {
			t.Fatalf("volc sync stderr missing %q:\nstdout:\n%s\nstderr:\n%s", want, syncVolcTenant.Stdout, syncVolcTenant.Stderr)
		}
	}
}

func seedVoices(t *testing.T, h *clitest.Harness) {
	t.Helper()

	c := h.ConnectClientFromContext("admin-a")
	defer c.Close()
	api, err := c.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, req := range []adminservice.VoiceUpsert{
		{
			Id:     "voice-1",
			Source: apitypes.VoiceSourceManual,
			Provider: apitypes.VoiceProvider{
				Kind: "minimax-tenant",
				Name: "main-cn",
			},
			Name:        ptr("Voice One"),
			Description: ptr("primary"),
		},
		{
			Id:     "voice-2",
			Source: apitypes.VoiceSourceManual,
			Provider: apitypes.VoiceProvider{
				Kind: "minimax-tenant",
				Name: "other-cn",
			},
			Name: ptr("Voice Two"),
		},
		{
			Id:     "volc-tenant:gizclaw-dev:ICL_cli_seed_voice",
			Source: apitypes.VoiceSourceManual,
			Provider: apitypes.VoiceProvider{
				Kind: "volc-tenant",
				Name: "gizclaw-dev",
			},
			Name:         ptr("Volc CLI Seed Voice"),
			Description:  ptr("seeded Volc voice for CLI examples"),
			ProviderData: testVolcVoiceProviderData(),
		},
	} {
		resp, err := api.CreateVoiceWithResponse(ctx, req)
		if err != nil {
			t.Fatalf("seed voice %q: %v", req.Id, err)
		}
		if resp.JSON200 == nil {
			t.Fatalf("seed voice %q got status %d: %s", req.Id, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
		}
	}
	credentialResp, err := api.CreateCredentialWithResponse(ctx, adminservice.CredentialUpsert{
		Name:     "volc-cli-credential",
		Provider: "volc",
		Body:     testVolcCredentialBody(),
	})
	if err != nil {
		t.Fatalf("seed volc credential: %v", err)
	}
	if credentialResp.JSON200 == nil {
		t.Fatalf("seed volc credential got status %d: %s", credentialResp.StatusCode(), strings.TrimSpace(string(credentialResp.Body)))
	}
	resourceIDs := []string{"seed-tts-2.0"}
	tenantResp, err := api.CreateVolcTenantWithResponse(ctx, adminservice.VolcTenantUpsert{
		Name:           "gizclaw-dev",
		CredentialName: "volc-cli-credential",
		Region:         ptr("cn-beijing"),
		ResourceIds:    &resourceIDs,
		Description:    ptr("seeded Volc tenant for CLI examples"),
	})
	if err != nil {
		t.Fatalf("seed volc tenant: %v", err)
	}
	if tenantResp.JSON200 == nil {
		t.Fatalf("seed volc tenant got status %d: %s", tenantResp.StatusCode(), strings.TrimSpace(string(tenantResp.Body)))
	}
}

func ptr(value string) *string {
	return &value
}

func testVolcVoiceProviderData() *apitypes.VoiceProviderData {
	resourceID := "seed-tts-2.0"
	state := "Success"
	status := "Available"
	voiceID := "ICL_cli_seed_voice"
	var out apitypes.VoiceProviderData
	if err := out.FromVolcTenantVoiceProviderData(apitypes.VolcTenantVoiceProviderData{
		ResourceId: &resourceID,
		State:      &state,
		Status:     &status,
		VoiceId:    &voiceID,
	}); err != nil {
		panic(err)
	}
	return &out
}

func testVolcCredentialBody() apitypes.CredentialBody {
	appID := "app-cli"
	secret := "secret"
	ak := "ak"
	var body apitypes.CredentialBody
	if err := body.FromVolcCredentialBody(apitypes.VolcCredentialBody{
		AppId:              &appID,
		OpenapiAccessKeyId: &ak,
		SecretAccessKey:    &secret,
	}); err != nil {
		panic(err)
	}
	return body
}
