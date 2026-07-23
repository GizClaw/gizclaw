package runtimeprofile

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var (
	profilesRoot     = kv.Key{"runtime-profiles", "by-name"}
	tokensRoot       = kv.Key{"registration-tokens", "by-name"}
	tokensByHashRoot = kv.Key{"registration-tokens", "by-token-hash"}
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
	tokenBytes       = 32
	tokenAttempts    = 8
)

var runtimeAliasPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Server owns RuntimeProfile and RegistrationToken state.
type Server struct {
	Store           kv.Store
	Now             func() time.Time
	Random          io.Reader
	ResolveResource func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error)
	mutationMu      sync.Mutex
}

type AdminService interface {
	ListRuntimeProfiles(context.Context, adminhttp.ListRuntimeProfilesRequestObject) (adminhttp.ListRuntimeProfilesResponseObject, error)
	CreateRuntimeProfile(context.Context, adminhttp.CreateRuntimeProfileRequestObject) (adminhttp.CreateRuntimeProfileResponseObject, error)
	DeleteRuntimeProfile(context.Context, adminhttp.DeleteRuntimeProfileRequestObject) (adminhttp.DeleteRuntimeProfileResponseObject, error)
	GetRuntimeProfile(context.Context, adminhttp.GetRuntimeProfileRequestObject) (adminhttp.GetRuntimeProfileResponseObject, error)
	PutRuntimeProfile(context.Context, adminhttp.PutRuntimeProfileRequestObject) (adminhttp.PutRuntimeProfileResponseObject, error)
	ListRegistrationTokens(context.Context, adminhttp.ListRegistrationTokensRequestObject) (adminhttp.ListRegistrationTokensResponseObject, error)
	CreateRegistrationToken(context.Context, adminhttp.CreateRegistrationTokenRequestObject) (adminhttp.CreateRegistrationTokenResponseObject, error)
	DeleteRegistrationToken(context.Context, adminhttp.DeleteRegistrationTokenRequestObject) (adminhttp.DeleteRegistrationTokenResponseObject, error)
	GetRegistrationToken(context.Context, adminhttp.GetRegistrationTokenRequestObject) (adminhttp.GetRegistrationTokenResponseObject, error)
}

var _ AdminService = (*Server)(nil)

// Registration is the connection-local result of consuming a RegistrationToken.
type Registration struct {
	TokenName      string
	RuntimeProfile apitypes.RuntimeProfile
	FirmwareID     *string
}

// ResolveProfile returns the current persisted revision for a profile name.
// Registrations pin the name, not a configuration snapshot.
func (s *Server) ResolveProfile(ctx context.Context, name string) (apitypes.RuntimeProfile, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	return GetProfile(ctx, store, strings.TrimSpace(name))
}

type tokenRecord struct {
	apitypes.RegistrationToken
	TokenHash string `json:"token_hash"`
}

func (s *Server) ResolveRegistration(ctx context.Context, rawToken string) (Registration, error) {
	store, err := s.store()
	if err != nil {
		return Registration{}, err
	}
	digest := tokenDigest(strings.TrimSpace(rawToken))
	nameBytes, err := store.Get(ctx, tokenHashKey(digest))
	if err != nil {
		return Registration{}, err
	}
	record, err := getTokenRecord(ctx, store, string(nameBytes))
	if err != nil {
		return Registration{}, err
	}
	if record.TokenHash != digest {
		return Registration{}, kv.ErrNotFound
	}
	profile, err := GetProfile(ctx, store, record.RuntimeProfileName)
	if err != nil {
		return Registration{}, err
	}
	return Registration{TokenName: record.Name, RuntimeProfile: profile, FirmwareID: cloneString(record.FirmwareId)}, nil
}

func (s *Server) ListRuntimeProfiles(ctx context.Context, request adminhttp.ListRuntimeProfilesRequestObject) (adminhttp.ListRuntimeProfilesResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.ListRuntimeProfiles500JSONResponse(internalError(err)), nil
	}
	items, hasNext, nextCursor, err := listProfiles(ctx, store, request.Params.Cursor, request.Params.Limit)
	if err != nil {
		return adminhttp.ListRuntimeProfiles500JSONResponse(internalError(err)), nil
	}
	return adminhttp.ListRuntimeProfiles200JSONResponse{Items: items, HasNext: hasNext, NextCursor: nextCursor}, nil
}

func (s *Server) CreateRuntimeProfile(ctx context.Context, request adminhttp.CreateRuntimeProfileRequestObject) (adminhttp.CreateRuntimeProfileResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.CreateRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	if request.Body == nil {
		return adminhttp.CreateRuntimeProfile400JSONResponse(invalid("request body required")), nil
	}
	item, err := normalizeProfile(*request.Body, "")
	if err != nil {
		return adminhttp.CreateRuntimeProfile400JSONResponse(invalid(err.Error())), nil
	}
	if err := s.validateResources(ctx, item.Spec); err != nil {
		return adminhttp.CreateRuntimeProfile400JSONResponse(invalid(err.Error())), nil
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if _, err := GetProfile(ctx, store, item.Name); err == nil {
		return adminhttp.CreateRuntimeProfile409JSONResponse(conflict("runtime profile already exists")), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	now := s.now()
	item.CreatedAt, item.UpdatedAt = now, now
	if err := writeProfile(ctx, store, item); err != nil {
		return adminhttp.CreateRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	return adminhttp.CreateRuntimeProfile200JSONResponse(item), nil
}

func (s *Server) GetRuntimeProfile(ctx context.Context, request adminhttp.GetRuntimeProfileRequestObject) (adminhttp.GetRuntimeProfileResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	name, err := pathName(request.Name)
	if err != nil {
		return nil, err
	}
	item, err := GetProfile(ctx, store, name)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.GetRuntimeProfile404JSONResponse(notFound("runtime profile", name)), nil
	}
	if err != nil {
		return adminhttp.GetRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	return adminhttp.GetRuntimeProfile200JSONResponse(item), nil
}

func (s *Server) PutRuntimeProfile(ctx context.Context, request adminhttp.PutRuntimeProfileRequestObject) (adminhttp.PutRuntimeProfileResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.PutRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	if request.Body == nil {
		return adminhttp.PutRuntimeProfile400JSONResponse(invalid("request body required")), nil
	}
	name, err := pathName(request.Name)
	if err != nil {
		return nil, err
	}
	item, err := normalizeProfile(*request.Body, name)
	if err != nil {
		return adminhttp.PutRuntimeProfile400JSONResponse(invalid(err.Error())), nil
	}
	if err := s.validateResources(ctx, item.Spec); err != nil {
		return adminhttp.PutRuntimeProfile400JSONResponse(invalid(err.Error())), nil
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	previous, getErr := GetProfile(ctx, store, name)
	if getErr != nil && !errors.Is(getErr, kv.ErrNotFound) {
		return adminhttp.PutRuntimeProfile500JSONResponse(internalError(getErr)), nil
	}
	now := s.now()
	item.CreatedAt, item.UpdatedAt = now, now
	if getErr == nil {
		item.CreatedAt = previous.CreatedAt
	}
	if err := writeProfile(ctx, store, item); err != nil {
		return adminhttp.PutRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	return adminhttp.PutRuntimeProfile200JSONResponse(item), nil
}

func (s *Server) DeleteRuntimeProfile(ctx context.Context, request adminhttp.DeleteRuntimeProfileRequestObject) (adminhttp.DeleteRuntimeProfileResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	name, err := pathName(request.Name)
	if err != nil {
		return nil, err
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	item, err := GetProfile(ctx, store, name)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.DeleteRuntimeProfile404JSONResponse(notFound("runtime profile", name)), nil
	}
	if err != nil {
		return adminhttp.DeleteRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	if err := store.Delete(ctx, profileKey(name)); err != nil {
		return adminhttp.DeleteRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	return adminhttp.DeleteRuntimeProfile200JSONResponse(item), nil
}
func (s *Server) ListRegistrationTokens(ctx context.Context, request adminhttp.ListRegistrationTokensRequestObject) (adminhttp.ListRegistrationTokensResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.ListRegistrationTokens500JSONResponse(internalError(err)), nil
	}
	items, hasNext, nextCursor, err := listTokens(ctx, store, request.Params.Cursor, request.Params.Limit)
	if err != nil {
		return adminhttp.ListRegistrationTokens500JSONResponse(internalError(err)), nil
	}
	return adminhttp.ListRegistrationTokens200JSONResponse{Items: items, HasNext: hasNext, NextCursor: nextCursor}, nil
}

func (s *Server) CreateRegistrationToken(ctx context.Context, request adminhttp.CreateRegistrationTokenRequestObject) (adminhttp.CreateRegistrationTokenResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	if request.Body == nil {
		return adminhttp.CreateRegistrationToken400JSONResponse(invalid("request body required")), nil
	}
	in := *request.Body
	name := strings.TrimSpace(in.Name)
	if err := customid.ValidateRegistrationTokenName(name); err != nil {
		return adminhttp.CreateRegistrationToken400JSONResponse(invalid(err.Error())), nil
	}
	profileName := strings.TrimSpace(in.RuntimeProfileName)
	if profileName == "" {
		return adminhttp.CreateRegistrationToken400JSONResponse(invalid("runtime_profile_name is required")), nil
	}
	var firmwareID *string
	if in.FirmwareId != nil {
		value := strings.TrimSpace(*in.FirmwareId)
		if value == "" {
			return adminhttp.CreateRegistrationToken400JSONResponse(invalid("firmware_id must not be empty")), nil
		}
		if s.ResolveResource == nil {
			return adminhttp.CreateRegistrationToken500JSONResponse(internalError(errors.New("resource resolver not configured"))), nil
		}
		resource, err := s.ResolveResource(ctx, apitypes.ResourceKindFirmware, value)
		if err != nil {
			return adminhttp.CreateRegistrationToken400JSONResponse(invalid("firmware_id does not exist")), nil
		}
		discriminator, err := resource.Discriminator()
		if err != nil || (discriminator != string(apitypes.ResourceKindFirmware) && discriminator != string(apitypes.ResourceKindFirmware)+"Resource") {
			return adminhttp.CreateRegistrationToken400JSONResponse(invalid("firmware_id does not reference a Firmware")), nil
		}
		firmwareID = &value
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if _, err := getTokenRecord(ctx, store, name); err == nil {
		return adminhttp.CreateRegistrationToken409JSONResponse(conflict("registration token already exists")), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	if _, err := GetProfile(ctx, store, profileName); err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.CreateRegistrationToken400JSONResponse(invalid("runtime_profile_name does not exist")), nil
		}
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	raw, err := s.newUniqueToken(ctx, store)
	if err != nil {
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	digest := tokenDigest(raw)
	createdAt := s.now()
	record := tokenRecord{RegistrationToken: apitypes.RegistrationToken{Name: name, RuntimeProfileName: profileName, FirmwareId: cloneString(firmwareID), CreatedAt: createdAt}, TokenHash: digest}
	encoded, err := json.Marshal(record)
	if err != nil {
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	if err := store.BatchSet(ctx, []kv.Entry{{Key: tokenKey(name), Value: encoded}, {Key: tokenHashKey(digest), Value: []byte(name)}}); err != nil {
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	return adminhttp.CreateRegistrationToken200JSONResponse(apitypes.RegistrationTokenCreateResult{Name: name, RuntimeProfileName: profileName, FirmwareId: cloneString(firmwareID), CreatedAt: createdAt, Token: raw}), nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func (s *Server) GetRegistrationToken(ctx context.Context, request adminhttp.GetRegistrationTokenRequestObject) (adminhttp.GetRegistrationTokenResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetRegistrationToken500JSONResponse(internalError(err)), nil
	}
	name, err := pathName(request.Name)
	if err != nil {
		return nil, err
	}
	record, err := getTokenRecord(ctx, store, name)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.GetRegistrationToken404JSONResponse(notFound("registration token", name)), nil
	}
	if err != nil {
		return adminhttp.GetRegistrationToken500JSONResponse(internalError(err)), nil
	}
	return adminhttp.GetRegistrationToken200JSONResponse(record.RegistrationToken), nil
}

func (s *Server) DeleteRegistrationToken(ctx context.Context, request adminhttp.DeleteRegistrationTokenRequestObject) (adminhttp.DeleteRegistrationTokenResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteRegistrationToken500JSONResponse(internalError(err)), nil
	}
	name, err := pathName(request.Name)
	if err != nil {
		return nil, err
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	record, err := getTokenRecord(ctx, store, name)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.DeleteRegistrationToken404JSONResponse(notFound("registration token", name)), nil
	}
	if err != nil {
		return adminhttp.DeleteRegistrationToken500JSONResponse(internalError(err)), nil
	}
	if err := store.BatchDelete(ctx, []kv.Key{tokenKey(name), tokenHashKey(record.TokenHash)}); err != nil {
		return adminhttp.DeleteRegistrationToken500JSONResponse(internalError(err)), nil
	}
	return adminhttp.DeleteRegistrationToken200JSONResponse(record.RegistrationToken), nil
}

func GetProfile(ctx context.Context, store kv.Store, name string) (apitypes.RuntimeProfile, error) {
	data, err := store.Get(ctx, profileKey(name))
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	var item apitypes.RuntimeProfile
	if err := json.Unmarshal(data, &item); err != nil {
		return apitypes.RuntimeProfile{}, fmt.Errorf("runtime profile: decode %s: %w", name, err)
	}
	if err := setProfileRevision(&item); err != nil {
		return apitypes.RuntimeProfile{}, fmt.Errorf("runtime profile: revision %s: %w", name, err)
	}
	return item, nil
}

func writeProfile(ctx context.Context, store kv.Store, item apitypes.RuntimeProfile) error {
	if err := setProfileRevision(&item); err != nil {
		return err
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return store.Set(ctx, profileKey(item.Name), data)
}

func getTokenRecord(ctx context.Context, store kv.Store, name string) (tokenRecord, error) {
	data, err := store.Get(ctx, tokenKey(name))
	if err != nil {
		return tokenRecord{}, err
	}
	var item tokenRecord
	if err := json.Unmarshal(data, &item); err != nil {
		return tokenRecord{}, fmt.Errorf("registration token: decode %s: %w", name, err)
	}
	return item, nil
}

func normalizeProfile(in adminhttp.RuntimeProfileUpsert, expectedName string) (apitypes.RuntimeProfile, error) {
	name := strings.TrimSpace(in.Name)
	if err := validateProfileName(name); err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if expectedName != "" && name != expectedName {
		return apitypes.RuntimeProfile{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
	}
	spec := in.Spec
	allAliases := make(map[string]string)
	workflowAliases := make(map[string]string)
	collections := make(apitypes.RuntimeProfileWorkflowCollections, len(spec.Workflows.Collections))
	for collection, bindings := range spec.Workflows.Collections {
		collection = strings.TrimSpace(collection)
		if err := ValidateAlias("workflow collection", collection); err != nil {
			return apitypes.RuntimeProfile{}, err
		}
		if _, exists := collections[collection]; exists {
			return apitypes.RuntimeProfile{}, fmt.Errorf("workflow collection %q is duplicated after normalization", collection)
		}
		normalized, err := normalizeBindingMap(bindings)
		if err != nil {
			return apitypes.RuntimeProfile{}, fmt.Errorf("workflows.collections.%s: %w", collection, err)
		}
		for alias := range normalized {
			if previous, exists := workflowAliases[alias]; exists {
				return apitypes.RuntimeProfile{}, fmt.Errorf("workflow alias %q is duplicated in collections %q and %q", alias, previous, collection)
			}
			workflowAliases[alias] = collection
			if err := registerProfileAlias(allAliases, alias, "workflow"); err != nil {
				return apitypes.RuntimeProfile{}, err
			}
		}
		collections[collection] = normalized
	}
	spec.Workflows.Collections = collections
	resourceMaps := []struct {
		name   string
		values *map[string]apitypes.RuntimeProfileBinding
	}{
		{name: "model", values: spec.Resources.Models},
		{name: "voice", values: spec.Resources.Voices},
		{name: "tool", values: spec.Resources.Tools},
		{name: "pet definition", values: spec.Resources.PetDefs},
		{name: "game definition", values: spec.Resources.GameDefs},
		{name: "badge definition", values: spec.Resources.BadgeDefs},
	}
	for _, resourceMap := range resourceMaps {
		if resourceMap.values == nil {
			continue
		}
		normalized, err := normalizeBindingMap(*resourceMap.values)
		if err != nil {
			return apitypes.RuntimeProfile{}, err
		}
		for alias := range normalized {
			if err := registerProfileAlias(allAliases, alias, resourceMap.name); err != nil {
				return apitypes.RuntimeProfile{}, err
			}
		}
		*resourceMap.values = normalized
	}
	if spec.Gameplay != nil && spec.Gameplay.Points != nil && spec.Gameplay.Points.InitialBalance != nil && *spec.Gameplay.Points.InitialBalance < 0 {
		return apitypes.RuntimeProfile{}, errors.New("gameplay.points.initial_balance must not be negative")
	}
	if spec.Gameplay != nil && spec.Gameplay.Adoption != nil && spec.Gameplay.Adoption.Pool != nil {
		if len(*spec.Gameplay.Adoption.Pool) > 0 && spec.Gameplay.Pet == nil {
			return apitypes.RuntimeProfile{}, errors.New("gameplay.pet is required when gameplay.adoption.pool is configured")
		}
		for i := range *spec.Gameplay.Adoption.Pool {
			entry := &(*spec.Gameplay.Adoption.Pool)[i]
			entry.PetDef = strings.TrimSpace(entry.PetDef)
			entry.Voice = strings.TrimSpace(entry.Voice)
			if entry.PetDef == "" || entry.Voice == "" || entry.Weight <= 0 {
				return apitypes.RuntimeProfile{}, fmt.Errorf("gameplay.adoption.pool[%d] requires pet_def, voice, and positive weight", i)
			}
			if entry.AdoptionCost != nil && *entry.AdoptionCost < 0 {
				return apitypes.RuntimeProfile{}, fmt.Errorf("gameplay.adoption.pool[%d].adoption_cost must not be negative", i)
			}
			if _, ok := bindingByAlias(spec.Resources.PetDefs, entry.PetDef); !ok {
				return apitypes.RuntimeProfile{}, fmt.Errorf("gameplay.adoption.pool[%d].pet_def %q is not declared in resources.pet_defs", i, entry.PetDef)
			}
			if _, ok := bindingByAlias(spec.Resources.Voices, entry.Voice); !ok {
				return apitypes.RuntimeProfile{}, fmt.Errorf("gameplay.adoption.pool[%d].voice %q is not declared in resources.voices", i, entry.Voice)
			}
		}
	}
	if spec.Gameplay != nil && spec.Gameplay.Pet != nil {
		if err := normalizePetGameplay(spec.Gameplay.Pet, spec.Resources); err != nil {
			return apitypes.RuntimeProfile{}, err
		}
	}
	item := apitypes.RuntimeProfile{Name: name, Spec: spec}
	if err := setProfileRevision(&item); err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	return item, nil
}

func registerProfileAlias(aliases map[string]string, alias, kind string) error {
	if previous, exists := aliases[alias]; exists {
		return fmt.Errorf("runtime profile alias %q is used by both %s and %s", alias, previous, kind)
	}
	aliases[alias] = kind
	return nil
}

func bindingByAlias(values *map[string]apitypes.RuntimeProfileBinding, alias string) (apitypes.RuntimeProfileBinding, bool) {
	if values == nil {
		return apitypes.RuntimeProfileBinding{}, false
	}
	binding, ok := (*values)[alias]
	return binding, ok
}

func (s *Server) validateResources(ctx context.Context, spec apitypes.RuntimeProfileSpec) error {
	if s == nil || s.ResolveResource == nil {
		return nil
	}
	resolve := func(path string, kind apitypes.ResourceKind, binding apitypes.RuntimeProfileBinding) (apitypes.Resource, error) {
		resource, err := s.ResolveResource(ctx, kind, binding.ResourceId)
		if err != nil {
			return apitypes.Resource{}, fmt.Errorf("%s.resource_id %q does not resolve to %s: %w", path, binding.ResourceId, kind, err)
		}
		discriminator, err := resource.Discriminator()
		if err != nil {
			return apitypes.Resource{}, fmt.Errorf("%s.resource_id %q returned a resource without a valid kind: %w", path, binding.ResourceId, err)
		}
		expected := string(kind)
		if discriminator != expected && discriminator != expected+"Resource" {
			return apitypes.Resource{}, fmt.Errorf("%s.resource_id %q returned kind %q, want %q", path, binding.ResourceId, discriminator, expected)
		}
		return resource, nil
	}
	type resolvedWorkflow struct {
		path     string
		resource apitypes.WorkflowResource
	}
	workflows := make([]resolvedWorkflow, 0)
	for collection, bindings := range spec.Workflows.Collections {
		for alias, binding := range bindings {
			path := "workflows.collections." + collection + "." + alias
			resource, err := resolve(path, apitypes.ResourceKindWorkflow, binding)
			if err != nil {
				return err
			}
			workflow, err := resource.AsWorkflowResource()
			if err != nil {
				return fmt.Errorf("%s.resource_id %q returned an invalid Workflow: %w", path, binding.ResourceId, err)
			}
			workflows = append(workflows, resolvedWorkflow{path: path, resource: workflow})
		}
	}
	models := make(map[string]apitypes.ModelResource)
	if spec.Resources.Models != nil {
		for alias, binding := range *spec.Resources.Models {
			path := "resources.models." + alias
			resource, err := resolve(path, apitypes.ResourceKindModel, binding)
			if err != nil {
				return err
			}
			model, err := resource.AsModelResource()
			if err != nil {
				return fmt.Errorf("%s.resource_id %q returned an invalid Model: %w", path, binding.ResourceId, err)
			}
			models[alias] = model
		}
	}
	groups := []struct {
		path   string
		kind   apitypes.ResourceKind
		values *map[string]apitypes.RuntimeProfileBinding
	}{
		{path: "resources.voices", kind: apitypes.ResourceKindVoice, values: spec.Resources.Voices},
		{path: "resources.tools", kind: apitypes.ResourceKindTool, values: spec.Resources.Tools},
		{path: "resources.game_defs", kind: apitypes.ResourceKindGameDef, values: spec.Resources.GameDefs},
		{path: "resources.badge_defs", kind: apitypes.ResourceKindBadgeDef, values: spec.Resources.BadgeDefs},
	}
	for _, group := range groups {
		if group.values == nil {
			continue
		}
		for alias, binding := range *group.values {
			if _, err := resolve(group.path+"."+alias, group.kind, binding); err != nil {
				return err
			}
		}
	}
	if spec.Resources.PetDefs != nil {
		for alias, binding := range *spec.Resources.PetDefs {
			resource, err := resolve("resources.pet_defs."+alias, apitypes.ResourceKindPetDef, binding)
			if err != nil {
				return err
			}
			petDef, err := resource.AsPetDefResource()
			if err != nil {
				return fmt.Errorf("resources.pet_defs.%s.resource_id %q returned an invalid PetDef: %w", alias, binding.ResourceId, err)
			}
			_ = petDef
		}
	}
	for _, workflow := range workflows {
		if err := validateWorkflowRuntimeAliases(workflow.path, workflow.resource.Spec, models, spec.Resources.Voices); err != nil {
			return err
		}
	}
	if spec.Gameplay != nil && spec.Gameplay.Pet != nil {
		if err := requirePetRuntimeAliases("gameplay.pet", models); err != nil {
			return err
		}
		if err := validatePetRewardModels(*spec.Gameplay.Pet, models); err != nil {
			return err
		}
	}
	return nil
}

func validatePetRewardModels(pet apitypes.RuntimeProfilePetGameplaySpec, models map[string]apitypes.ModelResource) error {
	for alias, game := range pet.Games {
		model := models[game.Reward.Model]
		if model.Spec.Kind != apitypes.ModelKindLlm {
			return fmt.Errorf("gameplay.pet.games.%s.reward.model alias %q has kind %q, want %q", alias, game.Reward.Model, model.Spec.Kind, apitypes.ModelKindLlm)
		}
	}
	return nil
}

func requirePetRuntimeAliases(path string, models map[string]apitypes.ModelResource) error {
	for _, model := range []struct {
		field string
		alias string
		kind  apitypes.ModelKind
	}{
		{field: "pet-chat", alias: "pet-chat", kind: apitypes.ModelKindLlm},
		{field: "pet-extract", alias: "pet-extract", kind: apitypes.ModelKindLlm},
		{field: "pet-asr", alias: "pet-asr", kind: apitypes.ModelKindAsr},
	} {
		alias := strings.TrimSpace(model.alias)
		resource, ok := models[alias]
		if !ok {
			return fmt.Errorf("%s.%s model alias %q is not declared in resources.models", path, model.field, alias)
		}
		if resource.Spec.Kind != model.kind {
			return fmt.Errorf("%s.%s model alias %q has kind %q, want %q", path, model.field, alias, resource.Spec.Kind, model.kind)
		}
	}
	return nil
}

func validateWorkflowRuntimeAliases(path string, workflow apitypes.WorkflowSpec, models map[string]apitypes.ModelResource, voices *map[string]apitypes.RuntimeProfileBinding) error {
	requireModel := func(field, alias string, kind apitypes.ModelKind) error {
		alias = strings.TrimSpace(alias)
		model, ok := models[alias]
		if !ok {
			return fmt.Errorf("%s.%s model alias %q is not declared in resources.models", path, field, alias)
		}
		if model.Spec.Kind != kind {
			return fmt.Errorf("%s.%s model alias %q has kind %q, want %q", path, field, alias, model.Spec.Kind, kind)
		}
		return nil
	}
	requireVoice := func(field, alias string) error {
		alias = strings.TrimSpace(alias)
		if _, ok := bindingByAlias(voices, alias); !ok {
			return fmt.Errorf("%s.%s voice alias %q is not declared in resources.voices", path, field, alias)
		}
		return nil
	}
	switch workflow.Driver {
	case apitypes.WorkflowDriverAstTranslate:
		if workflow.AstTranslate == nil {
			return fmt.Errorf("%s has no ast_translate spec", path)
		}
		if workflow.AstTranslate.LangPair == nil || strings.TrimSpace(*workflow.AstTranslate.LangPair) == "" {
			return fmt.Errorf("%s.lang_pair is required for Peer Workspace initialization", path)
		}
		if err := requireModel("translation_model", workflow.AstTranslate.TranslationModel, apitypes.ModelKindTranslation); err != nil {
			return err
		}
		if workflow.AstTranslate.Mode == nil || *workflow.AstTranslate.Mode != apitypes.ASTTranslateModeS2s {
			break
		}
		if workflow.AstTranslate.Voice == nil {
			return fmt.Errorf("%s.voice requires a RuntimeProfile Voice alias for s2s", path)
		}
		external, err := workflow.AstTranslate.Voice.AsASTTranslateExternalVoiceParameters()
		if err != nil || strings.TrimSpace(external.TtsVoice) == "" {
			return fmt.Errorf("%s.voice must use voice.tts_voice as a RuntimeProfile Voice alias for s2s", path)
		}
		return requireVoice("voice.tts_voice", external.TtsVoice)
	case apitypes.WorkflowDriverChatroom:
		if workflow.Chatroom != nil && workflow.Chatroom.Transcript != nil && workflow.Chatroom.Transcript.AsrModel != nil {
			return requireModel("transcript.asr_model", *workflow.Chatroom.Transcript.AsrModel, apitypes.ModelKindAsr)
		}
	case apitypes.WorkflowDriverPet:
		if workflow.Pet == nil {
			return fmt.Errorf("%s has no pet spec", path)
		}
		return requirePetRuntimeAliases(path, models)
	case apitypes.WorkflowDriverDoubaoRealtime:
		if workflow.DoubaoRealtime == nil {
			return fmt.Errorf("%s has no doubao_realtime spec", path)
		}
		if workflow.DoubaoRealtime.Tools != nil && len(*workflow.DoubaoRealtime.Tools) != 0 {
			return fmt.Errorf("%s.tools are unsupported until ToolCall is implemented", path)
		}
		if err := requireModel("model", workflow.DoubaoRealtime.Model, apitypes.ModelKindRealtime); err != nil {
			return err
		}
		if workflow.DoubaoRealtime.Audio == nil || workflow.DoubaoRealtime.Audio.Output.Voice == nil || strings.TrimSpace(*workflow.DoubaoRealtime.Audio.Output.Voice) == "" {
			return fmt.Errorf("%s.audio.output.voice requires a RuntimeProfile Voice alias", path)
		}
		return requireVoice("audio.output.voice", *workflow.DoubaoRealtime.Audio.Output.Voice)
	case apitypes.WorkflowDriverFlowcraft:
		if workflow.Flowcraft == nil {
			return fmt.Errorf("%s has no flowcraft spec", path)
		}
		flowcraft := *workflow.Flowcraft
		modelAliases := make([]struct {
			field string
			alias string
			kind  apitypes.ModelKind
		}, 0, len(flowcraft.Agent.Graph.Nodes)+4)
		for index, raw := range flowcraft.Agent.Graph.Nodes {
			if discriminator, _ := raw.Discriminator(); discriminator == "llm" {
				node, err := raw.AsFlowcraftLLMNode()
				if err != nil {
					return fmt.Errorf("%s.agent.graph.nodes[%d]: %w", path, index, err)
				}
				modelAliases = append(modelAliases, struct {
					field string
					alias string
					kind  apitypes.ModelKind
				}{field: fmt.Sprintf("agent.graph.nodes[%d].config.model", index), alias: node.Config.Model, kind: apitypes.ModelKindLlm})
			}
		}
		if flowcraft.Memory != nil && flowcraft.Memory.Enabled {
			if cfg := flowcraft.Memory.Extract; cfg != nil && (cfg.Enabled == nil || *cfg.Enabled) && cfg.Model != nil {
				modelAliases = append(modelAliases, struct {
					field, alias string
					kind         apitypes.ModelKind
				}{"memory.extract.model", *cfg.Model, apitypes.ModelKindLlm})
			}
			if cfg := flowcraft.Memory.Embedding; cfg != nil && cfg.Enabled != nil && *cfg.Enabled && cfg.Model != nil {
				modelAliases = append(modelAliases, struct {
					field, alias string
					kind         apitypes.ModelKind
				}{"memory.embedding.model", *cfg.Model, apitypes.ModelKindEmbedding})
			}
			if cfg := flowcraft.Memory.Rerank; cfg != nil && cfg.Enabled != nil && *cfg.Enabled && cfg.Model != nil {
				modelAliases = append(modelAliases, struct {
					field, alias string
					kind         apitypes.ModelKind
				}{"memory.rerank.model", *cfg.Model, apitypes.ModelKindLlm})
			}
		}
		if flowcraft.VoiceAdapter != nil && flowcraft.VoiceAdapter.AsrModel != nil {
			modelAliases = append(modelAliases, struct {
				field, alias string
				kind         apitypes.ModelKind
			}{"voice_adapter.asr_model", *flowcraft.VoiceAdapter.AsrModel, apitypes.ModelKindAsr})
		}
		for _, model := range modelAliases {
			if strings.TrimSpace(model.alias) != "" {
				if err := requireModel(model.field, model.alias, model.kind); err != nil {
					return err
				}
			}
		}
		if flowcraft.VoiceAdapter != nil {
			if flowcraft.VoiceAdapter.DefaultVoice != nil {
				if err := requireVoice("voice_adapter.default_voice", *flowcraft.VoiceAdapter.DefaultVoice); err != nil {
					return err
				}
			}
			if flowcraft.VoiceAdapter.NodeVoices != nil {
				for nodeID, alias := range *flowcraft.VoiceAdapter.NodeVoices {
					if err := requireVoice("voice_adapter.node_voices."+nodeID, alias); err != nil {
						return err
					}
				}
			}
		}
		if strings.TrimSpace(flowcraft.Agent.Graph.Entry) == "" || len(flowcraft.Agent.Graph.Nodes) == 0 {
			return fmt.Errorf("%s.agent.graph must have an entry and at least one node", path)
		}
		entryFound := false
		for _, raw := range flowcraft.Agent.Graph.Nodes {
			data, err := raw.MarshalJSON()
			if err != nil {
				return err
			}
			var node struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(data, &node); err != nil {
				return err
			}
			entryFound = entryFound || node.ID == flowcraft.Agent.Graph.Entry
		}
		if !entryFound {
			return fmt.Errorf("%s.agent.graph.entry %q is not a defined node", path, flowcraft.Agent.Graph.Entry)
		}
	}
	return nil
}

func validateProfileName(name string) error {
	if name == "default" {
		return nil
	}
	return customid.ValidateField("name", name)
}

func normalizeBindingMap(values map[string]apitypes.RuntimeProfileBinding) (map[string]apitypes.RuntimeProfileBinding, error) {
	out := make(map[string]apitypes.RuntimeProfileBinding, len(values))
	for alias, binding := range values {
		alias = strings.TrimSpace(alias)
		if err := ValidateAlias("resource alias", alias); err != nil {
			return nil, err
		}
		binding.ResourceId = strings.TrimSpace(binding.ResourceId)
		if binding.ResourceId == "" {
			return nil, fmt.Errorf("runtime profile binding %q requires resource_id", alias)
		}
		i18n := make(map[string]apitypes.RuntimeProfileI18nText, len(binding.I18n))
		for locale, text := range binding.I18n {
			locale = strings.TrimSpace(locale)
			if locale == "" {
				return nil, fmt.Errorf("runtime profile binding %q contains an empty locale", alias)
			}
			if _, exists := i18n[locale]; exists {
				return nil, fmt.Errorf("runtime profile binding %q contains duplicate locale %q", alias, locale)
			}
			text.DisplayName = strings.TrimSpace(text.DisplayName)
			if text.DisplayName == "" {
				return nil, fmt.Errorf("runtime profile binding %q locale %q requires display_name", alias, locale)
			}
			if text.Description != nil {
				description := strings.TrimSpace(*text.Description)
				text.Description = &description
			}
			i18n[locale] = text
		}
		binding.I18n = i18n
		for _, required := range []string{"en", "zh-CN"} {
			if _, ok := binding.I18n[required]; !ok {
				return nil, fmt.Errorf("runtime profile binding %q requires i18n.%s", alias, required)
			}
		}
		if _, exists := out[alias]; exists {
			return nil, fmt.Errorf("duplicate runtime profile resource alias %q", alias)
		}
		out[alias] = binding
	}
	return out, nil
}

// ValidateAlias applies the canonical RuntimeProfile alias contract used by
// profile bindings and resources that persist those aliases.
func ValidateAlias(kind, value string) error {
	if len(value) == 0 || len(value) > 63 || !runtimeAliasPattern.MatchString(value) {
		return fmt.Errorf("%s %q must be 1-63 characters of lowercase kebab-case", kind, value)
	}
	return nil
}

func setProfileRevision(item *apitypes.RuntimeProfile) error {
	encoded, err := json.Marshal(item.Spec)
	if err != nil {
		return fmt.Errorf("encode normalized spec: %w", err)
	}
	digest := sha256.Sum256(encoded)
	item.Revision = hex.EncodeToString(digest[:])
	return nil
}

func normalizePetGameplay(pet *apitypes.RuntimeProfilePetGameplaySpec, resources apitypes.RuntimeProfileResources) error {
	if pet.Experience.EnergyPerPetExp <= 0 {
		return errors.New("gameplay.pet.experience.energy_per_pet_exp must be positive")
	}
	if pet.Experience.Leveling.BaseExp <= 0 || pet.Experience.Leveling.LogScale < 0 || pet.Experience.Leveling.LogScale > 100 {
		return errors.New("gameplay.pet.experience.leveling requires positive base_exp and log_scale in 0..100")
	}
	weights := pet.Time.LifeDecay.ContributingWeights
	if weights.Health < 0 || weights.Satiety < 0 || weights.Hygiene < 0 || weights.Mood < 0 {
		return errors.New("gameplay.pet.time.life_decay.contributing_weights values must not be negative")
	}
	weightSum := weights.Health + weights.Satiety + weights.Hygiene + weights.Mood
	if math.Abs(weightSum-1) > 1e-9 {
		return fmt.Errorf("gameplay.pet.time.life_decay.contributing_weights must sum to 1, got %g", weightSum)
	}
	if pet.Time.LifeDecay.Exponent <= 1 || pet.Time.LifeDecay.MaxLossPerHour < 0 || pet.Time.EnergyRecoveryPerHour < 0 {
		return errors.New("gameplay.pet.time requires exponent greater than 1 and non-negative recovery/loss rates")
	}
	decay := pet.Time.CareDecayPerHour
	if decay.Health < 0 || decay.Satiety < 0 || decay.Hygiene < 0 || decay.Mood < 0 {
		return errors.New("gameplay.pet.time.care_decay_per_hour values must not be negative")
	}
	actions := map[string]apitypes.RuntimeProfilePetActionSpec{
		"feed":  pet.Actions.Feed,
		"bathe": pet.Actions.Bathe,
		"play":  pet.Actions.Play,
		"heal":  pet.Actions.Heal,
	}
	for name, action := range actions {
		if action.EnergyCost <= 0 || action.EnergyCost > 100 || action.StatDelta <= 0 || action.StatDelta > 100 {
			return fmt.Errorf("gameplay.pet.actions.%s requires energy_cost and stat_delta in 1..100", name)
		}
		if action.EnergyCost%pet.Experience.EnergyPerPetExp != 0 {
			return fmt.Errorf("gameplay.pet.actions.%s.energy_cost must be divisible by energy_per_pet_exp", name)
		}
	}
	normalized := make(map[string]apitypes.RuntimeProfileGameSpec, len(pet.Games))
	gameDefAliases := make(map[string]string, len(pet.Games))
	for alias, game := range pet.Games {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			return errors.New("game definition alias must not be empty")
		}
		if _, exists := normalized[alias]; exists {
			return fmt.Errorf("duplicate game definition alias %q", alias)
		}
		gameDef, ok := bindingByAlias(resources.GameDefs, alias)
		if !ok {
			return fmt.Errorf("gameplay.pet.games.%s is not declared in resources.game_defs", alias)
		}
		gameDefID := strings.TrimSpace(gameDef.ResourceId)
		if previous, duplicate := gameDefAliases[gameDefID]; duplicate {
			return fmt.Errorf("gameplay.pet.games.%s and gameplay.pet.games.%s resolve to the same GameDef %q", previous, alias, gameDefID)
		}
		gameDefAliases[gameDefID] = alias
		game.Reward.Model = strings.TrimSpace(game.Reward.Model)
		game.Reward.Prompt = strings.TrimSpace(game.Reward.Prompt)
		if _, ok := bindingByAlias(resources.Models, game.Reward.Model); !ok {
			return fmt.Errorf("gameplay.pet.games.%s.reward.model %q is not declared in resources.models", alias, game.Reward.Model)
		}
		if game.EnergyCost <= 0 || game.EnergyCost > 100 || game.PointsCost < 0 {
			return fmt.Errorf("gameplay.pet.games.%s requires energy_cost in 1..100 and non-negative points_cost", alias)
		}
		if game.Reward.Prompt == "" || game.Reward.PetExpMax < 0 || game.Reward.BadgeExpMaxPerBadge < 0 {
			return fmt.Errorf("gameplay.pet.games.%s.reward requires a prompt and non-negative maxima", alias)
		}
		normalized[alias] = game
	}
	pet.Games = normalized
	return nil
}

func listProfiles(ctx context.Context, store kv.Store, cursor *string, limit *int32) ([]apitypes.RuntimeProfile, bool, *string, error) {
	entries, hasNext, nextCursor, err := listPage(ctx, store, profilesRoot, cursor, limit)
	if err != nil {
		return nil, false, nil, err
	}
	items := make([]apitypes.RuntimeProfile, 0, len(entries))
	for _, entry := range entries {
		var item apitypes.RuntimeProfile
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return nil, false, nil, err
		}
		if err := setProfileRevision(&item); err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	return items, hasNext, nextCursor, nil
}

func listTokens(ctx context.Context, store kv.Store, cursor *string, limit *int32) ([]apitypes.RegistrationToken, bool, *string, error) {
	entries, hasNext, nextCursor, err := listPage(ctx, store, tokensRoot, cursor, limit)
	if err != nil {
		return nil, false, nil, err
	}
	items := make([]apitypes.RegistrationToken, 0, len(entries))
	for _, entry := range entries {
		var item tokenRecord
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return nil, false, nil, err
		}
		items = append(items, item.RegistrationToken)
	}
	return items, hasNext, nextCursor, nil
}

func listPage(ctx context.Context, store kv.Store, root kv.Key, cursor *string, limit *int32) ([]kv.Entry, bool, *string, error) {
	pageLimit := defaultListLimit
	if limit != nil && *limit > 0 {
		pageLimit = min(int(*limit), maxListLimit)
	}
	var after kv.Key
	if cursor != nil && *cursor != "" {
		after = append(append(kv.Key{}, root...), *cursor)
	}
	entries, err := kv.ListAfter(ctx, store, root, after, pageLimit+1)
	if err != nil {
		return nil, false, nil, err
	}
	if len(entries) <= pageLimit {
		return entries, false, nil, nil
	}
	entries = entries[:pageLimit]
	next := entries[len(entries)-1].Key[len(entries[len(entries)-1].Key)-1]
	return entries, true, &next, nil
}

func (s *Server) newToken() (string, error) {
	buf := make([]byte, tokenBytes)
	reader := s.Random
	if reader == nil {
		reader = rand.Reader
	}
	if _, err := io.ReadFull(reader, buf); err != nil {
		return "", fmt.Errorf("generate registration token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *Server) newUniqueToken(ctx context.Context, store kv.Store) (string, error) {
	for range tokenAttempts {
		raw, err := s.newToken()
		if err != nil {
			return "", err
		}
		_, err = store.Get(ctx, tokenHashKey(tokenDigest(raw)))
		if errors.Is(err, kv.ErrNotFound) {
			return raw, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", errors.New("generate registration token: repeated token collision")
}

func (s *Server) store() (kv.Store, error) {
	if s == nil || s.Store == nil {
		return nil, errors.New("runtime profile store not configured")
	}
	return s.Store, nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func tokenDigest(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func profileKey(name string) kv.Key   { return append(append(kv.Key{}, profilesRoot...), escape(name)) }
func tokenKey(name string) kv.Key     { return append(append(kv.Key{}, tokensRoot...), escape(name)) }
func tokenHashKey(hash string) kv.Key { return append(append(kv.Key{}, tokensByHashRoot...), hash) }

func escape(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(value, ":", "%3A")
}

func pathName(raw string) (string, error) {
	name, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("invalid path name: %w", err)
	}
	return name, nil
}

func invalid(message string) apitypes.ErrorResponse {
	return apitypes.NewErrorResponse("INVALID_RESOURCE", message)
}
func conflict(message string) apitypes.ErrorResponse {
	return apitypes.NewErrorResponse("RESOURCE_ALREADY_EXISTS", message)
}
func internalError(err error) apitypes.ErrorResponse {
	return apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())
}
func notFound(kind, name string) apitypes.ErrorResponse {
	return apitypes.NewErrorResponse("RESOURCE_NOT_FOUND", fmt.Sprintf("%s %q not found", kind, name))
}
