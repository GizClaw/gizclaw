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

var dashScopeTenantsRoot = kv.Key{"dashscope-tenants", "by-name"}

func (s *Server) ListDashScopeTenants(ctx context.Context, request adminhttp.ListDashScopeTenantsRequestObject) (adminhttp.ListDashScopeTenantsResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.ListDashScopeTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listDashScopeTenantsPage(ctx, store, cursor, limit)
	if err != nil {
		return adminhttp.ListDashScopeTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListDashScopeTenants200JSONResponse(adminhttp.DashScopeTenantList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateDashScopeTenant(ctx context.Context, request adminhttp.CreateDashScopeTenantRequestObject) (adminhttp.CreateDashScopeTenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.CreateDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateDashScopeTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DASHSCOPE_TENANT", "request body required")), nil
	}
	tenant, err := normalizeDashScopeTenantUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.CreateDashScopeTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DASHSCOPE_TENANT", err.Error())), nil
	}
	if _, err := store.Get(ctx, dashScopeTenantKey(string(tenant.Name))); err == nil {
		return adminhttp.CreateDashScopeTenant409JSONResponse(apitypes.NewErrorResponse("DASHSCOPE_TENANT_ALREADY_EXISTS", fmt.Sprintf("DashScope tenant %q already exists", tenant.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err := writeDashScopeTenant(ctx, store, tenant); err != nil {
		return adminhttp.CreateDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateDashScopeTenant200JSONResponse(tenant), nil
}

func (s *Server) GetDashScopeTenant(ctx context.Context, request adminhttp.GetDashScopeTenantRequestObject) (adminhttp.GetDashScopeTenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getDashScopeTenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetDashScopeTenant404JSONResponse(apitypes.NewErrorResponse("DASHSCOPE_TENANT_NOT_FOUND", fmt.Sprintf("DashScope tenant %q not found", name))), nil
		}
		return adminhttp.GetDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetDashScopeTenant200JSONResponse(tenant), nil
}

func (s *Server) PutDashScopeTenant(ctx context.Context, request adminhttp.PutDashScopeTenantRequestObject) (adminhttp.PutDashScopeTenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.PutDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutDashScopeTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DASHSCOPE_TENANT", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := normalizeDashScopeTenantUpsert(*request.Body, name)
	if err != nil {
		return adminhttp.PutDashScopeTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DASHSCOPE_TENANT", err.Error())), nil
	}
	previous, err := getDashScopeTenant(ctx, store, name)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err == nil {
		tenant.CreatedAt = previous.CreatedAt
	}
	if err := writeDashScopeTenant(ctx, store, tenant); err != nil {
		return adminhttp.PutDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutDashScopeTenant200JSONResponse(tenant), nil
}

func (s *Server) DeleteDashScopeTenant(ctx context.Context, request adminhttp.DeleteDashScopeTenantRequestObject) (adminhttp.DeleteDashScopeTenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getDashScopeTenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteDashScopeTenant404JSONResponse(apitypes.NewErrorResponse("DASHSCOPE_TENANT_NOT_FOUND", fmt.Sprintf("DashScope tenant %q not found", name))), nil
		}
		return adminhttp.DeleteDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, dashScopeTenantKey(string(tenant.Name))); err != nil {
		return adminhttp.DeleteDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteDashScopeTenant200JSONResponse(tenant), nil
}

func normalizeDashScopeTenantUpsert(in adminhttp.DashScopeTenantUpsert, expectedName string) (apitypes.DashScopeTenant, error) {
	name := strings.TrimSpace(string(in.Name))
	if name == "" {
		return apitypes.DashScopeTenant{}, errors.New("name is required")
	}
	if expectedName != "" && name != expectedName {
		return apitypes.DashScopeTenant{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
	}
	credentialName := strings.TrimSpace(string(in.CredentialName))
	if credentialName == "" {
		return apitypes.DashScopeTenant{}, errors.New("credential_name is required")
	}
	tenant := apitypes.DashScopeTenant{
		CredentialName: string(credentialName),
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

func listDashScopeTenantsPage(ctx context.Context, store kv.Store, cursor string, limit int) ([]apitypes.DashScopeTenant, bool, *string, error) {
	items := make([]apitypes.DashScopeTenant, 0, limit+1)
	for entry, err := range store.List(ctx, dashScopeTenantsRoot) {
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
		var tenant apitypes.DashScopeTenant
		if err := json.Unmarshal(entry.Value, &tenant); err != nil {
			return nil, false, nil, fmt.Errorf("dashscope tenants: decode tenant list %s: %w", entry.Key.String(), err)
		}
		items = append(items, tenant)
		if len(items) >= limit+1 {
			break
		}
	}
	if len(items) == 0 {
		return []apitypes.DashScopeTenant{}, false, nil, nil
	}
	hasNext := len(items) > limit
	if !hasNext {
		return items, false, nil, nil
	}
	page := items[:limit]
	next := escapeStoreSegment(string(page[len(page)-1].Name))
	return page, true, &next, nil
}

func writeDashScopeTenant(ctx context.Context, store kv.Store, tenant apitypes.DashScopeTenant) error {
	data, err := json.Marshal(tenant)
	if err != nil {
		return fmt.Errorf("dashscope tenants: encode tenant %s: %w", tenant.Name, err)
	}
	if err := store.Set(ctx, dashScopeTenantKey(string(tenant.Name)), data); err != nil {
		return fmt.Errorf("dashscope tenants: write tenant %s: %w", tenant.Name, err)
	}
	return nil
}

func getDashScopeTenant(ctx context.Context, store kv.Store, name string) (apitypes.DashScopeTenant, error) {
	data, err := store.Get(ctx, dashScopeTenantKey(name))
	if err != nil {
		return apitypes.DashScopeTenant{}, err
	}
	var tenant apitypes.DashScopeTenant
	if err := json.Unmarshal(data, &tenant); err != nil {
		return apitypes.DashScopeTenant{}, fmt.Errorf("dashscope tenants: decode tenant %s: %w", name, err)
	}
	return tenant, nil
}

func dashScopeTenantKey(name string) kv.Key {
	return append(append(kv.Key{}, dashScopeTenantsRoot...), escapeStoreSegment(name))
}
