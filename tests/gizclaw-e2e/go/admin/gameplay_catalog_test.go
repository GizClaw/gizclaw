//go:build gizclaw_e2e

package admin_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminAPIGameplayCatalogUserStory(t *testing.T) {
	env := newIsolatedGameplayAdminAPIHarness(t)

	petID := mutationName("petdef")
	badgeID := mutationName("badgedef")
	gameID := mutationName("gamedef")
	rulesetName := mutationName("ruleset")
	t.Cleanup(func() {
		_, _ = env.api.DeleteGameRulesetWithResponse(env.ctx, rulesetName)
		_, _ = env.api.DeleteGameDefWithResponse(env.ctx, gameID)
		_, _ = env.api.DeleteBadgeDefWithResponse(env.ctx, badgeID)
		_, _ = env.api.DeletePetDefWithResponse(env.ctx, petID)
	})

	petResp, err := env.api.CreatePetDefWithResponse(env.ctx, adminservice.PetDefUpsert{
		Id: petID,
		Spec: apitypes.PetDefSpec{
			DisplayName: "Admin E2E PetDef",
			Description: ptr("created by admin gameplay catalog e2e"),
		},
	})
	if err != nil {
		t.Fatalf("create pet def: %v", err)
	}
	requireStatusOK(t, petResp, petResp.Body)
	if petResp.JSON200 == nil || petResp.JSON200.Id != petID {
		t.Fatalf("create pet def = %#v", petResp.JSON200)
	}
	assetResp, err := env.api.UploadPetDefAssetWithBodyWithResponse(env.ctx, petID, "application/octet-stream", bytes.NewBufferString("petdef-asset"))
	if err != nil {
		t.Fatalf("upload pet def asset: %v", err)
	}
	requireStatusOK(t, assetResp, assetResp.Body)
	if assetResp.JSON200 == nil || assetResp.JSON200.AssetPath == nil {
		t.Fatalf("upload pet def asset = %#v", assetResp.JSON200)
	}
	assetGet, err := env.api.DownloadPetDefAssetWithResponse(env.ctx, petID)
	if err != nil {
		t.Fatalf("download pet def asset: %v", err)
	}
	requireStatusOK(t, assetGet, assetGet.Body)
	if string(assetGet.Body) != "petdef-asset" {
		t.Fatalf("pet def asset body = %q", string(assetGet.Body))
	}

	badgeResp, err := env.api.CreateBadgeDefWithResponse(env.ctx, adminservice.BadgeDefUpsert{
		Id:   badgeID,
		Spec: apitypes.BadgeDefSpec{DisplayName: "Admin E2E BadgeDef"},
	})
	if err != nil {
		t.Fatalf("create badge def: %v", err)
	}
	requireStatusOK(t, badgeResp, badgeResp.Body)
	iconResp, err := env.api.UploadBadgeDefIconWithBodyWithResponse(env.ctx, badgeID, "application/octet-stream", bytes.NewBufferString("badge-icon"))
	if err != nil {
		t.Fatalf("upload badge def icon: %v", err)
	}
	requireStatusOK(t, iconResp, iconResp.Body)
	iconGet, err := env.api.DownloadBadgeDefIconWithResponse(env.ctx, badgeID)
	if err != nil {
		t.Fatalf("download badge def icon: %v", err)
	}
	requireStatusOK(t, iconGet, iconGet.Body)
	if string(iconGet.Body) != "badge-icon" {
		t.Fatalf("badge def icon body = %q", string(iconGet.Body))
	}

	gameResp, err := env.api.CreateGameDefWithResponse(env.ctx, adminservice.GameDefUpsert{
		Id: gameID,
		Spec: apitypes.GameDefSpec{
			DisplayName: "Admin E2E GameDef",
			Outcomes:    &[]string{"win", "lose"},
		},
	})
	if err != nil {
		t.Fatalf("create game def: %v", err)
	}
	requireStatusOK(t, gameResp, gameResp.Body)

	initialBalance := int64(25)
	adoptionCost := int64(7)
	rulesetResp, err := env.api.CreateGameRulesetWithResponse(env.ctx, adminservice.GameRulesetUpsert{
		Name: rulesetName,
		Spec: apitypes.GameRulesetSpec{
			Enabled: true,
			Points:  &apitypes.GameRulesetPointsSpec{InitialBalance: &initialBalance},
			PetPool: []apitypes.GameRulesetPetPoolEntry{{
				PetdefId:     petID,
				Weight:       1,
				Rarity:       ptr("e2e"),
				AdoptionCost: &adoptionCost,
			}},
			BadgeDefIds: &[]string{badgeID},
			GameDefIds:  &[]string{gameID},
		},
	})
	if err != nil {
		t.Fatalf("create game ruleset: %v", err)
	}
	requireStatusOK(t, rulesetResp, rulesetResp.Body)
	if rulesetResp.JSON200 == nil || rulesetResp.JSON200.Name != rulesetName {
		t.Fatalf("create game ruleset = %#v", rulesetResp.JSON200)
	}

	listResp, err := env.api.ListGameRulesetsWithResponse(env.ctx, &adminservice.ListGameRulesetsParams{Limit: ptr[int32](100)})
	if err != nil {
		t.Fatalf("list game rulesets: %v", err)
	}
	requireStatusOK(t, listResp, listResp.Body)
	requireName(t, listResp.JSON200.Items, rulesetName, func(item apitypes.GameRuleset) string { return item.Name })

	var resource apitypes.Resource
	if err := resource.FromGameRulesetResource(apitypes.GameRulesetResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.GameRulesetResourceKindGameRuleset,
		Metadata:   apitypes.ResourceMetadata{Name: rulesetName},
		Spec:       rulesetResp.JSON200.Spec,
	}); err != nil {
		t.Fatalf("build game ruleset resource: %v", err)
	}
	applied, err := env.api.ApplyResourceWithResponse(env.ctx, resource)
	if err != nil {
		t.Fatalf("apply game ruleset resource: %v", err)
	}
	requireStatusOK(t, applied, applied.Body)
	gotResource, err := env.api.GetResourceWithResponse(env.ctx, apitypes.ResourceKindGameRuleset, rulesetName)
	if err != nil {
		t.Fatalf("get game ruleset resource: %v", err)
	}
	requireStatusOK(t, gotResource, gotResource.Body)
	gotRuleset, err := gotResource.JSON200.AsGameRulesetResource()
	if err != nil {
		t.Fatalf("decode game ruleset resource: %v", err)
	}
	if gotRuleset.Metadata.Name != rulesetName || len(gotRuleset.Spec.PetPool) != 1 {
		t.Fatalf("game ruleset resource = %#v", gotRuleset)
	}
}

func newIsolatedGameplayAdminAPIHarness(t *testing.T) *adminAPIHarness {
	t.Helper()

	h := clitest.NewHarnessForRoot(t, "tests/gizclaw-e2e/go/admin", "client-admin-gameplay")
	h.StartServerFromFixture("server_config.yaml")
	h.InstallFixedAdminContext("admin-gameplay").MustSucceed(t)
	admin := h.ConnectClientFromContext("admin-gameplay")
	t.Cleanup(func() { admin.Close() })
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin API client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return &adminAPIHarness{
		ctx:      ctx,
		h:        h,
		api:      api,
		adminKey: h.ContextPublicKey("admin-gameplay"),
		adminSN:  "admin",
	}
}
