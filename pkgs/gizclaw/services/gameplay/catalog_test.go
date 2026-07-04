package gameplay

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestCatalogAdminCRUDAndAssets(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 5, 11, 0, 0, 0, time.UTC)
	catalog := &Catalog{
		GameRulesets: kv.NewMemory(nil),
		PetDefs:      kv.NewMemory(nil),
		BadgeDefs:    kv.NewMemory(nil),
		GameDefs:     kv.NewMemory(nil),
		Assets:       objectstore.Dir(t.TempDir()),
		Now: func() time.Time {
			return now
		},
	}

	petResp, err := catalog.CreatePetDef(ctx, adminservice.CreatePetDefRequestObject{Body: &adminservice.PetDefUpsert{
		Id:   "petdef-a",
		Spec: apitypes.PetDefSpec{DisplayName: "Pet A"},
	}})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	if pet := requireResponse[adminservice.CreatePetDef200JSONResponse](t, petResp); pet.Id != "petdef-a" {
		t.Fatalf("CreatePetDef() = %#v", pet)
	}
	putPetResp, err := catalog.PutPetDef(ctx, adminservice.PutPetDefRequestObject{
		Id:   "petdef-a",
		Body: &adminservice.PetDefUpsert{Id: "ignored", Spec: apitypes.PetDefSpec{DisplayName: "Pet A2"}},
	})
	if err != nil {
		t.Fatalf("PutPetDef() error = %v", err)
	}
	if pet := requireResponse[adminservice.PutPetDef200JSONResponse](t, putPetResp); pet.Spec.DisplayName != "Pet A2" {
		t.Fatalf("PutPetDef() = %#v", pet)
	}
	getPetResp, err := catalog.GetPetDef(ctx, adminservice.GetPetDefRequestObject{Id: "petdef-a"})
	if err != nil {
		t.Fatalf("GetPetDef() error = %v", err)
	}
	requireResponse[adminservice.GetPetDef200JSONResponse](t, getPetResp)
	listPetResp, err := catalog.ListPetDefs(ctx, adminservice.ListPetDefsRequestObject{})
	if err != nil {
		t.Fatalf("ListPetDefs() error = %v", err)
	}
	if list := requireResponse[adminservice.ListPetDefs200JSONResponse](t, listPetResp); len(list.Items) != 1 {
		t.Fatalf("ListPetDefs() = %#v", list)
	}
	assetResp, err := catalog.UploadPetDefAsset(ctx, adminservice.UploadPetDefAssetRequestObject{Id: "petdef-a", Body: bytes.NewBufferString("pet-asset")})
	if err != nil {
		t.Fatalf("UploadPetDefAsset() error = %v", err)
	}
	if pet := requireResponse[adminservice.UploadPetDefAsset200JSONResponse](t, assetResp); pet.AssetPath == nil || *pet.AssetPath == "" {
		t.Fatalf("UploadPetDefAsset() = %#v", pet)
	}
	downloadAssetResp, err := catalog.DownloadPetDefAsset(ctx, adminservice.DownloadPetDefAssetRequestObject{Id: "petdef-a"})
	if err != nil {
		t.Fatalf("DownloadPetDefAsset() error = %v", err)
	}
	asset := requireResponse[adminservice.DownloadPetDefAsset200ApplicationoctetStreamResponse](t, downloadAssetResp)
	if got := readAllString(t, asset.Body); got != "pet-asset" || asset.ContentLength != int64(len("pet-asset")) {
		t.Fatalf("DownloadPetDefAsset() body=%q len=%d", got, asset.ContentLength)
	}

	badgeResp, err := catalog.CreateBadgeDef(ctx, adminservice.CreateBadgeDefRequestObject{Body: &adminservice.BadgeDefUpsert{
		Id:   "badge-a",
		Spec: apitypes.BadgeDefSpec{DisplayName: "Badge A"},
	}})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	requireResponse[adminservice.CreateBadgeDef200JSONResponse](t, badgeResp)
	putBadgeResp, err := catalog.PutBadgeDef(ctx, adminservice.PutBadgeDefRequestObject{
		Id:   "badge-a",
		Body: &adminservice.BadgeDefUpsert{Spec: apitypes.BadgeDefSpec{DisplayName: "Badge A2"}},
	})
	if err != nil {
		t.Fatalf("PutBadgeDef() error = %v", err)
	}
	if badge := requireResponse[adminservice.PutBadgeDef200JSONResponse](t, putBadgeResp); badge.Spec.DisplayName != "Badge A2" {
		t.Fatalf("PutBadgeDef() = %#v", badge)
	}
	getBadgeResp, err := catalog.GetBadgeDef(ctx, adminservice.GetBadgeDefRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("GetBadgeDef() error = %v", err)
	}
	requireResponse[adminservice.GetBadgeDef200JSONResponse](t, getBadgeResp)
	listBadgeResp, err := catalog.ListBadgeDefs(ctx, adminservice.ListBadgeDefsRequestObject{})
	if err != nil {
		t.Fatalf("ListBadgeDefs() error = %v", err)
	}
	if list := requireResponse[adminservice.ListBadgeDefs200JSONResponse](t, listBadgeResp); len(list.Items) != 1 {
		t.Fatalf("ListBadgeDefs() = %#v", list)
	}
	iconResp, err := catalog.UploadBadgeDefIcon(ctx, adminservice.UploadBadgeDefIconRequestObject{Id: "badge-a", Body: bytes.NewBufferString("badge-icon")})
	if err != nil {
		t.Fatalf("UploadBadgeDefIcon() error = %v", err)
	}
	if badge := requireResponse[adminservice.UploadBadgeDefIcon200JSONResponse](t, iconResp); badge.IconPath == nil || *badge.IconPath == "" {
		t.Fatalf("UploadBadgeDefIcon() = %#v", badge)
	}
	downloadIconResp, err := catalog.DownloadBadgeDefIcon(ctx, adminservice.DownloadBadgeDefIconRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("DownloadBadgeDefIcon() error = %v", err)
	}
	icon := requireResponse[adminservice.DownloadBadgeDefIcon200ApplicationoctetStreamResponse](t, downloadIconResp)
	if got := readAllString(t, icon.Body); got != "badge-icon" || icon.ContentLength != int64(len("badge-icon")) {
		t.Fatalf("DownloadBadgeDefIcon() body=%q len=%d", got, icon.ContentLength)
	}

	gameResp, err := catalog.CreateGameDef(ctx, adminservice.CreateGameDefRequestObject{Body: &adminservice.GameDefUpsert{
		Id:   "game-a",
		Spec: apitypes.GameDefSpec{DisplayName: "Game A"},
	}})
	if err != nil {
		t.Fatalf("CreateGameDef() error = %v", err)
	}
	requireResponse[adminservice.CreateGameDef200JSONResponse](t, gameResp)
	putGameResp, err := catalog.PutGameDef(ctx, adminservice.PutGameDefRequestObject{
		Id:   "game-a",
		Body: &adminservice.GameDefUpsert{Spec: apitypes.GameDefSpec{DisplayName: "Game A2"}},
	})
	if err != nil {
		t.Fatalf("PutGameDef() error = %v", err)
	}
	if game := requireResponse[adminservice.PutGameDef200JSONResponse](t, putGameResp); game.Spec.DisplayName != "Game A2" {
		t.Fatalf("PutGameDef() = %#v", game)
	}
	getGameResp, err := catalog.GetGameDef(ctx, adminservice.GetGameDefRequestObject{Id: "game-a"})
	if err != nil {
		t.Fatalf("GetGameDef() error = %v", err)
	}
	requireResponse[adminservice.GetGameDef200JSONResponse](t, getGameResp)
	listGameResp, err := catalog.ListGameDefs(ctx, adminservice.ListGameDefsRequestObject{})
	if err != nil {
		t.Fatalf("ListGameDefs() error = %v", err)
	}
	if list := requireResponse[adminservice.ListGameDefs200JSONResponse](t, listGameResp); len(list.Items) != 1 {
		t.Fatalf("ListGameDefs() = %#v", list)
	}

	rulesetResp, err := catalog.CreateGameRuleset(ctx, adminservice.CreateGameRulesetRequestObject{Body: &adminservice.GameRulesetUpsert{
		Name: "ruleset-a",
		Spec: apitypes.GameRulesetSpec{
			Enabled: true,
			PetPool: []apitypes.GameRulesetPetPoolEntry{{
				PetdefId: "petdef-a",
				Weight:   1,
			}},
		},
	}})
	if err != nil {
		t.Fatalf("CreateGameRuleset() error = %v", err)
	}
	requireResponse[adminservice.CreateGameRuleset200JSONResponse](t, rulesetResp)
	putRulesetResp, err := catalog.PutGameRuleset(ctx, adminservice.PutGameRulesetRequestObject{
		Name: "ruleset-a",
		Body: &adminservice.GameRulesetUpsert{Spec: apitypes.GameRulesetSpec{
			Enabled: false,
			PetPool: []apitypes.GameRulesetPetPoolEntry{{
				PetdefId: "petdef-a",
				Weight:   2,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("PutGameRuleset() error = %v", err)
	}
	if ruleset := requireResponse[adminservice.PutGameRuleset200JSONResponse](t, putRulesetResp); ruleset.Spec.Enabled {
		t.Fatalf("PutGameRuleset() = %#v", ruleset)
	}
	getRulesetResp, err := catalog.GetGameRuleset(ctx, adminservice.GetGameRulesetRequestObject{Name: "ruleset-a"})
	if err != nil {
		t.Fatalf("GetGameRuleset() error = %v", err)
	}
	requireResponse[adminservice.GetGameRuleset200JSONResponse](t, getRulesetResp)
	listRulesetsResp, err := catalog.ListGameRulesets(ctx, adminservice.ListGameRulesetsRequestObject{})
	if err != nil {
		t.Fatalf("ListGameRulesets() error = %v", err)
	}
	if list := requireResponse[adminservice.ListGameRulesets200JSONResponse](t, listRulesetsResp); len(list.Items) != 1 {
		t.Fatalf("ListGameRulesets() = %#v", list)
	}

	deleteRulesetResp, err := catalog.DeleteGameRuleset(ctx, adminservice.DeleteGameRulesetRequestObject{Name: "ruleset-a"})
	if err != nil {
		t.Fatalf("DeleteGameRuleset() error = %v", err)
	}
	requireResponse[adminservice.DeleteGameRuleset200JSONResponse](t, deleteRulesetResp)
	deleteGameResp, err := catalog.DeleteGameDef(ctx, adminservice.DeleteGameDefRequestObject{Id: "game-a"})
	if err != nil {
		t.Fatalf("DeleteGameDef() error = %v", err)
	}
	requireResponse[adminservice.DeleteGameDef200JSONResponse](t, deleteGameResp)
	deleteBadgeResp, err := catalog.DeleteBadgeDef(ctx, adminservice.DeleteBadgeDefRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("DeleteBadgeDef() error = %v", err)
	}
	requireResponse[adminservice.DeleteBadgeDef200JSONResponse](t, deleteBadgeResp)
	deletePetResp, err := catalog.DeletePetDef(ctx, adminservice.DeletePetDefRequestObject{Id: "petdef-a"})
	if err != nil {
		t.Fatalf("DeletePetDef() error = %v", err)
	}
	requireResponse[adminservice.DeletePetDef200JSONResponse](t, deletePetResp)
}

func TestCatalogAdminErrorsAndPagination(t *testing.T) {
	ctx := context.Background()
	catalog := &Catalog{
		GameRulesets: kv.NewMemory(nil),
		PetDefs:      kv.NewMemory(nil),
		BadgeDefs:    kv.NewMemory(nil),
		GameDefs:     kv.NewMemory(nil),
	}

	petMissingResp, err := catalog.GetPetDef(ctx, adminservice.GetPetDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("GetPetDef() error = %v", err)
	}
	requireResponse[adminservice.GetPetDef404JSONResponse](t, petMissingResp)
	createPetMissingBodyResp, err := catalog.CreatePetDef(ctx, adminservice.CreatePetDefRequestObject{})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	requireResponse[adminservice.CreatePetDef400JSONResponse](t, createPetMissingBodyResp)
	createPetInvalidResp, err := catalog.CreatePetDef(ctx, adminservice.CreatePetDefRequestObject{Body: &adminservice.PetDefUpsert{Id: "bad"}})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	requireResponse[adminservice.CreatePetDef400JSONResponse](t, createPetInvalidResp)

	createPet := func(id string) {
		t.Helper()
		resp, err := catalog.CreatePetDef(ctx, adminservice.CreatePetDefRequestObject{Body: &adminservice.PetDefUpsert{
			Id:   id,
			Spec: apitypes.PetDefSpec{DisplayName: id},
		}})
		if err != nil {
			t.Fatalf("CreatePetDef(%q) error = %v", id, err)
		}
		requireResponse[adminservice.CreatePetDef200JSONResponse](t, resp)
	}
	createPet("pet-a")
	createPet("pet-b")
	duplicatePetResp, err := catalog.CreatePetDef(ctx, adminservice.CreatePetDefRequestObject{Body: &adminservice.PetDefUpsert{
		Id:   "pet-a",
		Spec: apitypes.PetDefSpec{DisplayName: "again"},
	}})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	requireResponse[adminservice.CreatePetDef409JSONResponse](t, duplicatePetResp)

	limit := int32(1)
	firstPageResp, err := catalog.ListPetDefs(ctx, adminservice.ListPetDefsRequestObject{Params: adminservice.ListPetDefsParams{Limit: &limit}})
	if err != nil {
		t.Fatalf("ListPetDefs() error = %v", err)
	}
	firstPage := requireResponse[adminservice.ListPetDefs200JSONResponse](t, firstPageResp)
	if len(firstPage.Items) != 1 || !firstPage.HasNext || firstPage.NextCursor == nil {
		t.Fatalf("first page = %#v", firstPage)
	}
	secondPageResp, err := catalog.ListPetDefs(ctx, adminservice.ListPetDefsRequestObject{Params: adminservice.ListPetDefsParams{Limit: &limit, Cursor: firstPage.NextCursor}})
	if err != nil {
		t.Fatalf("ListPetDefs() second page error = %v", err)
	}
	secondPage := requireResponse[adminservice.ListPetDefs200JSONResponse](t, secondPageResp)
	if len(secondPage.Items) != 1 || secondPage.HasNext {
		t.Fatalf("second page = %#v", secondPage)
	}

	downloadPetAssetResp, err := catalog.DownloadPetDefAsset(ctx, adminservice.DownloadPetDefAssetRequestObject{Id: "pet-a"})
	if err != nil {
		t.Fatalf("DownloadPetDefAsset() error = %v", err)
	}
	requireResponse[adminservice.DownloadPetDefAsset404JSONResponse](t, downloadPetAssetResp)
	putPetMissingBodyResp, err := catalog.PutPetDef(ctx, adminservice.PutPetDefRequestObject{Id: "pet-a"})
	if err != nil {
		t.Fatalf("PutPetDef() error = %v", err)
	}
	requireResponse[adminservice.PutPetDef400JSONResponse](t, putPetMissingBodyResp)
	deletePetMissingResp, err := catalog.DeletePetDef(ctx, adminservice.DeletePetDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("DeletePetDef() error = %v", err)
	}
	requireResponse[adminservice.DeletePetDef404JSONResponse](t, deletePetMissingResp)
	uploadPetAssetResp, err := catalog.UploadPetDefAsset(ctx, adminservice.UploadPetDefAssetRequestObject{Id: "pet-a"})
	if err != nil {
		t.Fatalf("UploadPetDefAsset() error = %v", err)
	}
	requireResponse[adminservice.UploadPetDefAsset500JSONResponse](t, uploadPetAssetResp)

	badgeMissingResp, err := catalog.GetBadgeDef(ctx, adminservice.GetBadgeDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("GetBadgeDef() error = %v", err)
	}
	requireResponse[adminservice.GetBadgeDef404JSONResponse](t, badgeMissingResp)
	createBadgeInvalidResp, err := catalog.CreateBadgeDef(ctx, adminservice.CreateBadgeDefRequestObject{Body: &adminservice.BadgeDefUpsert{Id: "badge-a"}})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	requireResponse[adminservice.CreateBadgeDef400JSONResponse](t, createBadgeInvalidResp)
	badgeResp, err := catalog.CreateBadgeDef(ctx, adminservice.CreateBadgeDefRequestObject{Body: &adminservice.BadgeDefUpsert{
		Id:   "badge-a",
		Spec: apitypes.BadgeDefSpec{DisplayName: "Badge A"},
	}})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	requireResponse[adminservice.CreateBadgeDef200JSONResponse](t, badgeResp)
	downloadBadgeIconResp, err := catalog.DownloadBadgeDefIcon(ctx, adminservice.DownloadBadgeDefIconRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("DownloadBadgeDefIcon() error = %v", err)
	}
	requireResponse[adminservice.DownloadBadgeDefIcon404JSONResponse](t, downloadBadgeIconResp)
	putBadgeMissingBodyResp, err := catalog.PutBadgeDef(ctx, adminservice.PutBadgeDefRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("PutBadgeDef() error = %v", err)
	}
	requireResponse[adminservice.PutBadgeDef400JSONResponse](t, putBadgeMissingBodyResp)
	deleteBadgeMissingResp, err := catalog.DeleteBadgeDef(ctx, adminservice.DeleteBadgeDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("DeleteBadgeDef() error = %v", err)
	}
	requireResponse[adminservice.DeleteBadgeDef404JSONResponse](t, deleteBadgeMissingResp)
	uploadBadgeIconResp, err := catalog.UploadBadgeDefIcon(ctx, adminservice.UploadBadgeDefIconRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("UploadBadgeDefIcon() error = %v", err)
	}
	requireResponse[adminservice.UploadBadgeDefIcon500JSONResponse](t, uploadBadgeIconResp)

	gameMissingResp, err := catalog.GetGameDef(ctx, adminservice.GetGameDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("GetGameDef() error = %v", err)
	}
	requireResponse[adminservice.GetGameDef404JSONResponse](t, gameMissingResp)
	createGameInvalidResp, err := catalog.CreateGameDef(ctx, adminservice.CreateGameDefRequestObject{Body: &adminservice.GameDefUpsert{Id: "game-a"}})
	if err != nil {
		t.Fatalf("CreateGameDef() error = %v", err)
	}
	requireResponse[adminservice.CreateGameDef400JSONResponse](t, createGameInvalidResp)
	putGameMissingBodyResp, err := catalog.PutGameDef(ctx, adminservice.PutGameDefRequestObject{Id: "game-a"})
	if err != nil {
		t.Fatalf("PutGameDef() error = %v", err)
	}
	requireResponse[adminservice.PutGameDef400JSONResponse](t, putGameMissingBodyResp)
	deleteGameMissingResp, err := catalog.DeleteGameDef(ctx, adminservice.DeleteGameDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("DeleteGameDef() error = %v", err)
	}
	requireResponse[adminservice.DeleteGameDef404JSONResponse](t, deleteGameMissingResp)

	rulesetMissingResp, err := catalog.GetGameRuleset(ctx, adminservice.GetGameRulesetRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("GetGameRuleset() error = %v", err)
	}
	requireResponse[adminservice.GetGameRuleset404JSONResponse](t, rulesetMissingResp)
	createRulesetInvalidResp, err := catalog.CreateGameRuleset(ctx, adminservice.CreateGameRulesetRequestObject{Body: &adminservice.GameRulesetUpsert{Name: "ruleset-a"}})
	if err != nil {
		t.Fatalf("CreateGameRuleset() error = %v", err)
	}
	requireResponse[adminservice.CreateGameRuleset400JSONResponse](t, createRulesetInvalidResp)
	putRulesetMissingBodyResp, err := catalog.PutGameRuleset(ctx, adminservice.PutGameRulesetRequestObject{Name: "ruleset-a"})
	if err != nil {
		t.Fatalf("PutGameRuleset() error = %v", err)
	}
	requireResponse[adminservice.PutGameRuleset400JSONResponse](t, putRulesetMissingBodyResp)
	deleteRulesetMissingResp, err := catalog.DeleteGameRuleset(ctx, adminservice.DeleteGameRulesetRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("DeleteGameRuleset() error = %v", err)
	}
	requireResponse[adminservice.DeleteGameRuleset404JSONResponse](t, deleteRulesetMissingResp)

	missingStoreResp, err := (&Catalog{}).ListPetDefs(ctx, adminservice.ListPetDefsRequestObject{})
	if err != nil {
		t.Fatalf("ListPetDefs() error = %v", err)
	}
	requireResponse[adminservice.ListPetDefs500JSONResponse](t, missingStoreResp)
}

func requireResponse[T any](t *testing.T, value any) T {
	t.Helper()
	resp, ok := value.(T)
	if !ok {
		t.Fatalf("response = %#v, want %T", value, *new(T))
	}
	return resp
}

func readAllString(t *testing.T, reader io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}
