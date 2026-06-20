package adminminimaxtenants_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestAdminMiniMaxTenantsUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "505-admin-minimax-tenants")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	seedMiniMaxTenant(t, h)

	list := h.RunCLI("admin", "minimax-tenants", "list", "--context", "admin-a")
	list.MustSucceed(t)
	if !strings.Contains(list.Stdout, `"name":"main-cn"`) {
		t.Fatalf("minimax tenants list missing created item:\n%s", list.Stdout)
	}

	get := h.RunCLI("admin", "minimax-tenants", "get", "main-cn", "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"credential_name":"main-credential"`) {
		t.Fatalf("minimax tenants get missing credential:\n%s", get.Stdout)
	}
}

func seedMiniMaxTenant(t *testing.T, h *clitest.Harness) {
	t.Helper()

	c := h.ConnectClientFromContext("admin-a")
	defer c.Close()
	api, err := c.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	credentialResp, err := api.CreateCredentialWithResponse(ctx, adminservice.CredentialUpsert{
		Name:     "main-credential",
		Provider: "minimax",
		Body:     testMiniMaxCredentialBody("sk-test"),
	})
	if err != nil {
		t.Fatalf("seed minimax credential: %v", err)
	}
	if credentialResp.JSON200 == nil {
		t.Fatalf("seed minimax credential got status %d: %s", credentialResp.StatusCode(), strings.TrimSpace(string(credentialResp.Body)))
	}
	tenantResp, err := api.CreateMiniMaxTenantWithResponse(ctx, adminservice.MiniMaxTenantUpsert{
		Name:           "main-cn",
		AppId:          "app-1",
		GroupId:        "group-1",
		CredentialName: "main-credential",
		BaseUrl:        ptr("https://example.invalid"),
		Description:    ptr("primary"),
	})
	if err != nil {
		t.Fatalf("seed minimax tenant: %v", err)
	}
	if tenantResp.JSON200 == nil {
		t.Fatalf("seed minimax tenant got status %d: %s", tenantResp.StatusCode(), strings.TrimSpace(string(tenantResp.Body)))
	}
}

func ptr(value string) *string {
	return &value
}

func testMiniMaxCredentialBody(apiKey string) apitypes.CredentialBody {
	var body apitypes.CredentialBody
	if err := body.FromMiniMaxCredentialBody(apitypes.MiniMaxCredentialBody{ApiKey: ptr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}
