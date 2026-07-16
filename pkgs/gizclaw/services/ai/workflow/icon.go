package workflow

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

func (s *Server) DownloadWorkflowIcon(ctx context.Context, request adminhttp.DownloadWorkflowIconRequestObject) (adminhttp.DownloadWorkflowIconResponseObject, error) {
	name, format, err := workflowIconParams(request.Name, string(request.Format))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	doc, err := s.workflow(ctx, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DownloadWorkflowIcon404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", "workflow not found")), nil
		}
		return adminhttp.DownloadWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to load workflow icon metadata")), nil
	}
	if iconasset.Slot(doc.Icon, format) == nil {
		return adminhttp.DownloadWorkflowIcon404JSONResponse(apitypes.NewErrorResponse("ICON_NOT_FOUND", "workflow icon not found")), nil
	}
	reader, size, err := iconasset.Open(s.Assets, iconasset.ObjectName(name, format))
	if err != nil {
		if errors.Is(err, io.EOF) {
			return adminhttp.DownloadWorkflowIcon404JSONResponse(apitypes.NewErrorResponse("ICON_NOT_FOUND", "workflow icon not found")), nil
		}
		return adminhttp.DownloadWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to open workflow icon")), nil
	}
	if format == iconasset.FormatPNG {
		return adminhttp.DownloadWorkflowIcon200ImagepngResponse{Body: reader, ContentLength: size}, nil
	}
	return adminhttp.DownloadWorkflowIcon200ApplicationoctetStreamResponse{Body: reader, ContentLength: size}, nil
}

func (s *Server) UploadWorkflowIcon(ctx context.Context, request adminhttp.UploadWorkflowIconRequestObject) (adminhttp.UploadWorkflowIconResponseObject, error) {
	name, format, err := workflowIconParams(request.Name, string(request.Format))
	if err != nil {
		return adminhttp.UploadWorkflowIcon400JSONResponse(apitypes.NewErrorResponse("INVALID_ICON", err.Error())), nil
	}
	data, err := iconasset.ReadValidated(request.Body, format)
	if errors.Is(err, iconasset.ErrTooLarge) {
		return adminhttp.UploadWorkflowIcon413JSONResponse(apitypes.NewErrorResponse("ICON_TOO_LARGE", err.Error())), nil
	}
	if err != nil {
		return adminhttp.UploadWorkflowIcon400JSONResponse(apitypes.NewErrorResponse("INVALID_ICON", err.Error())), nil
	}
	unlock := s.IconLocks.Lock(name + ":" + string(format))
	defer unlock()
	_, err = s.workflow(ctx, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.UploadWorkflowIcon404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", "workflow not found")), nil
		}
		return adminhttp.UploadWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to load workflow icon metadata")), nil
	}
	if s.Assets == nil {
		return adminhttp.UploadWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow asset store not configured")), nil
	}
	objectName := iconasset.ObjectName(name, format)
	if err := s.Assets.Put(objectName, bytes.NewReader(data)); err != nil {
		return adminhttp.UploadWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to store workflow icon")), nil
	}
	recordUnlock := s.IconLocks.LockRecord(name)
	defer recordUnlock()
	doc, err := s.workflow(ctx, name)
	if err != nil {
		return adminhttp.UploadWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to reload workflow icon metadata")), nil
	}
	doc.Icon = iconasset.SetSlot(doc.Icon, format, &objectName)
	if err := s.writeWorkflow(ctx, doc); err != nil {
		return adminhttp.UploadWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to update workflow icon")), nil
	}
	return adminhttp.UploadWorkflowIcon200JSONResponse(doc), nil
}

func (s *Server) DeleteWorkflowIcon(ctx context.Context, request adminhttp.DeleteWorkflowIconRequestObject) (adminhttp.DeleteWorkflowIconResponseObject, error) {
	name, format, err := workflowIconParams(request.Name, string(request.Format))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	unlock := s.IconLocks.Lock(name + ":" + string(format))
	defer unlock()
	_, err = s.workflow(ctx, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteWorkflowIcon404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", "workflow not found")), nil
		}
		return adminhttp.DeleteWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if s.Assets == nil {
		return adminhttp.DeleteWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow asset store not configured")), nil
	}
	if err := s.Assets.Delete(iconasset.ObjectName(name, format)); err != nil {
		return adminhttp.DeleteWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to delete workflow icon")), nil
	}
	recordUnlock := s.IconLocks.LockRecord(name)
	defer recordUnlock()
	doc, err := s.workflow(ctx, name)
	if err != nil {
		return adminhttp.DeleteWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to reload workflow icon metadata")), nil
	}
	doc.Icon = iconasset.SetSlot(doc.Icon, format, nil)
	if err := s.writeWorkflow(ctx, doc); err != nil {
		return adminhttp.DeleteWorkflowIcon500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed to update workflow icon")), nil
	}
	return adminhttp.DeleteWorkflowIcon200JSONResponse(doc), nil
}

func workflowIconParams(rawName, rawFormat string) (string, iconasset.Format, error) {
	name, err := url.PathUnescape(rawName)
	if err != nil {
		return "", "", err
	}
	format, err := iconasset.ParseFormat(rawFormat)
	return name, format, err
}

func (s *Server) workflow(ctx context.Context, name string) (apitypes.Workflow, error) {
	if s == nil || s.Store == nil {
		return apitypes.Workflow{}, errors.New("workflow store not configured")
	}
	data, err := s.Store.Get(ctx, workflowKey(name))
	if err != nil {
		return apitypes.Workflow{}, err
	}
	return decodeWorkflow(data)
}

func (s *Server) writeWorkflow(ctx context.Context, doc apitypes.Workflow) error {
	_, raw, err := validateWorkflow(doc, doc.Name)
	if err != nil {
		return err
	}
	return s.Store.Set(ctx, workflowKey(doc.Name), raw)
}
