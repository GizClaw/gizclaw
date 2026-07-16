package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

const rpcFirmwareDownloadFrameSize = 32 * 1024

type rpcFirmwareDownloadService interface {
	PrepareFirmwareDownload(context.Context, rpcapi.FirmwareFilesDownloadRequest) (rpcapi.FirmwareFilesDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error)
}

func (s *rpcServer) handleFirmwareBinDownload(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsFirmwareFilesDownloadRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	service, ok := s.serverResources.(rpcFirmwareDownloadService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "firmware service not configured")
	}
	metadata, reader, rpcErr, err := service.PrepareFirmwareDownload(ctx, params)
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, err.Error())
	}
	if rpcErr != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcErr.Code, rpcErr.Message)
	}
	defer reader.Close()

	return writeRPCDownload(ctx, stream, req, metadata, (*rpcapi.RPCPayload).FromFirmwareFilesDownloadResponse, reader)
}

func writeReaderBinaryFrames(stream *rpcStream, reader io.Reader) error {
	if reader == nil {
		return fmt.Errorf("rpc: nil binary reader")
	}
	buf := make([]byte, rpcFirmwareDownloadFrameSize)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: buf[:n]}); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return stream.WriteEOS()
		}
		if err != nil {
			return err
		}
	}
}
