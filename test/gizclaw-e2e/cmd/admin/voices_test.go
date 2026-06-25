//go:build gizclaw_e2e

package admin_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestAdminVoicesUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "506-admin-voices")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-voices-sn").MustSucceed(t)

	list := h.RunCLI("admin", "voices", "list", "--context", "admin-a")
	list.MustSucceed(t)
	for _, want := range []string{`"id":"minimax-cloned-narrator"`, `"id":"volc-tenant:volc-lab:ICL_demo_voice"`} {
		if !strings.Contains(list.Stdout, want) {
			t.Fatalf("voices list missing %q:\n%s", want, list.Stdout)
		}
	}

	filtered := h.RunCLI("admin", "voices", "list", "--provider-name", "minimax-main", "--context", "admin-a")
	filtered.MustSucceed(t)
	if !strings.Contains(filtered.Stdout, `"id":"minimax-cloned-narrator"`) || strings.Contains(filtered.Stdout, `"id":"volc-tenant:volc-lab:ICL_demo_voice"`) {
		t.Fatalf("voices filtered list returned unexpected items:\n%s", filtered.Stdout)
	}

	get := h.RunCLI("admin", "voices", "get", "minimax-cloned-narrator", "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"name":"MiniMax Cloned Narrator"`) {
		t.Fatalf("voices get missing name:\n%s", get.Stdout)
	}

	showVolcVoice := h.RunCLI("admin", "--context", "admin-a", "show", "Voice", "volc-tenant:volc-lab:ICL_demo_voice")
	showVolcVoice.MustSucceed(t)
	for _, want := range []string{`"kind":"Voice"`, `"name":"volc-tenant:volc-lab:ICL_demo_voice"`, `"resource_id":"seed-tts-2.0"`} {
		if !strings.Contains(showVolcVoice.Stdout, want) {
			t.Fatalf("admin show Volc voice missing %q:\n%s", want, showVolcVoice.Stdout)
		}
	}

	showVolcTenant := h.RunCLI("admin", "--context", "admin-a", "show", "VolcTenant", "volc-lab")
	showVolcTenant.MustSucceed(t)
	for _, want := range []string{`"kind":"VolcTenant"`, `"name":"volc-lab"`, `"credential_name":"volc-lab-credential"`} {
		if !strings.Contains(showVolcTenant.Stdout, want) {
			t.Fatalf("admin show VolcTenant missing %q:\n%s", want, showVolcTenant.Stdout)
		}
	}

	showVolcCredential := h.RunCLI("admin", "--context", "admin-a", "show", "Credential", "volc-lab-credential")
	showVolcCredential.MustSucceed(t)
	for _, want := range []string{`"kind":"Credential"`, `"name":"volc-lab-credential"`, `"app_id":"volc-lab-app"`} {
		if !strings.Contains(showVolcCredential.Stdout, want) {
			t.Fatalf("admin show Volc credential missing %q:\n%s", want, showVolcCredential.Stdout)
		}
	}

	syncVolcTenant := h.RunCLI("admin", "volc-tenants", "--context", "admin-a", "sync-voices", "volc-lab")
	if syncVolcTenant.Err == nil {
		t.Fatalf("volc sync with incomplete credential should fail:\n%s", syncVolcTenant.Stdout)
	}
	for _, want := range []string{"INVALID_VOLC_TENANT", "missing openapi_access_key_id/secret_access_key"} {
		if !strings.Contains(syncVolcTenant.Stderr, want) {
			t.Fatalf("volc sync stderr missing %q:\nstdout:\n%s\nstderr:\n%s", want, syncVolcTenant.Stdout, syncVolcTenant.Stderr)
		}
	}
}
