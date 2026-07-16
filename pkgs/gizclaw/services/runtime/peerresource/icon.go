package peerresource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
)

func (s *Server) PrepareWorkflowIconDownload(ctx context.Context, params rpcapi.WorkflowIconDownloadRequest) (rpcapi.WorkflowIconDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	name := strings.TrimSpace(params.Name)
	format, err := iconasset.ParseFormat(string(params.Format))
	if name == "" || err != nil {
		return rpcapi.WorkflowIconDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: "workflow name and icon format are required"}, nil
	}
	if err := s.authorizeErr(ctx, workflowResource(name), apitypes.ACLPermissionRead); err != nil {
		if errors.Is(err, acl.ErrDenied) {
			return rpcapi.WorkflowIconDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeForbidden, Message: err.Error()}, nil
		}
		return rpcapi.WorkflowIconDownloadResponse{}, nil, nil, err
	}
	icons, ok := s.Workflows.(workflow.WorkflowIconAdminService)
	if !ok || icons == nil {
		return rpcapi.WorkflowIconDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "workflow icon service not configured"}, nil
	}
	resp, err := icons.DownloadWorkflowIcon(ctx, adminhttp.DownloadWorkflowIconRequestObject{Name: name, Format: adminhttp.DownloadWorkflowIconParamsFormat(format)})
	if err != nil {
		return rpcapi.WorkflowIconDownloadResponse{}, nil, nil, err
	}
	reader, size, rpcErr := workflowIconDownloadResult(resp)
	if rpcErr != nil {
		return rpcapi.WorkflowIconDownloadResponse{}, nil, rpcErr, nil
	}
	return rpcapi.WorkflowIconDownloadResponse{Name: name, Format: params.Format, SizeBytes: size}, reader, nil, nil
}

func (s *Server) PrepareWorkspaceIconDownload(ctx context.Context, params rpcapi.WorkspaceIconDownloadRequest) (rpcapi.WorkspaceIconDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	name := strings.TrimSpace(params.Name)
	format, err := iconasset.ParseFormat(string(params.Format))
	if name == "" || err != nil {
		return rpcapi.WorkspaceIconDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: "workspace name and icon format are required"}, nil
	}
	if err := s.authorizeErr(ctx, acl.WorkspaceResource(name), apitypes.ACLPermissionRead); err != nil {
		if errors.Is(err, acl.ErrDenied) {
			return rpcapi.WorkspaceIconDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeForbidden, Message: err.Error()}, nil
		}
		return rpcapi.WorkspaceIconDownloadResponse{}, nil, nil, err
	}
	icons, ok := s.Workspaces.(workspace.WorkspaceIconAdminService)
	if !ok || icons == nil {
		return rpcapi.WorkspaceIconDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "workspace icon service not configured"}, nil
	}
	resp, err := icons.DownloadWorkspaceIcon(ctx, adminhttp.DownloadWorkspaceIconRequestObject{Name: name, Format: adminhttp.DownloadWorkspaceIconParamsFormat(format)})
	if err != nil {
		return rpcapi.WorkspaceIconDownloadResponse{}, nil, nil, err
	}
	reader, size, rpcErr := workspaceIconDownloadResult(resp)
	if rpcErr != nil {
		return rpcapi.WorkspaceIconDownloadResponse{}, nil, rpcErr, nil
	}
	return rpcapi.WorkspaceIconDownloadResponse{Name: name, Format: params.Format, SizeBytes: size}, reader, nil, nil
}

func workflowIconDownloadResult(resp adminhttp.DownloadWorkflowIconResponseObject) (io.ReadCloser, int64, *rpcapi.RPCError) {
	switch value := resp.(type) {
	case adminhttp.DownloadWorkflowIcon200ApplicationoctetStreamResponse:
		return asReadCloser(value.Body), value.ContentLength, nil
	case adminhttp.DownloadWorkflowIcon200ImagepngResponse:
		return asReadCloser(value.Body), value.ContentLength, nil
	case adminhttp.DownloadWorkflowIcon404JSONResponse:
		return nil, 0, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "workflow icon not found"}
	case adminhttp.DownloadWorkflowIcon500JSONResponse:
		return nil, 0, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "failed to download workflow icon"}
	default:
		return nil, 0, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: fmt.Sprintf("unexpected workflow icon response %T", resp)}
	}
}

func workspaceIconDownloadResult(resp adminhttp.DownloadWorkspaceIconResponseObject) (io.ReadCloser, int64, *rpcapi.RPCError) {
	switch value := resp.(type) {
	case adminhttp.DownloadWorkspaceIcon200ApplicationoctetStreamResponse:
		return asReadCloser(value.Body), value.ContentLength, nil
	case adminhttp.DownloadWorkspaceIcon200ImagepngResponse:
		return asReadCloser(value.Body), value.ContentLength, nil
	case adminhttp.DownloadWorkspaceIcon404JSONResponse:
		return nil, 0, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "workspace icon not found"}
	case adminhttp.DownloadWorkspaceIcon500JSONResponse:
		return nil, 0, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "failed to download workspace icon"}
	default:
		return nil, 0, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: fmt.Sprintf("unexpected workspace icon response %T", resp)}
	}
}

func asReadCloser(reader io.Reader) io.ReadCloser {
	if closer, ok := reader.(io.ReadCloser); ok {
		return closer
	}
	return io.NopCloser(reader)
}
