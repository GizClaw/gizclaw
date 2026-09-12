package gizcli

import (
	"context"
	"errors"
	"fmt"
	"regexp"

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
	AudioPlayer AudioPlayerHandlers
	Status      func(context.Context) (rpcapi.PeerStatus, error)
	SetVolume   func(ctx context.Context, level int64, muted bool) (rpcapi.PeerStatus, error)
	PlaySound   func(ctx context.Context, sound string, durationMs *int64) error
	// Find rings the device's built-in find-me sound. durationMs is nil when
	// the caller leaves the ring time to the device.
	Find        func(ctx context.Context, durationMs *int64) error
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
	// GetSettings reports every option this device supports. An option the
	// device has no hardware for stays absent rather than carrying a
	// placeholder, which is how a caller tells "unsupported" from "off".
	GetSettings func(context.Context) (rpcapi.DeviceSettings, error)
	// SetSettings applies only the members present in the patch and answers
	// with the device's full settings afterwards, so the caller sees what was
	// accepted. An unsupported member is ignored rather than rejected.
	SetSettings func(ctx context.Context, patch rpcapi.DeviceSettings) (rpcapi.DeviceSettings, error)
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
}

// supportedDeviceMethods lists the client.* methods these handlers answer. It
// is derived from the installed handlers rather than a hand-kept list, so the
// answer to client.rpc.methods.get cannot drift from what the device accepts.
func (h *DeviceControlHandlers) supportedDeviceMethods() []string {
	if h == nil {
		return []string{string(rpcapi.RPCMethodClientRPCMethodsGet)}
	}
	installed := []struct {
		method  rpcapi.RPCMethod
		present bool
	}{
		{rpcapi.RPCMethodClientDeviceStatusGet, h.Status != nil},
		{rpcapi.RPCMethodClientDeviceVolumeSet, h.SetVolume != nil},
		{rpcapi.RPCMethodClientDeviceSoundPlay, h.PlaySound != nil},
		{rpcapi.RPCMethodClientDeviceFind, h.Find != nil},
		{rpcapi.RPCMethodClientDeviceReboot, h.Reboot != nil},
		{rpcapi.RPCMethodClientDeviceSettingsGet, h.GetSettings != nil},
		{rpcapi.RPCMethodClientDeviceSettingsSet, h.SetSettings != nil},
		{rpcapi.RPCMethodClientDeviceFactoryReset, h.FactoryReset != nil},
		{rpcapi.RPCMethodClientFirmwareUpdate, h.UpdateFirmware != nil},
		{rpcapi.RPCMethodClientWifiStatusGet, h.WifiStatus != nil},
		{rpcapi.RPCMethodClientWifiSavedList, h.SavedWifi != nil},
		{rpcapi.RPCMethodClientWifiSavedForget, h.ForgetWifi != nil},
		{rpcapi.RPCMethodClientWifiScan, h.ScanWifi != nil},
		{rpcapi.RPCMethodClientWifiConnect, h.ConnectWifi != nil},
		{rpcapi.RPCMethodClientDeviceAudioPlayerGet, h.AudioPlayer.Get != nil},
		{rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistGet, h.AudioPlayer.PlaylistGet != nil},
		{rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistSet, h.AudioPlayer.PlaylistSet != nil},
		{rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistAppend, h.AudioPlayer.PlaylistAppend != nil},
		{rpcapi.RPCMethodClientDeviceAudioPlayerPlay, h.AudioPlayer.Play != nil},
		{rpcapi.RPCMethodClientDeviceAudioPlayerStop, h.AudioPlayer.Stop != nil},
		{rpcapi.RPCMethodClientDeviceAudioPlayerModeSet, h.AudioPlayer.ModeSet != nil},
	}
	methods := make([]string, 0, len(installed)+1)
	for _, entry := range installed {
		if entry.present {
			methods = append(methods, string(entry.method))
		}
	}
	return append(methods, string(rpcapi.RPCMethodClientRPCMethodsGet))
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
	// A device with no device control handlers still implements the methods the
	// Client answers itself, so the capability list is answered before the
	// missing-handlers check rather than reported as unsupported.
	if req.Method == rpcapi.RPCMethodClientRPCMethodsGet {
		c.peer.observeClientRPC(req.Method)
		// client.info.get and client.identifiers.get are answered by the Client
		// itself, and client.social.ping only once a handler is installed; the
		// device control methods come from the installed handlers.
		methods := []string{string(rpcapi.RPCMethodClientInfoGet), string(rpcapi.RPCMethodClientIdentifiersGet)}
		if c.peer.socialPingHandler() != nil {
			methods = append(methods, string(rpcapi.RPCMethodClientSocialPing))
		}
		methods = append(methods, handlers.supportedDeviceMethods()...)
		return newRPCResultResponse(req.Id, rpcapi.ClientRPCMethodsGetResponse{Methods: methods}, (*rpcapi.RPCPayload).FromClientRPCMethodsGetResponse)
	}
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
	case rpcapi.RPCMethodClientDeviceFind:
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
	case rpcapi.RPCMethodClientDeviceSettingsGet:
		if handlers.GetSettings == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		c.peer.observeClientRPC(req.Method)
		settings, err := handlers.GetSettings(ctx)
		if err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientDeviceSettingsGetResponse{Value: settings}, (*rpcapi.RPCPayload).FromClientDeviceSettingsGetResponse)
	case rpcapi.RPCMethodClientDeviceSettingsSet:
		params := rpcapi.ClientDeviceSettingsSetRequest{}
		if req.Params != nil {
			decoded, err := req.Params.AsClientDeviceSettingsSetRequest()
			if err != nil {
				return rpcInvalidParams(req.Id), nil
			}
			params = decoded
		}
		if handlers.SetSettings == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		// Reject the whole patch before applying any of it, so a bad member
		// cannot leave the device half-configured.
		if !validDeviceSettingsPatch(params.Value) {
			return rpcInvalidParams(req.Id), nil
		}
		c.peer.observeClientRPC(req.Method)
		settings, err := handlers.SetSettings(ctx, params.Value)
		if err != nil {
			return deviceControlError(req.Id, err), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientDeviceSettingsSetResponse{Value: settings}, (*rpcapi.RPCPayload).FromClientDeviceSettingsSetResponse)
	case rpcapi.RPCMethodClientDeviceFactoryReset:
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

// deviceSettingsLocalePattern checks BCP 47 well-formedness at the subtag level:
// a 2-8 letter primary subtag followed by hyphen-separated 1-8 character
// alphanumeric subtags, such as "zh-CN", "zh-Hant-TW" or "es-419". It rejects
// POSIX forms like "zh_CN" and free text; whether the device offers that
// language is still the device's decision.
var deviceSettingsLocalePattern = regexp.MustCompile(`^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$`)

// validDeviceSettingsPatch mirrors the DeviceSettings ranges documented in
// api/proto/rpc/payload/system.proto. An unknown enum value is rejected, while
// an absent member simply leaves that option unchanged.
func validDeviceSettingsPatch(patch rpcapi.DeviceSettings) bool {
	percent := func(value *int64) bool { return value == nil || (*value >= 0 && *value <= 100) }
	if !percent(patch.ScreenBrightness) || !percent(patch.LedBrightness) {
		return false
	}
	if patch.ScreenOffTimeoutMs != nil && *patch.ScreenOffTimeoutMs < 0 {
		return false
	}
	if patch.Locale != nil && (len(*patch.Locale) > 35 || !deviceSettingsLocalePattern.MatchString(*patch.Locale)) {
		return false
	}
	if patch.DefaultInteractionMode != nil && !patch.DefaultInteractionMode.Valid() {
		return false
	}
	if patch.KeyFeedback != nil && !patch.KeyFeedback.Valid() {
		return false
	}
	return true
}
