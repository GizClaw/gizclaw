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

func (s *Server) ListGeminiTenants(ctx context.Context, request adminhttp.ListGeminiTenantsRequestObject) (adminhttp.ListGeminiTenantsResponseObject, error) {
	store, err := s.database()
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
	store, err := s.database()
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
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	created, err := createSQLTenant(ctx, store, "gemini", tenant)
	if err != nil {
		return adminhttp.CreateGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if !created {
		return adminhttp.CreateGeminiTenant409JSONResponse(apitypes.NewErrorResponse("GEMINI_TENANT_ALREADY_EXISTS", fmt.Sprintf("Gemini tenant %q already exists", tenant.Id))), nil
	}
	return adminhttp.CreateGeminiTenant200JSONResponse(tenant), nil
}

func (s *Server) GetGeminiTenant(ctx context.Context, request adminhttp.GetGeminiTenantRequestObject) (adminhttp.GetGeminiTenantResponseObject, error) {
	store, err := s.database()
	if err != nil {
		return adminhttp.GetGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	tenant, err := getGeminiTenant(ctx, store, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.GetGeminiTenant404JSONResponse(apitypes.NewErrorResponse("GEMINI_TENANT_NOT_FOUND", fmt.Sprintf("Gemini tenant %q not found", id))), nil
		}
		return adminhttp.GetGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetGeminiTenant200JSONResponse(tenant), nil
}

func (s *Server) PutGeminiTenant(ctx context.Context, request adminhttp.PutGeminiTenantRequestObject) (adminhttp.PutGeminiTenantResponseObject, error) {
	store, err := s.database()
	if err != nil {
		return adminhttp.PutGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutGeminiTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_GEMINI_TENANT", "request body required")), nil
	}
	id := string(request.Id)
	tenant, err := normalizeGeminiTenantUpsert(*request.Body, id)
	if err != nil {
		return adminhttp.PutGeminiTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_GEMINI_TENANT", err.Error())), nil
	}
	tenant.UpdatedAt = s.now()
	tenant, err = updateSQLTenant(ctx, store, "gemini", tenant)
	if errors.Is(err, sql.ErrNoRows) {
		return adminhttp.PutGeminiTenant404JSONResponse(apitypes.NewErrorResponse("GEMINI_TENANT_NOT_FOUND", fmt.Sprintf("tenant %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.PutGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.PutGeminiTenant200JSONResponse(tenant), nil
}

func (s *Server) DeleteGeminiTenant(ctx context.Context, request adminhttp.DeleteGeminiTenantRequestObject) (adminhttp.DeleteGeminiTenantResponseObject, error) {
	store, err := s.database()
	if err != nil {
		return adminhttp.DeleteGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	tenant, err := deleteSQLTenant[apitypes.GeminiTenant](ctx, store, "gemini", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.DeleteGeminiTenant404JSONResponse(apitypes.NewErrorResponse("GEMINI_TENANT_NOT_FOUND", fmt.Sprintf("Gemini tenant %q not found", id))), nil
		}
		return adminhttp.DeleteGeminiTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.DeleteGeminiTenant200JSONResponse(tenant), nil
}

func normalizeGeminiTenantUpsert(in adminhttp.GeminiTenantUpsert, expectedID string) (apitypes.GeminiTenant, error) {
	id := string(in.Id)
	if err := validateResourceID(id); err != nil {
		return apitypes.GeminiTenant{}, err
	}
	if expectedID != "" && id != expectedID {
		return apitypes.GeminiTenant{}, fmt.Errorf("id %q must match path id %q", id, expectedID)
	}
	credentialID := string(in.CredentialId)
	if err := validateResourceReference("credential_id", credentialID); err != nil {
		return apitypes.GeminiTenant{}, err
	}
	tenant := apitypes.GeminiTenant{
		CredentialId: credentialID,
		Id:           id,
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

func listGeminiTenantsPage(ctx context.Context, db *sqlx.DB, cursor string, limit int) ([]apitypes.GeminiTenant, bool, *string, error) {
	return listSQLTenants[apitypes.GeminiTenant](ctx, db, "gemini", cursor, limit)
}

func getGeminiTenant(ctx context.Context, db *sqlx.DB, id string) (apitypes.GeminiTenant, error) {
	return getSQLTenant[apitypes.GeminiTenant](ctx, db, "gemini", id)
}
