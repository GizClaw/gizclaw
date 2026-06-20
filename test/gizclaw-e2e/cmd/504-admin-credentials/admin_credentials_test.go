package admincredentials_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestAdminCredentialsUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "504-admin-credentials")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	seedCredentials(t, h)

	list := h.RunCLI("admin", "credentials", "list", "--context", "admin-a")
	list.MustSucceed(t)
	for _, want := range []string{`"name":"main"`, `"name":"tenant-key"`} {
		if !strings.Contains(list.Stdout, want) {
			t.Fatalf("credentials list missing %q:\n%s", want, list.Stdout)
		}
	}

	filtered := h.RunCLI("admin", "credentials", "list", "--provider", "openai", "--context", "admin-a")
	filtered.MustSucceed(t)
	if !strings.Contains(filtered.Stdout, `"name":"main"`) || strings.Contains(filtered.Stdout, `"name":"tenant-key"`) {
		t.Fatalf("credentials filtered list returned unexpected items:\n%s", filtered.Stdout)
	}

	get := h.RunCLI("admin", "credentials", "get", "main", "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"provider":"openai"`) {
		t.Fatalf("credentials get missing provider:\n%s", get.Stdout)
	}
}

func seedCredentials(t *testing.T, h *clitest.Harness) {
	t.Helper()

	c := h.ConnectClientFromContext("admin-a")
	defer c.Close()
	api, err := c.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, req := range []adminservice.CredentialUpsert{
		{
			Name:        "main",
			Provider:    "openai",
			Description: ptr("primary"),
			Body:        testOpenAICredentialBody("sk-test"),
		},
		{
			Name:     "tenant-key",
			Provider: "minimax",
			Body:     testMiniMaxCredentialBody("sk-minimax"),
		},
	} {
		resp, err := api.CreateCredentialWithResponse(ctx, req)
		if err != nil {
			t.Fatalf("seed credential %q: %v", req.Name, err)
		}
		if resp.JSON200 == nil {
			t.Fatalf("seed credential %q got status %d: %s", req.Name, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
		}
	}
}

func ptr(value string) *string {
	return &value
}

func testOpenAICredentialBody(apiKey string) apitypes.CredentialBody {
	var body apitypes.CredentialBody
	if err := body.FromOpenAICredentialBody(apitypes.OpenAICredentialBody{ApiKey: ptr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}

func testMiniMaxCredentialBody(apiKey string) apitypes.CredentialBody {
	var body apitypes.CredentialBody
	if err := body.FromMiniMaxCredentialBody(apitypes.MiniMaxCredentialBody{ApiKey: ptr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}
