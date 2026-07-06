//go:build gizclaw_e2e

package admin_test

import (
	"bytes"
	"context"
	"encoding/binary"
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
	petPixa := makeGameplayCatalogTestPixa(t, []string{"idle", "feed"})
	assetResp, err := env.api.UploadPetDefPixaWithBodyWithResponse(env.ctx, petID, "application/octet-stream", bytes.NewReader(petPixa))
	if err != nil {
		t.Fatalf("upload pet def pixa: %v", err)
	}
	requireStatusOK(t, assetResp, assetResp.Body)
	if assetResp.JSON200 == nil || assetResp.JSON200.PixaPath == nil {
		t.Fatalf("upload pet def pixa = %#v", assetResp.JSON200)
	}
	assetGet, err := env.api.DownloadPetDefPixaWithResponse(env.ctx, petID)
	if err != nil {
		t.Fatalf("download pet def pixa: %v", err)
	}
	requireStatusOK(t, assetGet, assetGet.Body)
	if !bytes.Equal(assetGet.Body, petPixa) {
		t.Fatalf("pet def pixa body len = %d want %d", len(assetGet.Body), len(petPixa))
	}

	badgeResp, err := env.api.CreateBadgeDefWithResponse(env.ctx, adminservice.BadgeDefUpsert{
		Id:   badgeID,
		Spec: apitypes.BadgeDefSpec{DisplayName: "Admin E2E BadgeDef"},
	})
	if err != nil {
		t.Fatalf("create badge def: %v", err)
	}
	requireStatusOK(t, badgeResp, badgeResp.Body)
	badgePixa := makeGameplayCatalogTestPixa(t, []string{"icon"})
	iconResp, err := env.api.UploadBadgeDefPixaWithBodyWithResponse(env.ctx, badgeID, "application/octet-stream", bytes.NewReader(badgePixa))
	if err != nil {
		t.Fatalf("upload badge def pixa: %v", err)
	}
	requireStatusOK(t, iconResp, iconResp.Body)
	iconGet, err := env.api.DownloadBadgeDefPixaWithResponse(env.ctx, badgeID)
	if err != nil {
		t.Fatalf("download badge def pixa: %v", err)
	}
	requireStatusOK(t, iconGet, iconGet.Body)
	if !bytes.Equal(iconGet.Body, badgePixa) {
		t.Fatalf("badge def pixa body len = %d want %d", len(iconGet.Body), len(badgePixa))
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

func makeGameplayCatalogTestPixa(t *testing.T, clips []string) []byte {
	t.Helper()
	if len(clips) == 0 {
		t.Fatal("makeGameplayCatalogTestPixa requires at least one clip")
	}
	const (
		headerSize       = 40
		clipEntrySize    = 56
		frameEntrySize   = 16
		clipNameSize     = 32
		paletteByteCount = 2
	)
	paletteOffset := headerSize
	clipOffset := paletteOffset + paletteByteCount
	frameOffset := clipOffset + len(clips)*clipEntrySize
	payload := []byte{0x00, 0xf8, 0xe0, 0x07}
	payloadOffset := frameOffset + frameEntrySize
	data := make([]byte, payloadOffset+len(payload))
	copy(data[:4], "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], 16)
	binary.LittleEndian.PutUint16(data[10:12], 16)
	binary.LittleEndian.PutUint16(data[12:14], 1)
	binary.LittleEndian.PutUint16(data[14:16], uint16(len(clips)))
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[20:24], uint32(paletteOffset))
	binary.LittleEndian.PutUint32(data[24:28], uint32(clipOffset))
	binary.LittleEndian.PutUint32(data[28:32], uint32(frameOffset))
	binary.LittleEndian.PutUint32(data[32:36], uint32(payloadOffset))
	binary.LittleEndian.PutUint32(data[36:40], uint32(len(payload)))
	for i, clip := range clips {
		base := clipOffset + i*clipEntrySize
		copy(data[base:base+clipNameSize], []byte(clip))
		binary.LittleEndian.PutUint32(data[base+36:base+40], 0)
		binary.LittleEndian.PutUint32(data[base+40:base+44], 1)
		binary.LittleEndian.PutUint32(data[base+44:base+48], 120)
		binary.LittleEndian.PutUint16(data[base+48:base+50], 1)
	}
	binary.LittleEndian.PutUint16(data[frameOffset:frameOffset+2], 120)
	data[frameOffset+2] = 0
	binary.LittleEndian.PutUint32(data[frameOffset+4:frameOffset+8], 0)
	binary.LittleEndian.PutUint32(data[frameOffset+8:frameOffset+12], uint32(len(payload)))
	copy(data[payloadOffset:], payload)
	return data
}
