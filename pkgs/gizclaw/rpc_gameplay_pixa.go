package gizclaw

import (
	"context"
	"errors"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type rpcGameplayPixaDownloadService interface {
	PreparePetDefPixaDownload(context.Context, rpcapi.PetDefPixaDownloadRequest) (rpcapi.PetDefPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error)
	PrepareBadgeDefPixaDownload(context.Context, rpcapi.BadgeDefPixaDownloadRequest) (rpcapi.BadgeDefPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error)
}

func (s *rpcServer) handlePetDefPixaDownload(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsPetDefPixaDownloadRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	service, ok := s.serverResources.(rpcGameplayPixaDownloadService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "gameplay service not configured")
	}
	metadata, reader, rpcErr, err := service.PreparePetDefPixaDownload(ctx, params)
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, err.Error())
	}
	if rpcErr != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcErr.Code, rpcErr.Message)
	}
	defer reader.Close()

	resp, err := newRPCResultResponse(req.Id, metadata, (*rpcapi.RPCResponse_Result).FromPetDefPixaDownloadResponse)
	if err != nil {
		return err
	}
	if err := stream.WriteResponseForMethod(req.Method, resp); err != nil {
		return err
	}
	if err := writeReaderBinaryFrames(stream, reader); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

func (s *rpcServer) handleBadgeDefPixaDownload(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsBadgeDefPixaDownloadRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	service, ok := s.serverResources.(rpcGameplayPixaDownloadService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "gameplay service not configured")
	}
	metadata, reader, rpcErr, err := service.PrepareBadgeDefPixaDownload(ctx, params)
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, err.Error())
	}
	if rpcErr != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcErr.Code, rpcErr.Message)
	}
	defer reader.Close()

	resp, err := newRPCResultResponse(req.Id, metadata, (*rpcapi.RPCResponse_Result).FromBadgeDefPixaDownloadResponse)
	if err != nil {
		return err
	}
	if err := stream.WriteResponseForMethod(req.Method, resp); err != nil {
		return err
	}
	if err := writeReaderBinaryFrames(stream, reader); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}
