package gameplay

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
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

	petResp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{Body: &adminhttp.PetDefUpsert{
		Id:   "petdef-a",
		Spec: apitypes.PetDefSpec{DisplayName: "Pet A"},
	}})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	if pet := requireResponse[adminhttp.CreatePetDef200JSONResponse](t, petResp); pet.Id != "petdef-a" {
		t.Fatalf("CreatePetDef() = %#v", pet)
	}
	putPetResp, err := catalog.PutPetDef(ctx, adminhttp.PutPetDefRequestObject{
		Id:   "petdef-a",
		Body: &adminhttp.PetDefUpsert{Id: "ignored", Spec: apitypes.PetDefSpec{DisplayName: "Pet A2"}},
	})
	if err != nil {
		t.Fatalf("PutPetDef() error = %v", err)
	}
	if pet := requireResponse[adminhttp.PutPetDef200JSONResponse](t, putPetResp); pet.Spec.DisplayName != "Pet A2" {
		t.Fatalf("PutPetDef() = %#v", pet)
	}
	getPetResp, err := catalog.GetPetDef(ctx, adminhttp.GetPetDefRequestObject{Id: "petdef-a"})
	if err != nil {
		t.Fatalf("GetPetDef() error = %v", err)
	}
	requireResponse[adminhttp.GetPetDef200JSONResponse](t, getPetResp)
	listPetResp, err := catalog.ListPetDefs(ctx, adminhttp.ListPetDefsRequestObject{})
	if err != nil {
		t.Fatalf("ListPetDefs() error = %v", err)
	}
	if list := requireResponse[adminhttp.ListPetDefs200JSONResponse](t, listPetResp); len(list.Items) != 1 {
		t.Fatalf("ListPetDefs() = %#v", list)
	}
	petPixa := makeTestPixa(t, []string{"idle", "feed"}, 16, 16)
	assetResp, err := catalog.UploadPetDefPixa(ctx, adminhttp.UploadPetDefPixaRequestObject{Id: "petdef-a", Body: bytes.NewReader(petPixa)})
	if err != nil {
		t.Fatalf("UploadPetDefPixa() error = %v", err)
	}
	if pet := requireResponse[adminhttp.UploadPetDefPixa200JSONResponse](t, assetResp); pet.PixaPath == nil || *pet.PixaPath == "" {
		t.Fatalf("UploadPetDefPixa() = %#v", pet)
	}
	downloadAssetResp, err := catalog.DownloadPetDefPixa(ctx, adminhttp.DownloadPetDefPixaRequestObject{Id: "petdef-a"})
	if err != nil {
		t.Fatalf("DownloadPetDefPixa() error = %v", err)
	}
	asset := requireResponse[adminhttp.DownloadPetDefPixa200ApplicationoctetStreamResponse](t, downloadAssetResp)
	if got := readAllBytes(t, asset.Body); !bytes.Equal(got, petPixa) || asset.ContentLength != int64(len(petPixa)) {
		t.Fatalf("DownloadPetDefPixa() len=%d want %d equal=%v", asset.ContentLength, len(petPixa), bytes.Equal(got, petPixa))
	}

	badgeResp, err := catalog.CreateBadgeDef(ctx, adminhttp.CreateBadgeDefRequestObject{Body: &adminhttp.BadgeDefUpsert{
		Id:   "badge-a",
		Spec: apitypes.BadgeDefSpec{DisplayName: "Badge A"},
	}})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateBadgeDef200JSONResponse](t, badgeResp)
	putBadgeResp, err := catalog.PutBadgeDef(ctx, adminhttp.PutBadgeDefRequestObject{
		Id:   "badge-a",
		Body: &adminhttp.BadgeDefUpsert{Spec: apitypes.BadgeDefSpec{DisplayName: "Badge A2"}},
	})
	if err != nil {
		t.Fatalf("PutBadgeDef() error = %v", err)
	}
	if badge := requireResponse[adminhttp.PutBadgeDef200JSONResponse](t, putBadgeResp); badge.Spec.DisplayName != "Badge A2" {
		t.Fatalf("PutBadgeDef() = %#v", badge)
	}
	getBadgeResp, err := catalog.GetBadgeDef(ctx, adminhttp.GetBadgeDefRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("GetBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.GetBadgeDef200JSONResponse](t, getBadgeResp)
	listBadgeResp, err := catalog.ListBadgeDefs(ctx, adminhttp.ListBadgeDefsRequestObject{})
	if err != nil {
		t.Fatalf("ListBadgeDefs() error = %v", err)
	}
	if list := requireResponse[adminhttp.ListBadgeDefs200JSONResponse](t, listBadgeResp); len(list.Items) != 1 {
		t.Fatalf("ListBadgeDefs() = %#v", list)
	}
	badgePixa := makeTestPixa(t, []string{"icon"}, 16, 16)
	iconResp, err := catalog.UploadBadgeDefPixa(ctx, adminhttp.UploadBadgeDefPixaRequestObject{Id: "badge-a", Body: bytes.NewReader(badgePixa)})
	if err != nil {
		t.Fatalf("UploadBadgeDefPixa() error = %v", err)
	}
	if badge := requireResponse[adminhttp.UploadBadgeDefPixa200JSONResponse](t, iconResp); badge.PixaPath == nil || *badge.PixaPath == "" {
		t.Fatalf("UploadBadgeDefPixa() = %#v", badge)
	}
	downloadIconResp, err := catalog.DownloadBadgeDefPixa(ctx, adminhttp.DownloadBadgeDefPixaRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("DownloadBadgeDefPixa() error = %v", err)
	}
	icon := requireResponse[adminhttp.DownloadBadgeDefPixa200ApplicationoctetStreamResponse](t, downloadIconResp)
	if got := readAllBytes(t, icon.Body); !bytes.Equal(got, badgePixa) || icon.ContentLength != int64(len(badgePixa)) {
		t.Fatalf("DownloadBadgeDefPixa() len=%d want %d equal=%v", icon.ContentLength, len(badgePixa), bytes.Equal(got, badgePixa))
	}

	gameResp, err := catalog.CreateGameDef(ctx, adminhttp.CreateGameDefRequestObject{Body: &adminhttp.GameDefUpsert{
		Id:   "game-a",
		Spec: apitypes.GameDefSpec{DisplayName: "Game A"},
	}})
	if err != nil {
		t.Fatalf("CreateGameDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateGameDef200JSONResponse](t, gameResp)
	putGameResp, err := catalog.PutGameDef(ctx, adminhttp.PutGameDefRequestObject{
		Id:   "game-a",
		Body: &adminhttp.GameDefUpsert{Spec: apitypes.GameDefSpec{DisplayName: "Game A2"}},
	})
	if err != nil {
		t.Fatalf("PutGameDef() error = %v", err)
	}
	if game := requireResponse[adminhttp.PutGameDef200JSONResponse](t, putGameResp); game.Spec.DisplayName != "Game A2" {
		t.Fatalf("PutGameDef() = %#v", game)
	}
	getGameResp, err := catalog.GetGameDef(ctx, adminhttp.GetGameDefRequestObject{Id: "game-a"})
	if err != nil {
		t.Fatalf("GetGameDef() error = %v", err)
	}
	requireResponse[adminhttp.GetGameDef200JSONResponse](t, getGameResp)
	listGameResp, err := catalog.ListGameDefs(ctx, adminhttp.ListGameDefsRequestObject{})
	if err != nil {
		t.Fatalf("ListGameDefs() error = %v", err)
	}
	if list := requireResponse[adminhttp.ListGameDefs200JSONResponse](t, listGameResp); len(list.Items) != 1 {
		t.Fatalf("ListGameDefs() = %#v", list)
	}

	rulesetResp, err := catalog.CreateGameRuleset(ctx, adminhttp.CreateGameRulesetRequestObject{Body: &adminhttp.GameRulesetUpsert{
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
	requireResponse[adminhttp.CreateGameRuleset200JSONResponse](t, rulesetResp)
	putRulesetResp, err := catalog.PutGameRuleset(ctx, adminhttp.PutGameRulesetRequestObject{
		Name: "ruleset-a",
		Body: &adminhttp.GameRulesetUpsert{Spec: apitypes.GameRulesetSpec{
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
	if ruleset := requireResponse[adminhttp.PutGameRuleset200JSONResponse](t, putRulesetResp); ruleset.Spec.Enabled {
		t.Fatalf("PutGameRuleset() = %#v", ruleset)
	}
	getRulesetResp, err := catalog.GetGameRuleset(ctx, adminhttp.GetGameRulesetRequestObject{Name: "ruleset-a"})
	if err != nil {
		t.Fatalf("GetGameRuleset() error = %v", err)
	}
	requireResponse[adminhttp.GetGameRuleset200JSONResponse](t, getRulesetResp)
	listRulesetsResp, err := catalog.ListGameRulesets(ctx, adminhttp.ListGameRulesetsRequestObject{})
	if err != nil {
		t.Fatalf("ListGameRulesets() error = %v", err)
	}
	if list := requireResponse[adminhttp.ListGameRulesets200JSONResponse](t, listRulesetsResp); len(list.Items) != 1 {
		t.Fatalf("ListGameRulesets() = %#v", list)
	}

	deleteRulesetResp, err := catalog.DeleteGameRuleset(ctx, adminhttp.DeleteGameRulesetRequestObject{Name: "ruleset-a"})
	if err != nil {
		t.Fatalf("DeleteGameRuleset() error = %v", err)
	}
	requireResponse[adminhttp.DeleteGameRuleset200JSONResponse](t, deleteRulesetResp)
	deleteGameResp, err := catalog.DeleteGameDef(ctx, adminhttp.DeleteGameDefRequestObject{Id: "game-a"})
	if err != nil {
		t.Fatalf("DeleteGameDef() error = %v", err)
	}
	requireResponse[adminhttp.DeleteGameDef200JSONResponse](t, deleteGameResp)
	deleteBadgeResp, err := catalog.DeleteBadgeDef(ctx, adminhttp.DeleteBadgeDefRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("DeleteBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.DeleteBadgeDef200JSONResponse](t, deleteBadgeResp)
	deletePetResp, err := catalog.DeletePetDef(ctx, adminhttp.DeletePetDefRequestObject{Id: "petdef-a"})
	if err != nil {
		t.Fatalf("DeletePetDef() error = %v", err)
	}
	requireResponse[adminhttp.DeletePetDef200JSONResponse](t, deletePetResp)
}

func TestCatalogAdminErrorsAndPagination(t *testing.T) {
	ctx := context.Background()
	catalog := &Catalog{
		GameRulesets: kv.NewMemory(nil),
		PetDefs:      kv.NewMemory(nil),
		BadgeDefs:    kv.NewMemory(nil),
		GameDefs:     kv.NewMemory(nil),
	}

	petMissingResp, err := catalog.GetPetDef(ctx, adminhttp.GetPetDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("GetPetDef() error = %v", err)
	}
	requireResponse[adminhttp.GetPetDef404JSONResponse](t, petMissingResp)
	createPetMissingBodyResp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	requireResponse[adminhttp.CreatePetDef400JSONResponse](t, createPetMissingBodyResp)
	createPetInvalidResp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{Body: &adminhttp.PetDefUpsert{Id: "bad"}})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	requireResponse[adminhttp.CreatePetDef400JSONResponse](t, createPetInvalidResp)

	createPet := func(id string) {
		t.Helper()
		resp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{Body: &adminhttp.PetDefUpsert{
			Id:   id,
			Spec: apitypes.PetDefSpec{DisplayName: id},
		}})
		if err != nil {
			t.Fatalf("CreatePetDef(%q) error = %v", id, err)
		}
		requireResponse[adminhttp.CreatePetDef200JSONResponse](t, resp)
	}
	createPet("pet-a")
	createPet("pet-b")
	duplicatePetResp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{Body: &adminhttp.PetDefUpsert{
		Id:   "pet-a",
		Spec: apitypes.PetDefSpec{DisplayName: "again"},
	}})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	requireResponse[adminhttp.CreatePetDef409JSONResponse](t, duplicatePetResp)

	limit := int32(1)
	firstPageResp, err := catalog.ListPetDefs(ctx, adminhttp.ListPetDefsRequestObject{Params: adminhttp.ListPetDefsParams{Limit: &limit}})
	if err != nil {
		t.Fatalf("ListPetDefs() error = %v", err)
	}
	firstPage := requireResponse[adminhttp.ListPetDefs200JSONResponse](t, firstPageResp)
	if len(firstPage.Items) != 1 || !firstPage.HasNext || firstPage.NextCursor == nil {
		t.Fatalf("first page = %#v", firstPage)
	}
	secondPageResp, err := catalog.ListPetDefs(ctx, adminhttp.ListPetDefsRequestObject{Params: adminhttp.ListPetDefsParams{Limit: &limit, Cursor: firstPage.NextCursor}})
	if err != nil {
		t.Fatalf("ListPetDefs() second page error = %v", err)
	}
	secondPage := requireResponse[adminhttp.ListPetDefs200JSONResponse](t, secondPageResp)
	if len(secondPage.Items) != 1 || secondPage.HasNext {
		t.Fatalf("second page = %#v", secondPage)
	}

	downloadPetAssetResp, err := catalog.DownloadPetDefPixa(ctx, adminhttp.DownloadPetDefPixaRequestObject{Id: "pet-a"})
	if err != nil {
		t.Fatalf("DownloadPetDefPixa() error = %v", err)
	}
	requireResponse[adminhttp.DownloadPetDefPixa404JSONResponse](t, downloadPetAssetResp)
	putPetMissingBodyResp, err := catalog.PutPetDef(ctx, adminhttp.PutPetDefRequestObject{Id: "pet-a"})
	if err != nil {
		t.Fatalf("PutPetDef() error = %v", err)
	}
	requireResponse[adminhttp.PutPetDef400JSONResponse](t, putPetMissingBodyResp)
	deletePetMissingResp, err := catalog.DeletePetDef(ctx, adminhttp.DeletePetDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("DeletePetDef() error = %v", err)
	}
	requireResponse[adminhttp.DeletePetDef404JSONResponse](t, deletePetMissingResp)
	uploadPetAssetResp, err := catalog.UploadPetDefPixa(ctx, adminhttp.UploadPetDefPixaRequestObject{Id: "pet-a"})
	if err != nil {
		t.Fatalf("UploadPetDefPixa() error = %v", err)
	}
	requireResponse[adminhttp.UploadPetDefPixa500JSONResponse](t, uploadPetAssetResp)
	invalidPetPixaResp, err := catalog.UploadPetDefPixa(ctx, adminhttp.UploadPetDefPixaRequestObject{Id: "pet-a", Body: bytes.NewBufferString("not-pixa")})
	if err != nil {
		t.Fatalf("UploadPetDefPixa() invalid error = %v", err)
	}
	requireResponse[adminhttp.UploadPetDefPixa500JSONResponse](t, invalidPetPixaResp)

	badgeMissingResp, err := catalog.GetBadgeDef(ctx, adminhttp.GetBadgeDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("GetBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.GetBadgeDef404JSONResponse](t, badgeMissingResp)
	createBadgeInvalidResp, err := catalog.CreateBadgeDef(ctx, adminhttp.CreateBadgeDefRequestObject{Body: &adminhttp.BadgeDefUpsert{Id: "badge-a"}})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateBadgeDef400JSONResponse](t, createBadgeInvalidResp)
	badgeResp, err := catalog.CreateBadgeDef(ctx, adminhttp.CreateBadgeDefRequestObject{Body: &adminhttp.BadgeDefUpsert{
		Id:   "badge-a",
		Spec: apitypes.BadgeDefSpec{DisplayName: "Badge A"},
	}})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateBadgeDef200JSONResponse](t, badgeResp)
	downloadBadgeIconResp, err := catalog.DownloadBadgeDefPixa(ctx, adminhttp.DownloadBadgeDefPixaRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("DownloadBadgeDefPixa() error = %v", err)
	}
	requireResponse[adminhttp.DownloadBadgeDefPixa404JSONResponse](t, downloadBadgeIconResp)
	putBadgeMissingBodyResp, err := catalog.PutBadgeDef(ctx, adminhttp.PutBadgeDefRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("PutBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.PutBadgeDef400JSONResponse](t, putBadgeMissingBodyResp)
	deleteBadgeMissingResp, err := catalog.DeleteBadgeDef(ctx, adminhttp.DeleteBadgeDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("DeleteBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.DeleteBadgeDef404JSONResponse](t, deleteBadgeMissingResp)
	uploadBadgeIconResp, err := catalog.UploadBadgeDefPixa(ctx, adminhttp.UploadBadgeDefPixaRequestObject{Id: "badge-a"})
	if err != nil {
		t.Fatalf("UploadBadgeDefPixa() error = %v", err)
	}
	requireResponse[adminhttp.UploadBadgeDefPixa500JSONResponse](t, uploadBadgeIconResp)
	invalidBadgePixaResp, err := catalog.UploadBadgeDefPixa(ctx, adminhttp.UploadBadgeDefPixaRequestObject{Id: "badge-a", Body: bytes.NewReader(makeTestPixa(t, []string{"idle"}, 16, 16))})
	if err != nil {
		t.Fatalf("UploadBadgeDefPixa() invalid error = %v", err)
	}
	requireResponse[adminhttp.UploadBadgeDefPixa500JSONResponse](t, invalidBadgePixaResp)

	gameMissingResp, err := catalog.GetGameDef(ctx, adminhttp.GetGameDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("GetGameDef() error = %v", err)
	}
	requireResponse[adminhttp.GetGameDef404JSONResponse](t, gameMissingResp)
	createGameInvalidResp, err := catalog.CreateGameDef(ctx, adminhttp.CreateGameDefRequestObject{Body: &adminhttp.GameDefUpsert{Id: "game-a"}})
	if err != nil {
		t.Fatalf("CreateGameDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateGameDef400JSONResponse](t, createGameInvalidResp)
	putGameMissingBodyResp, err := catalog.PutGameDef(ctx, adminhttp.PutGameDefRequestObject{Id: "game-a"})
	if err != nil {
		t.Fatalf("PutGameDef() error = %v", err)
	}
	requireResponse[adminhttp.PutGameDef400JSONResponse](t, putGameMissingBodyResp)
	deleteGameMissingResp, err := catalog.DeleteGameDef(ctx, adminhttp.DeleteGameDefRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("DeleteGameDef() error = %v", err)
	}
	requireResponse[adminhttp.DeleteGameDef404JSONResponse](t, deleteGameMissingResp)

	rulesetMissingResp, err := catalog.GetGameRuleset(ctx, adminhttp.GetGameRulesetRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("GetGameRuleset() error = %v", err)
	}
	requireResponse[adminhttp.GetGameRuleset404JSONResponse](t, rulesetMissingResp)
	createRulesetInvalidResp, err := catalog.CreateGameRuleset(ctx, adminhttp.CreateGameRulesetRequestObject{Body: &adminhttp.GameRulesetUpsert{Name: "ruleset-a"}})
	if err != nil {
		t.Fatalf("CreateGameRuleset() error = %v", err)
	}
	requireResponse[adminhttp.CreateGameRuleset400JSONResponse](t, createRulesetInvalidResp)
	putRulesetMissingBodyResp, err := catalog.PutGameRuleset(ctx, adminhttp.PutGameRulesetRequestObject{Name: "ruleset-a"})
	if err != nil {
		t.Fatalf("PutGameRuleset() error = %v", err)
	}
	requireResponse[adminhttp.PutGameRuleset400JSONResponse](t, putRulesetMissingBodyResp)
	deleteRulesetMissingResp, err := catalog.DeleteGameRuleset(ctx, adminhttp.DeleteGameRulesetRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("DeleteGameRuleset() error = %v", err)
	}
	requireResponse[adminhttp.DeleteGameRuleset404JSONResponse](t, deleteRulesetMissingResp)

	missingStoreResp, err := (&Catalog{}).ListPetDefs(ctx, adminhttp.ListPetDefsRequestObject{})
	if err != nil {
		t.Fatalf("ListPetDefs() error = %v", err)
	}
	requireResponse[adminhttp.ListPetDefs500JSONResponse](t, missingStoreResp)
}

func requireResponse[T any](t *testing.T, value any) T {
	t.Helper()
	resp, ok := value.(T)
	if !ok {
		t.Fatalf("response = %#v, want %T", value, *new(T))
	}
	return resp
}

func readAllBytes(t *testing.T, reader io.Reader) []byte {
	t.Helper()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return data
}

func makeTestPixa(t *testing.T, clips []string, width uint16, height uint16) []byte {
	t.Helper()
	if len(clips) == 0 {
		t.Fatal("makeTestPixa requires at least one clip")
	}
	const (
		headerSize     = 40
		clipEntrySize  = 56
		frameEntrySize = 16
	)
	paletteOffset := headerSize
	clipOffset := paletteOffset + 2
	frameOffset := clipOffset + len(clips)*clipEntrySize
	payload := []byte{0x00, 0xf8, 0xe0, 0x07}
	payloadOffset := frameOffset + frameEntrySize
	data := make([]byte, payloadOffset+len(payload))
	copy(data[:4], "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], width)
	binary.LittleEndian.PutUint16(data[10:12], height)
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
		copy(data[base:base+pixaClipNameSize], []byte(clip))
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
