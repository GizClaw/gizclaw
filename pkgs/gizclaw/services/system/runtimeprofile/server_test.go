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

func TestDeleteRuntimeProfileAllowsOnlyRecognizedLegacyBindings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	s := &Server{Store: store}
	legacy := []byte(`{"created_at":"2026-07-01T00:00:00Z","name":"default","spec":{"resources":{"workflows":{"chat":"flowcraft-chat"},"models":{"generate-model":"minimax-default"}}},"updated_at":"2026-07-01T00:00:00Z"}`)
	if err := store.Set(ctx, profileKey("default"), legacy); err != nil {
		t.Fatal(err)
	}

	response, err := s.DeleteRuntimeProfile(ctx, adminhttp.DeleteRuntimeProfileRequestObject{Name: "default"})
	if err != nil {
		t.Fatal(err)
	}
	deleted, ok := response.(adminhttp.DeleteRuntimeProfile200JSONResponse)
	if !ok || deleted.Name != "default" || deleted.Revision != "legacy-deleted" {
		t.Fatalf("DeleteRuntimeProfile(legacy) = %#v", response)
	}
	if _, err := store.Get(ctx, profileKey("default")); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("legacy profile remains after deletion: %v", err)
	}

	corrupt := []byte(`{"name":"default","spec":{"resources":{"models":{"generate-model":{"resource_id":123}}}}}`)
	if err := store.Set(ctx, profileKey("default"), corrupt); err != nil {
		t.Fatal(err)
	}
	response, err = s.DeleteRuntimeProfile(ctx, adminhttp.DeleteRuntimeProfileRequestObject{Name: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.DeleteRuntimeProfile500JSONResponse); !ok {
		t.Fatalf("DeleteRuntimeProfile(corrupt) = %#v, want 500", response)
	}
	if _, err := store.Get(ctx, profileKey("default")); err != nil {
		t.Fatalf("corrupt profile was deleted: %v", err)
	}
}

func TestRegistrationTokenIgnoresLegacyPersistedFirmwareName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	s := &Server{
		Store:  store,
		Random: strings.NewReader(strings.Repeat("z", tokenBytes)),
	}
	createProfile(t, s, "pet-runtime", nil)
	response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Name: "pet-board", RuntimeProfileName: "pet-runtime",
	}})
	if err != nil {
		t.Fatal(err)
	}
	created := response.(adminhttp.CreateRegistrationToken200JSONResponse)
	data, err := store.Get(ctx, tokenKey("pet-board"))
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy["firmware_name"] = "deleted-firmware"
	data, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, tokenKey("pet-board"), data); err != nil {
		t.Fatal(err)
	}
	if registration, err := s.ResolveRegistration(ctx, created.Token); err != nil || registration.RuntimeProfile.Name != "pet-runtime" {
		t.Fatalf("ResolveRegistration() = %#v, %v", registration, err)
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
	negative := int64(-1)
	pool := []apitypes.RuntimeProfilePetPoolEntry{{PetDef: "missing", Weight: 1, AdoptionCost: &negative}}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Name: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{}},
			Resources: apitypes.RuntimeProfileResources{},
			Gameplay:  &apitypes.RuntimeProfileGameplaySpec{Adoption: &apitypes.RuntimeProfileAdoptionSpec{Pool: &pool}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile400JSONResponse); !ok {
		t.Fatalf("response = %#v, want invalid gameplay rejection", response)
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
