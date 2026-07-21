package runtimeprofile

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestRegistrationTokenIsReturnedOnceAndStoredAsHash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	store := kv.NewMemory(nil)
	s := &Server{
		Store:  store,
		Now:    func() time.Time { return now },
		Random: strings.NewReader(strings.Repeat("x", tokenBytes)),
	}
	createProfile(t, s, "pet-runtime", map[string]string{
		"primary":   "model-a",
		"secondary": " model-b ",
		"duplicate": "model-a",
	})

	response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Name: "pet-board", RuntimeProfileName: "pet-runtime",
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := response.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok || created.Token == "" {
		t.Fatalf("create response = %#v, want one-time token", response)
	}
	raw := created.Token
	stored, err := store.Get(ctx, tokenKey("pet-board"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), raw) {
		t.Fatal("stored record contains raw token")
	}
	var private tokenRecord
	if err := json.Unmarshal(stored, &private); err != nil {
		t.Fatal(err)
	}
	if private.TokenHash != tokenDigest(raw) {
		t.Fatalf("stored digest = %q, want SHA-256 digest", private.TokenHash)
	}

	gotResponse, err := s.GetRegistrationToken(ctx, adminhttp.GetRegistrationTokenRequestObject{Name: "pet-board"})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := gotResponse.(adminhttp.GetRegistrationToken200JSONResponse)
	if !ok || got.Name != "pet-board" {
		t.Fatalf("get response = %#v", gotResponse)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), raw) || strings.Contains(string(encoded), "token_hash") {
		t.Fatalf("get response leaked token material: %s", encoded)
	}

	registration, err := s.ResolveRegistration(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if registration.RuntimeProfile.Name != "pet-runtime" {
		t.Fatalf("registration = %#v", registration)
	}
	models := *registration.RuntimeProfile.Spec.Resources.Models
	if len(models) != 3 || models["primary"].ResourceId != "model-a" || models["secondary"].ResourceId != "model-b" || models["duplicate"].ResourceId != "model-a" {
		t.Fatalf("normalized models = %#v", models)
	}
}

func TestRegistrationTokenCanBeReusedUntilDeleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	s := &Server{Store: store, Random: strings.NewReader(strings.Repeat("y", tokenBytes))}
	createProfile(t, s, "pet-runtime", nil)
	response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Name: "pet-board", RuntimeProfileName: "pet-runtime",
	}})
	if err != nil {
		t.Fatal(err)
	}
	created := response.(adminhttp.CreateRegistrationToken200JSONResponse)
	for range 2 {
		if _, err := s.ResolveRegistration(ctx, created.Token); err != nil {
			t.Fatalf("reusable token resolve: %v", err)
		}
	}
	if _, err := s.DeleteRegistrationToken(ctx, adminhttp.DeleteRegistrationTokenRequestObject{Name: "pet-board"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveRegistration(ctx, created.Token); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("resolve after delete error = %v, want not found", err)
	}
}

func TestRegistrationTokenAcceptsScopedAppName(t *testing.T) {
	t.Parallel()
	s := &Server{
		Store:  kv.NewMemory(nil),
		Random: strings.NewReader(strings.Repeat("a", tokenBytes)),
	}
	createProfile(t, s, "app-runtime", nil)
	response, err := s.CreateRegistrationToken(context.Background(), adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Name: "app:com.gizclaw.opensource", RuntimeProfileName: "app-runtime",
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := response.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok || created.Name != "app:com.gizclaw.opensource" || created.RuntimeProfileName != "app-runtime" {
		t.Fatalf("CreateRegistrationToken() = %#v", response)
	}
}

func TestRegistrationTokenBindsOptionalFirmwareReleaseLine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := &Server{
		Store:  kv.NewMemory(nil),
		Random: strings.NewReader(strings.Repeat("f", tokenBytes)),
		ResolveResource: func(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
			if kind != apitypes.ResourceKindFirmware || name != "h106" {
				return apitypes.Resource{}, kv.ErrNotFound
			}
			var resource apitypes.Resource
			err := resource.FromFirmwareResource(apitypes.FirmwareResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.FirmwareResourceKindFirmware,
				Metadata:   apitypes.ResourceMetadata{Name: name},
			})
			return resource, err
		},
	}
	createProfile(t, s, "h106-production", nil)
	firmwareID := " h106 "
	response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Name: "h106-token", RuntimeProfileName: "h106-production", FirmwareId: &firmwareID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := response.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok || created.FirmwareId == nil || *created.FirmwareId != "h106" {
		t.Fatalf("CreateRegistrationToken() = %#v, want h106 firmware binding", response)
	}
	listed, err := s.GetRegistrationToken(ctx, adminhttp.GetRegistrationTokenRequestObject{Name: "h106-token"})
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := listed.(adminhttp.GetRegistrationToken200JSONResponse)
	if !ok || stored.FirmwareId == nil || *stored.FirmwareId != "h106" {
		t.Fatalf("GetRegistrationToken() = %#v, want h106 firmware binding", listed)
	}
	registration, err := s.ResolveRegistration(ctx, created.Token)
	if err != nil {
		t.Fatal(err)
	}
	if registration.FirmwareID == nil || *registration.FirmwareID != "h106" {
		t.Fatalf("ResolveRegistration() = %#v, want h106 firmware binding", registration)
	}

	for _, test := range []struct {
		name       string
		firmwareID string
	}{
		{name: "empty-firmware", firmwareID: " "},
		{name: "missing-firmware", firmwareID: "missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
				Name: test.name, RuntimeProfileName: "h106-production", FirmwareId: &test.firmwareID,
			}})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := response.(adminhttp.CreateRegistrationToken400JSONResponse); !ok {
				t.Fatalf("CreateRegistrationToken() = %#v, want 400", response)
			}
		})
	}
}

func TestConcurrentRegistrationTokenCreateKeepsNameAndHashIndexesConsistent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	s := &Server{Store: store}
	createProfile(t, s, "pet-runtime", nil)

	const attempts = 16
	responses := make(chan adminhttp.CreateRegistrationTokenResponseObject, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
				Name: "pet-board", RuntimeProfileName: "pet-runtime",
			}})
			if err != nil {
				t.Errorf("CreateRegistrationToken() error = %v", err)
				return
			}
			responses <- response
		})
	}
	wg.Wait()
	close(responses)

	created := 0
	conflicts := 0
	var raw string
	for response := range responses {
		switch value := response.(type) {
		case adminhttp.CreateRegistrationToken200JSONResponse:
			created++
			raw = value.Token
		case adminhttp.CreateRegistrationToken409JSONResponse:
			conflicts++
		default:
			t.Fatalf("CreateRegistrationToken() response = %#v", response)
		}
	}
	if created != 1 || conflicts != attempts-1 || raw == "" {
		t.Fatalf("created=%d conflicts=%d raw_empty=%t", created, conflicts, raw == "")
	}
	if _, err := s.ResolveRegistration(ctx, raw); err != nil {
		t.Fatalf("ResolveRegistration() error = %v", err)
	}
}

func TestDanglingRuntimeProfileResourceNamesAreRejected(t *testing.T) {
	t.Parallel()
	s := &Server{
		Store: kv.NewMemory(nil),
		ResolveResource: func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) {
			return apitypes.Resource{}, kv.ErrNotFound
		},
	}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Name: "pet-runtime",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{
				"assistants": {"missing": runtimeProfileTestBinding("missing-workflow")},
			}},
			Resources: apitypes.RuntimeProfileResources{Models: new(map[string]apitypes.RuntimeProfileBinding{"missing": runtimeProfileTestBinding("missing-model")})},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile400JSONResponse); !ok {
		t.Fatalf("response = %#v, want invalid resource", response)
	}
}

func TestRuntimeProfileRejectsResolverReturningWrongResourceKind(t *testing.T) {
	t.Parallel()
	s := &Server{
		Store: kv.NewMemory(nil),
		ResolveResource: func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) {
			var resource apitypes.Resource
			err := resource.FromVoiceResource(apitypes.VoiceResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.VoiceResourceKindVoice,
				Metadata:   apitypes.ResourceMetadata{Name: "wrong-kind"},
			})
			return resource, err
		},
	}
	models := map[string]apitypes.RuntimeProfileBinding{"asr-model": runtimeProfileTestBinding("wrong-kind")}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Name: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{}},
			Resources: apitypes.RuntimeProfileResources{Models: &models},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile400JSONResponse); !ok {
		t.Fatalf("response = %#v, want wrong-kind rejection", response)
	}
}

func TestValidateFlowcraftRuntimeAliasesRejectsWrongModelKindAndMissingVoice(t *testing.T) {
	t.Parallel()
	voices := map[string]apitypes.RuntimeProfileBinding{"narrator": runtimeProfileTestBinding("voice-a")}
	models := map[string]apitypes.ModelResource{
		"generate-model": {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindEmbedding}},
	}
	workflow := apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverFlowcraft,
		Flowcraft: &apitypes.FlowcraftWorkflowSpec{
			"settings":      map[string]any{"generate_model": "generate-model"},
			"voice_adapter": map[string]any{"default_voice": "narrator"},
		},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.raids.demo", workflow, models, &voices); err == nil || !strings.Contains(err.Error(), "want \"llm\"") {
		t.Fatalf("validateWorkflowRuntimeAliases(wrong model kind) error = %v", err)
	}

	models["generate-model"] = apitypes.ModelResource{Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindLlm}}
	workflow.Flowcraft = &apitypes.FlowcraftWorkflowSpec{
		"settings":      map[string]any{"generate_model": "generate-model"},
		"voice_adapter": map[string]any{"default_voice": "missing-voice"},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.raids.demo", workflow, models, &voices); err == nil || !strings.Contains(err.Error(), "not declared in resources.voices") {
		t.Fatalf("validateWorkflowRuntimeAliases(missing voice) error = %v", err)
	}
}

func TestValidateVoiceProducingWorkflowsRequireRuntimeVoiceAliases(t *testing.T) {
	t.Parallel()
	voices := map[string]apitypes.RuntimeProfileBinding{
		"assistant":  runtimeProfileTestBinding("voice-assistant"),
		"translator": runtimeProfileTestBinding("voice-translator"),
	}
	models := map[string]apitypes.ModelResource{
		"realtime-model":    {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindRealtime}},
		"translation-model": {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindTranslation}},
	}
	s2s := apitypes.ASTTranslateModeS2s
	langPair := "auto"
	translation := apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverAstTranslate,
		AstTranslate: &apitypes.ASTTranslateWorkflowSpec{
			Mode: &s2s, TranslationModel: "translation-model", LangPair: &langPair,
		},
	}
	translation.AstTranslate.LangPair = nil
	if err := validateWorkflowRuntimeAliases("workflows.collections.translates.demo", translation, models, &voices); err == nil || !strings.Contains(err.Error(), "lang_pair is required") {
		t.Fatalf("validateWorkflowRuntimeAliases(AST without lang_pair) error = %v", err)
	}
	translation.AstTranslate.LangPair = &langPair
	if err := validateWorkflowRuntimeAliases("workflows.collections.translates.demo", translation, models, &voices); err == nil || !strings.Contains(err.Error(), "RuntimeProfile Voice alias") {
		t.Fatalf("validateWorkflowRuntimeAliases(AST without voice) error = %v", err)
	}
	internal := apitypes.ASTTranslateVoiceParameters{}
	if err := internal.FromASTTranslateInternalSpeakerParameters(apitypes.ASTTranslateInternalSpeakerParameters{SpeakerId: "provider-speaker"}); err != nil {
		t.Fatal(err)
	}
	translation.AstTranslate.Voice = &internal
	if err := validateWorkflowRuntimeAliases("workflows.collections.translates.demo", translation, models, &voices); err == nil || !strings.Contains(err.Error(), "voice.tts_voice") {
		t.Fatalf("validateWorkflowRuntimeAliases(AST provider speaker) error = %v", err)
	}
	external := apitypes.ASTTranslateVoiceParameters{}
	if err := external.FromASTTranslateExternalVoiceParameters(apitypes.ASTTranslateExternalVoiceParameters{TtsVoice: "translator"}); err != nil {
		t.Fatal(err)
	}
	translation.AstTranslate.Voice = &external
	if err := validateWorkflowRuntimeAliases("workflows.collections.translates.demo", translation, models, &voices); err != nil {
		t.Fatalf("validateWorkflowRuntimeAliases(AST alias) error = %v", err)
	}

	realtime := apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverDoubaoRealtime,
		DoubaoRealtime: &apitypes.DoubaoRealtimeWorkflowSpec{
			Model: "realtime-model",
		},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.demo", realtime, models, &voices); err == nil || !strings.Contains(err.Error(), "RuntimeProfile Voice alias") {
		t.Fatalf("validateWorkflowRuntimeAliases(Doubao without voice) error = %v", err)
	}
	voice := "assistant"
	realtime.DoubaoRealtime.Audio = &apitypes.DoubaoRealtimeAudio{
		Input:  apitypes.DoubaoRealtimeAudioInput{Format: apitypes.DoubaoRealtimeAudioFormat{Rate: 16000, Type: apitypes.DoubaoRealtimeAudioFormatTypePcm}},
		Output: apitypes.DoubaoRealtimeAudioOutput{Format: apitypes.DoubaoRealtimeAudioFormat{Rate: 24000, Type: apitypes.DoubaoRealtimeAudioFormatTypePcm}, Voice: &voice},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.demo", realtime, models, &voices); err != nil {
		t.Fatalf("validateWorkflowRuntimeAliases(Doubao alias) error = %v", err)
	}
}

func TestRuntimeProfileRejectsAliasesSharedAcrossResourceKinds(t *testing.T) {
	t.Parallel()
	s := &Server{Store: kv.NewMemory(nil)}
	models := map[string]apitypes.RuntimeProfileBinding{"assistant": runtimeProfileTestBinding("model-a")}
	voices := map[string]apitypes.RuntimeProfileBinding{"assistant": runtimeProfileTestBinding("voice-a")}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Name: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{}},
			Resources: apitypes.RuntimeProfileResources{Models: &models, Voices: &voices},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile400JSONResponse); !ok {
		t.Fatalf("response = %#v, want duplicate alias rejection", response)
	}
}

func TestRuntimeProfileRejectsWorkflowCollectionsDuplicatedAfterNormalization(t *testing.T) {
	t.Parallel()
	_, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{
		Name: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
			Collections: apitypes.RuntimeProfileWorkflowCollections{
				"assistants":   {},
				" assistants ": {},
			},
		}}}, "")
	if err == nil || !strings.Contains(err.Error(), "duplicated after normalization") {
		t.Fatalf("normalizeProfile() error = %v, want normalized collection collision", err)
	}
}

func TestRuntimeProfileRejectsInvalidGameplayReferences(t *testing.T) {
	t.Parallel()
	s := &Server{Store: kv.NewMemory(nil)}
	petDefs := map[string]apitypes.RuntimeProfileBinding{"pet": runtimeProfileTestBinding("petdef-basic")}
	pool := []apitypes.RuntimeProfilePetPoolEntry{{PetDef: "pet", Voice: "missing", Weight: 1}}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Name: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{}},
			Resources: apitypes.RuntimeProfileResources{PetDefs: &petDefs},
			Gameplay:  &apitypes.RuntimeProfileGameplaySpec{Adoption: &apitypes.RuntimeProfileAdoptionSpec{Pool: &pool}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile400JSONResponse); !ok {
		t.Fatalf("response = %#v, want undeclared adoption voice rejection", response)
	}
}

func TestRuntimeProfileRequiresPetPolicyForAdoption(t *testing.T) {
	t.Parallel()
	pool := []apitypes.RuntimeProfilePetPoolEntry{{PetDef: "pet", Voice: "voice", Weight: 1}}
	_, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{
		Name: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{}},
			Gameplay:  &apitypes.RuntimeProfileGameplaySpec{Adoption: &apitypes.RuntimeProfileAdoptionSpec{Pool: &pool}},
		},
	}, "")
	if err == nil || !strings.Contains(err.Error(), "gameplay.pet is required") {
		t.Fatalf("normalizeProfile() error = %v, want missing Pet policy rejection", err)
	}
}

func TestPetGameplayRejectsNegativeLifeDecayWeight(t *testing.T) {
	t.Parallel()
	pet := validPetGameplaySpecForTest()
	pet.Time.LifeDecay.ContributingWeights = apitypes.RuntimeProfileLifeWeightsSpec{
		Health: -0.1, Satiety: 0.4, Hygiene: 0.4, Mood: 0.3,
	}
	if err := normalizePetGameplay(&pet, apitypes.RuntimeProfileResources{}); err == nil || !strings.Contains(err.Error(), "must not be negative") {
		t.Fatalf("normalizePetGameplay() error = %v, want negative-weight rejection", err)
	}
}

func TestPetGameplayRewardModelMustBeLLM(t *testing.T) {
	t.Parallel()
	pet := validPetGameplaySpecForTest()
	pet.Games = map[string]apitypes.RuntimeProfileGameSpec{
		"puzzle": {Reward: apitypes.RuntimeProfileGameRewardSpec{Model: "reward"}},
	}
	models := map[string]apitypes.ModelResource{
		"reward": {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindEmbedding}},
	}
	if err := validatePetRewardModels(pet, models); err == nil || !strings.Contains(err.Error(), "want \"llm\"") {
		t.Fatalf("validatePetRewardModels() error = %v, want LLM-kind rejection", err)
	}
}

func TestPetGameplayRejectsDuplicateGameDefResources(t *testing.T) {
	t.Parallel()
	pet := validPetGameplaySpecForTest()
	game := apitypes.RuntimeProfileGameSpec{
		EnergyCost: 10,
		Reward:     apitypes.RuntimeProfileGameRewardSpec{Model: "reward", Prompt: "Evaluate."},
	}
	pet.Games = map[string]apitypes.RuntimeProfileGameSpec{"puzzle-a": game, "puzzle-b": game}
	gameDefs := map[string]apitypes.RuntimeProfileBinding{
		"puzzle-a": runtimeProfileTestBinding("game-puzzle"),
		"puzzle-b": runtimeProfileTestBinding("game-puzzle"),
	}
	models := map[string]apitypes.RuntimeProfileBinding{"reward": runtimeProfileTestBinding("model-reward")}
	resources := apitypes.RuntimeProfileResources{GameDefs: &gameDefs, Models: &models}
	if err := normalizePetGameplay(&pet, resources); err == nil || !strings.Contains(err.Error(), "same GameDef") {
		t.Fatalf("normalizePetGameplay() error = %v, want duplicate GameDef rejection", err)
	}
}

func TestPetGameplayRejectsUnboundedLogScale(t *testing.T) {
	t.Parallel()
	pet := validPetGameplaySpecForTest()
	pet.Experience.Leveling.LogScale = 101
	if err := normalizePetGameplay(&pet, apitypes.RuntimeProfileResources{}); err == nil || !strings.Contains(err.Error(), "0..100") {
		t.Fatalf("normalizePetGameplay() error = %v, want log-scale bound", err)
	}
}

func validPetGameplaySpecForTest() apitypes.RuntimeProfilePetGameplaySpec {
	action := apitypes.RuntimeProfilePetActionSpec{EnergyCost: 10, StatDelta: 10}
	return apitypes.RuntimeProfilePetGameplaySpec{
		Time: apitypes.RuntimeProfilePetTimeSpec{
			LifeDecay: apitypes.RuntimeProfileLifeDecaySpec{
				ContributingWeights: apitypes.RuntimeProfileLifeWeightsSpec{Health: 0.4, Satiety: 0.25, Hygiene: 0.2, Mood: 0.15},
				Exponent:            2,
			},
		},
		Experience: apitypes.RuntimeProfilePetExperienceSpec{
			EnergyPerPetExp: 5,
			Leveling:        apitypes.RuntimeProfileLevelingSpec{BaseExp: 30, LogScale: 10},
		},
		Actions: apitypes.RuntimeProfilePetActionsSpec{Feed: action, Bathe: action, Play: action, Heal: action},
	}
}

func TestRuntimeProfileAcceptsDefaultName(t *testing.T) {
	t.Parallel()
	s := &Server{Store: kv.NewMemory(nil)}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Name: "default",
		Spec: apitypes.RuntimeProfileSpec{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse)
	if !ok || created.Name != "default" {
		t.Fatalf("CreateRuntimeProfile() = %#v, want RuntimeProfile/default", response)
	}
}

func createProfile(t *testing.T, s *Server, name string, models map[string]string) {
	t.Helper()
	resources := apitypes.RuntimeProfileResources{}
	if models != nil {
		bindings := make(map[string]apitypes.RuntimeProfileBinding, len(models))
		for alias, resourceID := range models {
			bindings[alias] = runtimeProfileTestBinding(resourceID)
		}
		resources.Models = &bindings
	}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Name: name, Spec: apitypes.RuntimeProfileSpec{Resources: resources},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("create profile response = %#v", response)
	}
}

func runtimeProfileTestBinding(resourceID string) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{ResourceId: resourceID, I18n: map[string]apitypes.RuntimeProfileI18nText{
		"en": {DisplayName: "Test"}, "zh-CN": {DisplayName: "测试"},
	}}
}
