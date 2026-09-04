package gizclaw

import (
	"context"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type rpcWorkspaceHistoryAudioService interface {
	PrepareWorkspaceHistoryAudioDownload(context.Context, rpcapi.WorkspaceHistoryAudioDownloadRequest) (rpcapi.WorkspaceHistoryAudioDownloadResponse, io.ReadCloser, *rpcapi.RPCStatus, error)
}

func (s *rpcServer) handleWorkspaceHistoryAudioDownload(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInvalidArgument, "missing params")
	}
	params, err := req.Params.AsWorkspaceHistoryAudioDownloadRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInvalidArgument, "invalid params")
	}
	service, ok := s.serverResources.(rpcWorkspaceHistoryAudioService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInternal, "workspace history audio service not configured")
	}
	metadata, reader, rpcErr, err := service.PrepareWorkspaceHistoryAudioDownload(ctx, params)
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInternal, err.Error())
	}
	if rpcErr != nil {
		return writeRPCStatusResponse(stream, req.Id, rpcErr)
	}
	return writeHistoryAudioResponse(stream, req, metadata, reader, (*rpcapi.RPCPayload).FromWorkspaceHistoryAudioDownloadResponse)
}

func writeHistoryAudioResponse[T any](stream *rpcStream, req *rpcapi.RPCRequest, metadata T, reader io.ReadCloser, encode func(*rpcapi.RPCPayload, T) error) error {
	if reader == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInternal, "history audio reader not configured")
	}
	defer reader.Close()
	resp, err := newRPCResultResponse(req.Id, metadata, encode)
	if err != nil {
		return err
	}
	metadataEOS, err := stream.WriteResponseEnvelopeForMethod(req.Method, resp)
	if err != nil {
		return err
	}
	if metadataEOS {
		if err := stream.WriteEOS(); err != nil {
			return err
		}
	}
	return writeReaderBinaryFrames(stream, reader)
}
