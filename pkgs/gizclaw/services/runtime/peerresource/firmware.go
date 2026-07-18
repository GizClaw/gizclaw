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
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (s *Server) handleFirmwareList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Firmwares == nil {
		return internalError(req.Id, "firmware service not configured")
	}
	_, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsFirmwareListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	name := ""
	if s.FirmwareName != nil {
		name = s.FirmwareName()
	}
	if name == "" {
		return resultResponse(req.Id, adminhttp.FirmwareList{Items: []apitypes.Firmware{}}, (*rpcapi.RPCPayload).FromFirmwareListResponse)
	}
	resp, err := s.Firmwares.GetFirmware(ctx, adminhttp.GetFirmwareRequestObject{Name: name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	item, rpcResp, err := adminResult[apitypes.Firmware](resp.VisitGetFirmwareResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	return resultResponse(req.Id, adminhttp.FirmwareList{Items: []apitypes.Firmware{item}}, (*rpcapi.RPCPayload).FromFirmwareListResponse)
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
	item, _, err := s.firmwareSlot(ctx, params.FirmwareId, params.Channel)
	if err != nil {
		return rpcapi.FirmwareFilesDownloadResponse{}, nil, firmwareRPCErrorBody(err), nil
	}
	artifact, entry, reader, err := s.Firmwares.PrepareArtifactEntryDownload(ctx, item.Name, string(params.Channel), params.Path)
	if err != nil {
		return rpcapi.FirmwareFilesDownloadResponse{}, nil, firmwareDownloadRPCErrorBody(err), nil
	}
	return rpcapi.FirmwareFilesDownloadResponse{
		FirmwareId: item.Name,
		Channel:    params.Channel,
		Path:       entry.Path,
		Artifact:   rpcFirmwareArtifact(artifact),
		File:       rpcFirmwareArtifactEntry(entry),
	}, reader, nil, nil
}

func (s *Server) firmwareSlot(ctx context.Context, id string, channel rpcapi.FirmwareChannelName) (apitypes.Firmware, apitypes.FirmwareSlot, error) {
	item, err := s.firmwareGet(ctx, id)
	if err != nil {
		return apitypes.Firmware{}, apitypes.FirmwareSlot{}, err
	}
	if !channel.Valid() {
		return apitypes.Firmware{}, apitypes.FirmwareSlot{}, errInvalidFirmwareRequest
	}
	slot, ok := firmwareSlotByName(item.Slots, channel)
	if !ok {
		return apitypes.Firmware{}, apitypes.FirmwareSlot{}, errInvalidFirmwareRequest
	}
	return item, slot, nil
}

func (s *Server) firmwareGet(ctx context.Context, id string) (apitypes.Firmware, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.Firmware{}, errInvalidFirmwareRequest
	}
	if s.Firmwares == nil || s.Firmwares.Store == nil {
		return apitypes.Firmware{}, errors.New("firmware service not configured")
	}
	registered := ""
	if s.FirmwareName != nil {
		registered = s.FirmwareName()
	}
	if registered == "" || registered != id {
		return apitypes.Firmware{}, kv.ErrNotFound
	}
	return firmware.Get(ctx, s.Firmwares.Store, id)
}

func firmwareSlotByName(slots apitypes.FirmwareSlots, channel rpcapi.FirmwareChannelName) (apitypes.FirmwareSlot, bool) {
	switch channel {
	case rpcapi.FirmwareChannelNameStable:
		return slots.Stable, true
	case rpcapi.FirmwareChannelNameBeta:
		return slots.Beta, true
	case rpcapi.FirmwareChannelNameDevelop:
		return slots.Develop, true
	case rpcapi.FirmwareChannelNamePending:
		return slots.Pending, true
	default:
		return apitypes.FirmwareSlot{}, false
	}
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

func firmwareDownloadRPCErrorBody(err error) *rpcapi.RPCError {
	switch {
	case err == nil:
		return nil
	case firmware.IsInvalidArtifactError(err):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: err.Error()}
	case firmware.IsArtifactNotFoundError(err):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}
	default:
		return firmwareRPCErrorBody(err)
	}
}

func rpcFirmwareArtifact(artifact apitypes.FirmwareArtifact) rpcapi.FirmwareArtifact {
	return rpcapi.FirmwareArtifact{
		ContentType:  artifact.ContentType,
		FilesPath:    artifact.FilesPath,
		ManifestPath: artifact.ManifestPath,
		Sha256:       artifact.Sha256,
		Size:         artifact.Size,
		TarPath:      artifact.TarPath,
		UploadedAt:   artifact.UploadedAt,
	}
}

func rpcFirmwareArtifactEntry(entry apitypes.FirmwareArtifactEntry) rpcapi.FirmwareArtifactEntry {
	return rpcapi.FirmwareArtifactEntry{
		ContentType: entry.ContentType,
		ModTime:     entry.ModTime,
		Mode:        entry.Mode,
		Path:        entry.Path,
		Size:        entry.Size,
		Type:        rpcapi.FirmwareArtifactEntryType(entry.Type),
	}
}
