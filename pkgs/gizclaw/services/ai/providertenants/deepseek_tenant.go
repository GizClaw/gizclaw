package providertenants

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
)

func (s *Server) ListDeepSeekTenants(ctx context.Context, request adminhttp.ListDeepSeekTenantsRequestObject) (adminhttp.ListDeepSeekTenantsResponseObject, error) {
	store, err := s.database()
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
	store, err := s.database()
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
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	created, err := createSQLTenant(ctx, store, "deepseek", tenant)
	if err != nil {
		return adminhttp.CreateDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if !created {
		return adminhttp.CreateDeepSeekTenant409JSONResponse(apitypes.NewErrorResponse("DEEPSEEK_TENANT_ALREADY_EXISTS", fmt.Sprintf("DeepSeek tenant %q already exists", tenant.Id))), nil
	}
	return adminhttp.CreateDeepSeekTenant200JSONResponse(tenant), nil
}

func (s *Server) GetDeepSeekTenant(ctx context.Context, request adminhttp.GetDeepSeekTenantRequestObject) (adminhttp.GetDeepSeekTenantResponseObject, error) {
	store, err := s.database()
	if err != nil {
		return adminhttp.GetDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	tenant, err := getDeepSeekTenant(ctx, store, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.GetDeepSeekTenant404JSONResponse(apitypes.NewErrorResponse("DEEPSEEK_TENANT_NOT_FOUND", fmt.Sprintf("DeepSeek tenant %q not found", id))), nil
		}
		return adminhttp.GetDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetDeepSeekTenant200JSONResponse(tenant), nil
}

func (s *Server) PutDeepSeekTenant(ctx context.Context, request adminhttp.PutDeepSeekTenantRequestObject) (adminhttp.PutDeepSeekTenantResponseObject, error) {
	store, err := s.database()
	if err != nil {
		return adminhttp.PutDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutDeepSeekTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DEEPSEEK_TENANT", "request body required")), nil
	}
	id := string(request.Id)
	tenant, err := normalizeDeepSeekTenantUpsert(*request.Body, id)
	if err != nil {
		return adminhttp.PutDeepSeekTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_DEEPSEEK_TENANT", err.Error())), nil
	}
	tenant.UpdatedAt = s.now()
	tenant, err = updateSQLTenant(ctx, store, "deepseek", tenant)
	if errors.Is(err, sql.ErrNoRows) {
		return adminhttp.PutDeepSeekTenant404JSONResponse(apitypes.NewErrorResponse("DEEPSEEK_TENANT_NOT_FOUND", fmt.Sprintf("tenant %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.PutDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.PutDeepSeekTenant200JSONResponse(tenant), nil
}

func (s *Server) DeleteDeepSeekTenant(ctx context.Context, request adminhttp.DeleteDeepSeekTenantRequestObject) (adminhttp.DeleteDeepSeekTenantResponseObject, error) {
	store, err := s.database()
	if err != nil {
		return adminhttp.DeleteDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	tenant, err := deleteSQLTenant[apitypes.DeepSeekTenant](ctx, store, "deepseek", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.DeleteDeepSeekTenant404JSONResponse(apitypes.NewErrorResponse("DEEPSEEK_TENANT_NOT_FOUND", fmt.Sprintf("DeepSeek tenant %q not found", id))), nil
		}
		return adminhttp.DeleteDeepSeekTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.DeleteDeepSeekTenant200JSONResponse(tenant), nil
}

func normalizeDeepSeekTenantUpsert(in adminhttp.DeepSeekTenantUpsert, expectedID string) (apitypes.DeepSeekTenant, error) {
	id := string(in.Id)
	if err := validateResourceID(id); err != nil {
		return apitypes.DeepSeekTenant{}, err
	}
	if expectedID != "" && id != expectedID {
		return apitypes.DeepSeekTenant{}, fmt.Errorf("id %q must match path id %q", id, expectedID)
	}
	credentialID := string(in.CredentialId)
	if err := validateResourceReference("credential_id", credentialID); err != nil {
		return apitypes.DeepSeekTenant{}, err
	}
	tenant := apitypes.DeepSeekTenant{
		CredentialId: credentialID,
		Id:           id,
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

func listDeepSeekTenantsPage(ctx context.Context, db *sqlx.DB, cursor string, limit int) ([]apitypes.DeepSeekTenant, bool, *string, error) {
	return listSQLTenants[apitypes.DeepSeekTenant](ctx, db, "deepseek", cursor, limit)
}

func getDeepSeekTenant(ctx context.Context, db *sqlx.DB, id string) (apitypes.DeepSeekTenant, error) {
	return getSQLTenant[apitypes.DeepSeekTenant](ctx, db, "deepseek", id)
}
