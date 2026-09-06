package providertenants

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
)

func (s *Server) ListDashScopeTenants(ctx context.Context, request adminhttp.ListDashScopeTenantsRequestObject) (adminhttp.ListDashScopeTenantsResponseObject, error) {
	store, err := s.database()
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
	store, err := s.database()
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
	created, err := createSQLTenant(ctx, store, "dashscope", tenant)
	if err != nil {
		return adminhttp.CreateDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if !created {
		return adminhttp.CreateDashScopeTenant409JSONResponse(apitypes.NewErrorResponse("DASHSCOPE_TENANT_ALREADY_EXISTS", fmt.Sprintf("DashScope tenant %q already exists", tenant.Id))), nil
	}
	return adminhttp.CreateDashScopeTenant200JSONResponse(tenant), nil
}

func (s *Server) GetDashScopeTenant(ctx context.Context, request adminhttp.GetDashScopeTenantRequestObject) (adminhttp.GetDashScopeTenantResponseObject, error) {
	store, err := s.database()
	if err != nil {
		return adminhttp.GetDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	tenant, err := getDashScopeTenant(ctx, store, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.GetDashScopeTenant404JSONResponse(apitypes.NewErrorResponse("DASHSCOPE_TENANT_NOT_FOUND", fmt.Sprintf("DashScope tenant %q not found", id))), nil
		}
		return adminhttp.GetDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetDashScopeTenant200JSONResponse(tenant), nil
}

func (s *Server) PutDashScopeTenant(ctx context.Context, request adminhttp.PutDashScopeTenantRequestObject) (adminhttp.PutDashScopeTenantResponseObject, error) {
	store, err := s.database()
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
	tenant.UpdatedAt = s.now()
	tenant, err = updateSQLTenant(ctx, store, "dashscope", tenant)
	if errors.Is(err, sql.ErrNoRows) {
		return adminhttp.PutDashScopeTenant404JSONResponse(apitypes.NewErrorResponse("DASHSCOPE_TENANT_NOT_FOUND", fmt.Sprintf("tenant %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.PutDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.PutDashScopeTenant200JSONResponse(tenant), nil
}

func (s *Server) DeleteDashScopeTenant(ctx context.Context, request adminhttp.DeleteDashScopeTenantRequestObject) (adminhttp.DeleteDashScopeTenantResponseObject, error) {
	store, err := s.database()
	if err != nil {
		return adminhttp.DeleteDashScopeTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	tenant, err := deleteSQLTenant[apitypes.DashScopeTenant](ctx, store, "dashscope", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.DeleteDashScopeTenant404JSONResponse(apitypes.NewErrorResponse("DASHSCOPE_TENANT_NOT_FOUND", fmt.Sprintf("DashScope tenant %q not found", id))), nil
		}
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

func listDashScopeTenantsPage(ctx context.Context, db *sqlx.DB, cursor string, limit int) ([]apitypes.DashScopeTenant, bool, *string, error) {
	return listSQLTenants[apitypes.DashScopeTenant](ctx, db, "dashscope", cursor, limit)
}

func getDashScopeTenant(ctx context.Context, db *sqlx.DB, id string) (apitypes.DashScopeTenant, error) {
	return getSQLTenant[apitypes.DashScopeTenant](ctx, db, "dashscope", id)
}
