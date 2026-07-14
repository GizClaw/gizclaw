package gizclaw

import (
	"context"
	"errors"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type rpcGameplayPixaDownloadService interface {
	PreparePetPixaDownload(context.Context, rpcapi.PetPixaDownloadRequest) (rpcapi.PetPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error)
	PrepareBadgeDefPixaDownload(context.Context, rpcapi.BadgeDefPixaDownloadRequest) (rpcapi.BadgeDefPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error)
}

func (s *rpcServer) handlePetPixaDownload(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsServerPetPixaDownloadRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	service, ok := s.serverResources.(rpcGameplayPixaDownloadService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "gameplay service not configured")
	}
	metadata, reader, rpcErr, err := service.PreparePetPixaDownload(ctx, params)
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, err.Error())
	}
	if rpcErr != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcErr.Code, rpcErr.Message)
	}
	defer reader.Close()

	return writeRPCDownload(ctx, stream, req, metadata, (*rpcapi.RPCPayload).FromServerPetPixaDownloadResponse, reader)
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

	return writeRPCDownload(ctx, stream, req, metadata, (*rpcapi.RPCPayload).FromBadgeDefPixaDownloadResponse, reader)
}

func writeRPCDownload[T any](ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest, metadata T, encode func(*rpcapi.RPCPayload, T) error, reader io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
	if err := writeReaderBinaryFrames(stream, reader); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}
