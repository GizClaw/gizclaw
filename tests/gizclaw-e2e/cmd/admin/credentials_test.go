//go:build gizclaw_e2e

package admin_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminCredentialsUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "504-admin-credentials")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	list := h.RunCLI("admin", "credentials", "list", "--context", "admin-a")
	list.MustSucceed(t)
	for _, want := range []string{`"name":"fake-openai-credential-000"`, `"name":"fake-openai-credential-049"`} {
		if !strings.Contains(list.Stdout, want) {
			t.Fatalf("credentials list missing %q:\n%s", want, list.Stdout)
		}
	}

	filtered := h.RunCLI("admin", "credentials", "list", "--provider", "openai", "--context", "admin-a")
	filtered.MustSucceed(t)
	if !strings.Contains(filtered.Stdout, `"name":"fake-openai-credential-000"`) || strings.Contains(filtered.Stdout, `"provider":"minimax"`) {
		t.Fatalf("credentials filtered list returned unexpected items:\n%s", filtered.Stdout)
	}

	get := h.RunCLI("admin", "credentials", "get", "fake-openai-credential-000", "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"provider":"openai"`) {
		t.Fatalf("credentials get missing provider:\n%s", get.Stdout)
	}

	rpcGet := h.RunCLI("admin", "credentials", "get", "fake-openai-credential-000", "--context", "admin-a")
	rpcGet.MustSucceed(t)
	if !strings.Contains(rpcGet.Stdout, `"api_key":"sk-fake-openai-000"`) {
		t.Fatalf("credentials get missing fake api key:\n%s", rpcGet.Stdout)
	}
}
