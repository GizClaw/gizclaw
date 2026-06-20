package providertenants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/minimax-go"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	voicecatalog "github.com/GizClaw/gizclaw-go/pkg/gizclaw/voice"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

var (
	miniMaxTenantsRoot = kv.Key{"by-name"}
	credentialsRoot    = kv.Key{"by-name"}
)

const (
	defaultListLimit      = 50
	maxListLimit          = 200
	defaultMiniMaxBaseURL = "https://api.minimax.io"
	miniMaxProviderKind   = apitypes.VoiceProviderKind("minimax-tenant")
)

var fallbackMiniMaxBaseURLs = []string{
	"https://api.minimax.chat",
	"https://api.minimaxi.com",
}

type Server struct {
	Store                    kv.Store
	ModelStore               kv.Store
	TenantStore              kv.Store
	VolcTenantStore          kv.Store
	VoiceStore               kv.Store
	CredentialStore          kv.Store
	HTTPClient               *http.Client
	MiniMaxBaseURLs          []string
	VolcSpeakerClientFactory VolcSpeakerClientFactory
	Now                      func() time.Time
}

type ProviderTenantsAdminService interface {
	CreateDashScopeTenant(context.Context, adminservice.CreateDashScopeTenantRequestObject) (adminservice.CreateDashScopeTenantResponseObject, error)
	ListDashScopeTenants(context.Context, adminservice.ListDashScopeTenantsRequestObject) (adminservice.ListDashScopeTenantsResponseObject, error)
	DeleteDashScopeTenant(context.Context, adminservice.DeleteDashScopeTenantRequestObject) (adminservice.DeleteDashScopeTenantResponseObject, error)
	GetDashScopeTenant(context.Context, adminservice.GetDashScopeTenantRequestObject) (adminservice.GetDashScopeTenantResponseObject, error)
	PutDashScopeTenant(context.Context, adminservice.PutDashScopeTenantRequestObject) (adminservice.PutDashScopeTenantResponseObject, error)
	CreateGeminiTenant(context.Context, adminservice.CreateGeminiTenantRequestObject) (adminservice.CreateGeminiTenantResponseObject, error)
	ListGeminiTenants(context.Context, adminservice.ListGeminiTenantsRequestObject) (adminservice.ListGeminiTenantsResponseObject, error)
	DeleteGeminiTenant(context.Context, adminservice.DeleteGeminiTenantRequestObject) (adminservice.DeleteGeminiTenantResponseObject, error)
	GetGeminiTenant(context.Context, adminservice.GetGeminiTenantRequestObject) (adminservice.GetGeminiTenantResponseObject, error)
	PutGeminiTenant(context.Context, adminservice.PutGeminiTenantRequestObject) (adminservice.PutGeminiTenantResponseObject, error)
	CreateOpenAITenant(context.Context, adminservice.CreateOpenAITenantRequestObject) (adminservice.CreateOpenAITenantResponseObject, error)
	ListOpenAITenants(context.Context, adminservice.ListOpenAITenantsRequestObject) (adminservice.ListOpenAITenantsResponseObject, error)
	DeleteOpenAITenant(context.Context, adminservice.DeleteOpenAITenantRequestObject) (adminservice.DeleteOpenAITenantResponseObject, error)
	GetOpenAITenant(context.Context, adminservice.GetOpenAITenantRequestObject) (adminservice.GetOpenAITenantResponseObject, error)
	PutOpenAITenant(context.Context, adminservice.PutOpenAITenantRequestObject) (adminservice.PutOpenAITenantResponseObject, error)
	ListMiniMaxTenants(context.Context, adminservice.ListMiniMaxTenantsRequestObject) (adminservice.ListMiniMaxTenantsResponseObject, error)
	CreateMiniMaxTenant(context.Context, adminservice.CreateMiniMaxTenantRequestObject) (adminservice.CreateMiniMaxTenantResponseObject, error)
	DeleteMiniMaxTenant(context.Context, adminservice.DeleteMiniMaxTenantRequestObject) (adminservice.DeleteMiniMaxTenantResponseObject, error)
	GetMiniMaxTenant(context.Context, adminservice.GetMiniMaxTenantRequestObject) (adminservice.GetMiniMaxTenantResponseObject, error)
	PutMiniMaxTenant(context.Context, adminservice.PutMiniMaxTenantRequestObject) (adminservice.PutMiniMaxTenantResponseObject, error)
	SyncMiniMaxTenantVoices(context.Context, adminservice.SyncMiniMaxTenantVoicesRequestObject) (adminservice.SyncMiniMaxTenantVoicesResponseObject, error)
	ListVolcTenants(context.Context, adminservice.ListVolcTenantsRequestObject) (adminservice.ListVolcTenantsResponseObject, error)
	CreateVolcTenant(context.Context, adminservice.CreateVolcTenantRequestObject) (adminservice.CreateVolcTenantResponseObject, error)
	DeleteVolcTenant(context.Context, adminservice.DeleteVolcTenantRequestObject) (adminservice.DeleteVolcTenantResponseObject, error)
	GetVolcTenant(context.Context, adminservice.GetVolcTenantRequestObject) (adminservice.GetVolcTenantResponseObject, error)
	PutVolcTenant(context.Context, adminservice.PutVolcTenantRequestObject) (adminservice.PutVolcTenantResponseObject, error)
	SyncVolcTenantVoices(context.Context, adminservice.SyncVolcTenantVoicesRequestObject) (adminservice.SyncVolcTenantVoicesResponseObject, error)
}

var _ ProviderTenantsAdminService = (*Server)(nil)

func (s *Server) ListMiniMaxTenants(ctx context.Context, request adminservice.ListMiniMaxTenantsRequestObject) (adminservice.ListMiniMaxTenantsResponseObject, error) {
	store, err := s.tenantStore()
	if err != nil {
		return adminservice.ListMiniMaxTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listMiniMaxTenantsPage(ctx, store, cursor, limit)
	if err != nil {
		return adminservice.ListMiniMaxTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListMiniMaxTenants200JSONResponse(adminservice.MiniMaxTenantList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateMiniMaxTenant(ctx context.Context, request adminservice.CreateMiniMaxTenantRequestObject) (adminservice.CreateMiniMaxTenantResponseObject, error) {
	store, err := s.tenantStore()
	if err != nil {
		return adminservice.CreateMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminservice.CreateMiniMaxTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_MINIMAX_TENANT", "request body required")), nil
	}
	tenant, err := normalizeMiniMaxTenantUpsert(*request.Body, "")
	if err != nil {
		return adminservice.CreateMiniMaxTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_MINIMAX_TENANT", err.Error())), nil
	}
	credentialStore, err := s.credentialStore()
	if err != nil {
		return adminservice.CreateMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := validateTenantReferences(ctx, credentialStore, tenant); err != nil {
		return adminservice.CreateMiniMaxTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_MINIMAX_TENANT", err.Error())), nil
	}
	if _, err := store.Get(ctx, miniMaxTenantKey(string(tenant.Name))); err == nil {
		return adminservice.CreateMiniMaxTenant409JSONResponse(apitypes.NewErrorResponse("MINIMAX_TENANT_ALREADY_EXISTS", fmt.Sprintf("MiniMax tenant %q already exists", tenant.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminservice.CreateMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err := writeMiniMaxTenant(ctx, store, tenant); err != nil {
		return adminservice.CreateMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.CreateMiniMaxTenant200JSONResponse(tenant), nil
}

func (s *Server) DeleteMiniMaxTenant(ctx context.Context, request adminservice.DeleteMiniMaxTenantRequestObject) (adminservice.DeleteMiniMaxTenantResponseObject, error) {
	store, err := s.tenantStore()
	if err != nil {
		return adminservice.DeleteMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getMiniMaxTenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.DeleteMiniMaxTenant404JSONResponse(apitypes.NewErrorResponse("MINIMAX_TENANT_NOT_FOUND", fmt.Sprintf("MiniMax tenant %q not found", name))), nil
		}
		return adminservice.DeleteMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	voiceStore, err := s.voiceStore()
	if err != nil {
		return adminservice.DeleteMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := deleteMiniMaxTenantVoices(ctx, voiceStore, tenant.Name); err != nil {
		return adminservice.DeleteMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, miniMaxTenantKey(string(tenant.Name))); err != nil {
		return adminservice.DeleteMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.DeleteMiniMaxTenant200JSONResponse(tenant), nil
}

func (s *Server) GetMiniMaxTenant(ctx context.Context, request adminservice.GetMiniMaxTenantRequestObject) (adminservice.GetMiniMaxTenantResponseObject, error) {
	store, err := s.tenantStore()
	if err != nil {
		return adminservice.GetMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getMiniMaxTenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.GetMiniMaxTenant404JSONResponse(apitypes.NewErrorResponse("MINIMAX_TENANT_NOT_FOUND", fmt.Sprintf("MiniMax tenant %q not found", name))), nil
		}
		return adminservice.GetMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetMiniMaxTenant200JSONResponse(tenant), nil
}

func (s *Server) PutMiniMaxTenant(ctx context.Context, request adminservice.PutMiniMaxTenantRequestObject) (adminservice.PutMiniMaxTenantResponseObject, error) {
	store, err := s.tenantStore()
	if err != nil {
		return adminservice.PutMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminservice.PutMiniMaxTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_MINIMAX_TENANT", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := normalizeMiniMaxTenantUpsert(*request.Body, name)
	if err != nil {
		return adminservice.PutMiniMaxTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_MINIMAX_TENANT", err.Error())), nil
	}
	credentialStore, err := s.credentialStore()
	if err != nil {
		return adminservice.PutMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := validateTenantReferences(ctx, credentialStore, tenant); err != nil {
		return adminservice.PutMiniMaxTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_MINIMAX_TENANT", err.Error())), nil
	}
	previous, err := getMiniMaxTenant(ctx, store, name)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminservice.PutMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err == nil {
		tenant.CreatedAt = previous.CreatedAt
		tenant.LastSyncedAt = cloneTime(previous.LastSyncedAt)
	}
	if err := writeMiniMaxTenant(ctx, store, tenant); err != nil {
		return adminservice.PutMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.PutMiniMaxTenant200JSONResponse(tenant), nil
}

func (s *Server) SyncMiniMaxTenantVoices(ctx context.Context, request adminservice.SyncMiniMaxTenantVoicesRequestObject) (adminservice.SyncMiniMaxTenantVoicesResponseObject, error) {
	tenantStore, err := s.tenantStore()
	if err != nil {
		return adminservice.SyncMiniMaxTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	voiceStore, err := s.voiceStore()
	if err != nil {
		return adminservice.SyncMiniMaxTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	credentialStore, err := s.credentialStore()
	if err != nil {
		return adminservice.SyncMiniMaxTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getMiniMaxTenant(ctx, tenantStore, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.SyncMiniMaxTenantVoices404JSONResponse(apitypes.NewErrorResponse("MINIMAX_TENANT_NOT_FOUND", fmt.Sprintf("MiniMax tenant %q not found", name))), nil
		}
		return adminservice.SyncMiniMaxTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	credential, err := s.miniMaxCredentialForTenant(ctx, credentialStore, tenant)
	if err != nil {
		return adminservice.SyncMiniMaxTenantVoices400JSONResponse(apitypes.NewErrorResponse("INVALID_MINIMAX_TENANT", err.Error())), nil
	}
	upstream, err := s.listAllMiniMaxVoicesForTenant(ctx, tenant, credential)
	if err != nil {
		if miniMaxCredentialRejected(err) {
			return adminservice.SyncMiniMaxTenantVoices400JSONResponse(apitypes.NewErrorResponse("INVALID_MINIMAX_TENANT", fmt.Sprintf("MiniMax credential rejected by upstream: %v", err))), nil
		}
		return adminservice.SyncMiniMaxTenantVoices502JSONResponse(apitypes.NewErrorResponse("MINIMAX_SYNC_FAILED", err.Error())), nil
	}
	now := s.now()
	createdCount, updatedCount, deletedCount, err := reconcileTenantVoices(ctx, voiceStore, tenant, upstream, now)
	if err != nil {
		return adminservice.SyncMiniMaxTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	tenant.LastSyncedAt = &now
	tenant.UpdatedAt = now
	if err := writeMiniMaxTenant(ctx, tenantStore, tenant); err != nil {
		return adminservice.SyncMiniMaxTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.SyncMiniMaxTenantVoices200JSONResponse(adminservice.MiniMaxSyncVoicesResult{
		CreatedCount: createdCount,
		DeletedCount: deletedCount,
		SyncedAt:     now,
		TenantName:   tenant.Name,
		UpdatedCount: updatedCount,
	}), nil
}

func listMiniMaxTenantsPage(ctx context.Context, store kv.Store, cursor string, limit int) ([]apitypes.MiniMaxTenant, bool, *string, error) {
	entries, err := kv.ListAfter(ctx, store, miniMaxTenantsRoot, cursorAfterKey(miniMaxTenantsRoot, cursor), limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	pageEntries, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]apitypes.MiniMaxTenant, 0, len(pageEntries))
	for _, entry := range pageEntries {
		var tenant apitypes.MiniMaxTenant
		if err := json.Unmarshal(entry.Value, &tenant); err != nil {
			return nil, false, nil, fmt.Errorf("mmx: decode tenant list %s: %w", entry.Key.String(), err)
		}
		items = append(items, tenant)
	}
	return items, hasNext, nextCursor, nil
}

func normalizeMiniMaxTenantUpsert(in adminservice.MiniMaxTenantUpsert, expectedName string) (apitypes.MiniMaxTenant, error) {
	name := strings.TrimSpace(string(in.Name))
	if name == "" {
		return apitypes.MiniMaxTenant{}, errors.New("name is required")
	}
	if expectedName != "" && name != expectedName {
		return apitypes.MiniMaxTenant{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
	}
	appID := strings.TrimSpace(string(in.AppId))
	if appID == "" {
		return apitypes.MiniMaxTenant{}, errors.New("app_id is required")
	}
	groupID := strings.TrimSpace(string(in.GroupId))
	if groupID == "" {
		return apitypes.MiniMaxTenant{}, errors.New("group_id is required")
	}
	credentialName := strings.TrimSpace(string(in.CredentialName))
	if credentialName == "" {
		return apitypes.MiniMaxTenant{}, errors.New("credential_name is required")
	}
	tenant := apitypes.MiniMaxTenant{
		AppId:          string(appID),
		CredentialName: string(credentialName),
		GroupId:        string(groupID),
		Name:           string(name),
	}
	if in.BaseUrl != nil {
		baseURL := strings.TrimSpace(*in.BaseUrl)
		if baseURL != "" {
			parsed, err := url.Parse(baseURL)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				return apitypes.MiniMaxTenant{}, errors.New("base_url must be an absolute URL")
			}
			tenant.BaseUrl = &baseURL
		}
	}
	if in.Description != nil {
		description := strings.TrimSpace(*in.Description)
		if description != "" {
			tenant.Description = &description
		}
	}
	return tenant, nil
}

func validateTenantReferences(ctx context.Context, store kv.Store, tenant apitypes.MiniMaxTenant) error {
	if _, err := store.Get(ctx, credentialKey(string(tenant.CredentialName))); err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return fmt.Errorf("credential %q not found", tenant.CredentialName)
		}
		return err
	}
	return nil
}

func writeMiniMaxTenant(ctx context.Context, store kv.Store, tenant apitypes.MiniMaxTenant) error {
	data, err := json.Marshal(tenant)
	if err != nil {
		return fmt.Errorf("mmx: encode tenant %s: %w", tenant.Name, err)
	}
	if err := store.Set(ctx, miniMaxTenantKey(string(tenant.Name)), data); err != nil {
		return fmt.Errorf("mmx: write tenant %s: %w", tenant.Name, err)
	}
	return nil
}

func getMiniMaxTenant(ctx context.Context, store kv.Store, name string) (apitypes.MiniMaxTenant, error) {
	data, err := store.Get(ctx, miniMaxTenantKey(name))
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	var tenant apitypes.MiniMaxTenant
	if err := json.Unmarshal(data, &tenant); err != nil {
		return apitypes.MiniMaxTenant{}, fmt.Errorf("mmx: decode tenant %s: %w", name, err)
	}
	return tenant, nil
}

func (s *Server) miniMaxClientForTenant(ctx context.Context, store kv.Store, tenant apitypes.MiniMaxTenant) (*minimax.Client, error) {
	credential, err := s.miniMaxCredentialForTenant(ctx, store, tenant)
	if err != nil {
		return nil, err
	}
	apiKey, err := miniMaxAPIKey(credential)
	if err != nil {
		return nil, err
	}
	return s.newMiniMaxClient(apiKey, s.miniMaxBaseURLCandidates(tenant, credential)[0])
}

func (s *Server) miniMaxCredentialForTenant(ctx context.Context, store kv.Store, tenant apitypes.MiniMaxTenant) (apitypes.Credential, error) {
	credential, err := getCredential(ctx, store, string(tenant.CredentialName))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return apitypes.Credential{}, fmt.Errorf("credential %q not found", tenant.CredentialName)
		}
		return apitypes.Credential{}, err
	}
	provider := strings.TrimSpace(string(credential.Provider))
	if provider != "" && provider != "minimax" {
		return apitypes.Credential{}, fmt.Errorf("credential %q provider must be minimax", tenant.CredentialName)
	}
	if _, err := miniMaxAPIKey(credential); err != nil {
		return apitypes.Credential{}, err
	}
	return credential, nil
}

func (s *Server) listAllMiniMaxVoicesForTenant(ctx context.Context, tenant apitypes.MiniMaxTenant, credential apitypes.Credential) ([]minimax.Voice, error) {
	apiKey, err := miniMaxAPIKey(credential)
	if err != nil {
		return nil, err
	}
	var rejectedErr error
	for _, baseURL := range s.miniMaxBaseURLCandidates(tenant, credential) {
		client, err := s.newMiniMaxClient(apiKey, baseURL)
		if err != nil {
			return nil, err
		}
		voices, err := listAllMiniMaxVoices(ctx, client)
		if err == nil {
			return voices, nil
		}
		if !miniMaxCredentialRejected(err) {
			return nil, err
		}
		rejectedErr = err
	}
	if rejectedErr != nil {
		return nil, rejectedErr
	}
	return nil, errors.New("MiniMax voice endpoint list is empty")
}

func (s *Server) newMiniMaxClient(apiKey, baseURL string) (*minimax.Client, error) {
	client, err := minimax.NewClient(minimax.Config{
		APIKey:     apiKey,
		BaseURL:    baseURL,
		HTTPClient: s.HTTPClient,
	})
	if err != nil {
		return nil, fmt.Errorf("create MiniMax client: %w", err)
	}
	return client, nil
}

func miniMaxBaseURL(tenant apitypes.MiniMaxTenant) string {
	return miniMaxBaseURLCandidates(tenant, apitypes.Credential{}, nil)[0]
}

func (s *Server) miniMaxBaseURLCandidates(tenant apitypes.MiniMaxTenant, credential apitypes.Credential) []string {
	return miniMaxBaseURLCandidates(tenant, credential, s.MiniMaxBaseURLs)
}

func miniMaxBaseURLCandidates(tenant apitypes.MiniMaxTenant, credential apitypes.Credential, fallbackBaseURLs []string) []string {
	var candidates []string
	add := func(raw string) {
		baseURL := normalizeMiniMaxVoiceBaseURL(raw)
		if baseURL == "" {
			return
		}
		for _, existing := range candidates {
			if existing == baseURL {
				return
			}
		}
		candidates = append(candidates, baseURL)
	}
	if tenant.BaseUrl != nil {
		add(*tenant.BaseUrl)
	}
	body, _ := credential.Body.AsMiniMaxCredentialBody()
	add(ptrString(body.VoiceBaseUrl))
	add(ptrString(body.MinimaxVoiceBaseUrl))
	if len(fallbackBaseURLs) == 0 {
		fallbackBaseURLs = append([]string{defaultMiniMaxBaseURL}, fallbackMiniMaxBaseURLs...)
	}
	for _, baseURL := range fallbackBaseURLs {
		add(baseURL)
	}
	add(ptrString(body.BaseUrl))
	return candidates
}

func normalizeMiniMaxVoiceBaseURL(raw string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(raw), "/")
	if baseURL == "" {
		return ""
	}
	lower := strings.ToLower(baseURL)
	for _, suffix := range []string{"/anthropic", "/v1"} {
		if strings.HasSuffix(lower, suffix) {
			baseURL = strings.TrimRight(baseURL[:len(baseURL)-len(suffix)], "/")
			lower = strings.ToLower(baseURL)
		}
	}
	return baseURL
}

func miniMaxAPIKey(credential apitypes.Credential) (string, error) {
	body, err := credential.Body.AsMiniMaxCredentialBody()
	if err != nil {
		return "", err
	}
	if value := firstString(ptrString(body.ApiKey), ptrString(body.Token)); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("credential %q is missing api_key/token", credential.Name)
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func firstString(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func listAllMiniMaxVoices(ctx context.Context, client *minimax.Client) ([]minimax.Voice, error) {
	// Some MiniMax accounts do not return a fully populated catalog from a single
	// voice_type=all request. Fetch the aggregate view plus each concrete type and
	// merge by voice_id so sync operates on the full upstream catalog.
	voiceTypes := []string{"all", "system", "voice_cloning", "voice_generation"}
	merged := make(map[string]minimax.Voice)
	for _, voiceType := range voiceTypes {
		voices, err := listMiniMaxVoicesByType(ctx, client, voiceType)
		if err != nil {
			return nil, err
		}
		for _, voice := range voices {
			id := strings.TrimSpace(voice.VoiceID)
			if id == "" {
				return nil, errors.New("MiniMax returned voice without voice_id")
			}
			if existing, ok := merged[id]; ok {
				merged[id] = mergeMiniMaxVoice(existing, voice)
				continue
			}
			merged[id] = cloneMiniMaxVoice(voice)
		}
	}
	all := make([]minimax.Voice, 0, len(merged))
	for _, voice := range merged {
		all = append(all, voice)
	}
	sort.Slice(all, func(i, j int) bool {
		return strings.TrimSpace(all[i].VoiceID) < strings.TrimSpace(all[j].VoiceID)
	})
	return all, nil
}

func listMiniMaxVoicesByType(ctx context.Context, client *minimax.Client, voiceType string) ([]minimax.Voice, error) {
	const pageSize = 100
	var (
		all       []minimax.Voice
		pageToken string
	)
	for {
		req := &minimax.ListVoicesRequest{
			PageSize:  intPtr(pageSize),
			PageToken: pageToken,
			VoiceType: voiceType,
		}
		resp, err := client.Voice.ListVoices(ctx, req)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Voices...)
		if !resp.HasMore || strings.TrimSpace(resp.NextPageToken) == "" {
			return all, nil
		}
		pageToken = strings.TrimSpace(resp.NextPageToken)
	}
}

func miniMaxCredentialRejected(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid api key") ||
		strings.Contains(message, "login fail") ||
		strings.Contains(message, "status_code=2049") ||
		strings.Contains(message, "status_code=1004") ||
		strings.Contains(message, "status=401") ||
		strings.Contains(message, "status=403") ||
		strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "forbidden")
}

func mergeMiniMaxVoice(existing, candidate minimax.Voice) minimax.Voice {
	merged := cloneMiniMaxVoice(existing)
	if strings.TrimSpace(merged.VoiceID) == "" {
		return cloneMiniMaxVoice(candidate)
	}
	if strings.TrimSpace(merged.VoiceName) == "" {
		merged.VoiceName = strings.TrimSpace(candidate.VoiceName)
	}
	if len(merged.Description) == 0 && len(candidate.Description) > 0 {
		merged.Description = append([]string(nil), candidate.Description...)
	}
	if strings.TrimSpace(merged.CreatedTime) == "" {
		merged.CreatedTime = strings.TrimSpace(candidate.CreatedTime)
	}
	if strings.TrimSpace(merged.VoiceType) == "" {
		merged.VoiceType = strings.TrimSpace(candidate.VoiceType)
	}
	if len(candidate.Raw) == 0 {
		return merged
	}
	if merged.Raw == nil {
		merged.Raw = cloneMiniMaxRaw(candidate.Raw)
		return merged
	}
	for key, value := range candidate.Raw {
		if _, exists := merged.Raw[key]; exists {
			continue
		}
		merged.Raw[key] = append(json.RawMessage(nil), value...)
	}
	return merged
}

func cloneMiniMaxVoice(in minimax.Voice) minimax.Voice {
	out := in
	if in.Description != nil {
		out.Description = append([]string(nil), in.Description...)
	}
	out.Raw = cloneMiniMaxRaw(in.Raw)
	return out
}

func cloneMiniMaxRaw(in map[string]json.RawMessage) map[string]json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(in))
	for key, value := range in {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}

func reconcileTenantVoices(ctx context.Context, store kv.Store, tenant apitypes.MiniMaxTenant, upstream []minimax.Voice, now time.Time) (int32, int32, int32, error) {
	existing, err := voicecatalog.ListProvider(ctx, store, miniMaxProviderKind, string(tenant.Name))
	if err != nil {
		return 0, 0, 0, err
	}
	existingByProviderVoiceID := make(map[string]apitypes.Voice, len(existing))
	for _, voice := range existing {
		if voice.Source != apitypes.VoiceSourceSync {
			continue
		}
		providerVoiceID := voicecatalog.ProviderDataString(voice, "voice_id")
		if providerVoiceID == "" {
			continue
		}
		existingByProviderVoiceID[providerVoiceID] = voice
	}

	seen := make(map[string]struct{}, len(upstream))
	var createdCount, updatedCount int32
	for _, upstreamVoice := range upstream {
		providerVoiceID := strings.TrimSpace(upstreamVoice.VoiceID)
		if providerVoiceID == "" {
			return 0, 0, 0, errors.New("MiniMax returned voice without voice_id")
		}
		seen[providerVoiceID] = struct{}{}
		record := voiceFromMiniMax(tenant.Name, upstreamVoice, now)
		if previous, ok := existingByProviderVoiceID[providerVoiceID]; ok {
			record.CreatedAt = previous.CreatedAt
			if voicecatalog.SemanticEqual(previous, record) {
				record.UpdatedAt = previous.UpdatedAt
			} else {
				updatedCount++
			}
			previousCopy := previous
			if err := voicecatalog.Write(ctx, store, record, &previousCopy); err != nil {
				return 0, 0, 0, err
			}
			continue
		}
		if occupied, err := voicecatalog.Get(ctx, store, string(record.Id)); err == nil {
			if occupied.Source != apitypes.VoiceSourceSync {
				return 0, 0, 0, fmt.Errorf("voice id %q is occupied by non-sync resource", record.Id)
			}
			previousCopy := occupied
			if err := voicecatalog.Write(ctx, store, record, &previousCopy); err != nil {
				return 0, 0, 0, err
			}
			updatedCount++
			continue
		} else if !errors.Is(err, kv.ErrNotFound) {
			return 0, 0, 0, err
		}
		createdCount++
		if err := voicecatalog.Write(ctx, store, record, nil); err != nil {
			return 0, 0, 0, err
		}
	}

	var deletedCount int32
	for providerVoiceID, voice := range existingByProviderVoiceID {
		if _, ok := seen[providerVoiceID]; ok {
			continue
		}
		if err := voicecatalog.Delete(ctx, store, voice); err != nil {
			return 0, 0, 0, err
		}
		deletedCount++
	}
	return createdCount, updatedCount, deletedCount, nil
}

func voiceFromMiniMax(tenantName string, upstream minimax.Voice, now time.Time) apitypes.Voice {
	providerVoiceID := strings.TrimSpace(upstream.VoiceID)
	voiceID := voicecatalog.StableID(miniMaxProviderKind, string(tenantName), providerVoiceID)
	description := strings.TrimSpace(strings.Join(upstream.Description, ", "))
	name := strings.TrimSpace(upstream.VoiceName)
	voiceType := strings.TrimSpace(upstream.VoiceType)
	raw := rawMessagesToMap(upstream.Raw)
	providerValues := map[string]interface{}{
		"raw":      voicecatalog.RawMapValue(raw),
		"voice_id": providerVoiceID,
	}
	if voiceType != "" {
		providerValues["voice_type"] = voiceType
	}
	syncedAt := now
	voice := apitypes.Voice{
		CreatedAt: now,
		Id:        string(voiceID),
		Provider: apitypes.VoiceProvider{
			Kind: miniMaxProviderKind,
			Name: string(tenantName),
		},
		ProviderData: voicecatalog.ProviderData(miniMaxProviderKind, providerValues),
		Source:       apitypes.VoiceSourceSync,
		SyncedAt:     &syncedAt,
		UpdatedAt:    now,
	}
	if name != "" {
		voice.Name = &name
	}
	if description != "" {
		voice.Description = &description
	}
	return voice
}

func deleteMiniMaxTenantVoices(ctx context.Context, store kv.Store, tenantName string) error {
	voices, err := voicecatalog.ListProvider(ctx, store, miniMaxProviderKind, string(tenantName))
	if err != nil {
		return err
	}
	for _, voice := range voices {
		if voice.Source != apitypes.VoiceSourceSync {
			continue
		}
		if err := voicecatalog.Delete(ctx, store, voice); err != nil {
			return err
		}
	}
	return nil
}

func getCredential(ctx context.Context, store kv.Store, name string) (apitypes.Credential, error) {
	data, err := store.Get(ctx, credentialKey(name))
	if err != nil {
		return apitypes.Credential{}, err
	}
	var credential apitypes.Credential
	if err := json.Unmarshal(data, &credential); err != nil {
		return apitypes.Credential{}, fmt.Errorf("mmx: decode credential %s: %w", name, err)
	}
	return credential, nil
}

func rawMessagesToMap(raw map[string]json.RawMessage) *map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(raw))
	for key, value := range raw {
		var decoded interface{}
		if err := json.Unmarshal(value, &decoded); err != nil {
			out[key] = string(value)
			continue
		}
		out[key] = decoded
	}
	return &out
}

func miniMaxTenantKey(name string) kv.Key {
	return append(append(kv.Key{}, miniMaxTenantsRoot...), escapeStoreSegment(name))
}

func credentialKey(name string) kv.Key {
	return append(append(kv.Key{}, credentialsRoot...), escapeStoreSegment(name))
}

func escapeStoreSegment(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(value, ":", "%3A")
}

func unescapeStoreSegment(value string) string {
	unescaped, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return unescaped
}

func normalizeListParams(cursor *string, limit *int32) (string, int) {
	nextCursor := ""
	if cursor != nil {
		nextCursor = string(*cursor)
	}
	nextLimit := defaultListLimit
	if limit != nil {
		nextLimit = int(*limit)
	}
	if nextLimit <= 0 {
		nextLimit = defaultListLimit
	}
	if nextLimit > maxListLimit {
		nextLimit = maxListLimit
	}
	return nextCursor, nextLimit
}

func cursorAfterKey(prefix kv.Key, cursor string) kv.Key {
	if cursor == "" {
		return nil
	}
	after := append(kv.Key{}, prefix...)
	return append(after, cursor)
}

func paginateEntries(entries []kv.Entry, limit int) ([]kv.Entry, bool, *string) {
	if len(entries) == 0 {
		return nil, false, nil
	}
	hasNext := len(entries) > limit
	if !hasNext {
		return entries, false, nil
	}
	page := entries[:limit]
	if len(page) == 0 || len(page[len(page)-1].Key) == 0 {
		return page, true, nil
	}
	nextCursor := page[len(page)-1].Key[len(page[len(page)-1].Key)-1]
	return page, true, &nextCursor
}

func equalStringPtr(left, right *string) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return *left == *right
	}
}

func cloneTime(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneMap(in *map[string]interface{}) *map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(*in))
	for key, value := range *in {
		out[key] = value
	}
	return &out
}

func cloneVoiceProviderData(in *apitypes.VoiceProviderData) *apitypes.VoiceProviderData {
	if in == nil {
		return nil
	}
	data, err := in.MarshalJSON()
	if err != nil {
		return nil
	}
	var out apitypes.VoiceProviderData
	if err := out.UnmarshalJSON(data); err != nil {
		return nil
	}
	return &out
}

func intPtr(value int) *int {
	return &value
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Server) store() (kv.Store, error) {
	if s == nil {
		return nil, errors.New("provider tenant store not configured")
	}
	if s.ModelStore != nil {
		return s.ModelStore, nil
	}
	if s.Store == nil {
		return nil, errors.New("provider tenant store not configured")
	}
	return s.Store, nil
}

func (s *Server) tenantStore() (kv.Store, error) {
	if s == nil {
		return nil, errors.New("MiniMax tenant store not configured")
	}
	if s.TenantStore != nil {
		return s.TenantStore, nil
	}
	if s.Store == nil {
		return nil, errors.New("MiniMax tenant store not configured")
	}
	return s.Store, nil
}

func (s *Server) voiceStore() (kv.Store, error) {
	if s == nil {
		return nil, errors.New("MiniMax voice store not configured")
	}
	if s.VoiceStore != nil {
		return s.VoiceStore, nil
	}
	if s.Store == nil {
		return nil, errors.New("MiniMax voice store not configured")
	}
	return s.Store, nil
}

func (s *Server) credentialStore() (kv.Store, error) {
	if s == nil {
		return nil, errors.New("MiniMax credential store not configured")
	}
	if s.CredentialStore != nil {
		return s.CredentialStore, nil
	}
	if s.Store == nil {
		return nil, errors.New("MiniMax credential store not configured")
	}
	return s.Store, nil
}
