package gizclaw

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/internal/keyedlock"
)

const (
	deviceControlTimeout = 5 * time.Second
	maxDeviceSoundBytes  = 32
	maxDeviceSSIDBytes   = 32

	deviceOfflineCode      = "DEVICE_OFFLINE"
	deviceTimeoutCode      = "DEVICE_TIMEOUT"
	deviceRejectedCode     = "DEVICE_REJECTED"
	deviceUnsupportedCode  = "DEVICE_UNSUPPORTED"
	deviceErrorCode        = "DEVICE_ERROR"
	wifiNetworkNotFoundKey = "WIFI_NETWORK_NOT_FOUND"
)

// deviceController forwards Public HTTP control commands to the API key
// owner's active device connection as client.device.* / client.wifi.* RPCs.
//
// Commands for one owner are serialized in arrival order and never merged or
// replayed. After a device acknowledges a reboot, later commands answer
// DEVICE_OFFLINE until a different connection replaces the acknowledged one.
type deviceController struct {
	manager *Manager
	status  peertelemetry.PeerStatusStore
	timeout time.Duration
	now     func() time.Time

	locks keyedlock.Locker[giznet.PublicKey]

	mu        sync.Mutex
	rebooting map[giznet.PublicKey]giznet.Conn
}

func newDeviceController(manager *Manager, status peertelemetry.PeerStatusStore) *deviceController {
	return &deviceController{manager: manager, status: status, timeout: deviceControlTimeout, now: time.Now}
}

// deviceControlError is the redacted HTTP projection of one control failure.
type deviceControlError struct {
	Status  int
	Code    string
	Message string
}

func (e *deviceControlError) response() apitypes.ErrorResponse {
	return apiError(e.Code, e.Message)
}

func deviceOfflineError() *deviceControlError {
	return &deviceControlError{Status: http.StatusConflict, Code: deviceOfflineCode, Message: "device is offline"}
}

func (c *deviceController) controlTimeout() time.Duration {
	if c == nil || c.timeout <= 0 {
		return deviceControlTimeout
	}
	return c.timeout
}

func (c *deviceController) clock() time.Time {
	if c == nil || c.now == nil {
		return time.Now()
	}
	return c.now()
}

// rebootPending reports whether the connection that acknowledged a reboot is
// still the owner's active connection. A replaced or removed connection clears
// the marker.
func (c *deviceController) rebootPending(owner giznet.PublicKey) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	conn, ok := c.rebooting[owner]
	if !ok {
		return false
	}
	current, active := c.manager.Peer(owner)
	if active && current == conn {
		return true
	}
	delete(c.rebooting, owner)
	return false
}

// markRebooting records conn as the connection that acknowledged a reboot.
// Callers hold the owner command lock so a queued command cannot slip through
// between the acknowledgement and the marker.
func (c *deviceController) markRebooting(owner giznet.PublicKey, conn giznet.Conn) {
	if conn == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rebooting == nil {
		c.rebooting = make(map[giznet.PublicKey]giznet.Conn)
	}
	c.rebooting[owner] = conn
}

// deviceControlOptions tunes one forwarded control command.
type deviceControlOptions struct {
	// markReboot records the connection that answered the command as
	// rebooting before the owner command lock is released.
	markReboot bool
}

// callDeviceControl serializes one control RPC for owner and maps transport,
// timeout, and device RPC errors onto the Public HTTP error contract.
func callDeviceControl[T any](ctx context.Context, c *deviceController, owner giznet.PublicKey, opts deviceControlOptions, call func(context.Context, *rpcClient, net.Conn) (*T, error)) (*T, *deviceControlError) {
	if c == nil || c.manager == nil {
		return nil, &deviceControlError{Status: http.StatusInternalServerError, Code: publicHTTPInternalErrorCode, Message: http.StatusText(http.StatusInternalServerError)}
	}
	release, err := c.locks.Acquire(ctx, owner)
	if err != nil {
		return nil, &deviceControlError{Status: http.StatusInternalServerError, Code: publicHTTPInternalErrorCode, Message: http.StatusText(http.StatusInternalServerError)}
	}
	defer release()
	if c.rebootPending(owner) {
		return nil, deviceOfflineError()
	}
	// Capture the connection the command is about to use; a reconnect during
	// the call must not be mistaken for the acknowledging device.
	target, _ := c.manager.Peer(owner)
	callCtx, cancel := context.WithTimeout(ctx, c.controlTimeout())
	defer cancel()
	result, err := callPeerRPC(c.manager, callCtx, owner, func(client *rpcClient, conn net.Conn) (*T, error) {
		return call(callCtx, client, conn)
	})
	if err != nil {
		return nil, mapDeviceControlError(err, callCtx)
	}
	if opts.markReboot {
		c.markRebooting(owner, target)
	}
	return result, nil
}

func mapDeviceControlError(err error, ctx context.Context) *deviceControlError {
	switch {
	case errors.Is(err, ErrDeviceOffline), isPeerDisconnectedError(err):
		return deviceOfflineError()
	}
	var rpcErr rpcapi.Error
	if errors.As(err, &rpcErr) {
		switch rpcErr.Code {
		case rpcapi.RPCErrorCodeInvalidParams:
			return &deviceControlError{Status: http.StatusBadRequest, Code: deviceRejectedCode, Message: "device rejected the request parameters"}
		case rpcapi.RPCErrorCodeMethodNotFound:
			return &deviceControlError{Status: http.StatusNotImplemented, Code: deviceUnsupportedCode, Message: "device does not support this command"}
		case rpcapi.RPCErrorCodeNotFound:
			return &deviceControlError{Status: http.StatusNotFound, Code: wifiNetworkNotFoundKey, Message: "device has no matching resource"}
		default:
			return &deviceControlError{Status: http.StatusBadGateway, Code: deviceErrorCode, Message: "device returned an error"}
		}
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return &deviceControlError{Status: http.StatusGatewayTimeout, Code: deviceTimeoutCode, Message: "device did not respond in time"}
	}
	return &deviceControlError{Status: http.StatusBadGateway, Code: deviceErrorCode, Message: "device returned an error"}
}

// applyReportedStatus stores the PeerStatus the device reported in a control
// response under the same per-owner lock as telemetry status sync.
func (c *deviceController) applyReportedStatus(ctx context.Context, owner giznet.PublicKey, reported rpcapi.PeerStatus) (apitypes.PeerStatus, error) {
	status, err := convertRPCType[apitypes.PeerStatus](reported)
	if err != nil {
		return apitypes.PeerStatus{}, err
	}
	if c.status == nil {
		return status, nil
	}
	mu := c.manager.telemetryStatusLock(owner)
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	return peertelemetry.StatusSync{Store: c.status}.ApplyDeviceStatus(ctx, owner, status, c.clock())
}

func validateDeviceString(field, value string, maxBytes int) *deviceControlError {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return &deviceControlError{Status: http.StatusBadRequest, Code: publicHTTPInvalidRequestCode, Message: field + " must be non-empty valid UTF-8 of at most 32 bytes"}
	}
	return nil
}

func (s *peerHTTP) SetDeviceVolume(ctx context.Context, request peerhttp.SetDeviceVolumeRequestObject) (peerhttp.SetDeviceVolumeResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.SetDeviceVolume401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if request.Body == nil {
		return peerhttp.SetDeviceVolume400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "request body is required"))}, nil
	}
	if request.Body.Level < 0 || request.Body.Level > 100 {
		return peerhttp.SetDeviceVolume400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "level must be between 0 and 100"))}, nil
	}
	params := rpcapi.ClientDeviceVolumeSetRequest{Level: int64(request.Body.Level), Muted: request.Body.Muted}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientDeviceVolumeSetResponse, error) {
		return client.SetDeviceVolume(ctx, conn, "client.device.volume.set", params)
	})
	if controlErr != nil {
		return setDeviceVolumeError(controlErr), nil
	}
	status, err := s.DeviceControl.applyReportedStatus(ctx, owner, result.Value)
	if err != nil {
		return peerhttp.SetDeviceVolume500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.SetDeviceVolume200JSONResponse{Status: status}, nil
}

func setDeviceVolumeError(e *deviceControlError) peerhttp.SetDeviceVolumeResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusBadRequest:
		return peerhttp.SetDeviceVolume400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusConflict:
		return peerhttp.SetDeviceVolume409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.SetDeviceVolume501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.SetDeviceVolume504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.SetDeviceVolume500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.SetDeviceVolume502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func (s *peerHTTP) PlayDeviceSound(ctx context.Context, request peerhttp.PlayDeviceSoundRequestObject) (peerhttp.PlayDeviceSoundResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.PlayDeviceSound401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if request.Body == nil {
		return peerhttp.PlayDeviceSound400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "request body is required"))}, nil
	}
	if e := validateDeviceString("sound", request.Body.Sound, maxDeviceSoundBytes); e != nil {
		return playDeviceSoundError(e), nil
	}
	if request.Body.DurationMs != nil && *request.Body.DurationMs < 0 {
		return peerhttp.PlayDeviceSound400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "duration_ms must not be negative"))}, nil
	}
	params := rpcapi.ClientDeviceSoundPlayRequest{Sound: request.Body.Sound, DurationMs: request.Body.DurationMs}
	_, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientDeviceSoundPlayResponse, error) {
		return client.PlayDeviceSound(ctx, conn, "client.device.sound.play", params)
	})
	if controlErr != nil {
		return playDeviceSoundError(controlErr), nil
	}
	return peerhttp.PlayDeviceSound204Response{}, nil
}

func playDeviceSoundError(e *deviceControlError) peerhttp.PlayDeviceSoundResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusBadRequest:
		return peerhttp.PlayDeviceSound400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusConflict:
		return peerhttp.PlayDeviceSound409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.PlayDeviceSound501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.PlayDeviceSound504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.PlayDeviceSound500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.PlayDeviceSound502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func (s *peerHTTP) RebootDevice(ctx context.Context, request peerhttp.RebootDeviceRequestObject) (peerhttp.RebootDeviceResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.RebootDevice401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	params := rpcapi.ClientDeviceRebootRequest{}
	if request.Body != nil {
		if request.Body.DelayMs != nil && *request.Body.DelayMs < 0 {
			return peerhttp.RebootDevice400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "delay_ms must not be negative"))}, nil
		}
		params.DelayMs = request.Body.DelayMs
	}
	_, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{markReboot: true}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientDeviceRebootResponse, error) {
		return client.RebootDevice(ctx, conn, "client.device.reboot", params)
	})
	if controlErr != nil {
		return rebootDeviceError(controlErr), nil
	}
	return peerhttp.RebootDevice204Response{}, nil
}

func rebootDeviceError(e *deviceControlError) peerhttp.RebootDeviceResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusBadRequest:
		return peerhttp.RebootDevice400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusConflict:
		return peerhttp.RebootDevice409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.RebootDevice501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.RebootDevice504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.RebootDevice500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.RebootDevice502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func (s *peerHTTP) GetDeviceWifi(ctx context.Context, _ peerhttp.GetDeviceWifiRequestObject) (peerhttp.GetDeviceWifiResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetDeviceWifi401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientWifiStatusGetResponse, error) {
		return client.GetWifiStatus(ctx, conn, "client.wifi.status.get")
	})
	if controlErr != nil {
		return getDeviceWifiError(controlErr), nil
	}
	value := result.Value
	return peerhttp.GetDeviceWifi200JSONResponse{Connected: value.Connected, Ssid: value.Ssid, RssiDbm: value.RssiDbm, Ip: value.Ip, Bssid: value.Bssid}, nil
}

func getDeviceWifiError(e *deviceControlError) peerhttp.GetDeviceWifiResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusConflict:
		return peerhttp.GetDeviceWifi409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.GetDeviceWifi501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.GetDeviceWifi504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.GetDeviceWifi500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.GetDeviceWifi502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func (s *peerHTTP) ListDeviceSavedWifi(ctx context.Context, _ peerhttp.ListDeviceSavedWifiRequestObject) (peerhttp.ListDeviceSavedWifiResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ListDeviceSavedWifi401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientWifiSavedListResponse, error) {
		return client.ListSavedWifi(ctx, conn, "client.wifi.saved.list")
	})
	if controlErr != nil {
		return listDeviceSavedWifiError(controlErr), nil
	}
	networks := make([]peerhttp.DeviceWifiSavedNetwork, len(result.Networks))
	for i := range result.Networks {
		networks[i] = peerhttp.DeviceWifiSavedNetwork{Ssid: result.Networks[i].Ssid}
	}
	return peerhttp.ListDeviceSavedWifi200JSONResponse{Networks: networks}, nil
}

func listDeviceSavedWifiError(e *deviceControlError) peerhttp.ListDeviceSavedWifiResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusConflict:
		return peerhttp.ListDeviceSavedWifi409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.ListDeviceSavedWifi501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.ListDeviceSavedWifi504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.ListDeviceSavedWifi500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.ListDeviceSavedWifi502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func (s *peerHTTP) ForgetDeviceSavedWifi(ctx context.Context, request peerhttp.ForgetDeviceSavedWifiRequestObject) (peerhttp.ForgetDeviceSavedWifiResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ForgetDeviceSavedWifi401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	ssid := strings.TrimSpace(request.Ssid)
	if e := validateDeviceString("ssid", ssid, maxDeviceSSIDBytes); e != nil {
		return forgetDeviceSavedWifiError(e), nil
	}
	params := rpcapi.ClientWifiSavedForgetRequest{Ssid: ssid}
	_, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientWifiSavedForgetResponse, error) {
		return client.ForgetSavedWifi(ctx, conn, "client.wifi.saved.forget", params)
	})
	if controlErr != nil {
		return forgetDeviceSavedWifiError(controlErr), nil
	}
	return peerhttp.ForgetDeviceSavedWifi204Response{}, nil
}

func forgetDeviceSavedWifiError(e *deviceControlError) peerhttp.ForgetDeviceSavedWifiResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusBadRequest:
		return peerhttp.ForgetDeviceSavedWifi400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusNotFound:
		return peerhttp.ForgetDeviceSavedWifi404JSONResponse(body)
	case http.StatusConflict:
		return peerhttp.ForgetDeviceSavedWifi409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.ForgetDeviceSavedWifi501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.ForgetDeviceSavedWifi504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.ForgetDeviceSavedWifi500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.ForgetDeviceSavedWifi502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}
