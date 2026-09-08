package runtimeprofile

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"database/sql"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestRegistrationTokenIsReadableAndIndexedByToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	store := profileSQLTestDB(t)
	s := &Server{
		DB:  store,
		Now: func() time.Time { return now },
	}
	createProfile(t, s, "pet-runtime", map[string]string{
		"primary":   "model-a",
		"secondary": "model-b",
		"duplicate": "model-a",
	})

	response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Id: "pet-board", Token: " device-token ", RuntimeProfileId: "pet-runtime",
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := response.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok || created.Token != "device-token" || !created.CreatedAt.Equal(now) || !created.UpdatedAt.Equal(now) {
		t.Fatalf("create response = %#v, want complete persisted resource", response)
	}
	persisted, err := getRegistrationTokenByID(ctx, store, created.Id)
	if err != nil {
		t.Fatal(err)
	}
	if persisted != apitypes.RegistrationToken(created) {
		t.Fatalf("persisted = %#v, want %#v", persisted, created)
	}
	var indexedName string
	if err := store.QueryRowContext(ctx, "SELECT id FROM registration_tokens WHERE token=?", created.Token).Scan(&indexedName); err != nil {
		t.Fatal(err)
	}
	if indexedName != created.Id {
		t.Fatalf("token index = %q, want %q", indexedName, created.Id)
	}

	gotResponse, err := s.GetRegistrationToken(ctx, adminhttp.GetRegistrationTokenRequestObject{Id: created.Id})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := gotResponse.(adminhttp.GetRegistrationToken200JSONResponse)
	if !ok || apitypes.RegistrationToken(got) != apitypes.RegistrationToken(created) {
		t.Fatalf("get response = %#v", gotResponse)
	}
	listResponse, err := s.ListRegistrationTokens(ctx, adminhttp.ListRegistrationTokensRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	listed, ok := listResponse.(adminhttp.ListRegistrationTokens200JSONResponse)
	if !ok || len(listed.Items) != 1 || listed.Items[0] != apitypes.RegistrationToken(created) {
		t.Fatalf("list response = %#v", listResponse)
	}

	registration, err := s.ResolveRegistration(ctx, created.Token)
	if err != nil {
		t.Fatal(err)
	}
	if registration.RuntimeProfile.Id != "pet-runtime" {
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
	store := profileSQLTestDB(t)
	s := &Server{DB: store}
	createProfile(t, s, "pet-runtime", nil)
	response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Id: "pet-board", Token: "reusable-token", RuntimeProfileId: "pet-runtime",
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
	deleteResponse, err := s.DeleteRegistrationToken(ctx, adminhttp.DeleteRegistrationTokenRequestObject{Id: created.Id})
	if err != nil {
		t.Fatal(err)
	}
	deleted, ok := deleteResponse.(adminhttp.DeleteRegistrationToken200JSONResponse)
	if !ok || deleted.Token != created.Token {
		t.Fatalf("delete response = %#v, want complete resource", deleteResponse)
	}
	if _, err := s.ResolveRegistration(ctx, created.Token); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("resolve after delete error = %v, want not found", err)
	}
}

func TestPutRegistrationTokenReplacesTokenAndHashIndexAtomically(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := profileSQLTestDB(t)
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	s := &Server{DB: store, Now: func() time.Time { return now }}
	createProfile(t, s, "pet-runtime", nil)
	createResponse, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Id: "pet-board", Token: "old-token", RuntimeProfileId: "pet-runtime",
	}})
	if err != nil {
		t.Fatal(err)
	}
	created := createResponse.(adminhttp.CreateRegistrationToken200JSONResponse)

	now = now.Add(time.Minute)
	putResponse, err := s.PutRegistrationToken(ctx, adminhttp.PutRegistrationTokenRequestObject{
		Id:   created.Id,
		Body: &adminhttp.RegistrationTokenUpsert{Id: "pet-board", Token: "new-token", RuntimeProfileId: "pet-runtime"},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, ok := putResponse.(adminhttp.PutRegistrationToken200JSONResponse)
	if !ok || updated.Token != "new-token" || !updated.CreatedAt.Equal(created.CreatedAt) || !updated.UpdatedAt.Equal(now) {
		t.Fatalf("PutRegistrationToken() = %#v", putResponse)
	}
	if _, err := s.ResolveRegistration(ctx, "old-token"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ResolveRegistration(old-token) error = %v, want not found", err)
	}
	if _, err := s.ResolveRegistration(ctx, "new-token"); err != nil {
		t.Fatalf("ResolveRegistration(new-token) error = %v", err)
	}
	if _, _, _, err := resolveRegistrationSQL(ctx, store, "old-token"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("old hash index error = %v, want not found", err)
	}
}

func TestPutRegistrationTokenStoreFailurePreservesRecordAndIndexes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := profileSQLTestDB(t)
	s := &Server{DB: store}
	createProfile(t, s, "pet-runtime", nil)
	createResponse, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Id: "pet-board", Token: "old-token", RuntimeProfileId: "pet-runtime",
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := createResponse.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok {
		t.Fatalf("CreateRegistrationToken() = %#v", createResponse)
	}
	if _, err := store.ExecContext(ctx, `CREATE TRIGGER fail_token_update BEFORE UPDATE ON registration_tokens BEGIN SELECT RAISE(ABORT,'injected token update failure'); END`); err != nil {
		t.Fatal(err)
	}
	putResponse, err := s.PutRegistrationToken(ctx, adminhttp.PutRegistrationTokenRequestObject{
		Id:   created.Id,
		Body: &adminhttp.RegistrationTokenUpsert{Id: "pet-board", Token: "new-token", RuntimeProfileId: "pet-runtime"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := putResponse.(adminhttp.PutRegistrationToken500JSONResponse); !ok {
		t.Fatalf("PutRegistrationToken() = %#v, want 500", putResponse)
	}
	gotResponse, err := s.GetRegistrationToken(ctx, adminhttp.GetRegistrationTokenRequestObject{Id: created.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got := gotResponse.(adminhttp.GetRegistrationToken200JSONResponse).Token; got != "old-token" {
		t.Fatalf("stored token = %q, want old-token", got)
	}
	if _, err := s.ResolveRegistration(ctx, "old-token"); err != nil {
		t.Fatalf("ResolveRegistration(old-token) error = %v", err)
	}
	if _, err := s.ResolveRegistration(ctx, "new-token"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ResolveRegistration(new-token) error = %v, want not found", err)
	}
}

func TestRegistrationTokenCollisionLeavesBothResourcesUnchanged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := profileSQLTestDB(t)
	s := &Server{DB: store}
	createProfile(t, s, "pet-runtime", nil)
	createdIDs := map[string]string{}
	for _, item := range []adminhttp.RegistrationTokenUpsert{
		{Id: "first-token", Token: " shared-token ", RuntimeProfileId: "pet-runtime"},
		{Id: "second-token", Token: "second-token", RuntimeProfileId: "pet-runtime"},
	} {
		response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &item})
		if err != nil {
			t.Fatal(err)
		}
		created, ok := response.(adminhttp.CreateRegistrationToken200JSONResponse)
		if !ok {
			t.Fatalf("CreateRegistrationToken(%s) = %#v", item.Id, response)
		}
		createdIDs[item.Id] = created.Id
	}
	conflictingCreate := adminhttp.RegistrationTokenUpsert{Id: "third-token", Token: "shared-token", RuntimeProfileId: "pet-runtime"}
	response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &conflictingCreate})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRegistrationToken409JSONResponse); !ok {
		t.Fatalf("conflicting create = %#v, want 409", response)
	}
	conflictingPut := adminhttp.RegistrationTokenUpsert{Id: "second-token", Token: " shared-token ", RuntimeProfileId: "pet-runtime"}
	putResponse, err := s.PutRegistrationToken(ctx, adminhttp.PutRegistrationTokenRequestObject{Id: createdIDs["second-token"], Body: &conflictingPut})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := putResponse.(adminhttp.PutRegistrationToken409JSONResponse); !ok {
		t.Fatalf("conflicting put = %#v, want 409", putResponse)
	}
	second, err := s.GetRegistrationToken(ctx, adminhttp.GetRegistrationTokenRequestObject{Id: createdIDs["second-token"]})
	if err != nil {
		t.Fatal(err)
	}
	if got := second.(adminhttp.GetRegistrationToken200JSONResponse).Token; got != "second-token" {
		t.Fatalf("second token = %q, want unchanged second-token", got)
	}
	if _, err := s.ResolveRegistration(ctx, "shared-token"); err != nil {
		t.Fatalf("shared token no longer resolves: %v", err)
	}
	if _, err := s.ResolveRegistration(ctx, "second-token"); err != nil {
		t.Fatalf("second token no longer resolves: %v", err)
	}
}

func TestRegistrationTokenAcceptsScopedAppName(t *testing.T) {
	t.Parallel()
	s := &Server{
		DB: profileSQLTestDB(t),
	}
	createProfile(t, s, "app-runtime", nil)
	response, err := s.CreateRegistrationToken(context.Background(), adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Id: "app:com.gizclaw.opensource", Token: "desktop-token", RuntimeProfileId: "app-runtime",
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := response.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok || created.Id != "app:com.gizclaw.opensource" || created.RuntimeProfileId != "app-runtime" {
		t.Fatalf("CreateRegistrationToken() = %#v", response)
	}
}

func TestRegistrationTokenBindsOptionalFirmwareReleaseLine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := &Server{
		DB: profileSQLTestDB(t),
		ResolveResource: func(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
			if kind != apitypes.ResourceKindFirmware || name != "h106" {
				return apitypes.Resource{}, sql.ErrNoRows
			}
			var resource apitypes.Resource
			err := resource.FromFirmwareResource(apitypes.FirmwareResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.FirmwareResourceKindFirmware,
				Metadata:   apitypes.ResourceMetadata{Id: name},
				Spec:       apitypes.FirmwareSpec{},
			})
			return resource, err
		},
	}
	createProfile(t, s, "h106-production", nil)
	firmwareID := "h106"
	response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Id: "h106-token", Token: "h106-registration", RuntimeProfileId: "h106-production", FirmwareId: &firmwareID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := response.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok || created.FirmwareId == nil || *created.FirmwareId != "h106" {
		t.Fatalf("CreateRegistrationToken() = %#v, want h106 firmware binding", response)
	}
	listed, err := s.GetRegistrationToken(ctx, adminhttp.GetRegistrationTokenRequestObject{Id: created.Id})
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
		{name: "whitespace-firmware", firmwareID: " h106 "},
		{name: "missing-firmware", firmwareID: "missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
				Id: test.name, Token: test.name + "-token", RuntimeProfileId: "h106-production", FirmwareId: &test.firmwareID,
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
	store := profileSQLTestDB(t)
	s := &Server{DB: store}
	createProfile(t, s, "pet-runtime", nil)

	const attempts = 16
	responses := make(chan adminhttp.CreateRegistrationTokenResponseObject, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
				Id: "pet-board", Token: "concurrent-token", RuntimeProfileId: "pet-runtime",
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
		DB: profileSQLTestDB(t),
		ResolveResource: func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) {
			return apitypes.Resource{}, sql.ErrNoRows
		},
	}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Id: "pet-runtime",
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
		DB: profileSQLTestDB(t),
		ResolveResource: func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) {
			var resource apitypes.Resource
			err := resource.FromVoiceResource(apitypes.VoiceResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.VoiceResourceKindVoice,
				Metadata:   apitypes.ResourceMetadata{Id: "wrong-kind"},
			})
			return resource, err
		},
	}
	models := map[string]apitypes.RuntimeProfileBinding{"asr-model": runtimeProfileTestBinding("wrong-kind")}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Id: "test-profile",
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
	voices := map[string]apitypes.VoiceResource{"narrator": {}}
	models := map[string]apitypes.ModelResource{
		"generate-model": {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindEmbedding}},
	}
	workflow := apitypes.WorkflowSpec{
		Driver:    apitypes.WorkflowDriverFlowcraft,
		Flowcraft: runtimeProfileTestFlowcraftSpec(t, "generate-model", "narrator"),
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.raids.demo", workflow, models, voices); err == nil || !strings.Contains(err.Error(), "want \"llm\"") {
		t.Fatalf("validateWorkflowRuntimeAliases(wrong model kind) error = %v", err)
	}

	models["generate-model"] = apitypes.ModelResource{Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindLlm}}
	workflow.Flowcraft = runtimeProfileTestFlowcraftSpec(t, "generate-model", "missing-voice")
	if err := validateWorkflowRuntimeAliases("workflows.collections.raids.demo", workflow, models, voices); err == nil || !strings.Contains(err.Error(), "not declared in resources.voices") {
		t.Fatalf("validateWorkflowRuntimeAliases(missing voice) error = %v", err)
	}
}

func TestValidateDottedMemoryLayoutAndFlowcraftRuntimeAliases(t *testing.T) {
	t.Parallel()

	models := map[string]apitypes.ModelResource{
		"pet-care.extract":   {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindLlm}},
		"pet-care.embedding": {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindEmbedding}},
		"pet-care.rerank":    {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindLlm}},
		"pet-care.model":     {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindLlm}},
	}
	voices := map[string]apitypes.VoiceResource{"pet-care.pet": {}}
	layout := apitypes.MemoryLayoutSpec{Flowcraft: apitypes.FlowcraftMemoryLayoutPolicy{
		Extraction: apitypes.FlowcraftMemoryExtractionPolicy{Model: "pet-care.extract"},
		Embedding:  &apitypes.FlowcraftMemoryModelPolicy{Model: "pet-care.embedding"},
		Rerank:     &apitypes.FlowcraftMemoryModelPolicy{Model: "pet-care.rerank"},
	}}
	if err := validateMemoryLayoutRuntimeAliases("resources.memories.pet-care", apitypes.RuntimeProfileMemoryDriverFlowcraft, layout, models); err != nil {
		t.Fatalf("validateMemoryLayoutRuntimeAliases() error = %v", err)
	}
	workflow := apitypes.WorkflowSpec{
		Driver:    apitypes.WorkflowDriverFlowcraft,
		Flowcraft: runtimeProfileTestFlowcraftSpec(t, "pet-care.model", "pet-care.pet"),
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.pets.pet-care", workflow, models, voices); err != nil {
		t.Fatalf("validateWorkflowRuntimeAliases() error = %v", err)
	}

	delete(models, "pet-care.extract")
	if err := validateMemoryLayoutRuntimeAliases("resources.memories.pet-care", apitypes.RuntimeProfileMemoryDriverFlowcraft, layout, models); err == nil ||
		!strings.Contains(err.Error(), `model alias "pet-care.extract" is not declared`) {
		t.Fatalf("validateMemoryLayoutRuntimeAliases(missing dotted alias) error = %v", err)
	}
	delete(voices, "pet-care.pet")
	if err := validateWorkflowRuntimeAliases("workflows.collections.pets.pet-care", workflow, models, voices); err == nil ||
		!strings.Contains(err.Error(), `voice alias "pet-care.pet" is not declared`) {
		t.Fatalf("validateWorkflowRuntimeAliases(missing dotted alias) error = %v", err)
	}
}

func TestValidateVoiceProducingWorkflowsRequireRuntimeVoiceAliases(t *testing.T) {
	t.Parallel()
	voices := map[string]apitypes.VoiceResource{
		"assistant":  {},
		"translator": {},
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
	if err := validateWorkflowRuntimeAliases("workflows.collections.translates.demo", translation, models, voices); err == nil || !strings.Contains(err.Error(), "lang_pair is required") {
		t.Fatalf("validateWorkflowRuntimeAliases(AST without lang_pair) error = %v", err)
	}
	translation.AstTranslate.LangPair = &langPair
	if err := validateWorkflowRuntimeAliases("workflows.collections.translates.demo", translation, models, voices); err == nil || !strings.Contains(err.Error(), "RuntimeProfile Voice alias") {
		t.Fatalf("validateWorkflowRuntimeAliases(AST without voice) error = %v", err)
	}
	internal := apitypes.ASTTranslateVoiceParameters{}
	if err := internal.FromASTTranslateInternalSpeakerParameters(apitypes.ASTTranslateInternalSpeakerParameters{SpeakerId: "provider-speaker"}); err != nil {
		t.Fatal(err)
	}
	translation.AstTranslate.Voice = &internal
	if err := validateWorkflowRuntimeAliases("workflows.collections.translates.demo", translation, models, voices); err == nil || !strings.Contains(err.Error(), "voice.tts_voice") {
		t.Fatalf("validateWorkflowRuntimeAliases(AST provider speaker) error = %v", err)
	}
	external := apitypes.ASTTranslateVoiceParameters{}
	if err := external.FromASTTranslateExternalVoiceParameters(apitypes.ASTTranslateExternalVoiceParameters{TtsVoice: "translator"}); err != nil {
		t.Fatal(err)
	}
	translation.AstTranslate.Voice = &external
	if err := validateWorkflowRuntimeAliases("workflows.collections.translates.demo", translation, models, voices); err != nil {
		t.Fatalf("validateWorkflowRuntimeAliases(AST alias) error = %v", err)
	}

	realtime := apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverDoubaoRealtime,
		DoubaoRealtime: &apitypes.DoubaoRealtimeWorkflowSpec{
			Model: "realtime-model",
		},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.demo", realtime, models, voices); err == nil || !strings.Contains(err.Error(), "RuntimeProfile Voice alias") {
		t.Fatalf("validateWorkflowRuntimeAliases(Doubao without voice) error = %v", err)
	}
	voice := "assistant"
	realtime.DoubaoRealtime.Audio = &apitypes.DoubaoRealtimeAudio{
		Input:  apitypes.DoubaoRealtimeAudioInput{Format: apitypes.DoubaoRealtimeAudioFormat{Rate: 16000, Type: apitypes.DoubaoRealtimeAudioFormatTypePcm}},
		Output: apitypes.DoubaoRealtimeAudioOutput{Format: apitypes.DoubaoRealtimeAudioFormat{Rate: 24000, Type: apitypes.DoubaoRealtimeAudioFormatTypePcm}, Voice: &voice},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.demo", realtime, models, voices); err != nil {
		t.Fatalf("validateWorkflowRuntimeAliases(Doubao alias) error = %v", err)
	}
	voices[voice] = apitypes.VoiceResource{
		Spec: apitypes.VoiceSpec{
			Provider: apitypes.VoiceProvider{
				Kind: apitypes.VoiceProviderKindVolcTenant,
				Id:   "other-tenant",
			},
		},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.demo", realtime, models, voices); err == nil || !strings.Contains(err.Error(), "to match model alias") {
		t.Fatalf("validateWorkflowRuntimeAliases(Doubao incompatible voice) error = %v", err)
	}
	voices[voice] = apitypes.VoiceResource{}
	tools := []apitypes.DoubaoRealtimeFunctionTool{{
		Type: apitypes.DoubaoRealtimeFunctionToolTypeFunction,
		Name: "get_weather",
	}}
	realtime.DoubaoRealtime.Tools = &tools
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.demo", realtime, models, voices); err == nil || !strings.Contains(err.Error(), "tools are unsupported") {
		t.Fatalf("validateWorkflowRuntimeAliases(Doubao tools) error = %v", err)
	}
}

func TestValidateNewWorkflowRuntimeAliases(t *testing.T) {
	t.Parallel()
	dashMode := apitypes.DashScopeTenantModelProviderDataApiModeRealtime
	dashData := apitypes.ModelProviderData{}
	if err := dashData.FromDashScopeTenantModelProviderData(apitypes.DashScopeTenantModelProviderData{
		ApiMode: &dashMode,
	}); err != nil {
		t.Fatal(err)
	}
	duplexData := apitypes.ModelProviderData{}
	if err := duplexData.FromVolcTenantModelProviderData(apitypes.VolcTenantModelProviderData{
		ApiMode: apitypes.VolcTenantModelProviderDataApiModeRealtimeDuplex,
	}); err != nil {
		t.Fatal(err)
	}
	models := map[string]apitypes.ModelResource{
		"dash": {
			Spec: apitypes.ModelSpec{
				Kind:         apitypes.ModelKindRealtime,
				Provider:     apitypes.ModelProvider{Kind: apitypes.ModelProviderKindDashscopeTenant, Id: "dash-main"},
				ProviderData: dashData,
			},
		},
		"duplex": {
			Spec: apitypes.ModelSpec{
				Kind:         apitypes.ModelKindRealtimeDuplex,
				Provider:     apitypes.ModelProvider{Kind: apitypes.ModelProviderKindVolcTenant, Id: "volc-main"},
				ProviderData: duplexData,
			},
		},
		"llm": {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindLlm}},
	}
	voice := "assistant"
	voices := map[string]apitypes.VoiceResource{
		voice: {
			Spec: apitypes.VoiceSpec{
				Provider: apitypes.VoiceProvider{
					Kind: apitypes.VoiceProviderKindDashscopeTenant,
					Id:   "dash-main",
				},
			},
		},
	}
	dash := apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverDashscopeRealtime,
		DashscopeRealtime: &apitypes.DashScopeRealtimeWorkflowSpec{
			Model: "dash",
			Voice: &voice,
		},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.dash", dash, models, voices); err != nil {
		t.Fatalf("validate DashScope realtime aliases: %v", err)
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.dash", dash, models, nil); err == nil || !strings.Contains(err.Error(), ".voice") {
		t.Fatalf("validate DashScope missing voice alias error = %v", err)
	}
	voices[voice] = apitypes.VoiceResource{
		Spec: apitypes.VoiceSpec{
			Provider: apitypes.VoiceProvider{
				Kind: apitypes.VoiceProviderKindVolcTenant,
				Id:   "volc-main",
			},
		},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.dash", dash, models, voices); err == nil || !strings.Contains(err.Error(), "to match model alias") {
		t.Fatalf("validate DashScope incompatible voice error = %v", err)
	}

	duplex := apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverDoubaoRealtimeDuplex,
		DoubaoRealtimeDuplex: &apitypes.DoubaoRealtimeDuplexWorkflowSpec{
			Model: "duplex",
			Voice: &voice,
		},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.duplex", duplex, models, voices); err != nil {
		t.Fatalf("validate Doubao duplex aliases: %v", err)
	}
	voices[voice] = apitypes.VoiceResource{
		Spec: apitypes.VoiceSpec{
			Provider: apitypes.VoiceProvider{
				Kind: apitypes.VoiceProviderKindVolcTenant,
				Id:   "other-volc-tenant",
			},
		},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.duplex", duplex, models, voices); err == nil || !strings.Contains(err.Error(), "to match model alias") {
		t.Fatalf("validate Doubao duplex incompatible voice error = %v", err)
	}

	chatModel := apitypes.EinoNode{}
	if err := chatModel.FromEinoChatModelNode(apitypes.EinoChatModelNode{
		Id:    "model",
		Type:  apitypes.EinoChatModelNodeTypeChatModel,
		Model: "llm",
	}); err != nil {
		t.Fatal(err)
	}
	eino := apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverEino,
		Eino: &apitypes.EinoWorkflowSpec{
			Graph: apitypes.EinoGraph{
				Name:     "nested-model",
				Compile:  apitypes.EinoGraphCompile{NodeTriggerMode: apitypes.EinoGraphCompileNodeTriggerModeAnyPredecessor},
				State:    apitypes.EinoState{Fields: []apitypes.EinoStateField{}},
				Nodes:    []apitypes.EinoNode{chatModel},
				Edges:    []apitypes.EinoEdge{},
				Branches: []apitypes.EinoBranch{},
				Outputs:  []apitypes.EinoOutput{},
			},
		},
	}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.eino", eino, models, voices); err != nil {
		t.Fatalf("validate Eino aliases: %v", err)
	}
	asr, defaultVoice := "speech.asr", "speech.voice"
	nodeVoices := map[string]string{"answer": defaultVoice}
	eino.Eino.VoiceAdapter = &apitypes.VoiceAdapter{
		AsrModel: &asr, DefaultVoice: &defaultVoice, NodeVoices: &nodeVoices,
	}
	models[asr] = apitypes.ModelResource{Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindLlm}}
	voices[defaultVoice] = apitypes.VoiceResource{}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.eino", eino, models, voices); err == nil || !strings.Contains(err.Error(), `want "asr"`) {
		t.Fatalf("validate Eino wrong ASR kind error = %v", err)
	}
	models[asr] = apitypes.ModelResource{Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindAsr}}
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.eino", eino, models, voices); err != nil {
		t.Fatalf("validate Eino voice aliases: %v", err)
	}
	delete(voices, defaultVoice)
	if err := validateWorkflowRuntimeAliases("workflows.collections.assistants.eino", eino, models, voices); err == nil || !strings.Contains(err.Error(), "not declared in resources.voices") {
		t.Fatalf("validate Eino missing Voice error = %v", err)
	}
}

func TestRuntimeProfileRejectsAliasesSharedAcrossResourceKinds(t *testing.T) {
	t.Parallel()
	s := &Server{DB: profileSQLTestDB(t)}
	models := map[string]apitypes.RuntimeProfileBinding{"assistant": runtimeProfileTestBinding("model-a")}
	voices := map[string]apitypes.RuntimeProfileBinding{"assistant": runtimeProfileTestBinding("voice-a")}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Id: "test-profile",
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

func TestRuntimeProfileAliasGrammar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		alias   string
		wantErr bool
	}{
		{name: "legacy", alias: "pet-chat"},
		{name: "multiple scopes", alias: "story.journey.center-earth"},
		{name: "63 bytes", alias: strings.Repeat("a", 30) + "." + strings.Repeat("b", 32)},
		{name: "empty", wantErr: true},
		{name: "64 bytes", alias: strings.Repeat("a", 31) + "." + strings.Repeat("b", 32), wantErr: true},
		{name: "leading dot", alias: ".voice", wantErr: true},
		{name: "trailing dot", alias: "raid.", wantErr: true},
		{name: "empty segment", alias: "raid..voice", wantErr: true},
		{name: "leading segment hyphen", alias: "raid.-voice", wantErr: true},
		{name: "trailing segment hyphen", alias: "raid-.voice", wantErr: true},
		{name: "underscore", alias: "story.journey_center_earth", wantErr: true},
		{name: "uppercase", alias: "Story.journey", wantErr: true},
		{name: "slash", alias: "story/journey", wantErr: true},
		{name: "internal whitespace", alias: "story. journey", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateAlias("test alias", test.alias)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateAlias(%q) error = %v, wantErr %t", test.alias, err, test.wantErr)
			}
			if test.wantErr && (err == nil || !strings.Contains(err.Error(), "1-63 bytes of dot-separated lowercase kebab-case segments")) {
				t.Fatalf("ValidateAlias(%q) error = %v, want byte and segment grammar", test.alias, err)
			}
		})
	}
}

func TestRuntimeProfileCreateAndUpdatePreserveScopedAliases(t *testing.T) {
	t.Parallel()
	profile := scopedAliasProfileForTest(t)
	server := &Server{DB: profileSQLTestDB(t)}

	response, err := server.CreateRuntimeProfile(t.Context(), adminhttp.CreateRuntimeProfileRequestObject{Body: &profile})
	if err != nil {
		t.Fatalf("CreateRuntimeProfile() error = %v", err)
	}
	created, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse)
	if !ok {
		t.Fatalf("CreateRuntimeProfile() response = %#v", response)
	}
	assertScopedProfileAliases(t, created.Spec)

	updated := profile
	voices := map[string]apitypes.RuntimeProfileBinding{
		" journey.narrator ": runtimeProfileTestBinding("journey-voice-v2"),
		"journey-narrator":   runtimeProfileTestBinding("legacy-voice"),
	}
	updated.Spec.Resources.Voices = &voices
	putResponse, err := server.PutRuntimeProfile(t.Context(), adminhttp.PutRuntimeProfileRequestObject{Id: profile.Id, Body: &updated})
	if err != nil {
		t.Fatalf("PutRuntimeProfile() error = %v", err)
	}
	put, ok := putResponse.(adminhttp.PutRuntimeProfile200JSONResponse)
	if !ok {
		t.Fatalf("PutRuntimeProfile() response = %#v", putResponse)
	}
	assertScopedProfileAliases(t, put.Spec)
	if got := (*put.Spec.Resources.Voices)["journey.narrator"].ResourceId; got != "journey-voice-v2" {
		t.Fatalf("updated journey.narrator resource_id = %q, want journey-voice-v2", got)
	}
}

func scopedAliasProfileForTest(t *testing.T) adminhttp.RuntimeProfileUpsert {
	t.Helper()
	models := map[string]apitypes.RuntimeProfileBinding{
		"journey.model":     runtimeProfileTestBinding("journey-model"),
		"reward.evaluator":  runtimeProfileTestBinding("reward-model"),
		"game.reward-model": runtimeProfileTestBinding("game-reward-model"),
	}
	voices := map[string]apitypes.RuntimeProfileBinding{
		"journey.narrator": runtimeProfileTestBinding("journey-voice"),
		"journey-narrator": runtimeProfileTestBinding("legacy-voice"),
	}
	tools := map[string]apitypes.RuntimeProfileBinding{
		"journey.tool": runtimeProfileTestBinding("journey-tool"),
	}
	var memory apitypes.RuntimeProfileMemoryBinding
	if err := json.Unmarshal([]byte(`{
		"layout_id":"journey-memory-layout",
		"driver":"mem0",
		"connection":{"type":"mem0","project_id":"project","endpoint":"https://api.mem0.ai","api_key":"key"}
	}`), &memory); err != nil {
		t.Fatalf("decode Memory binding: %v", err)
	}
	memories := map[string]apitypes.RuntimeProfileMemoryBinding{
		"journey.memory": memory,
	}

	return adminhttp.RuntimeProfileUpsert{
		Id: "scoped-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				Collections: apitypes.RuntimeProfileWorkflowCollections{
					"story.catalog": {
						"story.journey-center-earth": runtimeProfileTestBinding("journey-workflow"),
					},
				},
			},
			Resources: apitypes.RuntimeProfileResources{
				Models: &models, Voices: &voices, Tools: &tools, Memories: &memories,
			},
		},
	}
}

func assertScopedProfileAliases(t *testing.T, spec apitypes.RuntimeProfileSpec) {
	t.Helper()
	if _, ok := spec.Workflows.Collections["story.catalog"]["story.journey-center-earth"]; !ok {
		t.Fatalf("Workflow collections = %#v", spec.Workflows.Collections)
	}
	for name, aliases := range map[string][]string{
		"models": {"journey.model", "reward.evaluator", "game.reward-model"},
		"voices": {"journey.narrator", "journey-narrator"},
		"tools":  {"journey.tool"},
	} {
		var bindings *map[string]apitypes.RuntimeProfileBinding
		switch name {
		case "models":
			bindings = spec.Resources.Models
		case "voices":
			bindings = spec.Resources.Voices
		case "tools":
			bindings = spec.Resources.Tools
		}
		for _, alias := range aliases {
			if bindings == nil {
				t.Fatalf("%s bindings are nil", name)
			}
			if _, ok := (*bindings)[alias]; !ok {
				t.Fatalf("%s aliases = %#v, missing %q", name, *bindings, alias)
			}
		}
	}
	if spec.Resources.Memories == nil {
		t.Fatal("Memory bindings are nil")
	}
	if _, ok := (*spec.Resources.Memories)["journey.memory"]; !ok {
		t.Fatalf("Memory aliases = %#v", *spec.Resources.Memories)
	}
}

func TestRuntimeProfileRejectsWorkflowCollectionsDuplicatedAfterNormalization(t *testing.T) {
	t.Parallel()
	_, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{
		Id: "test-profile",
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

func TestRuntimeProfileAcceptsDefaultName(t *testing.T) {
	t.Parallel()
	s := &Server{DB: profileSQLTestDB(t)}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Id: "default",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				Collections: apitypes.RuntimeProfileWorkflowCollections{},
			},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse)
	if !ok || created.Id != "default" {
		t.Fatalf("CreateRuntimeProfile() = %#v, want RuntimeProfile/default", response)
	}
}

func TestResolveProfileReturnsPersistedSnapshotWithoutResolvingResources(t *testing.T) {
	t.Parallel()
	s := &Server{DB: profileSQLTestDB(t)}
	createProfile(t, s, "owner-profile", nil)
	s.ResolveResource = func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) {
		t.Fatal("ResolveProfile() resolved a RuntimeProfile dependency")
		return apitypes.Resource{}, errors.New("unexpected resource resolution")
	}
	profile, err := s.ResolveProfile(t.Context(), "owner-profile")
	if err != nil {
		t.Fatalf("ResolveProfile() error = %v", err)
	}
	if profile.Id != "owner-profile" || profile.Revision == "" {
		t.Fatalf("ResolveProfile() = %#v, want persisted owner-profile snapshot", profile)
	}
}

func TestResolveOwnerProfileReturnsPersistedSnapshotWithoutResolvingResources(t *testing.T) {
	t.Parallel()
	s := &Server{DB: profileSQLTestDB(t)}
	createProfile(t, s, "owner-profile", nil)
	if err := s.BindOwnerProfile(t.Context(), "peer-a", "owner-profile"); err != nil {
		t.Fatalf("BindOwnerProfile() error = %v", err)
	}
	s.ResolveResource = func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) {
		t.Fatal("ResolveOwnerProfile() resolved a RuntimeProfile dependency")
		return apitypes.Resource{}, errors.New("unexpected resource resolution")
	}
	profile, err := s.ResolveOwnerProfile(t.Context(), "peer-a")
	if err != nil {
		t.Fatalf("ResolveOwnerProfile() error = %v", err)
	}
	if profile.Id != "owner-profile" || profile.Revision == "" {
		t.Fatalf("ResolveOwnerProfile() = %#v, want persisted owner-profile snapshot", profile)
	}
}

func TestResolveOwnerProfileDoesNotLockOtherOwners(t *testing.T) {
	db := profileSQLTestDB(t)
	server := &Server{DB: db}
	createProfile(t, server, "shared-profile", nil)
	for _, owner := range []string{"peer-a", "peer-b"} {
		if err := server.BindOwnerProfile(t.Context(), owner, "shared-profile"); err != nil {
			t.Fatal(err)
		}
	}
	release, err := server.ownerLocks.Acquire(t.Context(), "peer-a")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	got, err := server.ResolveOwnerProfile(ctx, "peer-b")
	if err != nil || got.Id != "shared-profile" {
		t.Fatalf("independent owner = %#v, %v", got, err)
	}
}

func BenchmarkResolveOwnerProfile(b *testing.B) {
	s := &Server{DB: profileSQLTestDB(b)}
	createProfile(b, s, "shared-profile", nil)
	owners := []string{"peer-0", "peer-1", "peer-2", "peer-3", "peer-4", "peer-5", "peer-6", "peer-7"}
	for _, owner := range owners {
		if err := s.BindOwnerProfile(b.Context(), owner, "shared-profile"); err != nil {
			b.Fatalf("BindOwnerProfile(%q) error = %v", owner, err)
		}
	}
	s.ResolveResource = func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) {
		b.Fatal("ResolveOwnerProfile() resolved a RuntimeProfile dependency")
		return apitypes.Resource{}, errors.New("unexpected resource resolution")
	}
	b.ResetTimer()
	var nextOwner atomic.Uint64
	b.RunParallel(func(pb *testing.PB) {
		owner := owners[(nextOwner.Add(1)-1)%uint64(len(owners))]
		for pb.Next() {
			if _, err := s.ResolveOwnerProfile(b.Context(), owner); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestOwnerProfileBindingSurvivesConnectionLifetimeAndLoadsCurrentRevision(t *testing.T) {
	t.Parallel()
	s := &Server{DB: profileSQLTestDB(t)}
	createProfile(t, s, "owner-profile", nil)
	if err := s.BindOwnerProfile(t.Context(), "peer-a", " owner-profile "); err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("BindOwnerProfile(whitespace ID) error = %v", err)
	}
	if _, err := s.ResolveProfile(t.Context(), " owner-profile "); err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("ResolveProfile(whitespace ID) error = %v", err)
	}
	if err := s.BindOwnerProfile(t.Context(), " peer-a ", "owner-profile"); err != nil {
		t.Fatalf("BindOwnerProfile() error = %v", err)
	}
	first, err := s.ResolveOwnerProfile(t.Context(), "peer-a")
	if err != nil || first.Id != "owner-profile" {
		t.Fatalf("ResolveOwnerProfile() = %#v, %v", first, err)
	}
	updated := adminhttp.RuntimeProfileUpsert{Id: first.Id, Spec: first.Spec}
	updated.Spec.Workflows.Collections = apitypes.RuntimeProfileWorkflowCollections{
		"assistants": {"chat": runtimeProfileTestBinding("chat-v2")},
	}
	response, err := s.PutRuntimeProfile(t.Context(), adminhttp.PutRuntimeProfileRequestObject{Id: first.Id, Body: &updated})
	if err != nil {
		t.Fatalf("PutRuntimeProfile() error = %v", err)
	}
	if _, ok := response.(adminhttp.PutRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("PutRuntimeProfile() response = %#v", response)
	}
	current, err := s.ResolveOwnerProfile(t.Context(), "peer-a")
	if err != nil {
		t.Fatalf("ResolveOwnerProfile(updated) error = %v", err)
	}
	if current.Spec.Workflows.Collections["assistants"]["chat"].ResourceId != "chat-v2" || current.Revision == first.Revision {
		t.Fatalf("ResolveOwnerProfile(updated) = %#v, initial revision %q", current, first.Revision)
	}
}

func TestBindOwnerProfileAndCommitRestoresPreviousBinding(t *testing.T) {
	t.Parallel()
	s := &Server{DB: profileSQLTestDB(t)}
	createProfile(t, s, "profile-a", nil)
	createProfile(t, s, "profile-b", nil)
	if err := s.BindOwnerProfile(t.Context(), "peer-a", "profile-a"); err != nil {
		t.Fatalf("BindOwnerProfile(profile-a) error = %v", err)
	}
	commitErr := errors.New("dependent commit failed")
	err := s.BindOwnerProfileAndCommit(t.Context(), "peer-a", "profile-b", func() error {
		return commitErr
	})
	if !errors.Is(err, commitErr) {
		t.Fatalf("BindOwnerProfileAndCommit() error = %v, want %v", err, commitErr)
	}
	current, err := s.ResolveOwnerProfile(t.Context(), "peer-a")
	if err != nil || current.Id != "profile-a" {
		t.Fatalf("ResolveOwnerProfile() = %#v, %v, want profile-a", current, err)
	}

	err = s.BindOwnerProfileAndCommit(t.Context(), "peer-b", "profile-b", func() error {
		return commitErr
	})
	if !errors.Is(err, commitErr) {
		t.Fatalf("BindOwnerProfileAndCommit(new owner) error = %v, want %v", err, commitErr)
	}
	if _, err := s.ResolveOwnerProfile(t.Context(), "peer-b"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ResolveOwnerProfile(new owner) error = %v, want not found", err)
	}
}

func TestBindOwnerProfileDoesNotBlockIndependentOwnerAndProfile(t *testing.T) {
	server := &Server{DB: profileSQLTestDB(t)}
	createProfile(t, server, "profile-a", nil)
	createProfile(t, server, "profile-b", nil)
	firstEntered := make(chan struct{})
	firstRelease := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- server.BindOwnerProfileAndCommit(t.Context(), "peer-a", "profile-a", func() error {
			close(firstEntered)
			<-firstRelease
			return nil
		})
	}()
	<-firstEntered

	sameDone := make(chan error, 1)
	go func() {
		_, err := server.ResolveOwnerProfile(t.Context(), "peer-a")
		sameDone <- err
	}()
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- server.BindOwnerProfile(t.Context(), "peer-b", "profile-b")
	}()
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("independent BindOwnerProfile() error = %v", err)
		}
	case <-time.After(time.Second):
		close(firstRelease)
		t.Fatal("independent owner/profile binding could not commit while first owner callback was blocked")
	}
	select {
	case err := <-sameDone:
		close(firstRelease)
		t.Fatalf("same-owner ResolveOwnerProfile completed before commit: %v", err)
	default:
	}

	close(firstRelease)
	if err := <-firstDone; err != nil {
		t.Fatalf("first BindOwnerProfileAndCommit() error = %v", err)
	}
	if err := <-sameDone; err != nil {
		t.Fatalf("same-owner ResolveOwnerProfile() error = %v", err)
	}
}

func TestBindOwnerProfileAndCommitRestoresBindingAfterRequestCancellation(t *testing.T) {
	t.Parallel()
	s := &Server{DB: profileSQLTestDB(t)}
	createProfile(t, s, "profile-a", nil)
	createProfile(t, s, "profile-b", nil)
	if err := s.BindOwnerProfile(t.Context(), "peer-a", "profile-a"); err != nil {
		t.Fatalf("BindOwnerProfile(profile-a) error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	commitErr := errors.New("dependent commit canceled")
	err := s.BindOwnerProfileAndCommit(ctx, "peer-a", "profile-b", func() error {
		cancel()
		return commitErr
	})
	if !errors.Is(err, commitErr) {
		t.Fatalf("BindOwnerProfileAndCommit() error = %v, want %v", err, commitErr)
	}
	current, err := s.ResolveOwnerProfile(t.Context(), "peer-a")
	if err != nil || current.Id != "profile-a" {
		t.Fatalf("ResolveOwnerProfile() = %#v, %v, want profile-a", current, err)
	}
}

func createProfile(t testing.TB, s *Server, name string, models map[string]string) {
	t.Helper()
	previousResolver := s.ResolveResource
	s.ResolveResource = func(ctx context.Context, kind apitypes.ResourceKind, resourceName string) (apitypes.Resource, error) {
		if kind == apitypes.ResourceKindWorkflow {
			spec := apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &apitypes.EinoWorkflowSpec{}}
			var resource apitypes.Resource
			err := resource.FromWorkflowResource(apitypes.WorkflowResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.WorkflowResourceKindWorkflow,
				Metadata:   apitypes.ResourceMetadata{Id: resourceName},
				Spec:       spec,
			})
			return resource, err
		}
		if kind == apitypes.ResourceKindModel {
			var resource apitypes.Resource
			err := resource.FromModelResource(apitypes.ModelResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.ModelResourceKindModel,
				Metadata:   apitypes.ResourceMetadata{Id: resourceName},
				Spec:       apitypes.ModelSpec{Kind: apitypes.ModelKindLlm},
			})
			return resource, err
		}
		if previousResolver != nil {
			return previousResolver(ctx, kind, resourceName)
		}
		return apitypes.Resource{}, sql.ErrNoRows
	}
	resources := apitypes.RuntimeProfileResources{}
	if models != nil {
		bindings := make(map[string]apitypes.RuntimeProfileBinding, len(models))
		for alias, resourceID := range models {
			bindings[alias] = runtimeProfileTestBinding(resourceID)
		}
		resources.Models = &bindings
	}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Id: name, Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				Collections: apitypes.RuntimeProfileWorkflowCollections{},
			},
			Resources: resources,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("create profile response = %#v", response)
	}
}

func TestNormalizeMemoryBindingEnforcesStrictDriverConnectionOneOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "Flowcraft Redis 8", raw: `{"layout_id":"pet-memory","driver":"flowcraft","connection":{"type":"flowcraft_redis8","url":"redis://redis:6379/0"}}`},
		{name: "Flowcraft BBH", raw: `{"layout_id":"pet-memory","driver":"flowcraft","connection":{"type":"flowcraft_bbh"}}`},
		{name: "opaque canonical layout ID", raw: `{"layout_id":"1234opaque","driver":"flowcraft","connection":{"type":"flowcraft_redis8","url":"rediss://redis.example:6379/0","tls_ca_file":"/etc/ssl/redis-ca.pem"}}`},
		{name: "Flowcraft object store", raw: `{"layout_id":"pet-memory","driver":"flowcraft","connection":{"type":"flowcraft_object_store","directory":"/var/lib/gizclaw/memory"}}`},
		{name: "Flowcraft PostgreSQL", raw: `{"layout_id":"pet-memory","driver":"flowcraft","connection":{"type":"flowcraft_postgresql","dsn":"postgres://gizclaw:secret@db/memory"}}`},
		{name: "Mem0", raw: `{"layout_id":"pet-memory","driver":"mem0","connection":{"type":"mem0","project_id":"project","endpoint":"https://api.mem0.ai","api_key":"key","poll_interval":"500ms"}}`},
		{name: "Volc Mem0", raw: `{"layout_id":"pet-memory","driver":"volc_mem0","connection":{"type":"volc_mem0","memory_project_id":"project","endpoint":"https://open.volcengineapi.com","api_key":"key"}}`},
		{name: "driver mismatch", raw: `{"layout_id":"pet-memory","driver":"mem0","connection":{"type":"flowcraft_redis8","url":"redis://redis:6379/0"}}`, wantErr: "cannot use connection type"},
		{name: "BBH driver mismatch", raw: `{"layout_id":"pet-memory","driver":"mem0","connection":{"type":"flowcraft_bbh"}}`, wantErr: "cannot use connection type"},
		{name: "invalid Redis URL", raw: `{"layout_id":"pet-memory","driver":"flowcraft","connection":{"type":"flowcraft_redis8","url":"http://redis:6379"}}`, wantErr: "redis or rediss URL"},
		{name: "non-numeric Redis database", raw: `{"layout_id":"pet-memory","driver":"flowcraft","connection":{"type":"flowcraft_redis8","url":"redis://redis:6379/not-a-database"}}`, wantErr: "valid single-endpoint"},
		{name: "Redis TLS verification disabled", raw: `{"layout_id":"pet-memory","driver":"flowcraft","connection":{"type":"flowcraft_redis8","url":"rediss://redis:6379/0?skip_verify=true"}}`, wantErr: "certificate verification"},
		{name: "Redis CA without TLS", raw: `{"layout_id":"pet-memory","driver":"flowcraft","connection":{"type":"flowcraft_redis8","url":"redis://redis:6379","tls_ca_file":"/ca.pem"}}`, wantErr: "requires a rediss URL"},
		{name: "missing Mem0 key", raw: `{"layout_id":"pet-memory","driver":"mem0","connection":{"type":"mem0","project_id":"project","endpoint":"https://api.mem0.ai","api_key":""}}`, wantErr: "project_id and api_key"},
		{name: "invalid endpoint", raw: `{"layout_id":"pet-memory","driver":"mem0","connection":{"type":"mem0","project_id":"project","endpoint":"mem0.local","api_key":"key"}}`, wantErr: "absolute http or https URL"},
		{name: "endpoint userinfo", raw: `{"layout_id":"pet-memory","driver":"mem0","connection":{"type":"mem0","project_id":"project","endpoint":"https://user:pass@api.mem0.ai","api_key":"key"}}`, wantErr: "userinfo, query, or fragment"},
		{name: "endpoint query", raw: `{"layout_id":"pet-memory","driver":"mem0","connection":{"type":"mem0","project_id":"project","endpoint":"https://api.mem0.ai?tenant=other","api_key":"key"}}`, wantErr: "userinfo, query, or fragment"},
		{name: "endpoint fragment", raw: `{"layout_id":"pet-memory","driver":"volc_mem0","connection":{"type":"volc_mem0","memory_project_id":"project","endpoint":"https://open.volcengineapi.com#other","api_key":"key"}}`, wantErr: "userinfo, query, or fragment"},
		{name: "invalid poll interval", raw: `{"layout_id":"pet-memory","driver":"volc_mem0","connection":{"type":"volc_mem0","memory_project_id":"project","endpoint":"https://open.volcengineapi.com","api_key":"key","poll_interval":"0s"}}`, wantErr: "positive duration"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var binding apitypes.RuntimeProfileMemoryBinding
			if err := json.Unmarshal([]byte(test.raw), &binding); err != nil {
				t.Fatal(err)
			}
			_, err := normalizeMemoryBinding(binding)
			if test.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("normalizeMemoryBinding() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestNormalizeMemoryBindingTrimsConnectionValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		raw   string
		wants []string
	}{
		{
			name:  "Flowcraft object store",
			raw:   `{"layout_id":"pet-memory","driver":"flowcraft","connection":{"type":"flowcraft_object_store","directory":" /var/lib/gizclaw/memory "}}`,
			wants: []string{`"directory":"/var/lib/gizclaw/memory"`},
		},
		{
			name:  "Flowcraft PostgreSQL",
			raw:   `{"layout_id":"pet-memory","driver":"flowcraft","connection":{"type":"flowcraft_postgresql","dsn":" postgres://db/memory "}}`,
			wants: []string{`"dsn":"postgres://db/memory"`},
		},
		{
			name:  "Mem0",
			raw:   `{"layout_id":"pet-memory","driver":"mem0","connection":{"type":"mem0","project_id":" project ","endpoint":" https://api.mem0.ai ","api_key":" key ","poll_interval":" 500ms "}}`,
			wants: []string{`"project_id":"project"`, `"endpoint":"https://api.mem0.ai"`, `"api_key":"key"`, `"poll_interval":"500ms"`},
		},
		{
			name:  "Volc Mem0",
			raw:   `{"layout_id":"pet-memory","driver":"volc_mem0","connection":{"type":"volc_mem0","memory_project_id":" project ","endpoint":" https://mem0.volc.example ","api_key":" key "}}`,
			wants: []string{`"memory_project_id":"project"`, `"endpoint":"https://mem0.volc.example"`, `"api_key":"key"`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var binding apitypes.RuntimeProfileMemoryBinding
			if err := json.Unmarshal([]byte(test.raw), &binding); err != nil {
				t.Fatal(err)
			}
			normalized, err := normalizeMemoryBinding(binding)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(normalized.Connection)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range test.wants {
				if !strings.Contains(string(raw), want) {
					t.Fatalf("normalized connection = %s, want %s", raw, want)
				}
			}
		})
	}
}

func TestRuntimeProfileRejectsMissingMemoryLayoutWithoutPersistingRevision(t *testing.T) {
	t.Parallel()
	store := profileSQLTestDB(t)
	server := &Server{
		DB: store,
		ResolveResource: func(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
			if kind == apitypes.ResourceKindMemoryLayout {
				return apitypes.Resource{}, sql.ErrNoRows
			}
			if kind != apitypes.ResourceKindWorkflow {
				return apitypes.Resource{}, sql.ErrNoRows
			}
			spec := apitypes.WorkflowSpec{
				Driver: apitypes.WorkflowDriverEino,
				Eino:   &apitypes.EinoWorkflowSpec{},
			}
			var resource apitypes.Resource
			err := resource.FromWorkflowResource(apitypes.WorkflowResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.WorkflowResourceKindWorkflow,
				Metadata:   apitypes.ResourceMetadata{Id: name},
				Spec:       spec,
			})
			return resource, err
		},
	}
	var binding apitypes.RuntimeProfileMemoryBinding
	if err := json.Unmarshal([]byte(`{
		"layout_id":"missing-layout",
		"driver":"mem0",
		"connection":{
			"type":"mem0",
			"project_id":"project",
			"endpoint":"https://api.mem0.ai",
			"api_key":"key"
		}
	}`), &binding); err != nil {
		t.Fatal(err)
	}
	memories := map[string]apitypes.RuntimeProfileMemoryBinding{"pet-memory": binding}
	response, err := server.CreateRuntimeProfile(t.Context(), adminhttp.CreateRuntimeProfileRequestObject{
		Body: &adminhttp.RuntimeProfileUpsert{
			Id: "default",
			Spec: apitypes.RuntimeProfileSpec{
				Workflows: apitypes.RuntimeProfileWorkflows{
					Collections: apitypes.RuntimeProfileWorkflowCollections{},
				},
				Resources: apitypes.RuntimeProfileResources{Memories: &memories},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	invalid, ok := response.(adminhttp.CreateRuntimeProfile400JSONResponse)
	if !ok || !strings.Contains(invalid.Error.Message, "missing-layout") {
		t.Fatalf("CreateRuntimeProfile() = %#v, want missing MemoryLayout rejection", response)
	}
	if _, err := GetProfile(t.Context(), store, "default"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("persisted profile after validation failure: %v", err)
	}
}

func runtimeProfileTestBinding(resourceID string) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{ResourceId: resourceID, I18n: map[string]apitypes.RuntimeProfileI18nText{
		"en": {DisplayName: "Test"}, "zh-CN": {DisplayName: "测试"},
	}}
}

func runtimeProfileTestFlowcraftSpec(t *testing.T, modelAlias, voiceAlias string) *apitypes.FlowcraftWorkflowSpec {
	t.Helper()
	publish := true
	var node apitypes.FlowcraftNode
	if err := node.FromFlowcraftLLMNode(apitypes.FlowcraftLLMNode{
		Id:      "answer",
		Type:    apitypes.FlowcraftLLMNodeTypeLlm,
		Publish: &publish,
		Config:  apitypes.FlowcraftLLMNodeConfig{Model: modelAlias},
	}); err != nil {
		t.Fatal(err)
	}
	return &apitypes.FlowcraftWorkflowSpec{
		Graph:        apitypes.FlowcraftGraph{Name: "Assistant", Entry: "answer", Nodes: []apitypes.FlowcraftNode{node}},
		VoiceAdapter: &apitypes.VoiceAdapter{DefaultVoice: &voiceAlias},
	}
}
