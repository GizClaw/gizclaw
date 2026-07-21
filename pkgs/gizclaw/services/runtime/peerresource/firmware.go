package peerresource

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type peerFirmwareBindingService interface {
	LoadPeer(context.Context, giznet.PublicKey) (apitypes.Peer, error)
}

type firmwarePeerService interface {
	GetFirmware(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error)
	PrepareArtifactEntryDownload(context.Context, string, string, string) (apitypes.FirmwareArtifact, apitypes.FirmwareArtifactEntry, io.ReadCloser, error)
}

func (s *Server) handleFirmwareGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	_, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsFirmwareGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	firmwareID, err := s.boundFirmwareID(ctx)
	if err != nil {
		return firmwareRPCError(req.Id, err)
	}
	if s.Firmwares == nil {
		return internalError(req.Id, "firmware service not configured")
	}
	response, err := s.Firmwares.GetFirmware(ctx, adminhttp.GetFirmwareRequestObject{Name: firmwareID})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, response.VisitGetFirmwareResponse, (*rpcapi.RPCPayload).FromFirmwareGetResponse)
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
	if !params.Channel.Valid() || strings.TrimSpace(params.Path) == "" {
		return rpcapi.FirmwareFilesDownloadResponse{}, nil, firmwareRPCErrorBody(errInvalidFirmwareRequest), nil
	}
	firmwareID, err := s.boundFirmwareID(ctx)
	if err != nil {
		return rpcapi.FirmwareFilesDownloadResponse{}, nil, firmwareRPCErrorBody(err), nil
	}
	if s.Firmwares == nil {
		return rpcapi.FirmwareFilesDownloadResponse{}, nil, nil, errors.New("firmware service not configured")
	}
	artifact, entry, reader, err := s.Firmwares.PrepareArtifactEntryDownload(ctx, firmwareID, string(params.Channel), params.Path)
	if err != nil {
		return rpcapi.FirmwareFilesDownloadResponse{}, nil, firmwareRPCErrorBody(err), nil
	}
	convertedArtifact, err := convertType[rpcapi.FirmwareArtifact](artifact)
	if err != nil {
		_ = reader.Close()
		return rpcapi.FirmwareFilesDownloadResponse{}, nil, nil, err
	}
	convertedEntry, err := convertType[rpcapi.FirmwareArtifactEntry](entry)
	if err != nil {
		_ = reader.Close()
		return rpcapi.FirmwareFilesDownloadResponse{}, nil, nil, err
	}
	return rpcapi.FirmwareFilesDownloadResponse{
		Artifact:   convertedArtifact,
		Channel:    params.Channel,
		File:       convertedEntry,
		FirmwareId: firmwareID,
		Path:       params.Path,
	}, reader, nil, nil
}

func (s *Server) boundFirmwareID(ctx context.Context) (string, error) {
	if s == nil || s.Peers == nil {
		return "", errors.New("peer service not configured")
	}
	item, err := s.Peers.LoadPeer(ctx, s.Caller)
	if err != nil {
		if errors.Is(err, peer.ErrPeerNotFound) {
			return "", errFirmwareNotBound
		}
		return "", err
	}
	if item.FirmwareId == nil || strings.TrimSpace(*item.FirmwareId) == "" {
		return "", errFirmwareNotBound
	}
	return strings.TrimSpace(*item.FirmwareId), nil
}

var (
	errInvalidFirmwareRequest = errors.New("invalid firmware request")
	errFirmwareNotBound       = errors.New("firmware is not bound to peer")
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
	case errors.Is(err, errFirmwareNotBound):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}
	case errors.Is(err, kv.ErrNotFound):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "firmware not found"}
	case firmware.IsArtifactNotFoundError(err):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "firmware artifact not found"}
	case errors.Is(err, errInvalidFirmwareRequest), firmware.IsInvalidArtifactError(err):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: err.Error()}
	default:
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}
	}
}
