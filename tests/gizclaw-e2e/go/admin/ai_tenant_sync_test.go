//go:build gizclaw_e2e

package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminAPISyncVolcTenantVoicesForWorkspaceUse(t *testing.T) {
	env := newAdminAPIHarness(t)
	tenant, found, err := clitest.VolcTenantByID(env.ctx, env.api, "volc-main")
	if err != nil || !found {
		t.Fatalf("resolve volc-main tenant: found=%v err=%v", found, err)
	}

	resp, err := env.api.SyncVolcTenantVoicesWithResponse(env.ctx, tenant.Id)
	if err != nil {
		t.Fatalf("sync Volc tenant voices: %v", err)
	}
	if resp.StatusCode() == 404 {
		t.Fatal("volc-main tenant is not configured in this e2e environment")
	}
	requireStatusOK(t, resp, resp.Body)
	if resp.JSON200 == nil || resp.JSON200.TenantId != tenant.Id || resp.JSON200.SyncedAt.IsZero() {
		t.Fatalf("sync Volc tenant voices = %#v", resp.JSON200)
	}

	providerKind := adminhttp.VoiceProviderKind(apitypes.VoiceProviderKindVolcTenant)
	source := adminhttp.VoiceSource(apitypes.VoiceSourceSync)
	voices := collectAdminPages(t, 200, func(cursor *string, limit int32) ([]apitypes.Voice, bool, *string) {
		resp, err := env.api.ListVoicesWithResponse(env.ctx, &adminhttp.ListVoicesParams{
			Cursor: cursor, Limit: &limit, ProviderKind: &providerKind, ProviderId: &tenant.Id, Source: &source,
		})
		if err != nil {
			t.Fatalf("list synced Volc voices: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatal("list synced Volc voices missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	for _, voiceID := range []string{
		"zh_female_vv_mars_bigtts",
		"zh_female_shaoergushi_mars_bigtts",
		"zh_male_sunwukong_mars_bigtts",
		"zh_male_tangseng_mars_bigtts",
		"zh_male_zhubajie_mars_bigtts",
		"ICL_zh_female_bingjiao3_tob",
	} {
		found := false
		for _, item := range voices {
			if item.Provider.Id != tenant.Id || item.ProviderData == nil || item.Source != apitypes.VoiceSourceSync {
				continue
			}
			provider, decodeErr := item.ProviderData.AsVolcTenantVoiceProviderData()
			found = decodeErr == nil && provider.VoiceId != nil && *provider.VoiceId == voiceID
			if found {
				break
			}
		}
		if !found {
			t.Fatalf("synced Volc provider voice %q is missing", voiceID)
		}
	}
}

func TestAdminAPISyncMiniMaxTenantVoices(t *testing.T) {
	env := newAdminAPIHarness(t)

	tenant, found := findRealMiniMaxTenant(t, env)
	if !found {
		t.Fatal("no real MiniMax tenant is configured in this e2e environment")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	resp, err := env.api.SyncMiniMaxTenantVoicesWithResponse(ctx, tenant.Id)
	if err != nil {
		t.Fatalf("sync MiniMax tenant voices: %v", err)
	}
	requireStatusOK(t, resp, resp.Body)
	if resp.JSON200 == nil || resp.JSON200.TenantId != tenant.Id || resp.JSON200.SyncedAt.IsZero() {
		t.Fatalf("sync MiniMax tenant voices = %#v", resp.JSON200)
	}

	providerKind := adminhttp.VoiceProviderKind(apitypes.VoiceProviderKindMinimaxTenant)
	source := adminhttp.VoiceSource(apitypes.VoiceSourceSync)
	limit := int32(50)
	voices, err := env.api.ListVoicesWithResponse(ctx, &adminhttp.ListVoicesParams{
		Limit:        &limit,
		ProviderKind: &providerKind,
		ProviderId:   &tenant.Id,
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
		t.Fatalf("sync MiniMax tenant %q did not produce or reconcile any voices", tenant.Id)
	}
}

func findRealMiniMaxTenant(t *testing.T, env *adminAPIHarness) (apitypes.MiniMaxTenant, bool) {
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
			if item.Id == want {
				return item, true
			}
		}
	}
	return apitypes.MiniMaxTenant{}, false
}
