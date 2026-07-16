package gizclaw

import (
	"context"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

type rpcAssetDownloadService interface {
	PrepareAssetDownload(context.Context, rpcpb.AssetDownloadRequest) (rpcpb.AssetDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error)
}

func (s *rpcServer) handleAssetDownload(ctx context.Context, stream *rpcStream, request *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if request.Params == nil {
		return writeRPCErrorResponse(stream, request.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := request.Params.AsAssetDownloadRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, request.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	service, ok := s.serverResources.(rpcAssetDownloadService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, request.Id, rpcapi.RPCErrorCodeInternalError, "asset service not configured")
	}
	metadata, reader, rpcErr, err := service.PrepareAssetDownload(ctx, params)
	if err != nil {
		return writeRPCErrorResponse(stream, request.Id, rpcapi.RPCErrorCodeInternalError, "asset download failed")
	}
	if rpcErr != nil {
		return writeRPCErrorResponse(stream, request.Id, rpcErr.Code, rpcErr.Message)
	}
	defer reader.Close()
	return writeRPCDownload(ctx, stream, request, metadata, (*rpcapi.RPCPayload).FromAssetDownloadResponse, reader)
}
