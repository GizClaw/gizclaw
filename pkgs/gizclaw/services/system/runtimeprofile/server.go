package runtimeprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/runtimealias"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var (
	profilesRoot        = kv.Key{"runtime-profiles", "by-id"}
	profilesByOwnerRoot = kv.Key{"runtime-profiles", "by-owner"}
	tokensRoot          = kv.Key{"registration-tokens", "by-id"}
	tokensByHashRoot    = kv.Key{"registration-tokens", "by-token-hash"}
)

const (
	defaultListLimit            = 50
	maxListLimit                = 200
	ownerBindingRollbackTimeout = 5 * time.Second
	maxWorkspaceRewardPrompt    = 8192
	maxWorkspaceRewardWindow    = 24 * time.Hour
	maxWorkspaceRewardPeriod    = 365 * 24 * time.Hour
)

var errResourceResolverNotConfigured = errors.New("resource resolver not configured")

// Server owns RuntimeProfile and RegistrationToken state.
type Server struct {
	Store           kv.Store
	Now             func() time.Time
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
	PutRegistrationToken(context.Context, adminhttp.PutRegistrationTokenRequestObject) (adminhttp.PutRegistrationTokenResponseObject, error)
}

var _ AdminService = (*Server)(nil)

// Registration is the connection-local result of consuming a RegistrationToken.
type Registration struct {
	TokenID        string
	RuntimeProfile apitypes.RuntimeProfile
	FirmwareID     *string
}

// ResolveProfile returns the current persisted revision for a profile ID.
// Registrations pin the ID, not a configuration snapshot.
func (s *Server) ResolveProfile(ctx context.Context, id string) (apitypes.RuntimeProfile, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if err := customid.ValidateResourceID(id); err != nil {
		return apitypes.RuntimeProfile{}, fmt.Errorf("runtime profile id: %w", err)
	}
	profile, err := GetProfile(ctx, store, id)
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if err := s.validateResources(ctx, profile.Spec); err != nil {
		return apitypes.RuntimeProfile{}, fmt.Errorf("runtime profile %q dependencies are invalid: %w", profile.Id, err)
	}
	return profile, nil
}

// BindOwnerProfile records the RuntimeProfile ID selected by an authenticated
// owner's successful registration. The ID remains resolvable after that
// connection closes; ResolveOwnerProfile always loads the current profile
// revision rather than persisting a configuration snapshot.
func (s *Server) BindOwnerProfile(ctx context.Context, owner, profileID string) error {
	return s.BindOwnerProfileAndCommit(ctx, owner, profileID, nil)
}

// BindOwnerProfileAndCommit changes an owner's selected RuntimeProfile and
// executes commit while the binding is isolated from concurrent readers. If
// commit fails, the previous binding is restored before the method returns.
func (s *Server) BindOwnerProfileAndCommit(ctx context.Context, owner, profileID string, commit func() error) error {
	store, err := s.store()
	if err != nil {
		return err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return errors.New("runtime profile owner and id are required")
	}
	if err := customid.ValidateResourceID(profileID); err != nil {
		return fmt.Errorf("runtime profile id: %w", err)
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if _, err := s.ResolveProfile(ctx, profileID); err != nil {
		return err
	}
	key := ownerProfileKey(owner)
	previous, previousErr := store.Get(ctx, key)
	if previousErr != nil && !errors.Is(previousErr, kv.ErrNotFound) {
		return previousErr
	}
	profile, err := GetProfile(ctx, store, profileID)
	if err != nil {
		return err
	}
	if err := store.Set(ctx, key, []byte(profile.Id)); err != nil {
		return err
	}
	if commit == nil {
		return nil
	}
	if err := commit(); err != nil {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ownerBindingRollbackTimeout)
		defer cancel()
		var rollbackErr error
		if previousErr == nil {
			rollbackErr = store.Set(rollbackCtx, key, previous)
		} else {
			rollbackErr = store.Delete(rollbackCtx, key)
		}
		if rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("restore owner RuntimeProfile binding: %w", rollbackErr))
		}
		return err
	}
	return nil
}

// ResolveOwnerProfile returns the current persisted revision of the profile
// most recently selected by an authenticated owner registration.
func (s *Server) ResolveOwnerProfile(ctx context.Context, owner string) (apitypes.RuntimeProfile, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return apitypes.RuntimeProfile{}, errors.New("runtime profile owner is required")
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	profileID, err := store.Get(ctx, ownerProfileKey(owner))
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	profile, err := getProfileByID(ctx, store, string(profileID))
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if err := s.validateResources(ctx, profile.Spec); err != nil {
		return apitypes.RuntimeProfile{}, fmt.Errorf("runtime profile %q dependencies are invalid: %w", profile.Id, err)
	}
	return profile, nil
}

// DeleteOwnerProfileBinding removes only the canonical owner's selected
// RuntimeProfile binding. Global profiles and registration tokens are retained.
func (s *Server) DeleteOwnerProfileBinding(ctx context.Context, owner string) error {
	store, err := s.store()
	if err != nil {
		return err
	}
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(owner)); err != nil || publicKey.IsZero() || publicKey.String() != owner {
		return errors.New("runtime profile owner must be a canonical public key")
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	key := ownerProfileKey(owner)
	if err := store.Delete(ctx, key); err != nil {
		return err
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
		if err == nil {
			return errors.New("runtime profile owner binding remains after deletion")
		}
		return err
	}
	return nil
}

func (s *Server) ResolveRegistration(ctx context.Context, rawToken string) (Registration, error) {
	store, err := s.store()
	if err != nil {
		return Registration{}, err
	}
	token := strings.TrimSpace(rawToken)
	digest := tokenDigest(token)
	idBytes, err := store.Get(ctx, tokenHashKey(digest))
	if err != nil {
		return Registration{}, err
	}
	item, err := getRegistrationTokenByID(ctx, store, string(idBytes))
	if err != nil {
		return Registration{}, err
	}
	// Hash-only records are intentionally incompatible and removed before rollout.
	if item.Token != token {
		return Registration{}, kv.ErrNotFound
	}
	profile, err := getProfileByID(ctx, store, item.RuntimeProfileId)
	if err != nil {
		return Registration{}, err
	}
	if item.FirmwareId != nil {
		if s.ResolveResource == nil {
			return Registration{}, errResourceResolverNotConfigured
		}
		resource, err := s.ResolveResource(ctx, apitypes.ResourceKindFirmware, *item.FirmwareId)
		if err != nil {
			return Registration{}, fmt.Errorf("resolve registration firmware: %w", err)
		}
		if _, err := resource.AsFirmwareResource(); err != nil {
			return Registration{}, errors.New("registration firmware_id does not reference a Firmware")
		}
	}
	return Registration{
		TokenID: item.Id, RuntimeProfile: profile,
		FirmwareID: cloneString(item.FirmwareId),
	}, nil
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
	now := s.now()
	item.CreatedAt, item.UpdatedAt = now, now
	if err := setProfileRevision(&item); err != nil {
		return adminhttp.CreateRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		return adminhttp.CreateRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	_, created, err := kv.CreateIfAbsent(ctx, store, kv.Entry{Key: profileKey(item.Id), Value: encoded}, nil)
	if err != nil {
		return adminhttp.CreateRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	if !created {
		return adminhttp.CreateRuntimeProfile409JSONResponse(conflict("runtime profile already exists")), nil
	}
	return adminhttp.CreateRuntimeProfile200JSONResponse(item), nil
}

func (s *Server) GetRuntimeProfile(ctx context.Context, request adminhttp.GetRuntimeProfileRequestObject) (adminhttp.GetRuntimeProfileResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	item, err := getProfileByID(ctx, store, id)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.GetRuntimeProfile404JSONResponse(notFound("runtime profile", id)), nil
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
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	item, err := normalizeProfile(*request.Body, id)
	if err != nil {
		return adminhttp.PutRuntimeProfile400JSONResponse(invalid(err.Error())), nil
	}
	if err := s.validateResources(ctx, item.Spec); err != nil {
		return adminhttp.PutRuntimeProfile400JSONResponse(invalid(err.Error())), nil
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	previous, getErr := getProfileByID(ctx, store, id)
	if errors.Is(getErr, kv.ErrNotFound) {
		return adminhttp.PutRuntimeProfile404JSONResponse(notFound("runtime profile", id)), nil
	}
	if getErr != nil {
		return adminhttp.PutRuntimeProfile500JSONResponse(internalError(getErr)), nil
	}
	now := s.now()
	item.CreatedAt, item.UpdatedAt = previous.CreatedAt, now
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
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	item, err := getProfileByID(ctx, store, id)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.DeleteRuntimeProfile404JSONResponse(notFound("runtime profile", id)), nil
	}
	if err != nil {
		return adminhttp.DeleteRuntimeProfile500JSONResponse(internalError(err)), nil
	}
	if err := store.Delete(ctx, profileKey(id)); err != nil {
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
	item, err := normalizeRegistrationToken(*request.Body, "")
	if err != nil {
		return adminhttp.CreateRegistrationToken400JSONResponse(invalid(err.Error())), nil
	}
	if err := s.validateRegistrationTokenFirmware(ctx, item); err != nil {
		if errors.Is(err, errResourceResolverNotConfigured) {
			return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
		}
		return adminhttp.CreateRegistrationToken400JSONResponse(invalid(err.Error())), nil
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if _, err := getProfileByID(ctx, store, item.RuntimeProfileId); err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.CreateRegistrationToken400JSONResponse(invalid("runtime_profile_id does not exist")), nil
		}
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	digest := tokenDigest(item.Token)
	if existing, err := store.Get(ctx, tokenHashKey(digest)); err == nil {
		return adminhttp.CreateRegistrationToken409JSONResponse(conflict(fmt.Sprintf("token is already used by registration token %q", string(existing)))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	now := s.now()
	item.CreatedAt, item.UpdatedAt = now, now
	encoded, err := json.Marshal(item)
	if err != nil {
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	_, created, err := kv.CreateIfAbsent(ctx, store,
		kv.Entry{Key: tokenKey(item.Id), Value: encoded},
		[]kv.Entry{{Key: tokenHashKey(digest), Value: []byte(item.Id)}},
	)
	if err != nil {
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	if !created {
		return adminhttp.CreateRegistrationToken409JSONResponse(conflict("registration token already exists")), nil
	}
	return adminhttp.CreateRegistrationToken200JSONResponse(item), nil
}

func (s *Server) PutRegistrationToken(ctx context.Context, request adminhttp.PutRegistrationTokenRequestObject) (adminhttp.PutRegistrationTokenResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.PutRegistrationToken500JSONResponse(internalError(err)), nil
	}
	if request.Body == nil {
		return adminhttp.PutRegistrationToken400JSONResponse(invalid("request body required")), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	item, err := normalizeRegistrationToken(*request.Body, id)
	if err != nil {
		return adminhttp.PutRegistrationToken400JSONResponse(invalid(err.Error())), nil
	}
	if err := s.validateRegistrationTokenFirmware(ctx, item); err != nil {
		if errors.Is(err, errResourceResolverNotConfigured) {
			return adminhttp.PutRegistrationToken500JSONResponse(internalError(err)), nil
		}
		return adminhttp.PutRegistrationToken400JSONResponse(invalid(err.Error())), nil
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	previous, getErr := getRegistrationTokenByID(ctx, store, id)
	if errors.Is(getErr, kv.ErrNotFound) {
		return adminhttp.PutRegistrationToken404JSONResponse(notFound("registration token", id)), nil
	}
	if getErr != nil {
		return adminhttp.PutRegistrationToken500JSONResponse(internalError(getErr)), nil
	}
	if _, err := getProfileByID(ctx, store, item.RuntimeProfileId); err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.PutRegistrationToken400JSONResponse(invalid("runtime_profile_id does not exist")), nil
		}
		return adminhttp.PutRegistrationToken500JSONResponse(internalError(err)), nil
	}
	digest := tokenDigest(item.Token)
	if existing, err := store.Get(ctx, tokenHashKey(digest)); err == nil && string(existing) != id {
		return adminhttp.PutRegistrationToken409JSONResponse(conflict(fmt.Sprintf("token is already used by registration token %q", string(existing)))), nil
	} else if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutRegistrationToken500JSONResponse(internalError(err)), nil
	}
	now := s.now()
	item.CreatedAt, item.UpdatedAt = previous.CreatedAt, now
	encoded, err := json.Marshal(item)
	if err != nil {
		return adminhttp.PutRegistrationToken500JSONResponse(internalError(err)), nil
	}
	deletes := make([]kv.Key, 0, 1)
	previousDigest := tokenDigest(previous.Token)
	if previousDigest != digest {
		deletes = append(deletes, tokenHashKey(previousDigest))
	}
	if err := store.BatchMutate(ctx, []kv.Entry{{Key: tokenKey(id), Value: encoded}, {Key: tokenHashKey(digest), Value: []byte(id)}}, deletes); err != nil {
		return adminhttp.PutRegistrationToken500JSONResponse(internalError(err)), nil
	}
	return adminhttp.PutRegistrationToken200JSONResponse(item), nil
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
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	item, err := getRegistrationTokenByID(ctx, store, id)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.GetRegistrationToken404JSONResponse(notFound("registration token", id)), nil
	}
	if err != nil {
		return adminhttp.GetRegistrationToken500JSONResponse(internalError(err)), nil
	}
	return adminhttp.GetRegistrationToken200JSONResponse(item), nil
}

func (s *Server) DeleteRegistrationToken(ctx context.Context, request adminhttp.DeleteRegistrationTokenRequestObject) (adminhttp.DeleteRegistrationTokenResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteRegistrationToken500JSONResponse(internalError(err)), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	item, err := getRegistrationTokenByID(ctx, store, id)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.DeleteRegistrationToken404JSONResponse(notFound("registration token", id)), nil
	}
	if err != nil {
		return adminhttp.DeleteRegistrationToken500JSONResponse(internalError(err)), nil
	}
	if err := store.BatchDelete(ctx, []kv.Key{tokenKey(id), tokenHashKey(tokenDigest(item.Token))}); err != nil {
		return adminhttp.DeleteRegistrationToken500JSONResponse(internalError(err)), nil
	}
	return adminhttp.DeleteRegistrationToken200JSONResponse(item), nil
}

func GetProfile(ctx context.Context, store kv.Store, id string) (apitypes.RuntimeProfile, error) {
	return getProfileByID(ctx, store, id)
}

func getProfileByID(ctx context.Context, store kv.Store, id string) (apitypes.RuntimeProfile, error) {
	data, err := store.Get(ctx, profileKey(id))
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	var item apitypes.RuntimeProfile
	if err := json.Unmarshal(data, &item); err != nil {
		return apitypes.RuntimeProfile{}, fmt.Errorf("runtime profile: decode %s: %w", id, err)
	}
	if err := setProfileRevision(&item); err != nil {
		return apitypes.RuntimeProfile{}, fmt.Errorf("runtime profile: revision %s: %w", id, err)
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
	return store.Set(ctx, profileKey(item.Id), data)
}

func getRegistrationTokenByID(ctx context.Context, store kv.Store, id string) (apitypes.RegistrationToken, error) {
	data, err := store.Get(ctx, tokenKey(id))
	if err != nil {
		return apitypes.RegistrationToken{}, err
	}
	var item apitypes.RegistrationToken
	if err := json.Unmarshal(data, &item); err != nil {
		return apitypes.RegistrationToken{}, fmt.Errorf("registration token: decode %s: %w", id, err)
	}
	return item, nil
}

func normalizeRegistrationToken(in adminhttp.RegistrationTokenUpsert, expectedID string) (apitypes.RegistrationToken, error) {
	id := in.Id
	if err := customid.ValidateResourceID(id); err != nil {
		return apitypes.RegistrationToken{}, err
	}
	if expectedID != "" && id != expectedID {
		return apitypes.RegistrationToken{}, fmt.Errorf("id %q must match path id %q", id, expectedID)
	}
	token := strings.TrimSpace(in.Token)
	if token == "" {
		return apitypes.RegistrationToken{}, errors.New("token is required")
	}
	profileID := in.RuntimeProfileId
	if err := customid.ValidateResourceID(profileID); err != nil {
		return apitypes.RegistrationToken{}, fmt.Errorf("runtime_profile_id: %w", err)
	}
	var firmwareID *string
	if in.FirmwareId != nil {
		value := *in.FirmwareId
		if err := customid.ValidateResourceID(value); err != nil {
			return apitypes.RegistrationToken{}, fmt.Errorf("firmware_id: %w", err)
		}
		firmwareID = &value
	}
	return apitypes.RegistrationToken{
		Id:               id,
		Token:            token,
		RuntimeProfileId: profileID,
		FirmwareId:       firmwareID,
	}, nil
}

func (s *Server) validateRegistrationTokenFirmware(ctx context.Context, item apitypes.RegistrationToken) error {
	if item.FirmwareId == nil {
		return nil
	}
	if s.ResolveResource == nil {
		return errResourceResolverNotConfigured
	}
	resource, err := s.ResolveResource(ctx, apitypes.ResourceKindFirmware, *item.FirmwareId)
	if err != nil {
		return errors.New("firmware_id does not exist")
	}
	discriminator, err := resource.Discriminator()
	if err != nil || (discriminator != string(apitypes.ResourceKindFirmware) && discriminator != string(apitypes.ResourceKindFirmware)+"Resource") {
		return errors.New("firmware_id does not reference a Firmware")
	}
	return nil
}

func normalizeProfile(in adminhttp.RuntimeProfileUpsert, expectedID string) (apitypes.RuntimeProfile, error) {
	id := in.Id
	if err := customid.ValidateResourceID(id); err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if expectedID != "" && id != expectedID {
		return apitypes.RuntimeProfile{}, fmt.Errorf("id %q must match path id %q", id, expectedID)
	}
	spec := in.Spec
	allAliases := make(map[string]string)
	workflowAliases := make(map[string]string)
	for path, workflowID := range map[string]string{
		"workflows.system.friend_chatroom": spec.Workflows.System.FriendChatroom,
		"workflows.system.group_chatroom":  spec.Workflows.System.GroupChatroom,
		"workflows.system.pet":             spec.Workflows.System.Pet,
	} {
		if err := customid.ValidateResourceID(workflowID); err != nil {
			return apitypes.RuntimeProfile{}, fmt.Errorf("%s: %w", path, err)
		}
	}
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
	if spec.Resources.Memories != nil {
		normalized := make(map[string]apitypes.RuntimeProfileMemoryBinding, len(*spec.Resources.Memories))
		for rawAlias, binding := range *spec.Resources.Memories {
			alias := strings.TrimSpace(rawAlias)
			if err := ValidateAlias("memory", alias); err != nil {
				return apitypes.RuntimeProfile{}, err
			}
			if _, duplicate := normalized[alias]; duplicate {
				return apitypes.RuntimeProfile{}, fmt.Errorf("memory alias %q is duplicated after normalization", alias)
			}
			if err := registerProfileAlias(allAliases, alias, "memory"); err != nil {
				return apitypes.RuntimeProfile{}, err
			}
			next, err := normalizeMemoryBinding(binding)
			if err != nil {
				return apitypes.RuntimeProfile{}, fmt.Errorf("resources.memories.%s: %w", alias, err)
			}
			normalized[alias] = next
		}
		spec.Resources.Memories = &normalized
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
			if entry.PetDef == "" || entry.Weight <= 0 {
				return apitypes.RuntimeProfile{}, fmt.Errorf("gameplay.adoption.pool[%d] requires pet_def and positive weight", i)
			}
			if entry.AdoptionCost != nil && *entry.AdoptionCost < 0 {
				return apitypes.RuntimeProfile{}, fmt.Errorf("gameplay.adoption.pool[%d].adoption_cost must not be negative", i)
			}
			if _, ok := bindingByAlias(spec.Resources.PetDefs, entry.PetDef); !ok {
				return apitypes.RuntimeProfile{}, fmt.Errorf("gameplay.adoption.pool[%d].pet_def %q is not declared in resources.pet_defs", i, entry.PetDef)
			}
		}
	}
	if spec.Gameplay != nil && spec.Gameplay.Pet != nil {
		if err := normalizePetGameplay(spec.Gameplay.Pet, spec.Resources); err != nil {
			return apitypes.RuntimeProfile{}, err
		}
	}
	if spec.Gameplay != nil && spec.Gameplay.WorkspaceReward != nil {
		if err := normalizeWorkspaceReward(spec.Gameplay.WorkspaceReward, spec.Resources); err != nil {
			return apitypes.RuntimeProfile{}, err
		}
	}
	item := apitypes.RuntimeProfile{Id: id, Spec: spec}
	if err := setProfileRevision(&item); err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	return item, nil
}

func normalizeMemoryBinding(binding apitypes.RuntimeProfileMemoryBinding) (apitypes.RuntimeProfileMemoryBinding, error) {
	if err := customid.ValidateResourceID(binding.LayoutId); err != nil {
		return binding, fmt.Errorf("layout_id: %w", err)
	}
	if !binding.Driver.Valid() {
		return binding, fmt.Errorf("unsupported driver %q", binding.Driver)
	}
	connectionType, err := binding.Connection.Discriminator()
	if err != nil {
		return binding, fmt.Errorf("connection: %w", err)
	}
	switch connectionType {
	case "flowcraft_bbh":
		if binding.Driver != apitypes.RuntimeProfileMemoryDriverFlowcraft {
			return binding, fmt.Errorf("driver %q cannot use connection type %q", binding.Driver, connectionType)
		}
	case "flowcraft_object_store":
		if binding.Driver != apitypes.RuntimeProfileMemoryDriverFlowcraft {
			return binding, fmt.Errorf("driver %q cannot use connection type %q", binding.Driver, connectionType)
		}
		value, err := binding.Connection.AsRuntimeProfileFlowcraftObjectStoreConnection()
		value.Directory = strings.TrimSpace(value.Directory)
		if err != nil || value.Directory == "" {
			return binding, errors.New("flowcraft_object_store connection requires directory")
		}
		if err := binding.Connection.FromRuntimeProfileFlowcraftObjectStoreConnection(value); err != nil {
			return binding, err
		}
	case "flowcraft_postgresql":
		if binding.Driver != apitypes.RuntimeProfileMemoryDriverFlowcraft {
			return binding, fmt.Errorf("driver %q cannot use connection type %q", binding.Driver, connectionType)
		}
		value, err := binding.Connection.AsRuntimeProfileFlowcraftPostgreSQLConnection()
		value.Dsn = strings.TrimSpace(value.Dsn)
		if err != nil || value.Dsn == "" {
			return binding, errors.New("flowcraft_postgresql connection requires dsn")
		}
		if err := binding.Connection.FromRuntimeProfileFlowcraftPostgreSQLConnection(value); err != nil {
			return binding, err
		}
	case "mem0":
		if binding.Driver != apitypes.RuntimeProfileMemoryDriverMem0 {
			return binding, fmt.Errorf("driver %q cannot use connection type %q", binding.Driver, connectionType)
		}
		value, err := binding.Connection.AsRuntimeProfileMem0Connection()
		if err != nil {
			return binding, err
		}
		value.ProjectId = strings.TrimSpace(value.ProjectId)
		value.Endpoint = strings.TrimSpace(value.Endpoint)
		value.ApiKey = strings.TrimSpace(value.ApiKey)
		value.PollInterval = trimOptionalString(value.PollInterval)
		if value.ProjectId == "" || value.ApiKey == "" {
			return binding, errors.New("mem0 connection requires project_id and api_key")
		}
		if err := validateMemoryEndpoint(value.Endpoint); err != nil {
			return binding, err
		}
		if err := validateMemoryPollInterval(value.PollInterval); err != nil {
			return binding, err
		}
		if err := binding.Connection.FromRuntimeProfileMem0Connection(value); err != nil {
			return binding, err
		}
	case "volc_mem0":
		if binding.Driver != apitypes.RuntimeProfileMemoryDriverVolcMem0 {
			return binding, fmt.Errorf("driver %q cannot use connection type %q", binding.Driver, connectionType)
		}
		value, err := binding.Connection.AsRuntimeProfileVolcMem0Connection()
		if err != nil {
			return binding, err
		}
		value.MemoryProjectId = strings.TrimSpace(value.MemoryProjectId)
		value.Endpoint = strings.TrimSpace(value.Endpoint)
		value.ApiKey = strings.TrimSpace(value.ApiKey)
		value.PollInterval = trimOptionalString(value.PollInterval)
		if value.MemoryProjectId == "" || value.ApiKey == "" {
			return binding, errors.New("volc_mem0 connection requires memory_project_id and api_key")
		}
		if err := validateMemoryEndpoint(value.Endpoint); err != nil {
			return binding, err
		}
		if err := validateMemoryPollInterval(value.PollInterval); err != nil {
			return binding, err
		}
		if err := binding.Connection.FromRuntimeProfileVolcMem0Connection(value); err != nil {
			return binding, err
		}
	default:
		return binding, fmt.Errorf("unsupported connection type %q", connectionType)
	}
	return binding, nil
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func validateMemoryEndpoint(raw string) error {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || endpoint.Host == "" || endpoint.Scheme != "https" && endpoint.Scheme != "http" {
		return errors.New("connection endpoint must be an absolute http or https URL")
	}
	if endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return errors.New("connection endpoint must not contain userinfo, query, or fragment")
	}
	return nil
}

func validateMemoryPollInterval(raw *string) error {
	if raw == nil {
		return nil
	}
	value, err := time.ParseDuration(strings.TrimSpace(*raw))
	if err != nil || value <= 0 {
		return errors.New("connection poll_interval must be a positive duration")
	}
	return nil
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
	workflows := make([]resolvedWorkflow, 0, 3)
	resolveSystemWorkflow := func(path, resourceID string, wantDriver apitypes.WorkflowDriver) error {
		resource, err := resolve(path, apitypes.ResourceKindWorkflow, apitypes.RuntimeProfileBinding{ResourceId: resourceID})
		if err != nil {
			return err
		}
		workflow, err := resource.AsWorkflowResource()
		if err != nil {
			return fmt.Errorf("%s %q returned an invalid Workflow: %w", path, resourceID, err)
		}
		if workflow.Spec.Driver != wantDriver {
			return fmt.Errorf("%s %q has driver %q, want %q", path, resourceID, workflow.Spec.Driver, wantDriver)
		}
		workflows = append(workflows, resolvedWorkflow{path: path, resource: workflow})
		return nil
	}
	if err := resolveSystemWorkflow("workflows.system.friend_chatroom", spec.Workflows.System.FriendChatroom, apitypes.WorkflowDriverChatroom); err != nil {
		return err
	}
	if err := resolveSystemWorkflow("workflows.system.group_chatroom", spec.Workflows.System.GroupChatroom, apitypes.WorkflowDriverChatroom); err != nil {
		return err
	}
	if err := resolveSystemWorkflow("workflows.system.pet", spec.Workflows.System.Pet, apitypes.WorkflowDriverPet); err != nil {
		return err
	}
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
	voices := make(map[string]apitypes.VoiceResource)
	if spec.Resources.Voices != nil {
		for alias, binding := range *spec.Resources.Voices {
			path := "resources.voices." + alias
			resource, err := resolve(path, apitypes.ResourceKindVoice, binding)
			if err != nil {
				return err
			}
			voice, err := resource.AsVoiceResource()
			if err != nil {
				return fmt.Errorf("%s.resource_id %q returned an invalid Voice: %w", path, binding.ResourceId, err)
			}
			voices[alias] = voice
		}
	}
	memories := make(map[string]apitypes.MemoryLayoutResource)
	if spec.Resources.Memories != nil {
		for alias, binding := range *spec.Resources.Memories {
			path := "resources.memories." + alias
			resource, err := s.ResolveResource(ctx, apitypes.ResourceKindMemoryLayout, binding.LayoutId)
			if err != nil {
				return fmt.Errorf("%s.layout_id %q does not resolve to MemoryLayout: %w", path, binding.LayoutId, err)
			}
			discriminator, err := resource.Discriminator()
			if err != nil || discriminator != string(apitypes.ResourceKindMemoryLayout) && discriminator != string(apitypes.ResourceKindMemoryLayout)+"Resource" {
				return fmt.Errorf("%s.layout_id %q did not return a MemoryLayout", path, binding.LayoutId)
			}
			layout, err := resource.AsMemoryLayoutResource()
			if err != nil {
				return fmt.Errorf("%s.layout_id %q returned an invalid MemoryLayout: %w", path, binding.LayoutId, err)
			}
			if err := validateMemoryLayoutRuntimeAliases(path, binding.Driver, layout.Spec, models); err != nil {
				return err
			}
			memories[alias] = layout
		}
	}
	groups := []struct {
		path   string
		kind   apitypes.ResourceKind
		values *map[string]apitypes.RuntimeProfileBinding
	}{
		{path: "resources.tools", kind: apitypes.ResourceKindTool, values: spec.Resources.Tools},
		{path: "resources.game_defs", kind: apitypes.ResourceKindGameDef, values: spec.Resources.GameDefs},
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
	badgeDefs := make(map[string]apitypes.BadgeDefSpec)
	if spec.Resources.BadgeDefs != nil {
		for alias, binding := range *spec.Resources.BadgeDefs {
			resource, err := resolve("resources.badge_defs."+alias, apitypes.ResourceKindBadgeDef, binding)
			if err != nil {
				return err
			}
			badgeDef, err := resource.AsBadgeDefResource()
			if err != nil {
				return fmt.Errorf("resources.badge_defs.%s.resource_id %q returned an invalid BadgeDef: %w", alias, binding.ResourceId, err)
			}
			badgeDefs[alias] = badgeDef.Spec
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
		if err := validateWorkflowRuntimeAliases(workflow.path, workflow.resource.Spec, models, voices, memories); err != nil {
			return err
		}
	}
	if spec.Gameplay != nil && spec.Gameplay.Pet != nil {
		if err := validatePetRewardModels(*spec.Gameplay.Pet, models); err != nil {
			return err
		}
	}
	if spec.Gameplay != nil && spec.Gameplay.WorkspaceReward != nil && spec.Gameplay.WorkspaceReward.Enabled {
		reward := spec.Gameplay.WorkspaceReward
		if reward.Evaluation == nil {
			return errors.New("gameplay.workspace_reward.evaluation is required when enabled")
		}
		model, ok := models[reward.Evaluation.Model]
		if !ok {
			return fmt.Errorf("gameplay.workspace_reward.evaluation.model alias %q is not declared in resources.models", reward.Evaluation.Model)
		}
		if model.Spec.Kind != apitypes.ModelKindLlm {
			return fmt.Errorf("gameplay.workspace_reward.evaluation.model alias %q has kind %q, want %q", reward.Evaluation.Model, model.Spec.Kind, apitypes.ModelKindLlm)
		}
		if reward.Badges != nil {
			for alias := range *reward.Badges {
				badgeDef, ok := badgeDefs[alias]
				if !ok {
					return fmt.Errorf("gameplay.workspace_reward.badges.%s is not declared in resources.badge_defs", alias)
				}
				if badgeDef.RewardPrompt == nil || strings.TrimSpace(*badgeDef.RewardPrompt) == "" {
					return fmt.Errorf("gameplay.workspace_reward.badges.%s requires BadgeDef reward_prompt", alias)
				}
			}
		}
	}
	return nil
}

func validateMemoryLayoutRuntimeAliases(path string, driver apitypes.RuntimeProfileMemoryDriver, layout apitypes.MemoryLayoutSpec, models map[string]apitypes.ModelResource) error {
	if driver != apitypes.RuntimeProfileMemoryDriverFlowcraft {
		return nil
	}
	requireModel := func(field, alias string, kind apitypes.ModelKind) error {
		model, ok := models[strings.TrimSpace(alias)]
		if !ok {
			return fmt.Errorf("%s.layout.%s model alias %q is not declared in resources.models", path, field, alias)
		}
		if model.Spec.Kind != kind {
			return fmt.Errorf("%s.layout.%s model alias %q has kind %q, want %q", path, field, alias, model.Spec.Kind, kind)
		}
		return nil
	}
	if err := requireModel("flowcraft.extraction.model", layout.Flowcraft.Extraction.Model, apitypes.ModelKindLlm); err != nil {
		return err
	}
	if layout.Flowcraft.Embedding != nil {
		if err := requireModel("flowcraft.embedding.model", layout.Flowcraft.Embedding.Model, apitypes.ModelKindEmbedding); err != nil {
			return err
		}
	}
	if layout.Flowcraft.Rerank != nil {
		if err := requireModel("flowcraft.rerank.model", layout.Flowcraft.Rerank.Model, apitypes.ModelKindLlm); err != nil {
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

func validateWorkflowRuntimeAliases(path string, workflow apitypes.WorkflowSpec, models map[string]apitypes.ModelResource, voices map[string]apitypes.VoiceResource, memorySets ...map[string]apitypes.MemoryLayoutResource) error {
	var memories map[string]apitypes.MemoryLayoutResource
	if len(memorySets) > 0 {
		memories = memorySets[0]
	}
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
	requireDashScopeRealtimeModel := func(field, alias string) error {
		if err := requireModel(field, alias, apitypes.ModelKindRealtime); err != nil {
			return err
		}
		model := models[strings.TrimSpace(alias)]
		if model.Spec.Provider.Kind != apitypes.ModelProviderKindDashscopeTenant {
			return fmt.Errorf("%s.%s model alias %q has provider %q, want %q", path, field, alias, model.Spec.Provider.Kind, apitypes.ModelProviderKindDashscopeTenant)
		}
		data, err := model.Spec.ProviderData.AsDashScopeTenantModelProviderData()
		if err != nil || data.ApiMode == nil || *data.ApiMode != apitypes.DashScopeTenantModelProviderDataApiModeRealtime {
			return fmt.Errorf("%s.%s model alias %q must use dashscope-tenant api_mode %q", path, field, alias, apitypes.DashScopeTenantModelProviderDataApiModeRealtime)
		}
		return nil
	}
	requireDoubaoRealtimeDuplexModel := func(field, alias string) error {
		if err := requireModel(field, alias, apitypes.ModelKindRealtimeDuplex); err != nil {
			return err
		}
		model := models[strings.TrimSpace(alias)]
		if model.Spec.Provider.Kind != apitypes.ModelProviderKindVolcTenant {
			return fmt.Errorf("%s.%s model alias %q has provider %q, want %q", path, field, alias, model.Spec.Provider.Kind, apitypes.ModelProviderKindVolcTenant)
		}
		data, err := model.Spec.ProviderData.AsVolcTenantModelProviderData()
		if err != nil || data.ApiMode != apitypes.VolcTenantModelProviderDataApiModeRealtimeDuplex {
			return fmt.Errorf("%s.%s model alias %q must use volc-tenant api_mode %q", path, field, alias, apitypes.VolcTenantModelProviderDataApiModeRealtimeDuplex)
		}
		return nil
	}
	requireVoice := func(field, alias string) error {
		alias = strings.TrimSpace(alias)
		if _, ok := voices[alias]; !ok {
			return fmt.Errorf("%s.%s voice alias %q is not declared in resources.voices", path, field, alias)
		}
		return nil
	}
	if workflow.Memory != nil {
		alias := strings.TrimSpace(string(*workflow.Memory))
		if _, ok := memories[alias]; !ok {
			return fmt.Errorf("%s.memory alias %q is not declared in resources.memories", path, alias)
		}
	}
	requireCompatibleVoice := func(field, voiceAlias, modelAlias string) error {
		if err := requireVoice(field, voiceAlias); err != nil {
			return err
		}
		voice := voices[strings.TrimSpace(voiceAlias)]
		model := models[strings.TrimSpace(modelAlias)]
		if voice.Spec.Provider.Kind != apitypes.VoiceProviderKind(model.Spec.Provider.Kind) ||
			voice.Spec.Provider.Id != model.Spec.Provider.Id {
			return fmt.Errorf(
				"%s.%s voice alias %q uses provider %q/%q, want %q/%q to match model alias %q",
				path, field, voiceAlias,
				voice.Spec.Provider.Kind, voice.Spec.Provider.Id,
				model.Spec.Provider.Kind, model.Spec.Provider.Id,
				modelAlias,
			)
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
		if workflow.Chatroom == nil || workflow.Chatroom.Transcript == nil {
			break
		}
		transcript := workflow.Chatroom.Transcript
		if transcript.Enabled != nil && *transcript.Enabled && (transcript.AsrModel == nil || strings.TrimSpace(*transcript.AsrModel) == "") {
			return fmt.Errorf("%s.transcript.asr_model is required when transcription is enabled", path)
		}
		if transcript.AsrModel != nil && strings.TrimSpace(*transcript.AsrModel) != "" {
			return requireModel("transcript.asr_model", *transcript.AsrModel, apitypes.ModelKindAsr)
		}
	case apitypes.WorkflowDriverPet:
		if workflow.Pet == nil {
			return fmt.Errorf("%s has no pet spec", path)
		}
		nested := apitypes.WorkflowSpec{
			Driver:               apitypes.WorkflowDriver(workflow.Pet.Driver),
			Toolkit:              workflow.Pet.Toolkit,
			Flowcraft:            workflow.Pet.Flowcraft,
			DoubaoRealtime:       workflow.Pet.DoubaoRealtime,
			DashscopeRealtime:    workflow.Pet.DashscopeRealtime,
			DoubaoRealtimeDuplex: workflow.Pet.DoubaoRealtimeDuplex,
			Eino:                 workflow.Pet.Eino,
			AstTranslate:         workflow.Pet.AstTranslate,
			Chatroom:             workflow.Pet.Chatroom,
		}
		return validateWorkflowRuntimeAliases(path+".pet", nested, models, voices, memories)
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
		return requireCompatibleVoice("audio.output.voice", *workflow.DoubaoRealtime.Audio.Output.Voice, workflow.DoubaoRealtime.Model)
	case apitypes.WorkflowDriverDashscopeRealtime:
		if workflow.DashscopeRealtime == nil {
			return fmt.Errorf("%s has no dashscope_realtime spec", path)
		}
		if err := requireDashScopeRealtimeModel("model", workflow.DashscopeRealtime.Model); err != nil {
			return err
		}
		if workflow.DashscopeRealtime.Voice != nil && strings.TrimSpace(*workflow.DashscopeRealtime.Voice) != "" {
			return requireCompatibleVoice("voice", *workflow.DashscopeRealtime.Voice, workflow.DashscopeRealtime.Model)
		}
	case apitypes.WorkflowDriverDoubaoRealtimeDuplex:
		if workflow.DoubaoRealtimeDuplex == nil {
			return fmt.Errorf("%s has no doubao_realtime_duplex spec", path)
		}
		if err := requireDoubaoRealtimeDuplexModel("model", workflow.DoubaoRealtimeDuplex.Model); err != nil {
			return err
		}
		if workflow.DoubaoRealtimeDuplex.Voice != nil && strings.TrimSpace(*workflow.DoubaoRealtimeDuplex.Voice) != "" {
			return requireCompatibleVoice("voice", *workflow.DoubaoRealtimeDuplex.Voice, workflow.DoubaoRealtimeDuplex.Model)
		}
	case apitypes.WorkflowDriverEino:
		if workflow.Eino == nil {
			return fmt.Errorf("%s has no eino spec", path)
		}
		return validateEinoRuntimeAliases(path, workflow.Eino.Graph, requireModel)
	case apitypes.WorkflowDriverFlowcraft:
		if workflow.Flowcraft == nil {
			return fmt.Errorf("%s has no flowcraft spec", path)
		}
		flowcraft := *workflow.Flowcraft
		modelAliases := make([]struct {
			field string
			alias string
			kind  apitypes.ModelKind
		}, 0, len(flowcraft.Graph.Nodes)+4)
		for index, raw := range flowcraft.Graph.Nodes {
			if discriminator, _ := raw.Discriminator(); discriminator == "llm" {
				node, err := raw.AsFlowcraftLLMNode()
				if err != nil {
					return fmt.Errorf("%s.graph.nodes[%d]: %w", path, index, err)
				}
				modelAliases = append(modelAliases, struct {
					field string
					alias string
					kind  apitypes.ModelKind
				}{field: fmt.Sprintf("graph.nodes[%d].config.model", index), alias: node.Config.Model, kind: apitypes.ModelKindLlm})
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
		if strings.TrimSpace(flowcraft.Graph.Entry) == "" || len(flowcraft.Graph.Nodes) == 0 {
			return fmt.Errorf("%s.graph must have an entry and at least one node", path)
		}
		entryFound := false
		for _, raw := range flowcraft.Graph.Nodes {
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
			entryFound = entryFound || node.ID == flowcraft.Graph.Entry
		}
		if !entryFound {
			return fmt.Errorf("%s.graph.entry %q is not a defined node", path, flowcraft.Graph.Entry)
		}
	}
	return nil
}

func validateEinoRuntimeAliases(path string, graph apitypes.EinoGraph, requireModel func(string, string, apitypes.ModelKind) error) error {
	for index, raw := range graph.Nodes {
		discriminator, err := raw.Discriminator()
		if err != nil {
			return fmt.Errorf("%s.graph.nodes[%d]: %w", path, index, err)
		}
		nodePath := fmt.Sprintf("graph.nodes[%d]", index)
		switch discriminator {
		case "chat_model":
			node, err := raw.AsEinoChatModelNode()
			if err != nil {
				return fmt.Errorf("%s.%s: %w", path, nodePath, err)
			}
			if err := requireModel(nodePath+".model", node.Model, apitypes.ModelKindLlm); err != nil {
				return err
			}
		case "batch":
			node, err := raw.AsEinoBatchNode()
			if err != nil {
				return fmt.Errorf("%s.%s: %w", path, nodePath, err)
			}
			if err := validateEinoRuntimeAliases(path+"."+nodePath+".graph", node.Graph, requireModel); err != nil {
				return err
			}
		case "subgraph":
			node, err := raw.AsEinoSubgraphNode()
			if err != nil {
				return fmt.Errorf("%s.%s: %w", path, nodePath, err)
			}
			if err := validateEinoRuntimeAliases(path+"."+nodePath+".graph", node.Graph, requireModel); err != nil {
				return err
			}
		case "race":
			node, err := raw.AsEinoRaceNode()
			if err != nil {
				return fmt.Errorf("%s.%s: %w", path, nodePath, err)
			}
			for branchIndex, branch := range node.Branches {
				branchPath := fmt.Sprintf("%s.%s.branches[%d].graph", path, nodePath, branchIndex)
				if err := validateEinoRuntimeAliases(branchPath, branch.Graph, requireModel); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func normalizeBindingMap(values map[string]apitypes.RuntimeProfileBinding) (map[string]apitypes.RuntimeProfileBinding, error) {
	out := make(map[string]apitypes.RuntimeProfileBinding, len(values))
	for alias, binding := range values {
		alias = strings.TrimSpace(alias)
		if err := ValidateAlias("resource alias", alias); err != nil {
			return nil, err
		}
		if err := customid.ValidateResourceID(binding.ResourceId); err != nil {
			return nil, fmt.Errorf("runtime profile binding %q resource_id: %w", alias, err)
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
	return runtimealias.Validate(kind, value)
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
		gameDefID := gameDef.ResourceId
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

func normalizeWorkspaceReward(reward *apitypes.RuntimeProfileWorkspaceRewardSpec, resources apitypes.RuntimeProfileResources) error {
	if reward == nil {
		return nil
	}
	if !reward.Enabled {
		*reward = apitypes.RuntimeProfileWorkspaceRewardSpec{Enabled: false}
		return nil
	}
	if reward.WorkspaceKinds == nil || reward.Debounce == nil || reward.Transcript == nil ||
		reward.Evaluation == nil || reward.Points == nil || reward.Badges == nil ||
		reward.RollingBudget == nil {
		return errors.New("gameplay.workspace_reward requires workspace_kinds, debounce, transcript, evaluation, points, badges, and rolling_budget when enabled")
	}
	kinds := append([]apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKinds(nil), (*reward.WorkspaceKinds)...)
	if len(kinds) == 0 || len(kinds) > 3 {
		return errors.New("gameplay.workspace_reward.workspace_kinds requires 1..3 entries")
	}
	seenKinds := make(map[apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKinds]struct{}, len(kinds))
	for _, kind := range kinds {
		if !kind.Valid() {
			return fmt.Errorf("gameplay.workspace_reward.workspace_kinds contains unsupported kind %q", kind)
		}
		if _, exists := seenKinds[kind]; exists {
			return fmt.Errorf("gameplay.workspace_reward.workspace_kinds contains duplicate kind %q", kind)
		}
		seenKinds[kind] = struct{}{}
	}
	slices.Sort(kinds)
	reward.WorkspaceKinds = &kinds

	quietPeriod, err := parseWorkspaceRewardDuration("gameplay.workspace_reward.debounce.quiet_period", reward.Debounce.QuietPeriod, maxWorkspaceRewardWindow)
	if err != nil {
		return err
	}
	maxWindowAge, err := parseWorkspaceRewardDuration("gameplay.workspace_reward.debounce.max_window_age", reward.Debounce.MaxWindowAge, maxWorkspaceRewardWindow)
	if err != nil {
		return err
	}
	if maxWindowAge < quietPeriod {
		return errors.New("gameplay.workspace_reward.debounce.max_window_age must be greater than or equal to quiet_period")
	}
	reward.Debounce.QuietPeriod = quietPeriod.String()
	reward.Debounce.MaxWindowAge = maxWindowAge.String()

	if reward.Transcript.MaxEntries <= 0 || reward.Transcript.MaxEntries > 1000 ||
		reward.Transcript.MaxTextBytes <= 0 || reward.Transcript.MaxTextBytes > 1<<20 {
		return errors.New("gameplay.workspace_reward.transcript requires max_entries in 1..1000 and max_text_bytes in 1..1048576")
	}
	evaluation := reward.Evaluation
	evaluation.Model = strings.TrimSpace(evaluation.Model)
	evaluation.PointsPrompt = strings.TrimSpace(evaluation.PointsPrompt)
	if evaluation.Model == "" {
		return errors.New("gameplay.workspace_reward.evaluation.model is required")
	}
	if _, ok := bindingByAlias(resources.Models, evaluation.Model); !ok {
		return fmt.Errorf("gameplay.workspace_reward.evaluation.model %q is not declared in resources.models", evaluation.Model)
	}
	if !utf8.ValidString(evaluation.PointsPrompt) || evaluation.PointsPrompt == "" || len([]byte(evaluation.PointsPrompt)) > maxWorkspaceRewardPrompt {
		return fmt.Errorf("gameplay.workspace_reward.evaluation.points_prompt must be 1..%d UTF-8 bytes", maxWorkspaceRewardPrompt)
	}
	if evaluation.ScoreMin < 0 || evaluation.ScoreMax < evaluation.ScoreMin ||
		evaluation.QualifyingScore < evaluation.ScoreMin || evaluation.QualifyingScore > evaluation.ScoreMax ||
		evaluation.ScoreMax > 1_000_000 {
		return errors.New("gameplay.workspace_reward.evaluation requires 0 <= score_min <= qualifying_score <= score_max <= 1000000")
	}
	if len(reward.Points.Tiers) == 0 || len(reward.Points.Tiers) > 100 {
		return errors.New("gameplay.workspace_reward.points.tiers requires 1..100 entries")
	}
	previousScore := int64(-1)
	for i, tier := range reward.Points.Tiers {
		if tier.MinScore < evaluation.ScoreMin || tier.MinScore > evaluation.ScoreMax || tier.MinScore <= previousScore {
			return fmt.Errorf("gameplay.workspace_reward.points.tiers[%d].min_score must be strictly increasing inside the score range", i)
		}
		if tier.Delta < 0 || tier.Delta > 1_000_000 {
			return fmt.Errorf("gameplay.workspace_reward.points.tiers[%d].delta must be in 0..1000000", i)
		}
		previousScore = tier.MinScore
	}
	if len(*reward.Badges) > 64 {
		return errors.New("gameplay.workspace_reward.badges supports at most 64 entries")
	}
	normalizedBadges := make(map[string]apitypes.RuntimeProfileWorkspaceRewardBadgeSpec, len(*reward.Badges))
	for rawAlias, policy := range *reward.Badges {
		alias := strings.TrimSpace(rawAlias)
		if err := ValidateAlias("workspace reward badge alias", alias); err != nil {
			return err
		}
		if _, ok := bindingByAlias(resources.BadgeDefs, alias); !ok {
			return fmt.Errorf("gameplay.workspace_reward.badges.%s is not declared in resources.badge_defs", alias)
		}
		if policy.MaxExpPerWindow <= 0 || policy.MaxExpPerWindow > 1_000_000 {
			return fmt.Errorf("gameplay.workspace_reward.badges.%s.max_exp_per_window must be in 1..1000000", alias)
		}
		if _, duplicate := normalizedBadges[alias]; duplicate {
			return fmt.Errorf("gameplay.workspace_reward.badges contains duplicate alias %q", alias)
		}
		normalizedBadges[alias] = policy
	}
	reward.Badges = &normalizedBadges

	period, err := parseWorkspaceRewardDuration("gameplay.workspace_reward.rolling_budget.period", reward.RollingBudget.Period, maxWorkspaceRewardPeriod)
	if err != nil {
		return err
	}
	reward.RollingBudget.Period = period.String()
	if reward.RollingBudget.PointsMax < 0 || reward.RollingBudget.PointsMax > 1_000_000_000 ||
		reward.RollingBudget.BadgeExpMax < 0 || reward.RollingBudget.BadgeExpMax > 1_000_000_000 {
		return errors.New("gameplay.workspace_reward.rolling_budget limits must be in 0..1000000000")
	}
	return nil
}

func parseWorkspaceRewardDuration(path, raw string, maximum time.Duration) (time.Duration, error) {
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || value <= 0 || value > maximum {
		return 0, fmt.Errorf("%s must be a positive Go duration no greater than %s", path, maximum)
	}
	return value, nil
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
		var item apitypes.RegistrationToken
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
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

func profileKey(id string) kv.Key { return append(append(kv.Key{}, profilesRoot...), escape(id)) }
func ownerProfileKey(owner string) kv.Key {
	return append(append(kv.Key{}, profilesByOwnerRoot...), escape(owner))
}
func tokenKey(id string) kv.Key       { return append(append(kv.Key{}, tokensRoot...), escape(id)) }
func tokenHashKey(hash string) kv.Key { return append(append(kv.Key{}, tokensByHashRoot...), hash) }

func escape(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(value, ":", "%3A")
}

func pathID(raw string) (string, error) {
	if err := customid.ValidateResourceID(raw); err != nil {
		return "", fmt.Errorf("invalid path id: %w", err)
	}
	return raw, nil
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
func notFound(kind, id string) apitypes.ErrorResponse {
	return apitypes.NewErrorResponse("RESOURCE_NOT_FOUND", fmt.Sprintf("%s %q not found", kind, id))
}
