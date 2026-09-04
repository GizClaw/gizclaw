package peerresource

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

func (s *Server) PrepareWorkspaceIconDownload(ctx context.Context, params rpcapi.WorkspaceIconDownloadRequest) (rpcapi.WorkspaceIconDownloadResponse, io.ReadCloser, *rpcapi.RPCStatus, error) {
	name := strings.TrimSpace(params.Name)
	format, err := iconasset.ParseFormat(string(params.Format))
	if name == "" || err != nil {
		return rpcapi.WorkspaceIconDownloadResponse{}, nil, &rpcapi.RPCStatus{Code: rpcapi.StatusCodeInvalidArgument, Message: "workspace name and icon format are required"}, nil
	}
	if response := s.requireWorkspaceAccess(ctx, "", name); response != nil {
		return rpcapi.WorkspaceIconDownloadResponse{}, nil, &rpcapi.RPCStatus{Code: response.Error.Code, Message: response.Error.Message}, nil
	}
	item, err := s.getWorkspaceByName(s.ownerContext(ctx), name)
	if err != nil {
		return rpcapi.WorkspaceIconDownloadResponse{}, nil, &rpcapi.RPCStatus{Code: rpcapi.StatusCodeNotFound, Message: "workspace not found"}, nil
	}
	icons, ok := s.Workspaces.(workspace.WorkspaceIconAdminService)
	if !ok || icons == nil {
		return rpcapi.WorkspaceIconDownloadResponse{}, nil, &rpcapi.RPCStatus{Code: rpcapi.StatusCodeInternal, Message: "workspace icon service not configured"}, nil
	}
	resp, err := icons.DownloadWorkspaceIcon(ctx, adminhttp.DownloadWorkspaceIconRequestObject{Id: item.Id, Format: adminhttp.DownloadWorkspaceIconParamsFormat(format)})
	if err != nil {
		return rpcapi.WorkspaceIconDownloadResponse{}, nil, nil, err
	}
	reader, size, rpcErr := workspaceIconDownloadResult(resp)
	if rpcErr != nil {
		return rpcapi.WorkspaceIconDownloadResponse{}, nil, rpcErr, nil
	}
	return rpcapi.WorkspaceIconDownloadResponse{Name: name, Format: params.Format, SizeBytes: size}, reader, nil, nil
}

func workspaceIconDownloadResult(resp adminhttp.DownloadWorkspaceIconResponseObject) (io.ReadCloser, int64, *rpcapi.RPCStatus) {
	switch value := resp.(type) {
	case adminhttp.DownloadWorkspaceIcon200ApplicationoctetStreamResponse:
		return asReadCloser(value.Body), value.ContentLength, nil
	case adminhttp.DownloadWorkspaceIcon200ImagepngResponse:
		return asReadCloser(value.Body), value.ContentLength, nil
	case adminhttp.DownloadWorkspaceIcon404JSONResponse:
		return nil, 0, &rpcapi.RPCStatus{Code: rpcapi.StatusCodeNotFound, Message: "workspace icon not found"}
	case adminhttp.DownloadWorkspaceIcon500JSONResponse:
		return nil, 0, &rpcapi.RPCStatus{Code: rpcapi.StatusCodeInternal, Message: "failed to download workspace icon"}
	default:
		return nil, 0, &rpcapi.RPCStatus{Code: rpcapi.StatusCodeInternal, Message: fmt.Sprintf("unexpected workspace icon response %T", resp)}
	}
}

func asReadCloser(reader io.Reader) io.ReadCloser {
	if closer, ok := reader.(io.ReadCloser); ok {
		return closer
	}
	return io.NopCloser(reader)
}
