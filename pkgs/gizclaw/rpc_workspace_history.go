package gizclaw

import (
	"context"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type rpcWorkspaceHistoryAudioService interface {
	PrepareWorkspaceHistoryAudioGet(context.Context, rpcapi.WorkspaceHistoryAudioGetRequest) (rpcapi.WorkspaceHistoryAudioGetResponse, io.ReadCloser, *rpcapi.RPCError, error)
}

func (s *rpcServer) handleWorkspaceHistoryAudioGet(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsWorkspaceHistoryAudioGetRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	service, ok := s.serverResources.(rpcWorkspaceHistoryAudioService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "workspace history audio service not configured")
	}
	metadata, reader, rpcErr, err := service.PrepareWorkspaceHistoryAudioGet(ctx, params)
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, err.Error())
	}
	if rpcErr != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcErr.Code, rpcErr.Message)
	}
	if reader == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "workspace history audio reader not configured")
	}
	defer reader.Close()
	resp, err := newRPCResultResponse(req.Id, metadata, (*rpcapi.RPCResponse_Result).FromWorkspaceHistoryAudioGetResponse)
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
