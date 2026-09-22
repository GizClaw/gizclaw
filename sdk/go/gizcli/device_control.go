package gizcli

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

var (
	// ErrDeviceRejected makes a device control handler answer INVALID_PARAMS,
	// which the Server maps to 400 DEVICE_REJECTED.
	ErrDeviceRejected = errors.New("gizclaw: device rejected the request")
	// ErrDeviceResourceNotFound makes a device control handler answer
	// NOT_FOUND, used by client.wifi.saved.forget for an unknown ssid.
	ErrDeviceResourceNotFound = errors.New("gizclaw: device resource not found")
)

// DeviceControlHandlers installs MHS state handlers and predefined tool/v0 procedures. A nil handler answers
// METHOD_NOT_FOUND, which the Server maps to 501 DEVICE_UNSUPPORTED.
type DeviceControlHandlers struct {
	// ReadMhsStates returns exactly the requested hardware keys, or NOT_FOUND.
	ReadMhsStates func(context.Context, *rpcpb.ClientMhsV0ReadRequest) (*rpcpb.ClientMhsV0ReadResponse, error)
	// WriteMhsStates validates the whole batch and enforces driver safety limits
	// before applying anything. Return actual values or reject the entire batch.
	WriteMhsStates func(context.Context, *rpcpb.ClientMhsV0WriteRequest) (*rpcpb.ClientMhsV0WriteResponse, error)
	AudioPlayer    AudioPlayerHandlers
	Status         func(context.Context) (rpcapi.PeerStatus, error)
	PlaySound      func(ctx context.Context, sound string, durationMs *int64) error
	// Find rings the device's built-in find-me sound. durationMs is nil when
	// the caller leaves the ring time to the device.
	Find        func(ctx context.Context, durationMs *int64) error
	Reboot      func(ctx context.Context, delayMs *int64) error
	SavedWifi   func(context.Context) ([]rpcapi.WifiSavedNetwork, error)
	ForgetWifi  func(ctx context.Context, ssid string) error
	ScanWifi    func(ctx context.Context, timeoutMs *int64) ([]rpcapi.WifiScanResult, error)
	ConnectWifi func(ctx context.Context, ssid string, passphrase *string) error
	// UpdateFirmware runs one OTA. channel names the channel to install and is
	// nil when the caller leaves the choice to the device; sha256 is the
	// package digest the caller resolved, and the handler answers
	// ErrDeviceRejected when it does not match the package the device resolves.
	UpdateFirmware func(ctx context.Context, channel *rpcapi.FirmwareChannelName, sha256 *string) error
	// FactoryReset erases device-local state. keepNetwork retains saved Wi-Fi
	// and cellular configuration so the device can reconnect without being
	// re-provisioned.
	//
	// Like Reboot, the handler must return promptly and only then perform the
	// reset: the Server's acknowledgement is written from this handler's
	// return, so a handler that erases connectivity or blocks before returning
	// leaves the caller without the response the method promises. Schedule the
	// reset on a timer or another goroutine and return nil.
	FactoryReset func(ctx context.Context, keepNetwork bool) error
	// SetRunWorkspace switches the Workspace the device runs to
	// request.WorkspaceName, already validated as non-empty; the Server has
	// resolved any workflow target to this one name. Kickoff is nil when the
	// caller leaves it at its default of false.
	//
	// The acknowledgement only means the device accepted the request. Return
	// promptly, then switch through server.run.workspace.reload-with-options;
	// the Server reports the committed Workspace, not this answer.
	SetRunWorkspace func(ctx context.Context, request rpcapi.ClientRunWorkspaceSetRequest) error
}

// supportedTools is derived from installed procedure handlers.
func (h *DeviceControlHandlers) supportedTools() []rpcpb.ClientTool {
	if h == nil {
		return nil
	}
	installed := []struct {
		tool    rpcpb.ClientTool
		present bool
	}{
		{rpcpb.ClientTool_CLIENT_TOOL_DEVICE_STATUS_GET, h.Status != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY, h.PlaySound != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FIND, h.Find != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_DEVICE_REBOOT, h.Reboot != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FACTORY_RESET, h.FactoryReset != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_RUN_WORKSPACE_SET, h.SetRunWorkspace != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_FIRMWARE_UPDATE, h.UpdateFirmware != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_LIST, h.SavedWifi != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_FORGET, h.ForgetWifi != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_WIFI_SCAN, h.ScanWifi != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_WIFI_CONNECT, h.ConnectWifi != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_GET, h.AudioPlayer.Get != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_GET, h.AudioPlayer.PlaylistGet != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_SET, h.AudioPlayer.PlaylistSet != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_APPEND, h.AudioPlayer.PlaylistAppend != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAY, h.AudioPlayer.Play != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_STOP, h.AudioPlayer.Stop != nil},
		{rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_MODE_SET, h.AudioPlayer.ModeSet != nil},
	}
	var tools []rpcpb.ClientTool
	for _, entry := range installed {
		if entry.present {
			tools = append(tools, entry.tool)
		}
	}
	return tools
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
	case rpcapi.RPCMethodClientMhsV0Read:
		if handlers.ReadMhsStates == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		if req.Params == nil {
			return rpcInvalidParams(req.Id), nil
		}
		params, err := req.Params.AsClientMhsV0ReadRequest()
		if err != nil || rpcapi.ValidateMhsRefs(params.GetStates()) != nil {
			return rpcInvalidParams(req.Id), nil
		}
		c.peer.observeClientRPC(req.Method)
		result, err := handlers.ReadMhsStates(ctx, params)
		if err != nil {
			return deviceControlError(req.Id, err), nil
		}
		if err := rpcapi.ValidateMhsStates(result.GetStates()); err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromClientMhsV0ReadResponse)
	case rpcapi.RPCMethodClientMhsV0Write:
		if handlers.WriteMhsStates == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		if req.Params == nil {
			return rpcInvalidParams(req.Id), nil
		}
		params, err := req.Params.AsClientMhsV0WriteRequest()
		if err != nil || rpcapi.ValidateMhsStates(params.GetStates()) != nil {
			return rpcInvalidParams(req.Id), nil
		}
		c.peer.observeClientRPC(req.Method)
		result, err := handlers.WriteMhsStates(ctx, params)
		if err != nil {
			return deviceControlError(req.Id, err), nil
		}
		if err := rpcapi.ValidateMhsStates(result.GetStates()); err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromClientMhsV0WriteResponse)

	default:
		return deviceControlUnsupported(req.Id, req.Method), nil
	}
}


func (c *rpcClient) handleDeviceTool(ctx context.Context, tool rpcpb.ClientTool, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	handlers := c.peer.deviceControlHandlers()
	if handlers == nil {
		return deviceControlUnsupported(req.Id, req.Method), nil
	}
	switch tool {
	case rpcpb.ClientTool_CLIENT_TOOL_DEVICE_STATUS_GET:
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

	case rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY:
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
	case rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FIND:
		params := rpcapi.ClientDeviceFindRequest{}
		if req.Params != nil {
			decoded, err := req.Params.AsClientDeviceFindRequest()
			if err != nil || (decoded.DurationMs != nil && *decoded.DurationMs < 0) {
				return rpcInvalidParams(req.Id), nil
			}
			params = decoded
		}
		if handlers.Find == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		if err := handlers.Find(ctx, params.DurationMs); err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientDeviceFindResponse{}, (*rpcapi.RPCPayload).FromClientDeviceFindResponse)
	case rpcpb.ClientTool_CLIENT_TOOL_DEVICE_REBOOT:
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

	case rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FACTORY_RESET:
		params := rpcapi.ClientDeviceFactoryResetRequest{}
		if req.Params != nil {
			decoded, err := req.Params.AsClientDeviceFactoryResetRequest()
			if err != nil {
				return rpcInvalidParams(req.Id), nil
			}
			params = decoded
		}
		if handlers.FactoryReset == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		keepNetwork := params.KeepNetwork != nil && *params.KeepNetwork
		if err := handlers.FactoryReset(ctx, keepNetwork); err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientDeviceFactoryResetResponse{}, (*rpcapi.RPCPayload).FromClientDeviceFactoryResetResponse)
	case rpcpb.ClientTool_CLIENT_TOOL_RUN_WORKSPACE_SET:
		if req.Params == nil {
			return rpcInvalidParams(req.Id), nil
		}
		params, err := req.Params.AsClientRunWorkspaceSetRequest()
		if err != nil || !params.Valid() {
			return rpcInvalidParams(req.Id), nil
		}
		if handlers.SetRunWorkspace == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		if err := handlers.SetRunWorkspace(ctx, params); err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientRunWorkspaceSetResponse{}, (*rpcapi.RPCPayload).FromClientRunWorkspaceSetResponse)
	case rpcpb.ClientTool_CLIENT_TOOL_FIRMWARE_UPDATE:
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

	case rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_LIST:
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
	case rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_FORGET:
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
	case rpcpb.ClientTool_CLIENT_TOOL_WIFI_SCAN:
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
	case rpcpb.ClientTool_CLIENT_TOOL_WIFI_CONNECT:
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
