package providertenants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var openAITenantsRoot = kv.Key{"openai-tenants", "by-id"}

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
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	created, err := createTenant(ctx, store, openAITenantKey(tenant.Id), tenant)
	if err != nil {
		return adminhttp.CreateOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if !created {
		return adminhttp.CreateOpenAITenant409JSONResponse(apitypes.NewErrorResponse("OPENAI_TENANT_ALREADY_EXISTS", fmt.Sprintf("OpenAI tenant %q already exists", tenant.Id))), nil
	}
	return adminhttp.CreateOpenAITenant200JSONResponse(tenant), nil
}

func (s *Server) GetOpenAITenant(ctx context.Context, request adminhttp.GetOpenAITenantRequestObject) (adminhttp.GetOpenAITenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	tenant, err := getOpenAITenant(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetOpenAITenant404JSONResponse(apitypes.NewErrorResponse("OPENAI_TENANT_NOT_FOUND", fmt.Sprintf("OpenAI tenant %q not found", id))), nil
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
	id := string(request.Id)
	tenant, err := normalizeOpenAITenantUpsert(*request.Body, id)
	if err != nil {
		return adminhttp.PutOpenAITenant400JSONResponse(apitypes.NewErrorResponse("INVALID_OPENAI_TENANT", err.Error())), nil
	}
	previous, err := getOpenAITenant(ctx, store, id)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutOpenAITenant404JSONResponse(apitypes.NewErrorResponse("OPENAI_TENANT_NOT_FOUND", fmt.Sprintf("OpenAI tenant %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.PutOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.UpdatedAt = now
	tenant.CreatedAt = previous.CreatedAt
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
	id := string(request.Id)
	tenant, err := getOpenAITenant(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteOpenAITenant404JSONResponse(apitypes.NewErrorResponse("OPENAI_TENANT_NOT_FOUND", fmt.Sprintf("OpenAI tenant %q not found", id))), nil
		}
		return adminhttp.DeleteOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := deleteTenant(ctx, store, openAITenantKey(tenant.Id)); err != nil {
		return adminhttp.DeleteOpenAITenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteOpenAITenant200JSONResponse(tenant), nil
}

func normalizeOpenAITenantUpsert(in adminhttp.OpenAITenantUpsert, expectedID string) (apitypes.OpenAITenant, error) {
	id := string(in.Id)
	if err := validateResourceID(id); err != nil {
		return apitypes.OpenAITenant{}, err
	}
	if expectedID != "" && id != expectedID {
		return apitypes.OpenAITenant{}, fmt.Errorf("id %q must match path id %q", id, expectedID)
	}
	credentialID := string(in.CredentialId)
	if err := validateResourceReference("credential_id", credentialID); err != nil {
		return apitypes.OpenAITenant{}, err
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
		ApiMode:      apiMode,
		CredentialId: credentialID,
		Id:           id,
		Kind:         kind,
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
	next := escapeStoreSegment(string(page[len(page)-1].Id))
	return page, true, &next, nil
}

func writeOpenAITenant(ctx context.Context, store kv.Store, tenant apitypes.OpenAITenant) error {
	data, err := json.Marshal(tenant)
	if err != nil {
		return fmt.Errorf("openai tenants: encode tenant %s: %w", tenant.Id, err)
	}
	if err := store.Set(ctx, openAITenantKey(string(tenant.Id)), data); err != nil {
		return fmt.Errorf("openai tenants: write tenant %s: %w", tenant.Id, err)
	}
	return nil
}

func getOpenAITenant(ctx context.Context, store kv.Store, id string) (apitypes.OpenAITenant, error) {
	data, err := store.Get(ctx, openAITenantKey(id))
	if err != nil {
		return apitypes.OpenAITenant{}, err
	}
	var tenant apitypes.OpenAITenant
	if err := json.Unmarshal(data, &tenant); err != nil {
		return apitypes.OpenAITenant{}, fmt.Errorf("openai tenants: decode tenant %s: %w", id, err)
	}
	return tenant, nil
}

func openAITenantKey(id string) kv.Key {
	return append(append(kv.Key{}, openAITenantsRoot...), escapeStoreSegment(id))
}
