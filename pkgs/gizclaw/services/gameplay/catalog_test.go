package gameplay

import (
	"bytes"
	"context"
	"io"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestCatalogStoresPetDefWithoutLocalI18n(t *testing.T) {
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
	if !reflect.DeepEqual(created.Spec, spec) {
		t.Fatalf("CreatePetDef() changed core spec\n got: %#v\nwant: %#v", created.Spec, spec)
	}
}

func TestCatalogNormalizesAndBoundsBadgeRewardPrompt(t *testing.T) {
	t.Parallel()
	catalog := testCatalog(t, time.Unix(1, 0))
	prompt := "  Award for sustained scientific curiosity.  "
	item, err := catalog.buildBadgeDef("badge-id", apitypes.BadgeDefSpec{
		DisplayName:  " Science ",
		RewardPrompt: &prompt,
	}, nil, time.Time{})
	if err != nil {
		t.Fatalf("buildBadgeDef() error = %v", err)
	}
	if item.Spec.DisplayName != "Science" || item.Spec.RewardPrompt == nil ||
		*item.Spec.RewardPrompt != "Award for sustained scientific curiosity." {
		t.Fatalf("normalized BadgeDef = %#v", item.Spec)
	}
	for name, value := range map[string]string{
		"empty":    " ",
		"too long": strings.Repeat("x", 8193),
		"utf8":     string([]byte{0xff}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := catalog.buildBadgeDef("badge-id", apitypes.BadgeDefSpec{
				DisplayName: "Science", RewardPrompt: &value,
			}, nil, time.Time{})
			if err == nil {
				t.Fatal("buildBadgeDef() succeeded")
			}
		})
	}
}

func TestCatalogPetDefPixaUploadRejectsBeforePublication(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1, 0).UTC()
	assets := newTestObjectStore(t)
	catalog := testCatalog(t, now)
	catalog.Assets = assets

	spec := testPetDefSpec("Transparent Pet")
	spec.Visual.Pixa.Metadata.Canvas = apitypes.PetDefPixaCanvasMetadata{Width: 4, Height: 4}
	createResp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{
		Body: &adminhttp.PetDefUpsert{Id: "transparent-pet", Spec: spec},
	})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	created := requireResponse[adminhttp.CreatePetDef200JSONResponse](t, createResp)
	petDefID := created.Id

	valid := makePixaFixture(t, 4, 4, []uint16{0, 0x07e0},
		[]testPixaClip{
			{name: "default", firstFrame: 0, frameCount: 1},
			{name: "bath", firstFrame: 0, frameCount: 1},
		},
		[]testPixaFrame{{
			frameType: 0,
			encoding:  1,
			payload:   paletteRLE([]byte{0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0}),
		}},
	)
	now = time.Unix(2, 0).UTC()
	catalog.Now = func() time.Time { return now }
	uploadResp, err := catalog.UploadPetDefPixa(ctx, adminhttp.UploadPetDefPixaRequestObject{
		Id: petDefID, Body: io.NopCloser(bytes.NewReader(valid)),
	})
	if err != nil {
		t.Fatalf("UploadPetDefPixa(valid) error = %v", err)
	}
	published := requireResponse[adminhttp.UploadPetDefPixa200JSONResponse](t, uploadResp)
	if published.PixaPath == nil || *published.PixaPath != path.Join(catalogAssetPrefix("pet-defs", string(petDefID)), "pixa") {
		t.Fatalf("published PixaPath = %v", published.PixaPath)
	}
	if !published.UpdatedAt.Equal(now) {
		t.Fatalf("published UpdatedAt = %v, want %v", published.UpdatedAt, now)
	}

	countedAssets := &countingObjectStore{ObjectStore: assets}
	catalog.Assets = countedAssets
	invalid := makePixaFixture(t, 4, 4, []uint16{0, 0x07e0},
		[]testPixaClip{
			{name: "default", firstFrame: 0, frameCount: 1},
			{name: "bath", firstFrame: 0, frameCount: 1},
		},
		[]testPixaFrame{{
			frameType: 0,
			encoding:  1,
			payload:   paletteRLE([]byte{1, 0, 0, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0}),
		}},
	)
	now = time.Unix(3, 0).UTC()
	rejectResp, err := catalog.UploadPetDefPixa(ctx, adminhttp.UploadPetDefPixaRequestObject{
		Id: petDefID, Body: io.NopCloser(bytes.NewReader(invalid)),
	})
	if err != nil {
		t.Fatalf("UploadPetDefPixa(invalid) error = %v", err)
	}
	rejected := requireResponse[adminhttp.UploadPetDefPixa500JSONResponse](t, rejectResp)
	if rejected.Error.Code != "INVALID_PET_DEF_PIXA" ||
		!strings.Contains(rejected.Error.Message, `clip "default" local frame 0`) {
		t.Fatalf("rejection = %#v", rejected.Error)
	}
	if countedAssets.puts != 0 {
		t.Fatalf("invalid upload called object store Put %d times", countedAssets.puts)
	}
	malformedResp, err := catalog.UploadPetDefPixa(ctx, adminhttp.UploadPetDefPixaRequestObject{
		Id: petDefID, Body: io.NopCloser(strings.NewReader("not a PIXA file")),
	})
	if err != nil {
		t.Fatalf("UploadPetDefPixa(malformed) error = %v", err)
	}
	malformed := requireResponse[adminhttp.UploadPetDefPixa500JSONResponse](t, malformedResp)
	if malformed.Error.Code != "INVALID_PET_DEF_PIXA" || !strings.Contains(malformed.Error.Message, "pixa:") {
		t.Fatalf("malformed rejection = %#v", malformed.Error)
	}
	if countedAssets.puts != 0 {
		t.Fatalf("malformed upload called object store Put %d times", countedAssets.puts)
	}
	oversizedResp, err := catalog.UploadPetDefPixa(ctx, adminhttp.UploadPetDefPixaRequestObject{
		Id:   petDefID,
		Body: io.NopCloser(io.LimitReader(zeroReader{}, petDefPixaMaxEncodedBytes+1)),
	})
	if err != nil {
		t.Fatalf("UploadPetDefPixa(oversized) error = %v", err)
	}
	oversized := requireResponse[adminhttp.UploadPetDefPixa500JSONResponse](t, oversizedResp)
	if oversized.Error.Code != "INVALID_PET_DEF_PIXA" ||
		!strings.Contains(oversized.Error.Message, "exceeds 16777216 byte limit") {
		t.Fatalf("oversized rejection = %#v", oversized.Error)
	}
	if countedAssets.puts != 0 {
		t.Fatalf("oversized upload called object store Put %d times", countedAssets.puts)
	}

	after, err := catalog.GetPetDefByID(ctx, petDefID)
	if err != nil {
		t.Fatalf("GetPetDefByID() error = %v", err)
	}
	if !reflect.DeepEqual(after, apitypes.PetDef(published)) {
		t.Fatalf("PetDef changed after rejected upload\n got: %#v\nwant: %#v", after, published)
	}
	downloadResp, err := catalog.DownloadPetDefPixa(ctx, adminhttp.DownloadPetDefPixaRequestObject{Id: petDefID})
	if err != nil {
		t.Fatalf("DownloadPetDefPixa() error = %v", err)
	}
	downloaded := requireResponse[adminhttp.DownloadPetDefPixa200ApplicationoctetStreamResponse](t, downloadResp)
	if closer, ok := downloaded.Body.(io.Closer); ok {
		defer closer.Close()
	}
	if got := readAllBytes(t, downloaded.Body); !bytes.Equal(got, valid) {
		t.Fatalf("downloaded PIXA changed after rejected upload")
	}
}

func TestCatalogOpaqueIDsUseSafeKVSegments(t *testing.T) {
	ctx := context.Background()
	catalog := testCatalog(t, time.Unix(1, 0))
	id := "tenant:catalog/item"

	petResp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{Body: &adminhttp.PetDefUpsert{
		Id: id, Spec: testPetDefSpec("Opaque Pet"),
	}})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	requireResponse[adminhttp.CreatePetDef200JSONResponse](t, petResp)

	badgeResp, err := catalog.CreateBadgeDef(ctx, adminhttp.CreateBadgeDefRequestObject{Body: &adminhttp.BadgeDefUpsert{
		Id: id, Spec: apitypes.BadgeDefSpec{DisplayName: "Opaque Badge"},
	}})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateBadgeDef200JSONResponse](t, badgeResp)

	gameResp, err := catalog.CreateGameDef(ctx, adminhttp.CreateGameDefRequestObject{Body: &adminhttp.GameDefUpsert{
		Id: id, Spec: apitypes.GameDefSpec{DisplayName: "Opaque Game"},
	}})
	if err != nil {
		t.Fatalf("CreateGameDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateGameDef200JSONResponse](t, gameResp)
}

func TestCatalogAssetPrefixesDoNotOverlapOpaqueIDs(t *testing.T) {
	assets := newTestObjectStore(t)
	parentPrefix := catalogAssetPrefix("pet-defs", "team")
	childPrefix := catalogAssetPrefix("pet-defs", "team/blue")
	childObject := path.Join(childPrefix, "pixa")
	if strings.HasPrefix(childPrefix, parentPrefix+"/") {
		t.Fatalf("catalog prefixes overlap: parent=%q child=%q", parentPrefix, childPrefix)
	}
	if err := assets.Put(childObject, strings.NewReader("child")); err != nil {
		t.Fatal(err)
	}
	if err := assets.DeletePrefix(parentPrefix); err != nil {
		t.Fatal(err)
	}
	reader, err := assets.Get(childObject)
	if err != nil {
		t.Fatalf("deleting %q removed %q: %v", parentPrefix, childObject, err)
	}
	_ = reader.Close()
}

type countingObjectStore struct {
	objectstore.ObjectStore
	puts int
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func (s *countingObjectStore) Put(name string, reader io.Reader) error {
	s.puts++
	return s.ObjectStore.Put(name, reader)
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
		Character: apitypes.PetDefCharacterSpec{
			Prompt: "Small friendly pixel pet.",
		},
		Voice: apitypes.PetDefVoiceSpec{
			Prompt: "Soft and curious.",
		},
		Visual: apitypes.PetDefVisualSpec{
			Bindings: apitypes.PetDefVisualBindingsSpec{
				Behaviors: apitypes.PetDefBehaviorBindingsSpec{Feed: "idle", Bathe: "bath", Play: "idle", Heal: "idle"},
				States:    apitypes.PetDefStateBindingsSpec{Idle: "idle", Sick: "idle", Dead: "idle"},
			},
			Refs: apitypes.PetDefVisualRefsSpec{},
			Pixa: apitypes.PetDefPixaSpec{
				AssetRef: "asset://pets/test/pet.pixa",
				Metadata: apitypes.PetDefPixaMetadata{
					Version: "1",
					Canvas:  apitypes.PetDefPixaCanvasMetadata{Width: 16, Height: 16},
					Clips: []apitypes.PetDefPixaClipMetadata{
						{Id: "idle", PixaClipName: "default"},
						{Id: "bath", PixaClipName: "bath"},
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
		Id: "petdef-basic", Spec: testPetDefSpec("Spark"),
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
	initialBalance, adoptionCost := int64(50), int64(15)
	petDefs := map[string]apitypes.RuntimeProfileBinding{"basic": gameplayTestBinding("petdef-basic")}
	voices := map[string]apitypes.RuntimeProfileBinding{"pet-voice": gameplayTestBinding("voice-basic")}
	models := map[string]apitypes.RuntimeProfileBinding{"reward": gameplayTestBinding("model-reward")}
	gameDefs := map[string]apitypes.RuntimeProfileBinding{"basic": gameplayTestBinding("game-basic")}
	badgeDefs := map[string]apitypes.RuntimeProfileBinding{"basic": gameplayTestBinding("badge-basic")}
	pool := []apitypes.RuntimeProfilePetPoolEntry{{PetDef: "basic", Weight: 10, AdoptionCost: &adoptionCost}}
	return apitypes.RuntimeProfile{
		Id: "default",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{
					Pet: "pet-care",
				},
				Collections: apitypes.RuntimeProfileWorkflowCollections{},
			},
			Resources: apitypes.RuntimeProfileResources{Models: &models, PetDefs: &petDefs, Voices: &voices, GameDefs: &gameDefs, BadgeDefs: &badgeDefs},
			Gameplay: &apitypes.RuntimeProfileGameplaySpec{
				Points:   &apitypes.RuntimeProfilePointsSpec{InitialBalance: &initialBalance},
				Adoption: &apitypes.RuntimeProfileAdoptionSpec{Pool: &pool},
				Pet:      testPetGameplaySpec(),
			},
		},
	}
}

func gameplayTestBinding(resourceID string) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{ResourceId: resourceID, I18n: map[string]apitypes.RuntimeProfileI18nText{
		"en": {DisplayName: resourceID}, "zh-CN": {DisplayName: resourceID},
	}}
}

func testPetGameplaySpec() *apitypes.RuntimeProfilePetGameplaySpec {
	return &apitypes.RuntimeProfilePetGameplaySpec{
		Time: apitypes.RuntimeProfilePetTimeSpec{
			CareDecayPerHour:      apitypes.RuntimeProfileCareDecaySpec{Satiety: 1.25, Hygiene: 0.75, Mood: 0.4},
			EnergyRecoveryPerHour: 10,
			LifeDecay: apitypes.RuntimeProfileLifeDecaySpec{
				ContributingWeights: apitypes.RuntimeProfileLifeWeightsSpec{Health: 0.4, Satiety: 0.25, Hygiene: 0.2, Mood: 0.15},
				MaxLossPerHour:      4, Exponent: 2,
			},
		},
		Experience: apitypes.RuntimeProfilePetExperienceSpec{
			EnergyPerPetExp: 5,
			Leveling:        apitypes.RuntimeProfileLevelingSpec{BaseExp: 30, LogScale: 10},
		},
		Actions: apitypes.RuntimeProfilePetActionsSpec{
			Feed:  apitypes.RuntimeProfilePetActionSpec{EnergyCost: 10, StatDelta: 10},
			Bathe: apitypes.RuntimeProfilePetActionSpec{EnergyCost: 10, StatDelta: 10},
			Play:  apitypes.RuntimeProfilePetActionSpec{EnergyCost: 10, StatDelta: 10},
			Heal:  apitypes.RuntimeProfilePetActionSpec{EnergyCost: 10, StatDelta: 10},
		},
		Games: map[string]apitypes.RuntimeProfileGameSpec{
			"basic": {
				EnergyCost: 10, PointsCost: 10,
				Reward: apitypes.RuntimeProfileGameRewardSpec{Model: "reward", PetExpMax: 10, BadgeExpMaxPerBadge: 5, Prompt: "Evaluate the validated game result."},
			},
		},
	}
}

type rewardEvaluatorFunc func(context.Context, RewardEvaluationRequest) (apitypes.GameRewardSpec, error)

func (fn rewardEvaluatorFunc) Evaluate(ctx context.Context, request RewardEvaluationRequest) (apitypes.GameRewardSpec, error) {
	return fn(ctx, request)
}
