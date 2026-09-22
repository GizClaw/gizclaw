package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
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

	// Bounds a device may not exceed in a wifi.scan tool answer. They match
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
// owner's active device connection through MHS and predefined tool RPCs.
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
	if rpcErr, ok := errors.AsType[rpcapi.Error](err); ok {
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

// firmwareSha256Pattern mirrors the sha256 pattern of the Public HTTP schema so
// a malformed digest fails before it reaches the device.
var firmwareSha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validateDeviceString(field, value string, maxBytes int) *deviceControlError {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return &deviceControlError{Status: http.StatusBadRequest, Code: publicHTTPInvalidRequestCode, Message: field + " must be non-empty valid UTF-8 of at most 32 bytes"}
	}
	return nil
}

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
