package providertenants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var openAITenantsRoot = kv.Key{"openai-tenants", "by-name"}

func (s *Server) ListOpenAITenants(ctx context.Context, request adminhttp.ListOpenAITenantsRequestObject) (adminhttp.ListOpenAITenantsResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.ListOpenAITenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listOpenAITenantsPage(ctx, store, cursor, limit)
	if err != nil {
		return adminhttp.ListOpenAITenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListOpenAITenants200JSONResponse(adminhttp.OpenAITenantList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateOpenAITenant(ctx context.Context, request adminhttp.CreateOpenAITenantRequestObject) (adminhttp.CreateOpenAITenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.CreateOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateOpenAITenant400JSONResponse(apitypes.NewErrorResponse("INVALID_OPENAI_TENANT", "request body required")), nil
	}
	tenant, err := normalizeOpenAITenantUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.CreateOpenAITenant400JSONResponse(apitypes.NewErrorResponse("INVALID_OPENAI_TENANT", err.Error())), nil
	}
	if _, err := store.Get(ctx, openAITenantKey(string(tenant.Name))); err == nil {
		return adminhttp.CreateOpenAITenant409JSONResponse(apitypes.NewErrorResponse("OPENAI_TENANT_ALREADY_EXISTS", fmt.Sprintf("OpenAI tenant %q already exists", tenant.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err := writeOpenAITenant(ctx, store, tenant); err != nil {
		return adminhttp.CreateOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateOpenAITenant200JSONResponse(tenant), nil
}

func (s *Server) GetOpenAITenant(ctx context.Context, request adminhttp.GetOpenAITenantRequestObject) (adminhttp.GetOpenAITenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getOpenAITenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetOpenAITenant404JSONResponse(apitypes.NewErrorResponse("OPENAI_TENANT_NOT_FOUND", fmt.Sprintf("OpenAI tenant %q not found", name))), nil
		}
		return adminhttp.GetOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetOpenAITenant200JSONResponse(tenant), nil
}

func (s *Server) PutOpenAITenant(ctx context.Context, request adminhttp.PutOpenAITenantRequestObject) (adminhttp.PutOpenAITenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.PutOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutOpenAITenant400JSONResponse(apitypes.NewErrorResponse("INVALID_OPENAI_TENANT", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := normalizeOpenAITenantUpsert(*request.Body, name)
	if err != nil {
		return adminhttp.PutOpenAITenant400JSONResponse(apitypes.NewErrorResponse("INVALID_OPENAI_TENANT", err.Error())), nil
	}
	previous, err := getOpenAITenant(ctx, store, name)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err == nil {
		tenant.CreatedAt = previous.CreatedAt
	}
	if err := writeOpenAITenant(ctx, store, tenant); err != nil {
		return adminhttp.PutOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutOpenAITenant200JSONResponse(tenant), nil
}

func (s *Server) DeleteOpenAITenant(ctx context.Context, request adminhttp.DeleteOpenAITenantRequestObject) (adminhttp.DeleteOpenAITenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getOpenAITenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteOpenAITenant404JSONResponse(apitypes.NewErrorResponse("OPENAI_TENANT_NOT_FOUND", fmt.Sprintf("OpenAI tenant %q not found", name))), nil
		}
		return adminhttp.DeleteOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, openAITenantKey(string(tenant.Name))); err != nil {
		return adminhttp.DeleteOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteOpenAITenant200JSONResponse(tenant), nil
}

func normalizeOpenAITenantUpsert(in adminhttp.OpenAITenantUpsert, expectedName string) (apitypes.OpenAITenant, error) {
	name := strings.TrimSpace(string(in.Name))
	if name == "" {
		return apitypes.OpenAITenant{}, errors.New("name is required")
	}
	if expectedName != "" && name != expectedName {
		return apitypes.OpenAITenant{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
	}
	credentialName := strings.TrimSpace(string(in.CredentialName))
	if credentialName == "" {
		return apitypes.OpenAITenant{}, errors.New("credential_name is required")
	}
	kind := apitypes.OpenAITenantKindCompatible
	if in.Kind != nil {
		kind = apitypes.OpenAITenantKind(strings.TrimSpace(string(*in.Kind)))
	}
	if kind == "" {
		kind = apitypes.OpenAITenantKindCompatible
	}
	if !kind.Valid() {
		return apitypes.OpenAITenant{}, fmt.Errorf("unsupported kind %q", kind)
	}
	apiMode := apitypes.OpenAITenantAPIModeChatCompletions
	if in.ApiMode != nil {
		apiMode = apitypes.OpenAITenantAPIMode(strings.TrimSpace(string(*in.ApiMode)))
	}
	if apiMode == "" {
		apiMode = apitypes.OpenAITenantAPIModeChatCompletions
	}
	if !apiMode.Valid() {
		return apitypes.OpenAITenant{}, fmt.Errorf("unsupported api_mode %q", apiMode)
	}
	tenant := apitypes.OpenAITenant{
		ApiMode:        apiMode,
		CredentialName: string(credentialName),
		Kind:           kind,
		Name:           string(name),
	}
	if in.BaseUrl != nil {
		baseURL := strings.TrimSpace(*in.BaseUrl)
		if baseURL != "" {
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

func listOpenAITenantsPage(ctx context.Context, store kv.Store, cursor string, limit int) ([]apitypes.OpenAITenant, bool, *string, error) {
	items := make([]apitypes.OpenAITenant, 0, limit+1)
	for entry, err := range store.List(ctx, openAITenantsRoot) {
		if err != nil {
			return nil, false, nil, err
		}
		if len(entry.Key) == 0 {
			continue
		}
		lastSegment := entry.Key[len(entry.Key)-1]
		if cursor != "" && lastSegment <= cursor {
			continue
		}
		var tenant apitypes.OpenAITenant
		if err := json.Unmarshal(entry.Value, &tenant); err != nil {
			return nil, false, nil, fmt.Errorf("openai tenants: decode tenant list %s: %w", entry.Key.String(), err)
		}
		items = append(items, tenant)
		if len(items) >= limit+1 {
			break
		}
	}
	if len(items) == 0 {
		return []apitypes.OpenAITenant{}, false, nil, nil
	}
	hasNext := len(items) > limit
	if !hasNext {
		return items, false, nil, nil
	}
	page := items[:limit]
	next := escapeStoreSegment(string(page[len(page)-1].Name))
	return page, true, &next, nil
}

func writeOpenAITenant(ctx context.Context, store kv.Store, tenant apitypes.OpenAITenant) error {
	data, err := json.Marshal(tenant)
	if err != nil {
		return fmt.Errorf("openai tenants: encode tenant %s: %w", tenant.Name, err)
	}
	if err := store.Set(ctx, openAITenantKey(string(tenant.Name)), data); err != nil {
		return fmt.Errorf("openai tenants: write tenant %s: %w", tenant.Name, err)
	}
	return nil
}

func getOpenAITenant(ctx context.Context, store kv.Store, name string) (apitypes.OpenAITenant, error) {
	data, err := store.Get(ctx, openAITenantKey(name))
	if err != nil {
		return apitypes.OpenAITenant{}, err
	}
	var tenant apitypes.OpenAITenant
	if err := json.Unmarshal(data, &tenant); err != nil {
		return apitypes.OpenAITenant{}, fmt.Errorf("openai tenants: decode tenant %s: %w", name, err)
	}
	return tenant, nil
}

func openAITenantKey(name string) kv.Key {
	return append(append(kv.Key{}, openAITenantsRoot...), escapeStoreSegment(name))
}
