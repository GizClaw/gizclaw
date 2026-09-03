package gizclaw

import (
	"context"
	"errors"
	"fmt"
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
	deviceControlTimeout   = 5 * time.Second
	deviceWifiScanTimeout  = 8 * time.Second
	minWifiScanTimeout     = time.Second
	maxWifiScanTimeout     = 15 * time.Second
	maxDeviceSoundBytes    = 32
	maxDeviceSSIDBytes     = 32
	minWifiPassphraseBytes = 8
	maxWifiPassphraseBytes = 63

	// Bounds a device may not exceed in a client.wifi.scan answer. They match
	// api/proto/rpc/nanopb.options, which only constrains the C SDK; a device
	// built on any other SDK can answer with more, so the Server enforces them
	// again before the values reach the Public HTTP contract.
	maxWifiScanResults    = 32
	maxWifiScanBSSIDBytes = 17
	maxWifiSecurityBytes  = 5

	deviceOfflineCode         = "DEVICE_OFFLINE"
	deviceTimeoutCode         = "DEVICE_TIMEOUT"
	deviceRejectedCode        = "DEVICE_REJECTED"
	deviceUnsupportedCode     = "DEVICE_UNSUPPORTED"
	deviceErrorCode           = "DEVICE_ERROR"
	wifiNetworkNotFoundKey    = "WIFI_NETWORK_NOT_FOUND"
	deviceResourceNotFoundKey = "DEVICE_RESOURCE_NOT_FOUND"
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

	mu            sync.Mutex
	transitioning map[giznet.PublicKey]giznet.Conn
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

// transitionPending reports whether a connection that acknowledged an action
// requiring disconnect is still the owner's active connection. A replaced or
// removed connection clears the marker.
func (c *deviceController) transitionPending(owner giznet.PublicKey) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	conn, ok := c.transitioning[owner]
	if !ok {
		return false
	}
	current, active := c.manager.Peer(owner)
	if active && current == conn {
		return true
	}
	delete(c.transitioning, owner)
	return false
}

// markTransitioning records conn as the connection that acknowledged an action
// requiring disconnect. Callers hold the owner command lock so a queued command
// cannot slip through between the acknowledgement and the marker.
func (c *deviceController) markTransitioning(owner giznet.PublicKey, conn giznet.Conn) {
	if conn == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.transitioning == nil {
		c.transitioning = make(map[giznet.PublicKey]giznet.Conn)
	}
	c.transitioning[owner] = conn
}

// deviceControlOptions tunes one forwarded control command.
type deviceControlOptions struct {
	// markTransition records the connection that answered the command as
	// transitioning before the owner command lock is released.
	markTransition bool
	timeout        time.Duration
	// notFoundCode is the Public HTTP error code for a NOT_FOUND answer from
	// this route. Routes that name a specific resource set it; the rest fall
	// back to the generic code rather than borrowing another route's.
	notFoundCode string
}

// callDeviceControl serializes one control RPC for owner and maps transport,
// timeout, and device RPC errors onto the Public HTTP error contract. after
// runs on success while the owner command lock is still held, so response
// write-back cannot interleave with a later command for the same owner.
func callDeviceControl[T any](ctx context.Context, c *deviceController, owner giznet.PublicKey, opts deviceControlOptions, call func(context.Context, *rpcClient, net.Conn) (*T, error), after func(context.Context, *T) error) (*T, *deviceControlError) {
	if c == nil || c.manager == nil {
		return nil, &deviceControlError{Status: http.StatusInternalServerError, Code: publicHTTPInternalErrorCode, Message: http.StatusText(http.StatusInternalServerError)}
	}
	release, err := c.locks.Acquire(ctx, owner)
	if err != nil {
		return nil, &deviceControlError{Status: http.StatusInternalServerError, Code: publicHTTPInternalErrorCode, Message: http.StatusText(http.StatusInternalServerError)}
	}
	defer release()
	if c.transitionPending(owner) {
		return nil, deviceOfflineError()
	}
	// Resolve the active connection once and dial the RPC stream on that
	// same connection, so the reboot marker always names the connection that
	// actually answered even if the device reconnects mid-command.
	target, ok := c.manager.Peer(owner)
	if !ok || target == nil {
		return nil, deviceOfflineError()
	}
	timeout := opts.timeout
	if timeout <= 0 {
		timeout = c.controlTimeout()
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stream, err := target.Dial(ServicePeerRPC)
	if err != nil {
		return nil, mapDeviceControlError(fmt.Errorf("dial peer rpc: %w", err), callCtx, opts.notFoundCode)
	}
	defer func() { _ = stream.Close() }()
	result, err := call(callCtx, &rpcClient{}, stream)
	if err != nil {
		return nil, mapDeviceControlError(err, callCtx, opts.notFoundCode)
	}
	if opts.markTransition {
		c.markTransitioning(owner, target)
	}
	if after != nil {
		if err := after(ctx, result); err != nil {
			return nil, &deviceControlError{Status: http.StatusInternalServerError, Code: publicHTTPInternalErrorCode, Message: http.StatusText(http.StatusInternalServerError)}
		}
	}
	return result, nil
}

// mapDeviceControlError projects one control failure onto the Public HTTP
// error contract. The HTTP status comes from the canonical status code table,
// so a code this function does not name specifically still reaches the client
// as itself instead of a bad gateway. notFoundCode names the resource a
// NOT_FOUND answer refers to on the calling route.
func mapDeviceControlError(err error, ctx context.Context, notFoundCode string) *deviceControlError {
	switch {
	case errors.Is(err, ErrDeviceOffline), isPeerDisconnectedError(err):
		return deviceOfflineError()
	}
	var rpcErr rpcapi.Error
	if errors.As(err, &rpcErr) {
		switch rpcErr.Code {
		case rpcapi.StatusCodeInvalidArgument, rpcapi.StatusCodeOutOfRange:
			return &deviceControlError{Status: http.StatusBadRequest, Code: deviceRejectedCode, Message: "device rejected the request parameters"}
		case rpcapi.StatusCodeUnimplemented:
			return &deviceControlError{Status: http.StatusNotImplemented, Code: deviceUnsupportedCode, Message: "device does not support this command"}
		case rpcapi.StatusCodeNotFound:
			code := notFoundCode
			if code == "" {
				code = deviceResourceNotFoundKey
			}
			return &deviceControlError{Status: http.StatusNotFound, Code: code, Message: "device has no matching resource"}
		case rpcapi.StatusCodeDeadlineExceeded:
			return &deviceControlError{Status: http.StatusGatewayTimeout, Code: deviceTimeoutCode, Message: "device did not respond in time"}
		case rpcapi.StatusCodeUnavailable:
			return deviceOfflineError()
		default:
			// A code with no projection stays a bad gateway: the fault is the
			// device's, and its own permission or state model is not the
			// Public HTTP contract's. Redacting it also keeps device detail
			// out of the response.
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
	var status apitypes.PeerStatus
	_, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientDeviceVolumeSetResponse, error) {
		return client.SetDeviceVolume(ctx, conn, "client.device.volume.set", params)
	}, func(ctx context.Context, result *rpcapi.ClientDeviceVolumeSetResponse) error {
		stored, err := s.DeviceControl.applyReportedStatus(ctx, owner, result.Value)
		status = stored
		return err
	})
	if controlErr != nil {
		return setDeviceVolumeError(controlErr), nil
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
	}, nil)
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
	_, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{markTransition: true}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientDeviceRebootResponse, error) {
		return client.RebootDevice(ctx, conn, "client.device.reboot", params)
	}, nil)
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
	}, nil)
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

func (s *peerHTTP) ScanDeviceWifi(ctx context.Context, request peerhttp.ScanDeviceWifiRequestObject) (peerhttp.ScanDeviceWifiResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ScanDeviceWifi401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	timeout := deviceWifiScanTimeout
	if request.Body != nil && request.Body.TimeoutMs != nil {
		timeout = wifiScanTimeout(*request.Body.TimeoutMs)
	}
	timeoutMs := timeout.Milliseconds()
	params := rpcapi.ClientWifiScanRequest{TimeoutMs: &timeoutMs}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{timeout: timeout}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientWifiScanResponse, error) {
		return client.ScanWifi(ctx, conn, "client.wifi.scan", params)
	}, nil)
	if controlErr != nil {
		return scanDeviceWifiError(controlErr), nil
	}
	networks, invalid := wifiScanResults(result.Networks)
	if invalid != nil {
		return scanDeviceWifiError(invalid), nil
	}
	return peerhttp.ScanDeviceWifi200JSONResponse{Networks: networks}, nil
}

// wifiScanTimeout clamps a caller-supplied scan duration in milliseconds to
// the supported range.
//
// The clamp runs on the integer before the conversion: milliseconds arrive as
// an unvalidated int64, and multiplying one large enough by time.Millisecond
// would wrap time.Duration negative and select the minimum where the caller
// asked for more than the maximum.
func wifiScanTimeout(milliseconds int64) time.Duration {
	milliseconds = min(max(milliseconds, minWifiScanTimeout.Milliseconds()), maxWifiScanTimeout.Milliseconds())
	return time.Duration(milliseconds) * time.Millisecond
}

// wifiScanResults projects an untrusted device answer onto the Public HTTP
// contract, rejecting the whole answer when it exceeds the declared bounds.
//
// The device is the sole source of these values and only the C SDK is bounded
// by nanopb, so an unbounded answer would otherwise be reflected verbatim to
// the API key holder. The failure is reported as a plain device error and
// never quotes the offending value.
func wifiScanResults(results []rpcapi.WifiScanResult) ([]peerhttp.DeviceWifiScanResult, *deviceControlError) {
	if len(results) > maxWifiScanResults {
		return nil, invalidDeviceScanError()
	}
	networks := make([]peerhttp.DeviceWifiScanResult, len(results))
	for i := range results {
		network := results[i]
		if !validDeviceScanString(&network.Ssid, maxDeviceSSIDBytes, true) ||
			!validDeviceScanString(network.Bssid, maxWifiScanBSSIDBytes, false) ||
			!validDeviceScanString(network.Security, maxWifiSecurityBytes, false) {
			return nil, invalidDeviceScanError()
		}
		networks[i] = peerhttp.DeviceWifiScanResult{
			Ssid: network.Ssid, Bssid: network.Bssid, RssiDbm: network.RssiDbm,
			FrequencyMhz: network.FrequencyMhz, Security: network.Security,
		}
	}
	return networks, nil
}

// validDeviceScanString reports whether an optional device-provided string
// fits the contract. An absent value is valid unless the field is required.
func validDeviceScanString(value *string, maxBytes int, required bool) bool {
	if value == nil {
		return !required
	}
	if required && *value == "" {
		return false
	}
	return len(*value) <= maxBytes && utf8.ValidString(*value)
}

func invalidDeviceScanError() *deviceControlError {
	return &deviceControlError{Status: http.StatusBadGateway, Code: deviceErrorCode, Message: "device returned an invalid scan result"}
}

func scanDeviceWifiError(e *deviceControlError) peerhttp.ScanDeviceWifiResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusBadRequest:
		return peerhttp.ScanDeviceWifi400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusConflict:
		return peerhttp.ScanDeviceWifi409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.ScanDeviceWifi501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.ScanDeviceWifi504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.ScanDeviceWifi500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.ScanDeviceWifi502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func (s *peerHTTP) ConnectDeviceWifi(ctx context.Context, request peerhttp.ConnectDeviceWifiRequestObject) (peerhttp.ConnectDeviceWifiResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ConnectDeviceWifi401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if request.Body == nil {
		return peerhttp.ConnectDeviceWifi400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "request body is required"))}, nil
	}
	if e := validateDeviceString("ssid", request.Body.Ssid, maxDeviceSSIDBytes); e != nil {
		return connectDeviceWifiError(e), nil
	}
	if request.Body.Passphrase != nil {
		length := len(*request.Body.Passphrase)
		if length < minWifiPassphraseBytes || length > maxWifiPassphraseBytes {
			return peerhttp.ConnectDeviceWifi400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "passphrase must be between 8 and 63 bytes"))}, nil
		}
	}
	params := rpcapi.ClientWifiConnectRequest{Ssid: request.Body.Ssid, Passphrase: request.Body.Passphrase}
	_, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{markTransition: true}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientWifiConnectResponse, error) {
		return client.ConnectWifi(ctx, conn, "client.wifi.connect", params)
	}, nil)
	if controlErr != nil {
		return connectDeviceWifiError(controlErr), nil
	}
	return peerhttp.ConnectDeviceWifi202Response{}, nil
}

func connectDeviceWifiError(e *deviceControlError) peerhttp.ConnectDeviceWifiResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusBadRequest:
		return peerhttp.ConnectDeviceWifi400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusConflict:
		return peerhttp.ConnectDeviceWifi409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.ConnectDeviceWifi501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.ConnectDeviceWifi504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.ConnectDeviceWifi500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.ConnectDeviceWifi502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func (s *peerHTTP) ListDeviceSavedWifi(ctx context.Context, _ peerhttp.ListDeviceSavedWifiRequestObject) (peerhttp.ListDeviceSavedWifiResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ListDeviceSavedWifi401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientWifiSavedListResponse, error) {
		return client.ListSavedWifi(ctx, conn, "client.wifi.saved.list")
	}, nil)
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
	_, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{notFoundCode: wifiNetworkNotFoundKey}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientWifiSavedForgetResponse, error) {
		return client.ForgetSavedWifi(ctx, conn, "client.wifi.saved.forget", params)
	}, nil)
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
