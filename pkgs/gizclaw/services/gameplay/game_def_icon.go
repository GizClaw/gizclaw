package gameplay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (c *Catalog) DownloadGameDefIcon(ctx context.Context, request adminhttp.DownloadGameDefIconRequestObject) (adminhttp.DownloadGameDefIconResponseObject, error) {
	id, format, err := gameDefIconParams(request.Id, string(request.Format))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	doc, err := c.GetGameDefByID(ctx, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DownloadGameDefIcon404JSONResponse(apitypes.NewErrorResponse("GAME_DEF_NOT_FOUND", "game def not found")), nil
		}
		return adminhttp.DownloadGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to load game def icon metadata")), nil
	}
	if iconasset.Slot(doc.Icon, format) == nil {
		return adminhttp.DownloadGameDefIcon404JSONResponse(apitypes.NewErrorResponse("ICON_NOT_FOUND", "game def icon not found")), nil
	}
	reader, size, err := iconasset.Open(c.Assets, iconasset.GameDefObjectName(id, format))
	if err != nil {
		if errors.Is(err, io.EOF) {
			return adminhttp.DownloadGameDefIcon404JSONResponse(apitypes.NewErrorResponse("ICON_NOT_FOUND", "game def icon not found")), nil
		}
		return adminhttp.DownloadGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to open game def icon")), nil
	}
	if format == iconasset.FormatPNG {
		return adminhttp.DownloadGameDefIcon200ImagepngResponse{Body: reader, ContentLength: size}, nil
	}
	return adminhttp.DownloadGameDefIcon200ApplicationoctetStreamResponse{Body: reader, ContentLength: size}, nil
}

func (c *Catalog) UploadGameDefIcon(ctx context.Context, request adminhttp.UploadGameDefIconRequestObject) (adminhttp.UploadGameDefIconResponseObject, error) {
	id, format, err := gameDefIconParams(request.Id, string(request.Format))
	if err != nil {
		return adminhttp.UploadGameDefIcon400JSONResponse(apitypes.NewErrorResponse("INVALID_ICON", err.Error())), nil
	}
	data, err := iconasset.ReadValidated(request.Body, format)
	if errors.Is(err, iconasset.ErrTooLarge) {
		return adminhttp.UploadGameDefIcon413JSONResponse(apitypes.NewErrorResponse("ICON_TOO_LARGE", err.Error())), nil
	}
	if err != nil {
		return adminhttp.UploadGameDefIcon400JSONResponse(apitypes.NewErrorResponse("INVALID_ICON", err.Error())), nil
	}
	unlock := c.IconLocks.Lock(id + ":" + string(format))
	defer unlock()
	_, err = c.GetGameDefByID(ctx, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.UploadGameDefIcon404JSONResponse(apitypes.NewErrorResponse("GAME_DEF_NOT_FOUND", "game def not found")), nil
		}
		return adminhttp.UploadGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to load game def icon metadata")), nil
	}
	if c.Assets == nil {
		return adminhttp.UploadGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "gameplay asset store not configured")), nil
	}
	objectName := iconasset.GameDefObjectName(id, format)
	if err := c.Assets.Put(objectName, bytes.NewReader(data)); err != nil {
		return adminhttp.UploadGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to store game def icon")), nil
	}
	recordUnlock := c.IconLocks.LockRecord(id)
	defer recordUnlock()
	doc, err := c.GetGameDefByID(ctx, id)
	if err != nil {
		return adminhttp.UploadGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to reload game def icon metadata")), nil
	}
	doc.Icon = iconasset.SetSlot(doc.Icon, format, &objectName)
	store, err := c.store(c.GameDefs, "game defs")
	if err != nil {
		return adminhttp.UploadGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "game def store not configured")), nil
	}
	if err := writeJSON(ctx, store, gameDefKey(id), doc); err != nil {
		return adminhttp.UploadGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to update game def icon")), nil
	}
	return adminhttp.UploadGameDefIcon200JSONResponse(doc), nil
}

func (c *Catalog) DeleteGameDefIcon(ctx context.Context, request adminhttp.DeleteGameDefIconRequestObject) (adminhttp.DeleteGameDefIconResponseObject, error) {
	id, format, err := gameDefIconParams(request.Id, string(request.Format))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	unlock := c.IconLocks.Lock(id + ":" + string(format))
	defer unlock()
	_, err = c.GetGameDefByID(ctx, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteGameDefIcon404JSONResponse(apitypes.NewErrorResponse("GAME_DEF_NOT_FOUND", "game def not found")), nil
		}
		return adminhttp.DeleteGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "game def store not configured")), nil
	}
	if c.Assets == nil {
		return adminhttp.DeleteGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "gameplay asset store not configured")), nil
	}
	if err := c.Assets.Delete(iconasset.GameDefObjectName(id, format)); err != nil {
		return adminhttp.DeleteGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to delete game def icon")), nil
	}
	recordUnlock := c.IconLocks.LockRecord(id)
	defer recordUnlock()
	doc, err := c.GetGameDefByID(ctx, id)
	if err != nil {
		return adminhttp.DeleteGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to reload game def icon metadata")), nil
	}
	doc.Icon = iconasset.SetSlot(doc.Icon, format, nil)
	store, err := c.store(c.GameDefs, "game defs")
	if err != nil {
		return adminhttp.DeleteGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := writeJSON(ctx, store, gameDefKey(id), doc); err != nil {
		return adminhttp.DeleteGameDefIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to update game def icon")), nil
	}
	return adminhttp.DeleteGameDefIcon200JSONResponse(doc), nil
}

func gameDefIconParams(rawID, rawFormat string) (string, iconasset.Format, error) {
	id, err := pathID(rawID)
	if err != nil {
		return "", "", err
	}
	format, err := iconasset.ParseFormat(rawFormat)
	return id, format, err
}
