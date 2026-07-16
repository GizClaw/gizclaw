package gizclaw

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type rpcResourceIconDownloadService interface {
	PrepareWorkflowIconDownload(context.Context, rpcapi.WorkflowIconDownloadRequest) (rpcapi.WorkflowIconDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error)
	PrepareWorkspaceIconDownload(context.Context, rpcapi.WorkspaceIconDownloadRequest) (rpcapi.WorkspaceIconDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error)
}

type rpcPeerIconService interface {
	DownloadSelfIcon(context.Context, giznet.PublicKey, iconasset.Format) (io.ReadCloser, int64, error)
	UploadSelfIcon(context.Context, giznet.PublicKey, iconasset.Format, io.Reader) (apitypes.DeviceInfo, error)
	DeleteSelfIcon(context.Context, giznet.PublicKey, iconasset.Format) (apitypes.DeviceInfo, error)
}

func (s *rpcServer) handleWorkflowIconDownload(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsWorkflowIconDownloadRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	service, ok := s.serverResources.(rpcResourceIconDownloadService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "workflow icon service not configured")
	}
	metadata, reader, rpcErr, err := service.PrepareWorkflowIconDownload(ctx, params)
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "failed to prepare workflow icon download")
	}
	if rpcErr != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcErr.Code, rpcErr.Message)
	}
	defer reader.Close()
	return writeRPCDownload(ctx, stream, req, metadata, (*rpcapi.RPCPayload).FromWorkflowIconDownloadResponse, reader)
}

func (s *rpcServer) handleWorkspaceIconDownload(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsWorkspaceIconDownloadRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	service, ok := s.serverResources.(rpcResourceIconDownloadService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "workspace icon service not configured")
	}
	metadata, reader, rpcErr, err := service.PrepareWorkspaceIconDownload(ctx, params)
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "failed to prepare workspace icon download")
	}
	if rpcErr != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcErr.Code, rpcErr.Message)
	}
	defer reader.Close()
	return writeRPCDownload(ctx, stream, req, metadata, (*rpcapi.RPCPayload).FromWorkspaceIconDownloadResponse, reader)
}

func (s *rpcServer) handleInfoIconDownload(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsServerInfoIconDownloadRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	format, err := iconasset.ParseFormat(string(params.Format))
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, err.Error())
	}
	service, ok := s.peer.(rpcPeerIconService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "peer icon service not configured")
	}
	reader, size, err := service.DownloadSelfIcon(ctx, s.callerPublicKey, format)
	if err != nil {
		code := rpcapi.RPCErrorCodeInternalError
		message := "failed to download peer icon"
		if errors.Is(err, peer.ErrPeerNotFound) || errors.Is(err, io.EOF) {
			code = rpcapi.RPCErrorCodeNotFound
			message = "peer icon not found"
		}
		return writeRPCErrorResponse(stream, req.Id, code, message)
	}
	defer reader.Close()
	metadata := rpcapi.ServerInfoIconDownloadResponse{Format: params.Format, SizeBytes: size}
	return writeRPCDownload(ctx, stream, req, metadata, (*rpcapi.RPCPayload).FromServerInfoIconDownloadResponse, reader)
}

func (s *rpcServer) handleInfoIconUpload(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsServerInfoIconUploadRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	format, err := iconasset.ParseFormat(string(params.Format))
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, err.Error())
	}
	data, err := readIconUploadFrames(stream)
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, err.Error())
	}
	service, ok := s.peer.(rpcPeerIconService)
	if !ok || service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "peer icon service not configured")
	}
	info, err := service.UploadSelfIcon(ctx, s.callerPublicKey, format, bytes.NewReader(data))
	if err != nil {
		code := rpcapi.RPCErrorCodeInternalError
		message := "failed to upload peer icon"
		switch {
		case errors.Is(err, iconasset.ErrInvalid), errors.Is(err, iconasset.ErrTooLarge):
			code = rpcapi.RPCErrorCodeInvalidParams
			message = err.Error()
		case errors.Is(err, peer.ErrPeerNotFound):
			code = rpcapi.RPCErrorCodeNotFound
			message = "peer not found"
		}
		return writeRPCErrorResponse(stream, req.Id, code, message)
	}
	result, err := convertRPCType[rpcapi.ServerInfoIconUploadResponse](info)
	if err != nil {
		return err
	}
	return writeRPCStreamResult(stream, req, result, (*rpcapi.RPCPayload).FromServerInfoIconUploadResponse)
}

func (s *rpcServer) handleInfoIconDelete(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcInvalidParams(req.Id), nil
	}
	params, err := req.Params.AsServerInfoIconDeleteRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	format, err := iconasset.ParseFormat(string(params.Format))
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	service, ok := s.peer.(rpcPeerIconService)
	if !ok || service == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer icon service not configured"}.RPCResponse(), nil
	}
	info, err := service.DeleteSelfIcon(ctx, s.callerPublicKey, format)
	if err != nil {
		code := rpcapi.RPCErrorCodeInternalError
		message := "failed to delete peer icon"
		if errors.Is(err, peer.ErrPeerNotFound) {
			code = rpcapi.RPCErrorCodeNotFound
			message = "peer not found"
		}
		return rpcapi.Error{RequestID: req.Id, Code: code, Message: message}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerInfoIconDeleteResponse](info)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerInfoIconDeleteResponse)
}

func readIconUploadFrames(stream *rpcStream) ([]byte, error) {
	var data bytes.Buffer
	for {
		frame, err := stream.ReadFrame()
		if err != nil {
			return nil, err
		}
		if frame.Type == rpcapi.FrameTypeEOS {
			break
		}
		if frame.Type != rpcapi.FrameTypeBinary {
			return nil, errors.New("rpc: expected binary icon frame")
		}
		if int64(data.Len()+len(frame.Payload)) > iconasset.MaxBytes {
			return nil, iconasset.ErrTooLarge
		}
		_, _ = data.Write(frame.Payload)
	}
	return data.Bytes(), nil
}

func writeRPCStreamResult[T any](stream *rpcStream, req *rpcapi.RPCRequest, result T, encode func(*rpcapi.RPCPayload, T) error) error {
	resp, err := newRPCResultResponse(req.Id, result, encode)
	if err != nil {
		return err
	}
	if _, err := stream.WriteResponseEnvelopeForMethod(req.Method, resp); err != nil {
		return err
	}
	return stream.WriteEOS()
}
