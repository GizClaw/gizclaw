package peerresource

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
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
}

func (s *Server) handleFirmwareGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFirmwareGetRequest)
	if !ok || !params.Channel.Valid() {
		return invalidParams(req.Id)
	}
	result, err := s.getFirmwareChannel(ctx, params.Channel)
	if err != nil {
		return firmwareRPCError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFirmwareGetResponse)
}

func (s *Server) getFirmwareChannel(ctx context.Context, channel rpcapi.FirmwareChannelName) (rpcapi.FirmwareGetResponse, error) {
	item, err := s.boundFirmware(ctx)
	if err != nil {
		return rpcapi.FirmwareGetResponse{}, err
	}
	slot := firmwareSlot(item.Slots, channel)
	if slot.Package == nil {
		return rpcapi.FirmwareGetResponse{}, errFirmwarePackageNotFound
	}
	return rpcapi.FirmwareGetResponse{
		Channel:     channel,
		Description: slot.Description,
		Url:         slot.Package.Url,
		Sha256:      slot.Package.Sha256,
		Size:        slot.Package.Size,
		Version:     slot.Package.Version,
	}, nil
}

func (s *Server) handleFirmwareMetadataGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFirmwareMetadataGetRequest)
	if !ok || firmware.ValidateMetadataKey(params.GetKey()) != nil {
		return invalidParams(req.Id)
	}
	item, err := s.boundFirmware(ctx)
	if err != nil {
		return firmwareRPCError(req.Id, err)
	}
	if item.Metadata == nil {
		return firmwareRPCError(req.Id, errFirmwareMetadataNotFound)
	}
	entry, exists := (*item.Metadata)[params.Key]
	if !exists {
		return firmwareRPCError(req.Id, errFirmwareMetadataNotFound)
	}
	value, err := json.Marshal(entry)
	if err != nil {
		return internalError(req.Id, "firmware metadata value unavailable")
	}
	var body rpcapi.RPCPayload
	if err := body.FromFirmwareMetadataGetResponse(&rpcpb.FirmwareMetadataGetResponse{
		Key: params.Key, Value: string(value),
	}); err != nil {
		return internalError(req.Id, "firmware metadata encoding unavailable")
	}
	return &rpcapi.RPCResponse{V: rpcapi.RPCVersionV1, Id: req.Id, Result: &body}
}

func (s *Server) boundFirmware(ctx context.Context) (apitypes.Firmware, error) {
	firmwareID, err := s.boundFirmwareID(ctx)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if s.Firmwares == nil {
		return apitypes.Firmware{}, errors.New("firmware service not configured")
	}
	response, err := s.Firmwares.GetFirmware(ctx, adminhttp.GetFirmwareRequestObject{Id: firmwareID})
	if err != nil {
		return apitypes.Firmware{}, err
	}
	switch response := response.(type) {
	case adminhttp.GetFirmware200JSONResponse:
		return apitypes.Firmware(response), nil
	case adminhttp.GetFirmware404JSONResponse:
		return apitypes.Firmware{}, kv.ErrNotFound
	case adminhttp.GetFirmware500JSONResponse:
		return apitypes.Firmware{}, errors.New("firmware lookup failed")
	default:
		return apitypes.Firmware{}, errors.New("unexpected firmware lookup response")
	}
}

func firmwareSlot(slots apitypes.FirmwareSlots, channel rpcapi.FirmwareChannelName) apitypes.FirmwareSlot {
	switch channel {
	case rpcapi.FirmwareChannelNameStable:
		return slots.Stable
	case rpcapi.FirmwareChannelNameBeta:
		return slots.Beta
	case rpcapi.FirmwareChannelNameDevelop:
		return slots.Develop
	default:
		return apitypes.FirmwareSlot{}
	}
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
	if item.FirmwareId == nil || customid.ValidateResourceID(*item.FirmwareId) != nil {
		return "", errFirmwareNotBound
	}
	return *item.FirmwareId, nil
}

var (
	errFirmwareNotBound         = errors.New("firmware is not bound to peer")
	errFirmwarePackageNotFound  = errors.New("firmware package not found")
	errFirmwareMetadataNotFound = errors.New("firmware metadata key not found")
)

func firmwareRPCError(id string, err error) *rpcapi.RPCResponse {
	body := firmwareRPCErrorBody(err)
	return rpcapi.Error{RequestID: id, Code: body.Code, Message: body.Message}.RPCResponse()
}

func firmwareRPCErrorBody(err error) *rpcapi.RPCStatus {
	switch {
	case errors.Is(err, errFirmwareNotBound):
		return &rpcapi.RPCStatus{Code: rpcapi.StatusCodeNotFound, Message: err.Error()}
	case errors.Is(err, kv.ErrNotFound):
		return &rpcapi.RPCStatus{Code: rpcapi.StatusCodeNotFound, Message: "firmware not found"}
	case errors.Is(err, errFirmwarePackageNotFound):
		return &rpcapi.RPCStatus{Code: rpcapi.StatusCodeNotFound, Message: err.Error()}
	case errors.Is(err, errFirmwareMetadataNotFound):
		return &rpcapi.RPCStatus{Code: rpcapi.StatusCodeNotFound, Message: err.Error()}
	default:
		return &rpcapi.RPCStatus{Code: rpcapi.StatusCodeInternal, Message: "firmware lookup unavailable"}
	}
}
