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

var dashScopeTenantsRoot = kv.Key{"dashscope-tenants", "by-id"}

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
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	created, err := createTenant(ctx, store, dashScopeTenantKey(tenant.Id), tenant)
	if err != nil {
		return adminhttp.CreateDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if !created {
		return adminhttp.CreateDashScopeTenant409JSONResponse(apitypes.NewErrorResponse("DASHSCOPE_TENANT_ALREADY_EXISTS", fmt.Sprintf("DashScope tenant %q already exists", tenant.Id))), nil
	}
	return adminhttp.CreateDashScopeTenant200JSONResponse(tenant), nil
}

func (s *Server) GetDashScopeTenant(ctx context.Context, request adminhttp.GetDashScopeTenantRequestObject) (adminhttp.GetDashScopeTenantResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	tenant, err := getDashScopeTenant(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetDashScopeTenant404JSONResponse(apitypes.NewErrorResponse("DASHSCOPE_TENANT_NOT_FOUND", fmt.Sprintf("DashScope tenant %q not found", id))), nil
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
	id := string(request.Id)
	tenant, err := normalizeDashScopeTenantUpsert(*request.Body, id)
	if err != nil {
		return adminhttp.PutDashScopeTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DASHSCOPE_TENANT", err.Error())), nil
	}
	previous, err := getDashScopeTenant(ctx, store, id)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutDashScopeTenant404JSONResponse(apitypes.NewErrorResponse("DASHSCOPE_TENANT_NOT_FOUND", fmt.Sprintf("DashScope tenant %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.PutDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	tenant.CreatedAt = previous.CreatedAt
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
	id := string(request.Id)
	tenant, err := getDashScopeTenant(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteDashScopeTenant404JSONResponse(apitypes.NewErrorResponse("DASHSCOPE_TENANT_NOT_FOUND", fmt.Sprintf("DashScope tenant %q not found", id))), nil
		}
		return adminhttp.DeleteDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := deleteTenant(ctx, store, dashScopeTenantKey(tenant.Id)); err != nil {
		return adminhttp.DeleteDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteDashScopeTenant200JSONResponse(tenant), nil
}

func normalizeDashScopeTenantUpsert(in adminhttp.DashScopeTenantUpsert, expectedID string) (apitypes.DashScopeTenant, error) {
	id := string(in.Id)
	if err := validateResourceID(id); err != nil {
		return apitypes.DashScopeTenant{}, err
	}
	if expectedID != "" && id != expectedID {
		return apitypes.DashScopeTenant{}, fmt.Errorf("id %q must match path id %q", id, expectedID)
	}
	credentialID := string(in.CredentialId)
	if err := validateResourceReference("credential_id", credentialID); err != nil {
		return apitypes.DashScopeTenant{}, err
	}
	tenant := apitypes.DashScopeTenant{
		CredentialId: credentialID,
		Id:           id,
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
	next := escapeStoreSegment(string(page[len(page)-1].Id))
	return page, true, &next, nil
}

func writeDashScopeTenant(ctx context.Context, store kv.Store, tenant apitypes.DashScopeTenant) error {
	data, err := json.Marshal(tenant)
	if err != nil {
		return fmt.Errorf("dashscope tenants: encode tenant %s: %w", tenant.Id, err)
	}
	if err := store.Set(ctx, dashScopeTenantKey(string(tenant.Id)), data); err != nil {
		return fmt.Errorf("dashscope tenants: write tenant %s: %w", tenant.Id, err)
	}
	return nil
}

func getDashScopeTenant(ctx context.Context, store kv.Store, id string) (apitypes.DashScopeTenant, error) {
	data, err := store.Get(ctx, dashScopeTenantKey(id))
	if err != nil {
		return apitypes.DashScopeTenant{}, err
	}
	var tenant apitypes.DashScopeTenant
	if err := json.Unmarshal(data, &tenant); err != nil {
		return apitypes.DashScopeTenant{}, fmt.Errorf("dashscope tenants: decode tenant %s: %w", id, err)
	}
	return tenant, nil
}

func dashScopeTenantKey(id string) kv.Key {
	return append(append(kv.Key{}, dashScopeTenantsRoot...), escapeStoreSegment(id))
}
