package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (s *Server) DownloadWorkspaceIcon(ctx context.Context, request adminhttp.DownloadWorkspaceIconRequestObject) (adminhttp.DownloadWorkspaceIconResponseObject, error) {
	name, format, err := workspaceIconParams(request.Name, string(request.Format))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	doc, err := s.workspace(ctx, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DownloadWorkspaceIcon404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "workspace not found")), nil
		}
		return adminhttp.DownloadWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to load workspace icon metadata")), nil
	}
	if iconasset.Slot(doc.Icon, format) == nil {
		return adminhttp.DownloadWorkspaceIcon404JSONResponse(apitypes.NewErrorResponse("ICON_NOT_FOUND", "workspace icon not found")), nil
	}
	reader, size, err := iconasset.Open(s.Assets, iconasset.ObjectName(name, format))
	if err != nil {
		if errors.Is(err, io.EOF) {
			return adminhttp.DownloadWorkspaceIcon404JSONResponse(apitypes.NewErrorResponse("ICON_NOT_FOUND", "workspace icon not found")), nil
		}
		return adminhttp.DownloadWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to open workspace icon")), nil
	}
	if format == iconasset.FormatPNG {
		return adminhttp.DownloadWorkspaceIcon200ImagepngResponse{Body: reader, ContentLength: size}, nil
	}
	return adminhttp.DownloadWorkspaceIcon200ApplicationoctetStreamResponse{Body: reader, ContentLength: size}, nil
}

func (s *Server) UploadWorkspaceIcon(ctx context.Context, request adminhttp.UploadWorkspaceIconRequestObject) (adminhttp.UploadWorkspaceIconResponseObject, error) {
	name, format, err := workspaceIconParams(request.Name, string(request.Format))
	if err != nil {
		return adminhttp.UploadWorkspaceIcon400JSONResponse(apitypes.NewErrorResponse("INVALID_ICON", err.Error())), nil
	}
	data, err := iconasset.ReadValidated(request.Body, format)
	if errors.Is(err, iconasset.ErrTooLarge) {
		return adminhttp.UploadWorkspaceIcon413JSONResponse(apitypes.NewErrorResponse("ICON_TOO_LARGE", err.Error())), nil
	}
	if err != nil {
		return adminhttp.UploadWorkspaceIcon400JSONResponse(apitypes.NewErrorResponse("INVALID_ICON", err.Error())), nil
	}
	unlock := s.IconLocks.Lock(name + ":" + string(format))
	defer unlock()
	_, err = s.workspace(ctx, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.UploadWorkspaceIcon404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "workspace not found")), nil
		}
		return adminhttp.UploadWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to load workspace icon metadata")), nil
	}
	if s.Assets == nil {
		return adminhttp.UploadWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workspace asset store not configured")), nil
	}
	objectName := iconasset.ObjectName(name, format)
	if err := s.Assets.Put(objectName, bytes.NewReader(data)); err != nil {
		return adminhttp.UploadWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to store workspace icon")), nil
	}
	recordUnlock := s.IconLocks.LockRecord(name)
	defer recordUnlock()
	doc, err := s.workspace(ctx, name)
	if err != nil {
		return adminhttp.UploadWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to reload workspace icon metadata")), nil
	}
	doc.Icon = iconasset.SetSlot(doc.Icon, format, &objectName)
	store, err := s.store()
	if err != nil {
		return adminhttp.UploadWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workspace store not configured")), nil
	}
	if err := writeWorkspace(ctx, store, doc); err != nil {
		return adminhttp.UploadWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to update workspace icon")), nil
	}
	return adminhttp.UploadWorkspaceIcon200JSONResponse(doc), nil
}

func (s *Server) DeleteWorkspaceIcon(ctx context.Context, request adminhttp.DeleteWorkspaceIconRequestObject) (adminhttp.DeleteWorkspaceIconResponseObject, error) {
	name, format, err := workspaceIconParams(request.Name, string(request.Format))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	unlock := s.IconLocks.Lock(name + ":" + string(format))
	defer unlock()
	_, err = s.workspace(ctx, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteWorkspaceIcon404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "workspace not found")), nil
		}
		return adminhttp.DeleteWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workspace store not configured")), nil
	}
	if s.Assets == nil {
		return adminhttp.DeleteWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workspace asset store not configured")), nil
	}
	if err := s.Assets.Delete(iconasset.ObjectName(name, format)); err != nil {
		return adminhttp.DeleteWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to delete workspace icon")), nil
	}
	recordUnlock := s.IconLocks.LockRecord(name)
	defer recordUnlock()
	doc, err := s.workspace(ctx, name)
	if err != nil {
		return adminhttp.DeleteWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to reload workspace icon metadata")), nil
	}
	doc.Icon = iconasset.SetSlot(doc.Icon, format, nil)
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := writeWorkspace(ctx, store, doc); err != nil {
		return adminhttp.DeleteWorkspaceIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to update workspace icon")), nil
	}
	return adminhttp.DeleteWorkspaceIcon200JSONResponse(doc), nil
}

func workspaceIconParams(rawName, rawFormat string) (string, iconasset.Format, error) {
	name, err := url.PathUnescape(rawName)
	if err != nil {
		return "", "", err
	}
	format, err := iconasset.ParseFormat(rawFormat)
	return name, format, err
}

func (s *Server) workspace(ctx context.Context, name string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	return getWorkspace(ctx, store, name)
}
