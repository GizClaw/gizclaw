package gameplay

import (
	"context"
	"encoding/binary"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestCatalogAcceptsOptionalDefaultLocaleI18nText(t *testing.T) {
	ctx := context.Background()
	catalog := testCatalog(t, time.Date(2026, 7, 5, 11, 0, 0, 0, time.UTC))
	spec := testPetDefSpec("Schema Valid Pet")
	i18n := apitypes.PetDefI18nSpec{DefaultLocale: "en", AdditionalProperties: map[string]apitypes.PetDefI18nCatalog{"en": {}}}
	resp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{Body: &adminhttp.PetDefUpsert{
		Id:   "schema-valid-pet",
		Spec: spec,
		I18n: &i18n,
	}})
	if err != nil {
		t.Fatalf("CreatePetDef() optional i18n error = %v", err)
	}
	requireResponse[adminhttp.CreatePetDef200JSONResponse](t, resp)

	missingDefaultLocaleSpec := testPetDefSpec("Bad Locale Pet")
	missingDefaultLocaleI18n := testPetDefI18n("Bad Locale Pet")
	missingDefaultLocaleI18n.DefaultLocale = "zh"
	missingDefaultLocaleResp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{Body: &adminhttp.PetDefUpsert{
		Id:   "bad-locale-pet",
		Spec: missingDefaultLocaleSpec,
		I18n: &missingDefaultLocaleI18n,
	}})
	if err != nil {
		t.Fatalf("CreatePetDef() missing default locale error = %v", err)
	}
	requireResponse[adminhttp.CreatePetDef400JSONResponse](t, missingDefaultLocaleResp)
}

func TestCatalogAcceptsPetDefWithoutI18n(t *testing.T) {
	catalog := &Catalog{PetDefs: kv.NewMemory(nil)}
	ctx := context.Background()
	spec := testPetDefSpec("No I18n")
	resp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{Body: &adminhttp.PetDefUpsert{
		Id:   "petdef-no-i18n",
		Spec: spec,
	}})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	created := requireResponse[adminhttp.CreatePetDef200JSONResponse](t, resp)
	if created.I18n.DefaultLocale != "" || len(created.I18n.AdditionalProperties) != 0 {
		t.Fatalf("CreatePetDef() i18n = %#v, want empty", created.I18n)
	}
	if !reflect.DeepEqual(created.Spec, spec) {
		t.Fatalf("CreatePetDef() changed core spec\n got: %#v\nwant: %#v", created.Spec, spec)
	}
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

func testPetDefSpec(displayName string) apitypes.PetDefSpec {
	return apitypes.PetDefSpec{
		Attr: apitypes.PetDefAttrSpec{
			Life: apitypes.PetAttrGroupSpec{
				"hunger": {Initial: 100},
				"clean":  {Initial: 100},
			},
			Progression: apitypes.PetAttrGroupSpec{
				"xp": {Initial: 0},
			},
		},
		Character: apitypes.PetDefCharacterSpec{
			Prompt: "Small friendly pixel pet.",
		},
		Voice: apitypes.PetDefVoiceSpec{
			VoiceId: "gizclaw-soft",
			Prompt:  "Soft and curious.",
		},
		Drive: apitypes.PetDefDriveSpec{Actions: []apitypes.PetDefActionSpec{
			{
				Id:           "idle",
				Cost:         0,
				VisualClipId: stringPtr("idle"),
			},
			{
				Id:           "bath",
				Cost:         10,
				VisualClipId: stringPtr("bath"),
				Effect: &apitypes.PetDefActionEffectSpec{
					AttrDelta:   &apitypes.PetAttrDelta{Life: &apitypes.PetLife{"clean": 10}},
					PetExpDelta: int64Ptr(90),
				},
			},
		}},
		Visual: apitypes.PetDefVisualSpec{
			Refs: apitypes.PetDefVisualRefsSpec{},
			Pixa: apitypes.PetDefPixaSpec{
				AssetRef: "asset://pets/test/pet.pixa",
				Metadata: apitypes.PetDefPixaMetadata{
					Version: "1",
					Canvas:  apitypes.PetDefPixaCanvasMetadata{Width: 16, Height: 16},
					Clips: []apitypes.PetDefPixaClipMetadata{
						{Id: "idle", ActionId: stringPtr("idle"), PixaClipName: "default"},
						{Id: "bath", ActionId: stringPtr("bath"), PixaClipName: "bath"},
					},
				},
			},
		},
	}
}

func testCatalog(t *testing.T, now time.Time) *Catalog {
	t.Helper()
	return &Catalog{
		PetDefs:   kv.NewMemory(nil),
		BadgeDefs: kv.NewMemory(nil),
		GameDefs:  kv.NewMemory(nil),
		Now:       func() time.Time { return now },
	}
}

func seedGameplayCatalog(t *testing.T, ctx context.Context, catalog *Catalog) apitypes.RuntimeProfile {
	t.Helper()
	petResp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{Body: &adminhttp.PetDefUpsert{
		Id: "petdef-basic", Spec: testPetDefSpec("Spark"), I18n: petDefI18nPtr("Spark"),
	}})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	requireResponse[adminhttp.CreatePetDef200JSONResponse](t, petResp)
	badgeResp, err := catalog.CreateBadgeDef(ctx, adminhttp.CreateBadgeDefRequestObject{Body: &adminhttp.BadgeDefUpsert{
		Id: "badge-basic", Spec: apitypes.BadgeDefSpec{DisplayName: "First Win"},
	}})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateBadgeDef200JSONResponse](t, badgeResp)
	gameResp, err := catalog.CreateGameDef(ctx, adminhttp.CreateGameDefRequestObject{Body: &adminhttp.GameDefUpsert{
		Id: "game-basic", Spec: apitypes.GameDefSpec{DisplayName: "Puzzle"},
	}})
	if err != nil {
		t.Fatalf("CreateGameDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateGameDef200JSONResponse](t, gameResp)
	initialBalance, adoptionCost, points, gameExp := int64(50), int64(15), int64(30), int64(20)
	badgeDelta := map[string]int64{"basic": 100}
	petDefs := map[string]apitypes.RuntimeProfileBinding{"basic": gameplayTestBinding("petdef-basic")}
	gameDefs := map[string]apitypes.RuntimeProfileBinding{"basic": gameplayTestBinding("game-basic")}
	badgeDefs := map[string]apitypes.RuntimeProfileBinding{"basic": gameplayTestBinding("badge-basic")}
	pool := []apitypes.RuntimeProfilePetPoolEntry{{PetDef: "basic", Weight: 10, AdoptionCost: &adoptionCost}}
	return apitypes.RuntimeProfile{
		Name: "default",
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{PetDefs: &petDefs, GameDefs: &gameDefs, BadgeDefs: &badgeDefs},
			Gameplay: &apitypes.RuntimeProfileGameplaySpec{
				Points:   &apitypes.RuntimeProfilePointsSpec{InitialBalance: &initialBalance},
				Adoption: &apitypes.RuntimeProfileAdoptionSpec{Pool: &pool},
				Rewards: &apitypes.RuntimeProfileDriveSpec{Games: &map[string]apitypes.RuntimeProfileRewardSpec{
					"basic": {PointsDelta: &points, PetExpDelta: &gameExp, BadgeExpDelta: &badgeDelta},
				}},
			},
		},
	}
}

func gameplayTestBinding(resourceID string) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{ResourceId: resourceID, I18n: map[string]apitypes.RuntimeProfileI18nText{
		"en": {DisplayName: resourceID}, "zh-CN": {DisplayName: resourceID},
	}}
}

func testPetDefI18n(displayName string) apitypes.PetDefI18nSpec {
	description := "Test pet."
	return apitypes.PetDefI18nSpec{
		DefaultLocale: "en",
		AdditionalProperties: map[string]apitypes.PetDefI18nCatalog{
			"en": {
				DisplayName: &displayName,
				Description: &description,
				Attr: &apitypes.PetDefI18nAttrSpec{
					Life: &apitypes.PetDefI18nAttrGroup{
						"hunger": {DisplayName: "Hunger"},
						"clean":  {DisplayName: "Clean"},
					},
					Progression: &apitypes.PetDefI18nAttrGroup{"xp": {DisplayName: "XP"}},
				},
				Drive: &apitypes.PetDefI18nDriveSpec{Actions: &map[string]apitypes.PetDefI18nDisplayText{
					"idle": {DisplayName: "Idle"},
					"bath": {DisplayName: "Bath"},
				}},
			},
		},
	}
}

func petDefI18nPtr(displayName string) *apitypes.PetDefI18nSpec {
	value := testPetDefI18n(displayName)
	return &value
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
	payload := make([]byte, int(width)*int(height)*2)
	for i := 0; i < len(payload); i += 4 {
		copy(payload[i:], []byte{0x00, 0xf8, 0xe0, 0x07})
	}
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
