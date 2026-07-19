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
	"net/url"
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

// Server owns RuntimeProfile and RegistrationToken state.
type Server struct {
	Store          kv.Store
	Now            func() time.Time
	Random         io.Reader
	FirmwareExists func(context.Context, string) (bool, error)
	mutationMu     sync.Mutex
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
	FirmwareName   string
	RuntimeProfile apitypes.RuntimeProfile
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
	if s.FirmwareExists != nil {
		exists, err := s.FirmwareExists(ctx, record.FirmwareName)
		if err != nil {
			return Registration{}, err
		}
		if !exists {
			return Registration{}, kv.ErrNotFound
		}
	}
	return Registration{TokenName: record.Name, FirmwareName: record.FirmwareName, RuntimeProfile: profile}, nil
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
	if err := customid.ValidateField("name", name); err != nil {
		return adminhttp.CreateRegistrationToken400JSONResponse(invalid(err.Error())), nil
	}
	firmwareName := strings.TrimSpace(in.FirmwareName)
	profileName := strings.TrimSpace(in.RuntimeProfileName)
	if firmwareName == "" || profileName == "" {
		return adminhttp.CreateRegistrationToken400JSONResponse(invalid("firmware_name and runtime_profile_name are required")), nil
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
	if s.FirmwareExists != nil {
		exists, err := s.FirmwareExists(ctx, firmwareName)
		if err != nil {
			return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
		}
		if !exists {
			return adminhttp.CreateRegistrationToken400JSONResponse(invalid("firmware_name does not exist")), nil
		}
	}
	raw, err := s.newUniqueToken(ctx, store)
	if err != nil {
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	digest := tokenDigest(raw)
	createdAt := s.now()
	record := tokenRecord{RegistrationToken: apitypes.RegistrationToken{Name: name, FirmwareName: firmwareName, RuntimeProfileName: profileName, CreatedAt: createdAt}, TokenHash: digest}
	encoded, err := json.Marshal(record)
	if err != nil {
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	if err := store.BatchSet(ctx, []kv.Entry{{Key: tokenKey(name), Value: encoded}, {Key: tokenHashKey(digest), Value: []byte(name)}}); err != nil {
		return adminhttp.CreateRegistrationToken500JSONResponse(internalError(err)), nil
	}
	return adminhttp.CreateRegistrationToken200JSONResponse(apitypes.RegistrationTokenCreateResult{Name: name, FirmwareName: firmwareName, RuntimeProfileName: profileName, CreatedAt: createdAt, Token: raw}), nil
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
	return item, nil
}

func writeProfile(ctx context.Context, store kv.Store, item apitypes.RuntimeProfile) error {
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
	if err := customid.ValidateField("name", name); err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if expectedName != "" && name != expectedName {
		return apitypes.RuntimeProfile{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
	}
	spec := in.Spec
	for _, resourceMap := range []*map[string]string{spec.Resources.Workflows, spec.Resources.Models, spec.Resources.Voices, spec.Resources.Tools, spec.Resources.PetDefs, spec.Resources.GameDefs, spec.Resources.BadgeDefs} {
		if resourceMap == nil {
			continue
		}
		normalized, err := normalizeResourceMap(*resourceMap)
		if err != nil {
			return apitypes.RuntimeProfile{}, err
		}
		*resourceMap = normalized
	}
	if spec.Gameplay != nil && spec.Gameplay.PetPool != nil {
		for i := range *spec.Gameplay.PetPool {
			entry := &(*spec.Gameplay.PetPool)[i]
			entry.PetDef = strings.TrimSpace(entry.PetDef)
			if entry.PetDef == "" || entry.Weight <= 0 {
				return apitypes.RuntimeProfile{}, fmt.Errorf("gameplay.pet_pool[%d] requires pet_def and positive weight", i)
			}
		}
	}
	if spec.Gameplay != nil && spec.Gameplay.Drive != nil {
		if err := normalizeRewardAliases(spec.Gameplay.Drive.DefaultReward); err != nil {
			return apitypes.RuntimeProfile{}, fmt.Errorf("gameplay.drive.default_reward: %w", err)
		}
		if spec.Gameplay.Drive.GameRewards != nil {
			normalized, err := normalizeGameRewards(*spec.Gameplay.Drive.GameRewards)
			if err != nil {
				return apitypes.RuntimeProfile{}, fmt.Errorf("gameplay.drive.game_rewards: %w", err)
			}
			*spec.Gameplay.Drive.GameRewards = normalized
		}
	}
	return apitypes.RuntimeProfile{Name: name, Spec: spec}, nil
}

func normalizeResourceMap(values map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(values))
	for alias, value := range values {
		alias = strings.TrimSpace(alias)
		value = strings.TrimSpace(value)
		if alias == "" || value == "" {
			return nil, errors.New("runtime profile resource aliases and names must not be empty")
		}
		if _, exists := out[alias]; exists {
			return nil, fmt.Errorf("duplicate runtime profile resource alias %q", alias)
		}
		out[alias] = value
	}
	return out, nil
}

func normalizeGameRewards(values map[string]apitypes.RuntimeProfileRewardSpec) (map[string]apitypes.RuntimeProfileRewardSpec, error) {
	out := make(map[string]apitypes.RuntimeProfileRewardSpec, len(values))
	for alias, reward := range values {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			return nil, errors.New("game definition alias must not be empty")
		}
		if _, exists := out[alias]; exists {
			return nil, fmt.Errorf("duplicate game definition alias %q", alias)
		}
		if err := normalizeRewardAliases(&reward); err != nil {
			return nil, fmt.Errorf("%s: %w", alias, err)
		}
		out[alias] = reward
	}
	return out, nil
}

func normalizeRewardAliases(reward *apitypes.RuntimeProfileRewardSpec) error {
	if reward == nil || reward.BadgeExpDelta == nil {
		return nil
	}
	out := make(map[string]int64, len(*reward.BadgeExpDelta))
	for alias, delta := range *reward.BadgeExpDelta {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			return errors.New("badge definition alias must not be empty")
		}
		if _, exists := out[alias]; exists {
			return fmt.Errorf("duplicate badge definition alias %q", alias)
		}
		out[alias] = delta
	}
	*reward.BadgeExpDelta = out
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
