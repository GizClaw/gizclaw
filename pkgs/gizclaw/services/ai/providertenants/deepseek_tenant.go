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

var deepSeekTenantsRoot = kv.Key{"by-name"}

func (s *Server) ListDeepSeekTenants(ctx context.Context, request adminhttp.ListDeepSeekTenantsRequestObject) (adminhttp.ListDeepSeekTenantsResponseObject, error) {
	store, err := s.deepSeekTenantStore()
	if err != nil {
		return adminhttp.ListDeepSeekTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listDeepSeekTenantsPage(ctx, store, cursor, limit)
	if err != nil {
		return adminhttp.ListDeepSeekTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListDeepSeekTenants200JSONResponse(adminhttp.DeepSeekTenantList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateDeepSeekTenant(ctx context.Context, request adminhttp.CreateDeepSeekTenantRequestObject) (adminhttp.CreateDeepSeekTenantResponseObject, error) {
	store, err := s.deepSeekTenantStore()
	if err != nil {
		return adminhttp.CreateDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateDeepSeekTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DEEPSEEK_TENANT", "request body required")), nil
	}
	tenant, err := normalizeDeepSeekTenantUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.CreateDeepSeekTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DEEPSEEK_TENANT", err.Error())), nil
	}
	if _, err := store.Get(ctx, deepSeekTenantKey(string(tenant.Name))); err == nil {
		return adminhttp.CreateDeepSeekTenant409JSONResponse(apitypes.NewErrorResponse("DEEPSEEK_TENANT_ALREADY_EXISTS", fmt.Sprintf("DeepSeek tenant %q already exists", tenant.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err := writeDeepSeekTenant(ctx, store, tenant); err != nil {
		return adminhttp.CreateDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateDeepSeekTenant200JSONResponse(tenant), nil
}

func (s *Server) GetDeepSeekTenant(ctx context.Context, request adminhttp.GetDeepSeekTenantRequestObject) (adminhttp.GetDeepSeekTenantResponseObject, error) {
	store, err := s.deepSeekTenantStore()
	if err != nil {
		return adminhttp.GetDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getDeepSeekTenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetDeepSeekTenant404JSONResponse(apitypes.NewErrorResponse("DEEPSEEK_TENANT_NOT_FOUND", fmt.Sprintf("DeepSeek tenant %q not found", name))), nil
		}
		return adminhttp.GetDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetDeepSeekTenant200JSONResponse(tenant), nil
}

func (s *Server) PutDeepSeekTenant(ctx context.Context, request adminhttp.PutDeepSeekTenantRequestObject) (adminhttp.PutDeepSeekTenantResponseObject, error) {
	store, err := s.deepSeekTenantStore()
	if err != nil {
		return adminhttp.PutDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutDeepSeekTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DEEPSEEK_TENANT", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := normalizeDeepSeekTenantUpsert(*request.Body, name)
	if err != nil {
		return adminhttp.PutDeepSeekTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DEEPSEEK_TENANT", err.Error())), nil
	}
	previous, err := getDeepSeekTenant(ctx, store, name)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err == nil {
		tenant.CreatedAt = previous.CreatedAt
	}
	if err := writeDeepSeekTenant(ctx, store, tenant); err != nil {
		return adminhttp.PutDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutDeepSeekTenant200JSONResponse(tenant), nil
}

func (s *Server) DeleteDeepSeekTenant(ctx context.Context, request adminhttp.DeleteDeepSeekTenantRequestObject) (adminhttp.DeleteDeepSeekTenantResponseObject, error) {
	store, err := s.deepSeekTenantStore()
	if err != nil {
		return adminhttp.DeleteDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getDeepSeekTenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteDeepSeekTenant404JSONResponse(apitypes.NewErrorResponse("DEEPSEEK_TENANT_NOT_FOUND", fmt.Sprintf("DeepSeek tenant %q not found", name))), nil
		}
		return adminhttp.DeleteDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, deepSeekTenantKey(string(tenant.Name))); err != nil {
		return adminhttp.DeleteDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteDeepSeekTenant200JSONResponse(tenant), nil
}

func normalizeDeepSeekTenantUpsert(in adminhttp.DeepSeekTenantUpsert, expectedName string) (apitypes.DeepSeekTenant, error) {
	name := strings.TrimSpace(string(in.Name))
	if name == "" {
		return apitypes.DeepSeekTenant{}, errors.New("name is required")
	}
	if expectedName != "" && name != expectedName {
		return apitypes.DeepSeekTenant{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
	}
	credentialName := strings.TrimSpace(string(in.CredentialName))
	if credentialName == "" {
		return apitypes.DeepSeekTenant{}, errors.New("credential_name is required")
	}
	tenant := apitypes.DeepSeekTenant{
		CredentialName: string(credentialName),
		Name:           string(name),
	}
	if in.BaseUrl != nil {
		baseURL := strings.TrimSpace(*in.BaseUrl)
		if baseURL != "" {
			parsed, err := url.Parse(baseURL)
			if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return apitypes.DeepSeekTenant{}, errors.New("base_url must be an absolute HTTP(S) URL")
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

func listDeepSeekTenantsPage(ctx context.Context, store kv.Store, cursor string, limit int) ([]apitypes.DeepSeekTenant, bool, *string, error) {
	items := make([]apitypes.DeepSeekTenant, 0, limit+1)
	for entry, err := range store.List(ctx, deepSeekTenantsRoot) {
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
		var tenant apitypes.DeepSeekTenant
		if err := json.Unmarshal(entry.Value, &tenant); err != nil {
			return nil, false, nil, fmt.Errorf("deepseek tenants: decode tenant list %s: %w", entry.Key.String(), err)
		}
		items = append(items, tenant)
		if len(items) >= limit+1 {
			break
		}
	}
	if len(items) == 0 {
		return []apitypes.DeepSeekTenant{}, false, nil, nil
	}
	hasNext := len(items) > limit
	if !hasNext {
		return items, false, nil, nil
	}
	page := items[:limit]
	next := escapeStoreSegment(string(page[len(page)-1].Name))
	return page, true, &next, nil
}

func writeDeepSeekTenant(ctx context.Context, store kv.Store, tenant apitypes.DeepSeekTenant) error {
	data, err := json.Marshal(tenant)
	if err != nil {
		return fmt.Errorf("deepseek tenants: encode tenant %s: %w", tenant.Name, err)
	}
	if err := store.Set(ctx, deepSeekTenantKey(string(tenant.Name)), data); err != nil {
		return fmt.Errorf("deepseek tenants: write tenant %s: %w", tenant.Name, err)
	}
	return nil
}

func getDeepSeekTenant(ctx context.Context, store kv.Store, name string) (apitypes.DeepSeekTenant, error) {
	data, err := store.Get(ctx, deepSeekTenantKey(name))
	if err != nil {
		return apitypes.DeepSeekTenant{}, err
	}
	var tenant apitypes.DeepSeekTenant
	if err := json.Unmarshal(data, &tenant); err != nil {
		return apitypes.DeepSeekTenant{}, fmt.Errorf("deepseek tenants: decode tenant %s: %w", name, err)
	}
	return tenant, nil
}

func deepSeekTenantKey(name string) kv.Key {
	return append(append(kv.Key{}, deepSeekTenantsRoot...), escapeStoreSegment(name))
}
