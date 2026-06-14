package peerresource

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/firmware"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func (s *Server) handleFirmwareList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Firmwares == nil {
		return internalError(req.Id, "firmware service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFirmwareListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := s.Firmwares.ListFirmwares(ctx, adminservice.ListFirmwaresRequestObject{
		Params: adminservice.ListFirmwaresParams{Cursor: params.Cursor, Limit: int32Ptr(params.Limit)},
	})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	list, rpcResp, err := adminResult[adminservice.FirmwareList](resp.VisitListFirmwaresResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	items := make([]apitypes.Firmware, 0, len(list.Items))
	for _, item := range list.Items {
		err := s.authorizeErr(ctx, acl.FirmwareResource(item.Name), apitypes.ACLPermissionFirmwareRead)
		if errors.Is(err, acl.ErrDenied) {
			continue
		}
		if err != nil {
			return authError(req.Id, err)
		}
		items = append(items, item)
	}
	return resultResponse(req.Id, adminservice.FirmwareList{Items: items, HasNext: list.HasNext, NextCursor: list.NextCursor}, (*rpcapi.RPCResponse_Result).FromFirmwareListResponse)
}

func (s *Server) handleFirmwareGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFirmwareGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	item, err := s.firmwareGet(ctx, params.FirmwareId)
	if err != nil {
		return firmwareRPCError(req.Id, err)
	}
	return resultResponse(req.Id, item, (*rpcapi.RPCResponse_Result).FromFirmwareGetResponse)
}

func (s *Server) handleFirmwareDownload(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFirmwareDownloadRequest)
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
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFirmwareDownloadResponse)
}

func (s *Server) PrepareFirmwareDownload(ctx context.Context, params rpcapi.FirmwareDownloadRequest) (rpcapi.FirmwareDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	item, slot, err := s.firmwareSlot(ctx, params.FirmwareId, params.Channel)
	if err != nil {
		return rpcapi.FirmwareDownloadResponse{}, nil, firmwareRPCErrorBody(err), nil
	}
	artifact, ok := firmwareArtifact(slot, params.ArtifactName)
	if !ok {
		return rpcapi.FirmwareDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "firmware artifact not found"}, nil
	}
	if artifact.Path == nil || strings.TrimSpace(*artifact.Path) == "" {
		return rpcapi.FirmwareDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "firmware artifact payload not uploaded"}, nil
	}
	if s.Firmwares == nil || s.Firmwares.Assets == nil {
		return rpcapi.FirmwareDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "firmware asset store not configured"}, nil
	}
	reader, err := s.Firmwares.Assets.Get(*artifact.Path)
	if err != nil {
		return rpcapi.FirmwareDownloadResponse{}, nil, nil, err
	}
	return rpcapi.FirmwareDownloadResponse{
		FirmwareId: item.Name,
		Channel:    params.Channel,
		Artifact:   firmwareBinMetadata(*artifact),
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
	if err := s.authorizeErr(ctx, acl.FirmwareResource(id), apitypes.ACLPermissionFirmwareRead); err != nil {
		return apitypes.Firmware{}, err
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

func firmwareArtifact(slot apitypes.FirmwareSlot, name string) (*apitypes.FirmwareArtifact, bool) {
	name = strings.TrimSpace(name)
	if name == "" || slot.Artifacts == nil {
		return nil, false
	}
	for i := range *slot.Artifacts {
		if (*slot.Artifacts)[i].Name == name {
			return &(*slot.Artifacts)[i], true
		}
	}
	return nil, false
}

func firmwareBinMetadata(artifact apitypes.FirmwareArtifact) rpcapi.FirmwareBinMetadata {
	return rpcapi.FirmwareBinMetadata{
		Name:        artifact.Name,
		Kind:        rpcapi.FirmwareArtifactKind(artifact.Kind),
		Size:        artifact.Size,
		Sha256:      artifact.Sha256,
		ContentType: artifact.ContentType,
		UploadedAt:  artifact.UploadedAt,
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
	case errors.Is(err, acl.ErrDenied):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeForbidden, Message: err.Error()}
	case errors.Is(err, errInvalidFirmwareRequest):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: err.Error()}
	case err.Error() == "acl service not configured":
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}
	default:
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}
	}
}
