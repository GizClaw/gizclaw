package gizclaw

import (
	"context"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type rpcWorkspaceHistoryAudioService interface {
	PrepareWorkspaceHistoryAudioGet(context.Context, rpcapi.WorkspaceHistoryAudioGetRequest) (rpcapi.WorkspaceHistoryAudioGetResponse, io.ReadCloser, *rpcapi.RPCError, error)
}

type rpcFriendGroupMessageAudioService interface {
	PrepareFriendGroupMessageAudioGet(context.Context, rpcapi.FriendGroupMessageAudioGetRequest) (rpcapi.FriendGroupMessageAudioGetResponse, io.ReadCloser, *rpcapi.RPCError, error)
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
	return writeHistoryAudioResponse(stream, req, metadata, reader, (*rpcapi.RPCPayload).FromWorkspaceHistoryAudioGetResponse)
}

func (s *rpcServer) handleFriendGroupMessageAudioGet(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsFriendGroupMessageAudioGetRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	service, ok := s.serverResources.(rpcFriendGroupMessageAudioService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "friend group message audio service not configured")
	}
	metadata, reader, rpcErr, err := service.PrepareFriendGroupMessageAudioGet(ctx, params)
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, err.Error())
	}
	if rpcErr != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcErr.Code, rpcErr.Message)
	}
	return writeHistoryAudioResponse(stream, req, metadata, reader, (*rpcapi.RPCPayload).FromFriendGroupMessageAudioGetResponse)
}

func writeHistoryAudioResponse[T any](stream *rpcStream, req *rpcapi.RPCRequest, metadata T, reader io.ReadCloser, encode func(*rpcapi.RPCPayload, T) error) error {
	if reader == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "history audio reader not configured")
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
