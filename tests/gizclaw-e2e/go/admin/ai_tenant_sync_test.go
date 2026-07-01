//go:build gizclaw_e2e

package admin_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPISyncVolcTenantVoicesForWorkspaceUse(t *testing.T) {
	env := newAdminAPIHarness(t)

	resp, err := env.api.SyncVolcTenantVoicesWithResponse(env.ctx, "volc-main")
	if err != nil {
		t.Fatalf("sync Volc tenant voices: %v", err)
	}
	if resp.StatusCode() == 404 {
		t.Skip("volc-main tenant is not configured in this e2e environment")
	}
	requireStatusOK(t, resp, resp.Body)
	if resp.JSON200 == nil || resp.JSON200.TenantName != "volc-main" || resp.JSON200.SyncedAt.IsZero() {
		t.Fatalf("sync Volc tenant voices = %#v", resp.JSON200)
	}

	for _, voiceID := range []string{
		"volc-tenant:volc-main:zh_female_vv_mars_bigtts",
		"volc-tenant:volc-main:zh_female_shaoergushi_mars_bigtts",
		"volc-tenant:volc-main:zh_male_sunwukong_mars_bigtts",
		"volc-tenant:volc-main:zh_male_tangseng_mars_bigtts",
		"volc-tenant:volc-main:zh_male_zhubajie_mars_bigtts",
		"volc-tenant:volc-main:ICL_zh_female_bingjiao3_tob",
	} {
		get, err := env.api.GetVoiceWithResponse(env.ctx, voiceID)
		if err != nil {
			t.Fatalf("get synced Volc voice %q: %v", voiceID, err)
		}
		requireStatusOK(t, get, get.Body)
		if get.JSON200 == nil || get.JSON200.Id != voiceID || get.JSON200.Source != apitypes.VoiceSourceSync {
			t.Fatalf("synced Volc voice %q = %#v", voiceID, get.JSON200)
		}
	}
}

func TestAdminAPISyncMiniMaxTenantVoices(t *testing.T) {
	env := newAdminAPIHarness(t)

	tenantName := findRealMiniMaxTenant(t, env)
	if tenantName == "" {
		t.Skip("no real MiniMax tenant is configured in this e2e environment")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	resp, err := env.api.SyncMiniMaxTenantVoicesWithResponse(ctx, tenantName)
	if err != nil {
		t.Fatalf("sync MiniMax tenant voices: %v", err)
	}
	requireStatusOK(t, resp, resp.Body)
	if resp.JSON200 == nil || resp.JSON200.TenantName != tenantName || resp.JSON200.SyncedAt.IsZero() {
		t.Fatalf("sync MiniMax tenant voices = %#v", resp.JSON200)
	}

	providerKind := adminservice.VoiceProviderKind(apitypes.VoiceProviderKindMinimaxTenant)
	source := adminservice.VoiceSource(apitypes.VoiceSourceSync)
	limit := int32(50)
	voices, err := env.api.ListVoicesWithResponse(ctx, &adminservice.ListVoicesParams{
		Limit:        &limit,
		ProviderKind: &providerKind,
		ProviderName: &tenantName,
		Source:       &source,
	})
	if err != nil {
		t.Fatalf("list synced MiniMax voices: %v", err)
	}
	requireStatusOK(t, voices, voices.Body)
	if voices.JSON200 == nil {
		t.Fatalf("list synced MiniMax voices missing JSON200")
	}
	if len(voices.JSON200.Items) == 0 && resp.JSON200.CreatedCount+resp.JSON200.UpdatedCount+resp.JSON200.DeletedCount == 0 {
		t.Fatalf("sync MiniMax tenant %q did not produce or reconcile any voices", tenantName)
	}
}

func findRealMiniMaxTenant(t *testing.T, env *adminAPIHarness) string {
	t.Helper()

	resp, err := env.api.ListMiniMaxTenantsWithResponse(env.ctx, nil)
	if err != nil {
		t.Fatalf("list MiniMax tenants: %v", err)
	}
	requireStatusOK(t, resp, resp.Body)
	if resp.JSON200 == nil {
		t.Fatalf("list MiniMax tenants missing JSON200")
	}
	for _, want := range []string{"minimax-cn", "minimax-global"} {
		for _, item := range resp.JSON200.Items {
			if strings.TrimSpace(item.Name) == want {
				return item.Name
			}
		}
	}
	return ""
}
