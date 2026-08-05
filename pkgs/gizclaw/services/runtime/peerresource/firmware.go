package peerresource

import (
	"context"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
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
	firmwareID, err := s.boundFirmwareID(ctx)
	if err != nil {
		return rpcapi.FirmwareGetResponse{}, err
	}
	if s.Firmwares == nil {
		return rpcapi.FirmwareGetResponse{}, errors.New("firmware service not configured")
	}
	response, err := s.Firmwares.GetFirmware(ctx, adminhttp.GetFirmwareRequestObject{Id: firmwareID})
	if err != nil {
		return rpcapi.FirmwareGetResponse{}, err
	}
	var item apitypes.Firmware
	switch response := response.(type) {
	case adminhttp.GetFirmware200JSONResponse:
		item = apitypes.Firmware(response)
	case adminhttp.GetFirmware404JSONResponse:
		return rpcapi.FirmwareGetResponse{}, kv.ErrNotFound
	case adminhttp.GetFirmware500JSONResponse:
		return rpcapi.FirmwareGetResponse{}, errors.New("firmware lookup failed")
	default:
		return rpcapi.FirmwareGetResponse{}, errors.New("unexpected firmware lookup response")
	}
	slot := firmwareSlot(item.Slots, channel)
	if slot.Package == nil {
		return rpcapi.FirmwareGetResponse{}, errFirmwarePackageNotFound
	}
	return rpcapi.FirmwareGetResponse{
		FirmwareName: item.Name,
		Channel:      channel,
		Description:  slot.Description,
		Url:          slot.Package.Url,
		Sha256:       slot.Package.Sha256,
		Size:         slot.Package.Size,
	}, nil
}

func firmwareSlot(slots apitypes.FirmwareSlots, channel rpcapi.FirmwareChannelName) apitypes.FirmwareSlot {
	switch channel {
	case rpcapi.FirmwareChannelNameStable:
		return slots.Stable
	case rpcapi.FirmwareChannelNameBeta:
		return slots.Beta
	case rpcapi.FirmwareChannelNameDevelop:
		return slots.Develop
	case rpcapi.FirmwareChannelNamePending:
		return slots.Pending
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
	if item.FirmwareId == nil || strings.TrimSpace(*item.FirmwareId) == "" {
		return "", errFirmwareNotBound
	}
	return strings.TrimSpace(*item.FirmwareId), nil
}

var (
	errFirmwareNotBound        = errors.New("firmware is not bound to peer")
	errFirmwarePackageNotFound = errors.New("firmware package not found")
)

func firmwareRPCError(id string, err error) *rpcapi.RPCResponse {
	body := firmwareRPCErrorBody(err)
	return rpcapi.Error{RequestID: id, Code: body.Code, Message: body.Message}.RPCResponse()
}

func firmwareRPCErrorBody(err error) *rpcapi.RPCError {
	switch {
	case errors.Is(err, errFirmwareNotBound):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}
	case errors.Is(err, kv.ErrNotFound):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "firmware not found"}
	case errors.Is(err, errFirmwarePackageNotFound):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}
	default:
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "firmware lookup unavailable"}
	}
}
