package gizcli

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

var (
	// ErrDeviceRejected makes a device control handler answer INVALID_PARAMS,
	// which the Server maps to 400 DEVICE_REJECTED.
	ErrDeviceRejected = errors.New("gizclaw: device rejected the request")
	// ErrDeviceResourceNotFound makes a device control handler answer
	// NOT_FOUND, used by client.wifi.saved.forget for an unknown ssid.
	ErrDeviceResourceNotFound = errors.New("gizclaw: device resource not found")
)

// DeviceControlHandlers implements the Server-initiated client.device.* and
// client.wifi.* methods for this Client. A nil handler answers
// METHOD_NOT_FOUND, which the Server maps to 501 DEVICE_UNSUPPORTED.
type DeviceControlHandlers struct {
	Status      func(context.Context) (rpcapi.PeerStatus, error)
	SetVolume   func(ctx context.Context, level int64, muted bool) (rpcapi.PeerStatus, error)
	PlaySound   func(ctx context.Context, sound string, durationMs *int64) error
	Reboot      func(ctx context.Context, delayMs *int64) error
	WifiStatus  func(context.Context) (rpcapi.WifiStatus, error)
	SavedWifi   func(context.Context) ([]rpcapi.WifiSavedNetwork, error)
	ForgetWifi  func(ctx context.Context, ssid string) error
	ScanWifi    func(ctx context.Context, timeoutMs *int64) ([]rpcapi.WifiScanResult, error)
	ConnectWifi func(ctx context.Context, ssid string, passphrase *string) error
	// UpdateFirmware runs one OTA. channel names the channel to install and is
	// nil when the caller leaves the choice to the device; sha256 is the
	// package digest the caller resolved, and the handler answers
	// ErrDeviceRejected when it does not match the package the device resolves.
	UpdateFirmware func(ctx context.Context, channel *rpcapi.FirmwareChannelName, sha256 *string) error
}

// HandleDeviceControl installs the device control providers for this Client.
func (c *Client) HandleDeviceControl(handlers DeviceControlHandlers) error {
	if c == nil {
		return errors.New("gizclaw: nil client")
	}
	c.deviceMu.Lock()
	defer c.deviceMu.Unlock()
	c.deviceHandlers = &handlers
	return nil
}

func (c *Client) deviceControlHandlers() *DeviceControlHandlers {
	if c == nil {
		return nil
	}
	c.deviceMu.RLock()
	defer c.deviceMu.RUnlock()
	return c.deviceHandlers
}

func deviceControlError(id string, err error) *rpcapi.RPCResponse {
	var rpcErr rpcapi.Error
	switch {
	case errors.As(err, &rpcErr):
		return rpcapi.Error{RequestID: id, Code: rpcErr.Code, Message: rpcErr.Message}.RPCResponse()
	case errors.Is(err, ErrDeviceRejected):
		return rpcapi.Error{RequestID: id, Code: rpcapi.StatusCodeInvalidArgument, Message: err.Error()}.RPCResponse()
	case errors.Is(err, ErrDeviceResourceNotFound):
		return rpcapi.Error{RequestID: id, Code: rpcapi.StatusCodeNotFound, Message: err.Error()}.RPCResponse()
	default:
		return rpcapi.Error{RequestID: id, Code: rpcapi.StatusCodeInternal, Message: "device handler failed"}.RPCResponse()
	}
}

func deviceControlUnsupported(id string, method rpcapi.RPCMethod) *rpcapi.RPCResponse {
	return rpcapi.Error{RequestID: id, Code: rpcapi.StatusCodeUnimplemented, Message: fmt.Sprintf("unsupported method: %s", method)}.RPCResponse()
}

func (c *rpcClient) handleDeviceControl(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	handlers := c.peer.deviceControlHandlers()
	if handlers == nil {
		return deviceControlUnsupported(req.Id, req.Method), nil
	}
	switch req.Method {
	case rpcapi.RPCMethodClientDeviceStatusGet:
		if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsClientDeviceStatusGetRequest); err != nil {
			return rpcInvalidParams(req.Id), nil
		}
		if handlers.Status == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		status, err := handlers.Status(ctx)
		if err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientDeviceStatusGetResponse{Value: status}, (*rpcapi.RPCPayload).FromClientDeviceStatusGetResponse)
	case rpcapi.RPCMethodClientDeviceVolumeSet:
		if req.Params == nil {
			return rpcInvalidParams(req.Id), nil
		}
		params, err := req.Params.AsClientDeviceVolumeSetRequest()
		if err != nil || params.Level < 0 || params.Level > 100 {
			return rpcInvalidParams(req.Id), nil
		}
		if handlers.SetVolume == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		status, err := handlers.SetVolume(ctx, params.Level, params.Muted)
		if err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientDeviceVolumeSetResponse{Value: status}, (*rpcapi.RPCPayload).FromClientDeviceVolumeSetResponse)
	case rpcapi.RPCMethodClientDeviceSoundPlay:
		if req.Params == nil {
			return rpcInvalidParams(req.Id), nil
		}
		params, err := req.Params.AsClientDeviceSoundPlayRequest()
		if err != nil || params.Sound == "" || len(params.Sound) > 32 {
			return rpcInvalidParams(req.Id), nil
		}
		if handlers.PlaySound == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		if err := handlers.PlaySound(ctx, params.Sound, params.DurationMs); err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientDeviceSoundPlayResponse{}, (*rpcapi.RPCPayload).FromClientDeviceSoundPlayResponse)
	case rpcapi.RPCMethodClientDeviceReboot:
		params := rpcapi.ClientDeviceRebootRequest{}
		if req.Params != nil {
			decoded, err := req.Params.AsClientDeviceRebootRequest()
			if err != nil {
				return rpcInvalidParams(req.Id), nil
			}
			params = decoded
		}
		if handlers.Reboot == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		if err := handlers.Reboot(ctx, params.DelayMs); err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientDeviceRebootResponse{}, (*rpcapi.RPCPayload).FromClientDeviceRebootResponse)
	case rpcapi.RPCMethodClientFirmwareUpdate:
		params := rpcapi.ClientFirmwareUpdateRequest{}
		if req.Params != nil {
			decoded, err := req.Params.AsClientFirmwareUpdateRequest()
			if err != nil {
				return rpcInvalidParams(req.Id), nil
			}
			params = decoded
		}
		if params.Channel != nil && !params.Channel.Valid() {
			return rpcInvalidParams(req.Id), nil
		}
		if handlers.UpdateFirmware == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		if err := handlers.UpdateFirmware(ctx, params.Channel, params.Sha256); err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientFirmwareUpdateResponse{}, (*rpcapi.RPCPayload).FromClientFirmwareUpdateResponse)
	case rpcapi.RPCMethodClientWifiStatusGet:
		if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsClientWifiStatusGetRequest); err != nil {
			return rpcInvalidParams(req.Id), nil
		}
		if handlers.WifiStatus == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		status, err := handlers.WifiStatus(ctx)
		if err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientWifiStatusGetResponse{Value: status}, (*rpcapi.RPCPayload).FromClientWifiStatusGetResponse)
	case rpcapi.RPCMethodClientWifiSavedList:
		if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsClientWifiSavedListRequest); err != nil {
			return rpcInvalidParams(req.Id), nil
		}
		if handlers.SavedWifi == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		networks, err := handlers.SavedWifi(ctx)
		if err != nil {
			return deviceControlError(req.Id, err), nil
		}
		if networks == nil {
			networks = []rpcapi.WifiSavedNetwork{}
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientWifiSavedListResponse{Networks: networks}, (*rpcapi.RPCPayload).FromClientWifiSavedListResponse)
	case rpcapi.RPCMethodClientWifiSavedForget:
		if req.Params == nil {
			return rpcInvalidParams(req.Id), nil
		}
		params, err := req.Params.AsClientWifiSavedForgetRequest()
		if err != nil || params.Ssid == "" || len(params.Ssid) > 32 {
			return rpcInvalidParams(req.Id), nil
		}
		if handlers.ForgetWifi == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		if err := handlers.ForgetWifi(ctx, params.Ssid); err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientWifiSavedForgetResponse{}, (*rpcapi.RPCPayload).FromClientWifiSavedForgetResponse)
	case rpcapi.RPCMethodClientWifiScan:
		params := rpcapi.ClientWifiScanRequest{}
		if req.Params != nil {
			decoded, err := req.Params.AsClientWifiScanRequest()
			if err != nil {
				return rpcInvalidParams(req.Id), nil
			}
			params = decoded
		}
		if params.TimeoutMs != nil && (*params.TimeoutMs < 1000 || *params.TimeoutMs > 15000) {
			return rpcInvalidParams(req.Id), nil
		}
		if handlers.ScanWifi == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		networks, err := handlers.ScanWifi(ctx, params.TimeoutMs)
		if err != nil {
			return deviceControlError(req.Id, err), nil
		}
		if networks == nil {
			networks = []rpcapi.WifiScanResult{}
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientWifiScanResponse{Networks: networks}, (*rpcapi.RPCPayload).FromClientWifiScanResponse)
	case rpcapi.RPCMethodClientWifiConnect:
		if req.Params == nil {
			return rpcInvalidParams(req.Id), nil
		}
		params, err := req.Params.AsClientWifiConnectRequest()
		if err != nil || params.Ssid == "" || len(params.Ssid) > 32 ||
			(params.Passphrase != nil && (len(*params.Passphrase) < 8 || len(*params.Passphrase) > 63)) {
			return rpcInvalidParams(req.Id), nil
		}
		if handlers.ConnectWifi == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		if err := handlers.ConnectWifi(ctx, params.Ssid, params.Passphrase); err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientWifiConnectResponse{}, (*rpcapi.RPCPayload).FromClientWifiConnectResponse)
	default:
		return deviceControlUnsupported(req.Id, req.Method), nil
	}
}
