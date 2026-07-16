package gizclaw

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
)

const defaultAssetMaxBytes int64 = 64 << 20

func (s *adminService) UploadAsset(ctx context.Context, request adminhttp.UploadAssetRequestObject) (adminhttp.UploadAssetResponseObject, error) {
	if s.Assets == nil {
		return adminhttp.UploadAsset500JSONResponse(assetHTTPError("ASSET_SERVICE_NOT_CONFIGURED", "asset service is not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.UploadAsset400JSONResponse(assetHTTPError("INVALID_ASSET", "request body is required")), nil
	}
	maxBytes := s.AssetMaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultAssetMaxBytes
	}
	created, err := s.Assets.Put(ctx, asset.PutRequest{
		MediaType: request.Params.MediaType,
		MaxBytes:  maxBytes,
		ExpiresAt: request.Params.ExpiresAt,
	}, request.Body)
	if err != nil {
		switch {
		case errors.Is(err, asset.ErrInvalid):
			return adminhttp.UploadAsset400JSONResponse(assetHTTPError("INVALID_ASSET", err.Error())), nil
		case errors.Is(err, asset.ErrTooLarge):
			return adminhttp.UploadAsset413JSONResponse(assetHTTPError("ASSET_TOO_LARGE", "asset exceeds the configured size limit")), nil
		default:
			return adminhttp.UploadAsset500JSONResponse(assetHTTPError("ASSET_BACKEND_ERROR", "asset could not be stored")), nil
		}
	}
	return adminhttp.UploadAsset201JSONResponse(assetAPI(created, nil)), nil
}

func (s *adminService) GetAsset(ctx context.Context, request adminhttp.GetAssetRequestObject) (adminhttp.GetAssetResponseObject, error) {
	ref, err := asset.ParseRef(request.Params.Ref)
	if err != nil {
		return adminhttp.GetAsset400JSONResponse(assetHTTPError("INVALID_ASSET_REF", err.Error())), nil
	}
	if s.Assets == nil {
		return adminhttp.GetAsset500JSONResponse(assetHTTPError("ASSET_SERVICE_NOT_CONFIGURED", "asset service is not configured")), nil
	}
	stored, err := s.Assets.Get(ctx, ref)
	if err != nil {
		if errors.Is(err, asset.ErrNotFound) {
			return adminhttp.GetAsset404JSONResponse(assetHTTPError("ASSET_NOT_FOUND", "asset not found")), nil
		}
		return adminhttp.GetAsset500JSONResponse(assetHTTPError("ASSET_BACKEND_ERROR", "asset metadata could not be read")), nil
	}
	bindings, err := s.Assets.Bindings(ctx, ref)
	if err != nil {
		return adminhttp.GetAsset500JSONResponse(assetHTTPError("ASSET_BACKEND_ERROR", "asset bindings could not be read")), nil
	}
	return adminhttp.GetAsset200JSONResponse(assetAPI(stored, bindings)), nil
}

func (s *adminService) DownloadAsset(ctx context.Context, request adminhttp.DownloadAssetRequestObject) (adminhttp.DownloadAssetResponseObject, error) {
	ref, err := asset.ParseRef(request.Params.Ref)
	if err != nil {
		return adminhttp.DownloadAsset400JSONResponse(assetHTTPError("INVALID_ASSET_REF", err.Error())), nil
	}
	if s.Assets == nil {
		return adminhttp.DownloadAsset500JSONResponse(assetHTTPError("ASSET_SERVICE_NOT_CONFIGURED", "asset service is not configured")), nil
	}
	stored, reader, err := s.Assets.Open(ctx, ref)
	if err != nil {
		if errors.Is(err, asset.ErrNotFound) {
			return adminhttp.DownloadAsset404JSONResponse(assetHTTPError("ASSET_NOT_FOUND", "asset not found")), nil
		}
		return adminhttp.DownloadAsset500JSONResponse(assetHTTPError("ASSET_BACKEND_ERROR", "asset content could not be read")), nil
	}
	return assetDownloadResponse{Asset: stored, Body: reader}, nil
}

func (s *adminService) DeleteAsset(ctx context.Context, request adminhttp.DeleteAssetRequestObject) (adminhttp.DeleteAssetResponseObject, error) {
	ref, err := asset.ParseRef(request.Params.Ref)
	if err != nil {
		return adminhttp.DeleteAsset400JSONResponse(assetHTTPError("INVALID_ASSET_REF", err.Error())), nil
	}
	if s.Assets == nil {
		return adminhttp.DeleteAsset500JSONResponse(assetHTTPError("ASSET_SERVICE_NOT_CONFIGURED", "asset service is not configured")), nil
	}
	if err := s.Assets.Delete(ctx, ref); err != nil {
		switch {
		case errors.Is(err, asset.ErrNotFound):
			return adminhttp.DeleteAsset404JSONResponse(assetHTTPError("ASSET_NOT_FOUND", "asset not found")), nil
		case errors.Is(err, asset.ErrInUse), errors.Is(err, asset.ErrConflict):
			return adminhttp.DeleteAsset409JSONResponse(assetHTTPError("ASSET_IN_USE", "asset is still referenced")), nil
		default:
			return adminhttp.DeleteAsset500JSONResponse(assetHTTPError("ASSET_DELETE_FAILED", "asset could not be deleted safely")), nil
		}
	}
	return adminhttp.DeleteAsset204Response{}, nil
}

type assetDownloadResponse struct {
	Asset asset.Asset
	Body  io.ReadCloser
}

func (response assetDownloadResponse) VisitDownloadAssetResponse(ctx *fiber.Ctx) error {
	defer response.Body.Close()
	metadata := response.Asset.Metadata
	ctx.Response().Header.SetContentType(metadata.MediaType)
	ctx.Response().Header.Set("Content-Length", strconv.FormatInt(metadata.SizeBytes, 10))
	ctx.Response().Header.Set("ETag", strconv.Quote(hex.EncodeToString(metadata.SHA256[:])))
	ctx.Status(http.StatusOK)
	_, err := io.Copy(ctx.Response().BodyWriter(), response.Body)
	return err
}

func assetAPI(stored asset.Asset, bindings []asset.Binding) apitypes.Asset {
	metadata := stored.Metadata
	result := apitypes.Asset{
		Metadata: apitypes.AssetMetadata{
			Ref:       metadata.Ref.String(),
			MediaType: metadata.MediaType,
			SizeBytes: metadata.SizeBytes,
			Sha256:    hex.EncodeToString(metadata.SHA256[:]),
			CreatedAt: metadata.CreatedAt,
			ExpiresAt: metadata.ExpiresAt,
		},
		Bindings: make([]apitypes.AssetBinding, 0, len(bindings)),
	}
	for _, binding := range bindings {
		result.Bindings = append(result.Bindings, apitypes.AssetBinding{
			OwnerKind: apitypes.AssetOwnerKind(binding.Owner.Kind),
			OwnerId:   binding.Owner.ID,
		})
	}
	return result
}

func assetHTTPError(code, message string) apitypes.ErrorResponse {
	return apitypes.NewErrorResponse(code, message)
}

var _ adminhttp.DownloadAssetResponseObject = assetDownloadResponse{}
