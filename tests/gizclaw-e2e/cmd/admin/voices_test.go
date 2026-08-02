//go:build gizclaw_e2e

package admin_test

import (
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminVoicesUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "506-admin-voices")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-voices-sn").MustSucceed(t)

	list := h.RunCLI("admin", "voices", "list", "--context", "admin-a")
	list.MustSucceed(t)
	adminFirstResource(t, list.Stdout)
	miniMaxTenants := h.RunCLI("admin", "minimax-tenants", "list", "--context", "admin-a")
	miniMaxTenants.MustSucceed(t)
	miniMaxTenantID := adminResourceID(t, miniMaxTenants.Stdout, "minimax-cn")

	filtered := h.RunCLI("admin", "voices", "list", "--provider-id", miniMaxTenantID, "--context", "admin-a")
	filtered.MustSucceed(t)
	if !strings.Contains(filtered.Stdout, `"name":"minimax-narrator-clone"`) || strings.Contains(filtered.Stdout, `"kind":"volc-tenant"`) {
		t.Fatalf("voices filtered list returned unexpected items:\n%s", filtered.Stdout)
	}
	miniMaxVoiceID := adminResourceID(t, filtered.Stdout, "minimax-narrator-clone")

	get := h.RunCLI("admin", "voices", "get", miniMaxVoiceID, "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"name":"minimax-narrator-clone"`) {
		t.Fatalf("voices get missing name:\n%s", get.Stdout)
	}

	volcTenants := h.RunCLI("admin", "volc-tenants", "list", "--context", "admin-a")
	volcTenants.MustSucceed(t)
	volcTenantID := adminResourceID(t, volcTenants.Stdout, "volc-main")
	volcVoices := h.RunCLI("admin", "voices", "list", "--provider-id", volcTenantID, "--context", "admin-a")
	volcVoices.MustSucceed(t)
	volcVoice := adminFirstResource(t, volcVoices.Stdout)
	showVolcVoice := h.RunCLI("admin", "--context", "admin-a", "show", "Voice", volcVoice.ID)
	showVolcVoice.MustSucceed(t)
	for _, want := range []string{`"kind":"Voice"`, `"name":"` + volcVoice.Name + `"`, `"provider":{"id":"` + volcTenantID + `","kind":"volc-tenant"}`} {
		if !strings.Contains(showVolcVoice.Stdout, want) {
			t.Fatalf("admin show Volc voice missing %q:\n%s", want, showVolcVoice.Stdout)
		}
	}

	credentials := h.RunCLI("admin", "credentials", "list", "--context", "admin-a")
	credentials.MustSucceed(t)
	volcCredentialID := adminResourceID(t, credentials.Stdout, "volc-main-credential")
	showVolcTenant := h.RunCLI("admin", "--context", "admin-a", "show", "VolcTenant", volcTenantID)
	showVolcTenant.MustSucceed(t)
	for _, want := range []string{`"kind":"VolcTenant"`, `"name":"volc-main"`, `"credential_id":"` + volcCredentialID + `"`} {
		if !strings.Contains(showVolcTenant.Stdout, want) {
			t.Fatalf("admin show VolcTenant missing %q:\n%s", want, showVolcTenant.Stdout)
		}
	}

	showVolcCredential := h.RunCLI("admin", "--context", "admin-a", "show", "Credential", volcCredentialID)
	showVolcCredential.MustSucceed(t)
	for _, want := range []string{`"kind":"Credential"`, `"name":"volc-main-credential"`} {
		if !strings.Contains(showVolcCredential.Stdout, want) {
			t.Fatalf("admin show Volc credential missing %q:\n%s", want, showVolcCredential.Stdout)
		}
	}

	syncVolcTenant := h.RunCLI("admin", "volc-tenants", "--context", "admin-a", "sync-voices", volcTenantID)
	syncVolcTenant.MustSucceed(t)
	if !strings.Contains(syncVolcTenant.Stdout, `"tenant_id":"`+volcTenantID+`"`) {
		t.Fatalf("volc sync output missing canonical tenant ID:\n%s", syncVolcTenant.Stdout)
	}
}
