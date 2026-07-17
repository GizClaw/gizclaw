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
	if !strings.Contains(list.Stdout, `"id":"minimax-narrator-clone"`) || !strings.Contains(list.Stdout, `"id":"volc-tenant:volc-main:zh_female_vv_mars_bigtts"`) {
		t.Skipf("provider voice resources are not configured in this e2e environment: %s", strings.TrimSpace(list.Stdout))
	}
	for _, want := range []string{`"id":"minimax-narrator-clone"`, `"id":"volc-tenant:volc-main:zh_female_vv_mars_bigtts"`} {
		if !strings.Contains(list.Stdout, want) {
			t.Fatalf("voices list missing %q:\n%s", want, list.Stdout)
		}
	}

	filtered := h.RunCLI("admin", "voices", "list", "--provider-name", "minimax-cn", "--context", "admin-a")
	filtered.MustSucceed(t)
	if !strings.Contains(filtered.Stdout, `"id":"minimax-narrator-clone"`) || strings.Contains(filtered.Stdout, `"id":"volc-tenant:volc-main:zh_female_vv_mars_bigtts"`) {
		t.Fatalf("voices filtered list returned unexpected items:\n%s", filtered.Stdout)
	}

	get := h.RunCLI("admin", "voices", "get", "minimax-narrator-clone", "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"name":"MiniMax Narrator Clone"`) {
		t.Fatalf("voices get missing name:\n%s", get.Stdout)
	}

	showVolcVoice := h.RunCLI("admin", "--context", "admin-a", "show", "Voice", "volc-tenant:volc-main:zh_female_vv_mars_bigtts")
	showVolcVoice.MustSucceed(t)
	for _, want := range []string{`"kind":"Voice"`, `"name":"volc-tenant:volc-main:zh_female_vv_mars_bigtts"`, `"resource_id":"seed-tts-1.0"`} {
		if !strings.Contains(showVolcVoice.Stdout, want) {
			t.Fatalf("admin show Volc voice missing %q:\n%s", want, showVolcVoice.Stdout)
		}
	}

	showVolcTenant := h.RunCLI("admin", "--context", "admin-a", "show", "VolcTenant", "volc-main")
	showVolcTenant.MustSucceed(t)
	for _, want := range []string{`"kind":"VolcTenant"`, `"name":"volc-main"`, `"credential_name":"volc-main-credential"`} {
		if !strings.Contains(showVolcTenant.Stdout, want) {
			t.Fatalf("admin show VolcTenant missing %q:\n%s", want, showVolcTenant.Stdout)
		}
	}

	showVolcCredential := h.RunCLI("admin", "--context", "admin-a", "show", "Credential", "volc-main-credential")
	showVolcCredential.MustSucceed(t)
	for _, want := range []string{`"kind":"Credential"`, `"name":"volc-main-credential"`} {
		if !strings.Contains(showVolcCredential.Stdout, want) {
			t.Fatalf("admin show Volc credential missing %q:\n%s", want, showVolcCredential.Stdout)
		}
	}

	syncVolcTenant := h.RunCLI("admin", "volc-tenants", "--context", "admin-a", "sync-voices", "volc-main")
	syncVolcTenant.MustSucceed(t)
	if !strings.Contains(syncVolcTenant.Stdout, `"tenant_name":"volc-main"`) {
		t.Fatalf("volc sync output missing tenant name:\n%s", syncVolcTenant.Stdout)
	}
}
