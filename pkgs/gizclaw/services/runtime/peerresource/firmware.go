package peerresource

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (s *Server) handleFirmwareList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	_, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsFirmwareListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	return resultResponse(req.Id, adminhttp.FirmwareList{Items: []apitypes.Firmware{}}, (*rpcapi.RPCPayload).FromFirmwareListResponse)
}

func (s *Server) handleFirmwareGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFirmwareGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	item, err := s.firmwareGet(ctx, params.FirmwareId)
	if err != nil {
		return firmwareRPCError(req.Id, err)
	}
	return resultResponse(req.Id, item, (*rpcapi.RPCPayload).FromFirmwareGetResponse)
}

func (s *Server) handleFirmwareDownload(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFirmwareFilesDownloadRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, reader, rpcErr, err := s.PrepareFirmwareDownload(ctx, params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if reader != nil {
		_ = reader.Close()
	}
	if rpcErr != nil {
		rpcErr.Message = strings.TrimSpace(rpcErr.Message)
		return rpcapi.Error{RequestID: req.Id, Code: rpcErr.Code, Message: rpcErr.Message}.RPCResponse()
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFirmwareFilesDownloadResponse)
}

func (s *Server) PrepareFirmwareDownload(ctx context.Context, params rpcapi.FirmwareFilesDownloadRequest) (rpcapi.FirmwareFilesDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	_, err := s.firmwareGet(ctx, params.FirmwareId)
	if err != nil {
		return rpcapi.FirmwareFilesDownloadResponse{}, nil, firmwareRPCErrorBody(err), nil
	}
	return rpcapi.FirmwareFilesDownloadResponse{}, nil, firmwareRPCErrorBody(kv.ErrNotFound), nil
}

func (s *Server) firmwareGet(ctx context.Context, id string) (apitypes.Firmware, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.Firmware{}, errInvalidFirmwareRequest
	}
	return apitypes.Firmware{}, kv.ErrNotFound
}

var (
	errInvalidFirmwareRequest = errors.New("invalid firmware request")
)

func firmwareRPCError(id string, err error) *rpcapi.RPCResponse {
	body := firmwareRPCErrorBody(err)
	if body == nil {
		return internalError(id, err.Error())
	}
	return rpcapi.Error{RequestID: id, Code: body.Code, Message: body.Message}.RPCResponse()
}

func firmwareRPCErrorBody(err error) *rpcapi.RPCError {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, kv.ErrNotFound):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "firmware not found"}
	case errors.Is(err, errInvalidFirmwareRequest):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: err.Error()}
	default:
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}
	}
}
