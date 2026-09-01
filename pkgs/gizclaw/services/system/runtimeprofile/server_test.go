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

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestRegistrationTokenIsReadableAndIndexedByHash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	store := kv.NewMemory(nil)
	s := &Server{
		Store: store,
		Now:   func() time.Time { return now },
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
	stored, err := store.Get(ctx, tokenKey(created.Id))
	if err != nil {
		t.Fatal(err)
	}
	var persisted apitypes.RegistrationToken
	if err := json.Unmarshal(stored, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != apitypes.RegistrationToken(created) {
		t.Fatalf("persisted = %#v, want %#v", persisted, created)
	}
	indexedName, err := store.Get(ctx, tokenHashKey(tokenDigest(created.Token)))
	if err != nil {
		t.Fatal(err)
	}
	if string(indexedName) != created.Id {
		t.Fatalf("hash index = %q, want %q", indexedName, created.Id)
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
	store := kv.NewMemory(nil)
	s := &Server{Store: store}
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
	if _, err := s.ResolveRegistration(ctx, created.Token); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("resolve after delete error = %v, want not found", err)
	}
}

func TestPutRegistrationTokenReplacesTokenAndHashIndexAtomically(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	s := &Server{Store: store, Now: func() time.Time { return now }}
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
	if _, err := s.ResolveRegistration(ctx, "old-token"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("ResolveRegistration(old-token) error = %v, want not found", err)
	}
	if _, err := s.ResolveRegistration(ctx, "new-token"); err != nil {
		t.Fatalf("ResolveRegistration(new-token) error = %v", err)
	}
	if _, err := store.Get(ctx, tokenHashKey(tokenDigest("old-token"))); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("old hash index error = %v, want not found", err)
	}
}

type failingBatchMutateStore struct {
	kv.Store
}

func (f failingBatchMutateStore) BatchMutate(context.Context, []kv.Entry, []kv.Key) error {
	return errors.New("injected BatchMutate failure")
}

type blockingProfileGetStore struct {
	kv.Store
	key     string
	enabled atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (s *blockingProfileGetStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	if s.enabled.Load() && key.String() == s.key {
		s.entered <- struct{}{}
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.Store.Get(ctx, key)
}

func (s *blockingProfileGetStore) CreateIfAbsent(ctx context.Context, guard kv.Entry, entries []kv.Entry) ([]byte, bool, error) {
	return kv.CreateIfAbsent(ctx, s.Store, guard, entries)
}

func TestPutRegistrationTokenStoreFailurePreservesRecordAndIndexes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	s := &Server{Store: store}
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
	s.Store = failingBatchMutateStore{Store: store}
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
	if _, err := s.ResolveRegistration(ctx, "new-token"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("ResolveRegistration(new-token) error = %v, want not found", err)
	}
}

func TestRegistrationTokenCollisionLeavesBothResourcesUnchanged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	s := &Server{Store: store}
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
		Store: kv.NewMemory(nil),
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
		Store: kv.NewMemory(nil),
		ResolveResource: func(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
			if kind != apitypes.ResourceKindFirmware || name != "h106" {
				return apitypes.Resource{}, kv.ErrNotFound
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
	store := kv.NewMemory(nil)
	s := &Server{Store: store}
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
		Store: kv.NewMemory(nil),
		ResolveResource: func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) {
			return apitypes.Resource{}, kv.ErrNotFound
		},
	}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Id: "pet-runtime",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{System: runtimeProfileTestSystemWorkflows(), Collections: apitypes.RuntimeProfileWorkflowCollections{
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

func TestNormalizeProfileRequiresExactSystemWorkflowIDs(t *testing.T) {
	t.Parallel()
	base := adminhttp.RuntimeProfileUpsert{
		Id: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{
					FriendChatroom: "chatroom",
					GroupChatroom:  "chatroom",
					Pet:            "pet-care",
				},
				Collections: apitypes.RuntimeProfileWorkflowCollections{},
			},
		},
	}
	normalized, err := normalizeProfile(base, "")
	if err != nil {
		t.Fatalf("normalizeProfile() error = %v", err)
	}
	if got := normalized.Spec.Workflows.System; got.FriendChatroom != "chatroom" || got.GroupChatroom != "chatroom" || got.Pet != "pet-care" {
		t.Fatalf("normalized system Workflows = %#v", got)
	}
	withWhitespace := base
	withWhitespace.Spec.Workflows.System.FriendChatroom = " chatroom "
	if _, err := normalizeProfile(withWhitespace, ""); err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("normalizeProfile(whitespace ID) error = %v", err)
	}
	for _, field := range []string{"friend_chatroom", "group_chatroom", "pet"} {
		invalid := base
		switch field {
		case "friend_chatroom":
			invalid.Spec.Workflows.System.FriendChatroom = " "
		case "group_chatroom":
			invalid.Spec.Workflows.System.GroupChatroom = " "
		case "pet":
			invalid.Spec.Workflows.System.Pet = " "
		}
		if _, err := normalizeProfile(invalid, ""); err == nil || !strings.Contains(err.Error(), "workflows.system."+field) {
			t.Fatalf("normalizeProfile(empty %s) error = %v", field, err)
		}
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
				Metadata:   apitypes.ResourceMetadata{Id: "wrong-kind"},
			})
			return resource, err
		},
	}
	models := map[string]apitypes.RuntimeProfileBinding{"asr-model": runtimeProfileTestBinding("wrong-kind")}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Id: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{System: runtimeProfileTestSystemWorkflows(), Collections: apitypes.RuntimeProfileWorkflowCollections{}},
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

func TestValidateChatroomRuntimeAliasesRequiresASRWhenTranscriptionIsEnabled(t *testing.T) {
	t.Parallel()
	enabled := true
	workflow := apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverChatroom,
		Chatroom: &apitypes.ChatRoomWorkflowSpec{
			History:    apitypes.ChatRoomWorkflowHistorySpec{},
			Transcript: &apitypes.ChatRoomWorkflowTranscriptSpec{Enabled: &enabled},
		},
	}
	if err := validateWorkflowRuntimeAliases("workflows.system.friend_chatroom", workflow, nil, nil); err == nil || !strings.Contains(err.Error(), "asr_model is required") {
		t.Fatalf("validateWorkflowRuntimeAliases(missing ASR alias) error = %v", err)
	}
	asr := "asr"
	workflow.Chatroom.Transcript.AsrModel = &asr
	models := map[string]apitypes.ModelResource{"asr": {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindLlm}}}
	if err := validateWorkflowRuntimeAliases("workflows.system.friend_chatroom", workflow, models, nil); err == nil || !strings.Contains(err.Error(), `want "asr"`) {
		t.Fatalf("validateWorkflowRuntimeAliases(wrong ASR kind) error = %v", err)
	}
	models["asr"] = apitypes.ModelResource{Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindAsr}}
	if err := validateWorkflowRuntimeAliases("workflows.system.friend_chatroom", workflow, models, nil); err != nil {
		t.Fatalf("validateWorkflowRuntimeAliases(valid ASR alias) error = %v", err)
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

func TestValidatePetRuntimeAliases(t *testing.T) {
	t.Parallel()
	pet := apitypes.PetWorkflowSpec{
		Driver:    apitypes.ReusableWorkflowDriverFlowcraft,
		Flowcraft: runtimeProfileTestFlowcraftSpec(t, "pet-chat", "pet-voice"),
	}
	workflow := apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverPet, Pet: &pet}
	models := map[string]apitypes.ModelResource{
		"pet-chat": {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindLlm}},
	}
	if err := validateWorkflowRuntimeAliases("workflows.system.pet", workflow, models, nil); err == nil || !strings.Contains(err.Error(), "pet-voice") {
		t.Fatalf("validateWorkflowRuntimeAliases(missing nested voice) error = %v", err)
	}
	voices := map[string]apitypes.VoiceResource{"pet-voice": {}}
	if err := validateWorkflowRuntimeAliases("workflows.system.pet", workflow, models, voices); err != nil {
		t.Fatalf("validateWorkflowRuntimeAliases(valid nested aliases) error = %v", err)
	}
}

func TestPetGameplayValidatesConfiguredRewardModels(t *testing.T) {
	t.Parallel()
	pet := validPetGameplaySpecForTest()
	models := map[string]apitypes.ModelResource{}
	if err := validatePetRewardModels(pet, models); err != nil {
		t.Fatalf("validatePetRewardModels() error = %v", err)
	}
}

func TestRuntimeProfileRejectsAliasesSharedAcrossResourceKinds(t *testing.T) {
	t.Parallel()
	s := &Server{Store: kv.NewMemory(nil)}
	models := map[string]apitypes.RuntimeProfileBinding{"assistant": runtimeProfileTestBinding("model-a")}
	voices := map[string]apitypes.RuntimeProfileBinding{"assistant": runtimeProfileTestBinding("voice-a")}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Id: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{System: runtimeProfileTestSystemWorkflows(), Collections: apitypes.RuntimeProfileWorkflowCollections{}},
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
	server := &Server{Store: kv.NewMemory(nil)}
	t.Cleanup(func() { _ = server.Store.Close() })

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
	petDefs := map[string]apitypes.RuntimeProfileBinding{
		"pet-care.definition": runtimeProfileTestBinding("pet-definition"),
	}
	gameDefs := map[string]apitypes.RuntimeProfileBinding{
		"journey.game": runtimeProfileTestBinding("journey-game"),
	}
	badgeDefs := map[string]apitypes.RuntimeProfileBinding{
		"reward.science": runtimeProfileTestBinding("science-badge"),
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

	pet := validPetGameplaySpecForTest()
	pet.Games = map[string]apitypes.RuntimeProfileGameSpec{
		"journey.game": {
			EnergyCost: 10,
			Reward: apitypes.RuntimeProfileGameRewardSpec{
				Model: "game.reward-model", Prompt: "Evaluate the game result.", PetExpMax: 10, BadgeExpMaxPerBadge: 5,
			},
		},
	}
	pool := []apitypes.RuntimeProfilePetPoolEntry{{PetDef: "pet-care.definition", Weight: 1}}
	reward := validWorkspaceRewardProfileForTest().Spec.Gameplay.WorkspaceReward
	reward.Evaluation.Model = "reward.evaluator"
	rewardBadges := map[string]apitypes.RuntimeProfileWorkspaceRewardBadgeSpec{
		"reward.science": {MaxExpPerWindow: 5},
	}
	reward.Badges = &rewardBadges

	return adminhttp.RuntimeProfileUpsert{
		Id: "scoped-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: runtimeProfileTestSystemWorkflows(),
				Collections: apitypes.RuntimeProfileWorkflowCollections{
					"story.catalog": {
						"story.journey-center-earth": runtimeProfileTestBinding("journey-workflow"),
					},
				},
			},
			Resources: apitypes.RuntimeProfileResources{
				Models: &models, Voices: &voices, Tools: &tools, PetDefs: &petDefs,
				GameDefs: &gameDefs, BadgeDefs: &badgeDefs, Memories: &memories,
			},
			Gameplay: &apitypes.RuntimeProfileGameplaySpec{
				Adoption: &apitypes.RuntimeProfileAdoptionSpec{Pool: &pool},
				Pet:      &pet, WorkspaceReward: reward,
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
		"models":     {"journey.model", "reward.evaluator", "game.reward-model"},
		"voices":     {"journey.narrator", "journey-narrator"},
		"tools":      {"journey.tool"},
		"pet_defs":   {"pet-care.definition"},
		"game_defs":  {"journey.game"},
		"badge_defs": {"reward.science"},
	} {
		var bindings *map[string]apitypes.RuntimeProfileBinding
		switch name {
		case "models":
			bindings = spec.Resources.Models
		case "voices":
			bindings = spec.Resources.Voices
		case "tools":
			bindings = spec.Resources.Tools
		case "pet_defs":
			bindings = spec.Resources.PetDefs
		case "game_defs":
			bindings = spec.Resources.GameDefs
		case "badge_defs":
			bindings = spec.Resources.BadgeDefs
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
	if spec.Gameplay == nil || spec.Gameplay.Adoption == nil || spec.Gameplay.Adoption.Pool == nil ||
		(*spec.Gameplay.Adoption.Pool)[0].PetDef != "pet-care.definition" ||
		spec.Gameplay.Pet == nil || spec.Gameplay.Pet.Games["journey.game"].Reward.Model != "game.reward-model" ||
		spec.Gameplay.WorkspaceReward == nil || spec.Gameplay.WorkspaceReward.Evaluation.Model != "reward.evaluator" {
		t.Fatalf("gameplay dotted references = %#v", spec.Gameplay)
	}
	if _, ok := (*spec.Gameplay.WorkspaceReward.Badges)["reward.science"]; !ok {
		t.Fatalf("workspace reward Badge aliases = %#v", *spec.Gameplay.WorkspaceReward.Badges)
	}
}

func TestRuntimeProfileRejectsWorkflowCollectionsDuplicatedAfterNormalization(t *testing.T) {
	t.Parallel()
	_, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{
		Id: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
			System: runtimeProfileTestSystemWorkflows(),
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
	pool := []apitypes.RuntimeProfilePetPoolEntry{{PetDef: "missing", Weight: 1}}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Id: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{System: runtimeProfileTestSystemWorkflows(), Collections: apitypes.RuntimeProfileWorkflowCollections{}},
			Resources: apitypes.RuntimeProfileResources{PetDefs: &petDefs},
			Gameplay:  &apitypes.RuntimeProfileGameplaySpec{Adoption: &apitypes.RuntimeProfileAdoptionSpec{Pool: &pool}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile400JSONResponse); !ok {
		t.Fatalf("response = %#v, want undeclared adoption PetDef rejection", response)
	}
}

func TestRuntimeProfileNormalizesWorkspaceRewardPolicy(t *testing.T) {
	t.Parallel()
	upsert := validWorkspaceRewardProfileForTest()
	normalized, err := normalizeProfile(upsert, "")
	if err != nil {
		t.Fatalf("normalizeProfile() error = %v", err)
	}
	reward := normalized.Spec.Gameplay.WorkspaceReward
	if reward == nil || !reward.Enabled {
		t.Fatalf("workspace reward = %#v", reward)
	}
	if reward.Debounce.QuietPeriod != "1m0s" ||
		reward.Debounce.MaxWindowAge != "10m0s" ||
		reward.RollingBudget.Period != "24h0m0s" {
		t.Fatalf("normalized durations = %#v, %#v", reward.Debounce, reward.RollingBudget)
	}
	if got := *reward.WorkspaceKinds; len(got) != 2 ||
		got[0] != apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKindsDirectChatroom ||
		got[1] != apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKindsWorkflow {
		t.Fatalf("normalized workspace kinds = %#v", got)
	}
	pointsOnly := validWorkspaceRewardProfileForTest()
	emptyBadges := map[string]apitypes.RuntimeProfileWorkspaceRewardBadgeSpec{}
	pointsOnly.Spec.Gameplay.WorkspaceReward.Badges = &emptyBadges
	if _, err := normalizeProfile(pointsOnly, ""); err != nil {
		t.Fatalf("normalizeProfile(points only) error = %v", err)
	}

	disabled := upsert
	disabled.Spec.Gameplay.WorkspaceReward.Enabled = false
	disabledProfile, err := normalizeProfile(disabled, "")
	if err != nil {
		t.Fatalf("normalizeProfile(disabled) error = %v", err)
	}
	if got := disabledProfile.Spec.Gameplay.WorkspaceReward; got == nil || got.Enabled ||
		got.Debounce != nil || got.Evaluation != nil {
		t.Fatalf("disabled workspace reward = %#v, want canonical disabled policy", got)
	}
}

func TestRuntimeProfileRejectsInvalidWorkspaceRewardPolicy(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*apitypes.RuntimeProfileWorkspaceRewardSpec){
		"incomplete": func(reward *apitypes.RuntimeProfileWorkspaceRewardSpec) {
			reward.Transcript = nil
		},
		"duplicate kind": func(reward *apitypes.RuntimeProfileWorkspaceRewardSpec) {
			kinds := []apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKinds{
				apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKindsWorkflow,
				apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKindsWorkflow,
			}
			reward.WorkspaceKinds = &kinds
		},
		"window before quiet": func(reward *apitypes.RuntimeProfileWorkspaceRewardSpec) {
			reward.Debounce.MaxWindowAge = "30s"
		},
		"score bounds": func(reward *apitypes.RuntimeProfileWorkspaceRewardSpec) {
			reward.Evaluation.QualifyingScore = 101
		},
		"tier order": func(reward *apitypes.RuntimeProfileWorkspaceRewardSpec) {
			reward.Points.Tiers = append(reward.Points.Tiers,
				apitypes.RuntimeProfileWorkspaceRewardPointsTier{MinScore: 80, Delta: 20})
		},
		"unknown badge": func(reward *apitypes.RuntimeProfileWorkspaceRewardSpec) {
			badges := map[string]apitypes.RuntimeProfileWorkspaceRewardBadgeSpec{
				"unknown": {MaxExpPerWindow: 5},
			}
			reward.Badges = &badges
		},
		"unbounded period": func(reward *apitypes.RuntimeProfileWorkspaceRewardSpec) {
			reward.RollingBudget.Period = "8761h"
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			upsert := validWorkspaceRewardProfileForTest()
			mutate(upsert.Spec.Gameplay.WorkspaceReward)
			if _, err := normalizeProfile(upsert, ""); err == nil {
				t.Fatal("normalizeProfile() succeeded")
			}
		})
	}
}

func TestRuntimeProfileValidatesWorkspaceRewardResources(t *testing.T) {
	t.Parallel()
	normalized, err := normalizeProfile(validWorkspaceRewardProfileForTest(), "")
	if err != nil {
		t.Fatalf("normalizeProfile() error = %v", err)
	}
	rewardPrompt := "Reward scientific reasoning."
	for name, test := range map[string]struct {
		modelKind    apitypes.ModelKind
		rewardPrompt *string
		wantError    string
	}{
		"valid generic LLM alias": {
			modelKind: apitypes.ModelKindLlm, rewardPrompt: &rewardPrompt,
		},
		"wrong model kind": {
			modelKind: apitypes.ModelKindAsr, rewardPrompt: &rewardPrompt,
			wantError: `want "llm"`,
		},
		"missing Badge prompt": {
			modelKind: apitypes.ModelKindLlm,
			wantError: "requires BadgeDef reward_prompt",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := &Server{
				ResolveResource: workspaceRewardResourceResolverForTest(
					t,
					test.modelKind,
					test.rewardPrompt,
				),
			}
			err := server.validateResources(t.Context(), normalized.Spec)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("validateResources() error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateResources() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func workspaceRewardResourceResolverForTest(
	t *testing.T,
	modelKind apitypes.ModelKind,
	rewardPrompt *string,
) func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) {
	t.Helper()
	return func(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
		var resource apitypes.Resource
		switch kind {
		case apitypes.ResourceKindWorkflow:
			spec := apitypes.WorkflowSpec{
				Driver:   apitypes.WorkflowDriverChatroom,
				Chatroom: &apitypes.ChatRoomWorkflowSpec{History: apitypes.ChatRoomWorkflowHistorySpec{}},
			}
			if name == "pet-care" {
				spec = apitypes.WorkflowSpec{
					Driver: apitypes.WorkflowDriverPet,
					Pet: &apitypes.PetWorkflowSpec{
						Driver: apitypes.ReusableWorkflowDriverChatroom,
						Chatroom: &apitypes.ChatRoomWorkflowSpec{
							History: apitypes.ChatRoomWorkflowHistorySpec{},
						},
					},
				}
			}
			err := resource.FromWorkflowResource(apitypes.WorkflowResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.WorkflowResourceKindWorkflow,
				Metadata:   apitypes.ResourceMetadata{Id: name},
				Spec:       spec,
			})
			return resource, err
		case apitypes.ResourceKindModel:
			err := resource.FromModelResource(apitypes.ModelResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.ModelResourceKindModel,
				Metadata:   apitypes.ResourceMetadata{Id: name},
				Spec:       apitypes.ModelSpec{Kind: modelKind},
			})
			return resource, err
		case apitypes.ResourceKindBadgeDef:
			err := resource.FromBadgeDefResource(apitypes.BadgeDefResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.BadgeDefResourceKindBadgeDef,
				Metadata:   apitypes.ResourceMetadata{Id: name},
				Spec: apitypes.BadgeDefSpec{
					DisplayName: "Science", RewardPrompt: rewardPrompt,
				},
			})
			return resource, err
		default:
			return apitypes.Resource{}, kv.ErrNotFound
		}
	}
}

func validWorkspaceRewardProfileForTest() adminhttp.RuntimeProfileUpsert {
	models := map[string]apitypes.RuntimeProfileBinding{
		"reward-evaluator": runtimeProfileTestBinding("model-reward"),
	}
	badgeDefs := map[string]apitypes.RuntimeProfileBinding{
		"science": runtimeProfileTestBinding("badge-science"),
	}
	kinds := []apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKinds{
		apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKindsWorkflow,
		apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKindsDirectChatroom,
	}
	badges := map[string]apitypes.RuntimeProfileWorkspaceRewardBadgeSpec{
		"science": {MaxExpPerWindow: 5},
	}
	return adminhttp.RuntimeProfileUpsert{
		Id: "workspace-reward-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: runtimeProfileTestSystemWorkflows(), Collections: apitypes.RuntimeProfileWorkflowCollections{},
			},
			Resources: apitypes.RuntimeProfileResources{Models: &models, BadgeDefs: &badgeDefs},
			Gameplay: &apitypes.RuntimeProfileGameplaySpec{
				WorkspaceReward: &apitypes.RuntimeProfileWorkspaceRewardSpec{
					Enabled:        true,
					WorkspaceKinds: &kinds,
					Debounce: &apitypes.RuntimeProfileWorkspaceRewardDebounceSpec{
						QuietPeriod: " 60s ", MaxWindowAge: "10m",
					},
					Transcript: &apitypes.RuntimeProfileWorkspaceRewardTranscriptSpec{
						MaxEntries: 20, MaxTextBytes: 4096,
					},
					Evaluation: &apitypes.RuntimeProfileWorkspaceRewardEvaluationSpec{
						Model: " reward-evaluator ", PointsPrompt: " Reward good learning. ",
						ScoreMin: 0, ScoreMax: 100, QualifyingScore: 80,
					},
					Points: &apitypes.RuntimeProfileWorkspaceRewardPointsSpec{
						Tiers: []apitypes.RuntimeProfileWorkspaceRewardPointsTier{
							{MinScore: 80, Delta: 10}, {MinScore: 90, Delta: 20},
						},
					},
					Badges: &badges,
					RollingBudget: &apitypes.RuntimeProfileWorkspaceRewardRollingBudgetSpec{
						Period: "24h", PointsMax: 100, BadgeExpMax: 50,
					},
				},
			},
		},
	}
}

func TestRuntimeProfileRequiresPetPolicyForAdoption(t *testing.T) {
	t.Parallel()
	pool := []apitypes.RuntimeProfilePetPoolEntry{{PetDef: "pet", Weight: 1}}
	_, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{
		Id: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{System: runtimeProfileTestSystemWorkflows(), Collections: apitypes.RuntimeProfileWorkflowCollections{}},
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
		Id: "default",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				System:      runtimeProfileTestSystemWorkflows(),
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
	s := &Server{Store: kv.NewMemory(nil)}
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
	s := &Server{Store: kv.NewMemory(nil)}
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

func TestResolveOwnerProfileReadsSharedProfileConcurrently(t *testing.T) {
	t.Parallel()
	store := &blockingProfileGetStore{
		Store:   kv.NewMemory(nil),
		key:     profileKey("shared-profile").String(),
		entered: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	s := &Server{Store: store}
	createProfile(t, s, "shared-profile", nil)
	for _, owner := range []string{"peer-a", "peer-b"} {
		if err := s.BindOwnerProfile(t.Context(), owner, "shared-profile"); err != nil {
			t.Fatalf("BindOwnerProfile(%q) error = %v", owner, err)
		}
	}
	store.enabled.Store(true)
	results := make(chan error, 2)
	for _, owner := range []string{"peer-a", "peer-b"} {
		go func() {
			_, err := s.ResolveOwnerProfile(t.Context(), owner)
			results <- err
		}()
	}
	for range 2 {
		select {
		case <-store.entered:
		case <-time.After(time.Second):
			close(store.release)
			t.Fatal("shared RuntimeProfile reads did not enter the store concurrently")
		}
	}
	close(store.release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("ResolveOwnerProfile() error = %v", err)
		}
	}
}

func BenchmarkResolveOwnerProfile(b *testing.B) {
	s := &Server{Store: kv.NewMemory(nil)}
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
	s := &Server{Store: kv.NewMemory(nil)}
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
	updated.Spec.Workflows.System.Pet = "pet-care-v2"
	previousResolver := s.ResolveResource
	s.ResolveResource = func(ctx context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
		if kind == apitypes.ResourceKindWorkflow && name == "pet-care-v2" {
			var resource apitypes.Resource
			err := resource.FromWorkflowResource(apitypes.WorkflowResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.WorkflowResourceKindWorkflow,
				Metadata:   apitypes.ResourceMetadata{Id: name},
				Spec: apitypes.WorkflowSpec{
					Driver: apitypes.WorkflowDriverPet,
					Pet: &apitypes.PetWorkflowSpec{
						Driver:   apitypes.ReusableWorkflowDriverChatroom,
						Chatroom: &apitypes.ChatRoomWorkflowSpec{History: apitypes.ChatRoomWorkflowHistorySpec{}},
					},
				},
			})
			return resource, err
		}
		return previousResolver(ctx, kind, name)
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
	if current.Spec.Workflows.System.Pet != "pet-care-v2" || current.Revision == first.Revision {
		t.Fatalf("ResolveOwnerProfile(updated) = %#v, initial revision %q", current, first.Revision)
	}
}

func TestBindOwnerProfileAndCommitRestoresPreviousBinding(t *testing.T) {
	t.Parallel()
	s := &Server{Store: kv.NewMemory(nil)}
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
	if _, err := s.ResolveOwnerProfile(t.Context(), "peer-b"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("ResolveOwnerProfile(new owner) error = %v, want not found", err)
	}
}

func TestBindOwnerProfileDoesNotBlockIndependentOwnerAndProfile(t *testing.T) {
	server := &Server{Store: kv.NewMemory(nil)}
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
	s := &Server{Store: kv.NewMemory(nil)}
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
			driver := apitypes.WorkflowDriverChatroom
			spec := apitypes.WorkflowSpec{Driver: driver, Chatroom: &apitypes.ChatRoomWorkflowSpec{History: apitypes.ChatRoomWorkflowHistorySpec{}}}
			if resourceName == "pet-care" {
				driver = apitypes.WorkflowDriverPet
				spec = apitypes.WorkflowSpec{
					Driver: driver,
					Pet: &apitypes.PetWorkflowSpec{
						Driver:   apitypes.ReusableWorkflowDriverChatroom,
						Chatroom: &apitypes.ChatRoomWorkflowSpec{History: apitypes.ChatRoomWorkflowHistorySpec{}},
					},
				}
			}
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
		return apitypes.Resource{}, kv.ErrNotFound
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
				System:      runtimeProfileTestSystemWorkflows(),
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

func TestPersistedFlowcraftBBHProfileRemainsReadable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	const id = "legacy-bbh-profile"
	raw := []byte(`{"id":"legacy-bbh-profile","spec":{"resources":{"memories":{"legacy":{"layout_id":"default-memory","driver":"flowcraft","connection":{"type":"flowcraft_bbh"}}}}}}`)
	if err := store.Set(ctx, profileKey(id), raw); err != nil {
		t.Fatal(err)
	}
	item, err := getProfileByID(ctx, store, id)
	if err != nil {
		t.Fatalf("getProfileByID() error = %v", err)
	}
	binding := (*item.Spec.Resources.Memories)["legacy"]
	connectionType, err := binding.Connection.Discriminator()
	if err != nil || connectionType != "flowcraft_bbh" {
		t.Fatalf("legacy connection = %q, error = %v", connectionType, err)
	}
	stored, err := store.Get(ctx, profileKey(id))
	if err != nil || string(stored) != string(raw) {
		t.Fatalf("BBH profile changed while read: stored=%s error=%v", stored, err)
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
	store := kv.NewMemory(nil)
	server := &Server{
		Store: store,
		ResolveResource: func(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
			if kind == apitypes.ResourceKindMemoryLayout {
				return apitypes.Resource{}, kv.ErrNotFound
			}
			if kind != apitypes.ResourceKindWorkflow {
				return apitypes.Resource{}, kv.ErrNotFound
			}
			spec := apitypes.WorkflowSpec{
				Driver:   apitypes.WorkflowDriverChatroom,
				Chatroom: &apitypes.ChatRoomWorkflowSpec{History: apitypes.ChatRoomWorkflowHistorySpec{}},
			}
			if name == "pet-care" {
				spec = apitypes.WorkflowSpec{
					Driver: apitypes.WorkflowDriverPet,
					Pet: &apitypes.PetWorkflowSpec{
						Driver:   apitypes.ReusableWorkflowDriverChatroom,
						Chatroom: &apitypes.ChatRoomWorkflowSpec{History: apitypes.ChatRoomWorkflowHistorySpec{}},
					},
				}
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
					System: runtimeProfileTestSystemWorkflows(), Collections: apitypes.RuntimeProfileWorkflowCollections{},
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
	if _, err := GetProfile(t.Context(), store, "default"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("persisted profile after validation failure: %v", err)
	}
}

func runtimeProfileTestBinding(resourceID string) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{ResourceId: resourceID, I18n: map[string]apitypes.RuntimeProfileI18nText{
		"en": {DisplayName: "Test"}, "zh-CN": {DisplayName: "测试"},
	}}
}

func runtimeProfileTestSystemWorkflows() apitypes.RuntimeProfileSystemWorkflows {
	return apitypes.RuntimeProfileSystemWorkflows{
		FriendChatroom: "chatroom",
		GroupChatroom:  "chatroom",
		Pet:            "pet-care",
	}
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
