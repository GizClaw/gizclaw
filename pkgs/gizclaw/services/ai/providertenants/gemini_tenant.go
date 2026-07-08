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

var geminiTenantsRoot = kv.Key{"gemini-tenants", "by-name"}

func (s *Server) ListGeminiTenants(ctx context.Context, request adminhttp.ListGeminiTenantsRequestObject) (adminhttp.ListGeminiTenantsResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.ListGeminiTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listGeminiTenantsPage(ctx, store, cursor, limit)
	if err != nil {
		return adminhttp.ListGeminiTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListGeminiTenants200JSONResponse(adminhttp.GeminiTenantList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateGeminiTenant(ctx context.Context, request adminhttp.CreateGeminiTenantRequestObject) (adminhttp.CreateGeminiTenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.CreateGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateGeminiTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_GEMINI_TENANT", "request body required")), nil
	}
	tenant, err := normalizeGeminiTenantUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.CreateGeminiTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_GEMINI_TENANT", err.Error())), nil
	}
	if _, err := store.Get(ctx, geminiTenantKey(string(tenant.Name))); err == nil {
		return adminhttp.CreateGeminiTenant409JSONResponse(apitypes.NewErrorResponse("GEMINI_TENANT_ALREADY_EXISTS", fmt.Sprintf("Gemini tenant %q already exists", tenant.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err := writeGeminiTenant(ctx, store, tenant); err != nil {
		return adminhttp.CreateGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateGeminiTenant200JSONResponse(tenant), nil
}

func (s *Server) GetGeminiTenant(ctx context.Context, request adminhttp.GetGeminiTenantRequestObject) (adminhttp.GetGeminiTenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getGeminiTenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetGeminiTenant404JSONResponse(apitypes.NewErrorResponse("GEMINI_TENANT_NOT_FOUND", fmt.Sprintf("Gemini tenant %q not found", name))), nil
		}
		return adminhttp.GetGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetGeminiTenant200JSONResponse(tenant), nil
}

func (s *Server) PutGeminiTenant(ctx context.Context, request adminhttp.PutGeminiTenantRequestObject) (adminhttp.PutGeminiTenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.PutGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutGeminiTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_GEMINI_TENANT", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := normalizeGeminiTenantUpsert(*request.Body, name)
	if err != nil {
		return adminhttp.PutGeminiTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_GEMINI_TENANT", err.Error())), nil
	}
	previous, err := getGeminiTenant(ctx, store, name)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err == nil {
		tenant.CreatedAt = previous.CreatedAt
	}
	if err := writeGeminiTenant(ctx, store, tenant); err != nil {
		return adminhttp.PutGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutGeminiTenant200JSONResponse(tenant), nil
}

func (s *Server) DeleteGeminiTenant(ctx context.Context, request adminhttp.DeleteGeminiTenantRequestObject) (adminhttp.DeleteGeminiTenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getGeminiTenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteGeminiTenant404JSONResponse(apitypes.NewErrorResponse("GEMINI_TENANT_NOT_FOUND", fmt.Sprintf("Gemini tenant %q not found", name))), nil
		}
		return adminhttp.DeleteGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, geminiTenantKey(string(tenant.Name))); err != nil {
		return adminhttp.DeleteGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteGeminiTenant200JSONResponse(tenant), nil
}

func normalizeGeminiTenantUpsert(in adminhttp.GeminiTenantUpsert, expectedName string) (apitypes.GeminiTenant, error) {
	name := strings.TrimSpace(string(in.Name))
	if name == "" {
		return apitypes.GeminiTenant{}, errors.New("name is required")
	}
	if expectedName != "" && name != expectedName {
		return apitypes.GeminiTenant{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
	}
	credentialName := strings.TrimSpace(string(in.CredentialName))
	if credentialName == "" {
		return apitypes.GeminiTenant{}, errors.New("credential_name is required")
	}
	tenant := apitypes.GeminiTenant{
		CredentialName: string(credentialName),
		Name:           string(name),
	}
	if in.ProjectId != nil {
		projectID := strings.TrimSpace(*in.ProjectId)
		if projectID != "" {
			tenant.ProjectId = &projectID
		}
	}
	if in.Location != nil {
		location := strings.TrimSpace(*in.Location)
		if location != "" {
			tenant.Location = &location
		}
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

func listGeminiTenantsPage(ctx context.Context, store kv.Store, cursor string, limit int) ([]apitypes.GeminiTenant, bool, *string, error) {
	items := make([]apitypes.GeminiTenant, 0, limit+1)
	for entry, err := range store.List(ctx, geminiTenantsRoot) {
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
		var tenant apitypes.GeminiTenant
		if err := json.Unmarshal(entry.Value, &tenant); err != nil {
			return nil, false, nil, fmt.Errorf("gemini tenants: decode tenant list %s: %w", entry.Key.String(), err)
		}
		items = append(items, tenant)
		if len(items) >= limit+1 {
			break
		}
	}
	if len(items) == 0 {
		return []apitypes.GeminiTenant{}, false, nil, nil
	}
	hasNext := len(items) > limit
	if !hasNext {
		return items, false, nil, nil
	}
	page := items[:limit]
	next := escapeStoreSegment(string(page[len(page)-1].Name))
	return page, true, &next, nil
}

func writeGeminiTenant(ctx context.Context, store kv.Store, tenant apitypes.GeminiTenant) error {
	data, err := json.Marshal(tenant)
	if err != nil {
		return fmt.Errorf("gemini tenants: encode tenant %s: %w", tenant.Name, err)
	}
	if err := store.Set(ctx, geminiTenantKey(string(tenant.Name)), data); err != nil {
		return fmt.Errorf("gemini tenants: write tenant %s: %w", tenant.Name, err)
	}
	return nil
}

func getGeminiTenant(ctx context.Context, store kv.Store, name string) (apitypes.GeminiTenant, error) {
	data, err := store.Get(ctx, geminiTenantKey(name))
	if err != nil {
		return apitypes.GeminiTenant{}, err
	}
	var tenant apitypes.GeminiTenant
	if err := json.Unmarshal(data, &tenant); err != nil {
		return apitypes.GeminiTenant{}, fmt.Errorf("gemini tenants: decode tenant %s: %w", name, err)
	}
	return tenant, nil
}

func geminiTenantKey(name string) kv.Key {
	return append(append(kv.Key{}, geminiTenantsRoot...), escapeStoreSegment(name))
}
