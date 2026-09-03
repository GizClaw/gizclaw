package gizclaw

import (
	"context"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type rpcResourceIconDownloadService interface {
	PrepareWorkspaceIconDownload(context.Context, rpcapi.WorkspaceIconDownloadRequest) (rpcapi.WorkspaceIconDownloadResponse, io.ReadCloser, *rpcapi.RPCStatus, error)
}

func (s *rpcServer) handleWorkspaceIconDownload(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInvalidArgument, "missing params")
	}
	params, err := req.Params.AsWorkspaceIconDownloadRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInvalidArgument, "invalid params")
	}
	service, ok := s.serverResources.(rpcResourceIconDownloadService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInternal, "workspace icon service not configured")
	}
	metadata, reader, rpcErr, err := service.PrepareWorkspaceIconDownload(ctx, params)
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInternal, "failed to prepare workspace icon download")
	}
	if rpcErr != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcErr.Code, rpcErr.Message)
	}
	defer reader.Close()
	return writeRPCDownload(ctx, stream, req, metadata, (*rpcapi.RPCPayload).FromWorkspaceIconDownloadResponse, reader)
}
