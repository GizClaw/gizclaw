package gizclaw

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
)

func (s *peerHTTP) ListAPIKeys(ctx context.Context, request peerhttp.ListAPIKeysRequestObject) (peerhttp.ListAPIKeysResponseObject, error) {
	principal, err := apikey.PrincipalFromContext(ctx)
	if err != nil {
		return peerhttp.ListAPIKeys401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(apiError("INVALID_API_KEY", "missing or invalid bearer API key"))}, nil
	}
	cursor := ""
	if request.Params.Cursor != nil {
		cursor = *request.Params.Cursor
	}
	limit := 0
	if request.Params.Limit != nil {
		limit = int(*request.Params.Limit)
	}
	result, err := s.APIKeys.List(ctx, principal, cursor, limit)
	if errors.Is(err, apikey.ErrInvalidCursor) {
		return peerhttp.ListAPIKeys400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError("INVALID_REQUEST", err.Error()))}, nil
	}
	if errors.Is(err, apikey.ErrForbidden) {
		return peerhttp.ListAPIKeys403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(apiError("API_KEY_MANAGEMENT_FORBIDDEN", err.Error()))}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]peerhttp.APIKey, len(result.Items))
	for i := range result.Items {
		items[i] = publicAPIKey(result.Items[i])
	}
	var next *string
	if result.NextCursor != "" {
		next = &result.NextCursor
	}
	return peerhttp.ListAPIKeys200JSONResponse{Items: items, NextCursor: next}, nil
}

func (s *peerHTTP) CreateAPIKey(ctx context.Context, request peerhttp.CreateAPIKeyRequestObject) (peerhttp.CreateAPIKeyResponseObject, error) {
	principal, err := apikey.PrincipalFromContext(ctx)
	if err != nil {
		return peerhttp.CreateAPIKey401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(apiError("INVALID_API_KEY", "missing or invalid bearer API key"))}, nil
	}
	if !principal.Key.ManageAPIKeys {
		return peerhttp.CreateAPIKey403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(apiError("API_KEY_MANAGEMENT_FORBIDDEN", apikey.ErrForbidden.Error()))}, nil
	}
	if request.Body == nil {
		return peerhttp.CreateAPIKey400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError("INVALID_REQUEST", "request body is required"))}, nil
	}
	created, err := s.APIKeys.Create(ctx, principal.Key.Owner, request.Body.DisplayName, request.Body.ManageApiKeys)
	if errors.Is(err, apikey.ErrInvalidDisplayName) {
		return peerhttp.CreateAPIKey400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError("INVALID_DISPLAY_NAME", err.Error()))}, nil
	}
	if errors.Is(err, apikey.ErrOwnerRetired) {
		return peerhttp.CreateAPIKey409JSONResponse{ConflictJSONResponse: peerhttp.ConflictJSONResponse(apiError("PEER_DELETED", err.Error()))}, nil
	}
	if err != nil {
		return nil, err
	}
	return peerhttp.CreateAPIKey201JSONResponse{Value: publicAPIKey(created.Key), ApiKey: created.Secret}, nil
}

func (s *peerHTTP) GetSelfAPIKey(ctx context.Context, _ peerhttp.GetSelfAPIKeyRequestObject) (peerhttp.GetSelfAPIKeyResponseObject, error) {
	principal, err := apikey.PrincipalFromContext(ctx)
	if err != nil {
		return peerhttp.GetSelfAPIKey401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(apiError("INVALID_API_KEY", "missing or invalid bearer API key"))}, nil
	}
	return peerhttp.GetSelfAPIKey200JSONResponse(publicAPIKey(s.APIKeys.GetSelf(principal))), nil
}

func (s *peerHTTP) RevokeSelfAPIKey(ctx context.Context, _ peerhttp.RevokeSelfAPIKeyRequestObject) (peerhttp.RevokeSelfAPIKeyResponseObject, error) {
	principal, err := apikey.PrincipalFromContext(ctx)
	if err != nil {
		return peerhttp.RevokeSelfAPIKey401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(apiError("INVALID_API_KEY", "missing or invalid bearer API key"))}, nil
	}
	if err := s.APIKeys.RevokeSelf(ctx, principal); err != nil {
		return nil, err
	}
	return peerhttp.RevokeSelfAPIKey204Response{}, nil
}

func (s *peerHTTP) GetAPIKey(ctx context.Context, request peerhttp.GetAPIKeyRequestObject) (peerhttp.GetAPIKeyResponseObject, error) {
	principal, err := apikey.PrincipalFromContext(ctx)
	if err != nil {
		return peerhttp.GetAPIKey401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(apiError("INVALID_API_KEY", "missing or invalid bearer API key"))}, nil
	}
	item, err := s.APIKeys.Get(ctx, principal, request.ApiKeyName)
	switch {
	case errors.Is(err, apikey.ErrForbidden):
		return peerhttp.GetAPIKey403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(apiError("API_KEY_MANAGEMENT_FORBIDDEN", err.Error()))}, nil
	case errors.Is(err, apikey.ErrNotFound):
		return peerhttp.GetAPIKey404JSONResponse{NotFoundJSONResponse: peerhttp.NotFoundJSONResponse(apiError("API_KEY_NOT_FOUND", err.Error()))}, nil
	case err != nil:
		return nil, err
	default:
		return peerhttp.GetAPIKey200JSONResponse(publicAPIKey(item)), nil
	}
}

func (s *peerHTTP) RevokeAPIKey(ctx context.Context, request peerhttp.RevokeAPIKeyRequestObject) (peerhttp.RevokeAPIKeyResponseObject, error) {
	principal, err := apikey.PrincipalFromContext(ctx)
	if err != nil {
		return peerhttp.RevokeAPIKey401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(apiError("INVALID_API_KEY", "missing or invalid bearer API key"))}, nil
	}
	err = s.APIKeys.Revoke(ctx, principal, request.ApiKeyName)
	switch {
	case errors.Is(err, apikey.ErrForbidden):
		return peerhttp.RevokeAPIKey403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(apiError("API_KEY_MANAGEMENT_FORBIDDEN", err.Error()))}, nil
	case errors.Is(err, apikey.ErrNotFound):
		return peerhttp.RevokeAPIKey404JSONResponse{NotFoundJSONResponse: peerhttp.NotFoundJSONResponse(apiError("API_KEY_NOT_FOUND", err.Error()))}, nil
	case err != nil:
		return nil, err
	default:
		return peerhttp.RevokeAPIKey204Response{}, nil
	}
}

func publicAPIKey(item apikey.Key) peerhttp.APIKey {
	return peerhttp.APIKey{
		Name: item.Name, DisplayName: item.DisplayName, Prefix: item.Prefix,
		ManageApiKeys: item.ManageAPIKeys, CreatedAt: item.CreatedAt,
	}
}

func apiError(code, message string) apitypes.ErrorResponse {
	return apitypes.NewErrorResponse(code, message)
}
