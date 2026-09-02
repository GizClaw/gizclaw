package gizclaw

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// fakeDeviceConn serves the device side of client.* RPCs over an in-memory
// pipe so control handlers exercise the real RPC framing and timeout paths.
type fakeDeviceConn struct {
	testGiznetConn
	dispatch func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error)
	calls    atomic.Int32
	methods  chan rpcapi.RPCMethod
	// onDial runs before the RPC stream is returned, letting tests interleave
	// a reconnect between connection lookup and RPC dispatch.
	onDial func()
}

func newFakeDeviceConn(dispatch func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error)) *fakeDeviceConn {
	return &fakeDeviceConn{dispatch: dispatch, methods: make(chan rpcapi.RPCMethod, 16)}
}

func (c *fakeDeviceConn) Dial(uint64) (net.Conn, error) {
	if c.onDial != nil {
		c.onDial()
	}
	client, server := net.Pipe()
	go func() {
		_ = handleRPC(server, func(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
			c.calls.Add(1)
			select {
			case c.methods <- req.Method:
			default:
			}
			return c.dispatch(ctx, req)
		})
	}()
	return client, nil
}

func deviceStatusResult(id string, status rpcapi.PeerStatus) (*rpcapi.RPCResponse, error) {
	return newRPCResultResponse(id, rpcapi.ClientDeviceVolumeSetResponse{Value: status}, (*rpcapi.RPCPayload).FromClientDeviceVolumeSetResponse)
}

func TestDeviceControlVolumeRoundTripWritesStatus(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	reportedAt := time.Date(2026, 9, 2, 9, 30, 0, 0, time.UTC)
	var lastRequest rpcapi.ClientDeviceVolumeSetRequest
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		switch req.Method {
		case rpcapi.RPCMethodClientDeviceVolumeSet:
			params, err := req.Params.AsClientDeviceVolumeSetRequest()
			if err != nil {
				return nil, err
			}
			lastRequest = params
			level := int(params.Level)
			return deviceStatusResult(req.Id, rpcapi.PeerStatus{Volume: &level, Muted: &params.Muted, BatteryPercent: new(58), ReportedAt: &reportedAt})
		default:
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: "unsupported"}.RPCResponse(), nil
		}
	})
	f.manager.SetPeerUp(f.owner, device)

	response := f.do(t, http.MethodPut, "/gizclaw/v1/device/volume", `{"level":35,"muted":true}`)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT volume status = %d body=%s", response.Code, response.Body.String())
	}
	result := decodeJSON[peerhttp.DeviceControlStatus](t, response)
	if result.Status.Volume == nil || *result.Status.Volume != 35 || result.Status.Muted == nil || !*result.Status.Muted || result.Status.BatteryPercent == nil || *result.Status.BatteryPercent != 58 {
		t.Fatalf("control status = %+v", result.Status)
	}
	if result.Status.ReportedAt == nil || !result.Status.ReportedAt.Equal(reportedAt) {
		t.Fatalf("reported_at = %v, want device time", result.Status.ReportedAt)
	}
	if lastRequest.Level != 35 || !lastRequest.Muted {
		t.Fatalf("device received %+v", lastRequest)
	}

	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/status", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", response.Code, response.Body.String())
	}
	status := decodeJSON[apitypes.PeerStatus](t, response)
	if status.Volume == nil || *status.Volume != 35 || status.Muted == nil || !*status.Muted || !status.ReportedAt.Equal(reportedAt) {
		t.Fatalf("stored status after control = %+v", status)
	}

	// Equal inputs are forwarded again, never merged or replayed from cache.
	if response := f.do(t, http.MethodPut, "/gizclaw/v1/device/volume", `{"level":35,"muted":true}`); response.Code != http.StatusOK {
		t.Fatalf("repeated PUT volume status = %d", response.Code)
	}
	if device.calls.Load() != 2 {
		t.Fatalf("device calls = %d, want 2", device.calls.Load())
	}

	for _, body := range []string{`{"level":101,"muted":false}`, `{"level":-1,"muted":false}`, ``} {
		response := f.do(t, http.MethodPut, "/gizclaw/v1/device/volume", body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("PUT volume %q status = %d body=%s", body, response.Code, response.Body.String())
		}
	}
	if device.calls.Load() != 2 {
		t.Fatalf("validation failures reached the device: calls = %d", device.calls.Load())
	}
}

func TestDeviceControlOfflineAndTimeout(t *testing.T) {
	f := newDeviceHTTPFixture(t)

	response := f.do(t, http.MethodPut, "/gizclaw/v1/device/volume", `{"level":10,"muted":false}`)
	if response.Code != http.StatusConflict || errorCode(t, response) != deviceOfflineCode {
		t.Fatalf("offline PUT status = %d body=%s", response.Code, response.Body.String())
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/gizclaw/v1/device/actions/play-sound"},
		{http.MethodPost, "/gizclaw/v1/device/actions/reboot"},
		{http.MethodGet, "/gizclaw/v1/device/wifi"},
		{http.MethodGet, "/gizclaw/v1/device/wifi/saved"},
		{http.MethodDelete, "/gizclaw/v1/device/wifi/saved/home"},
	} {
		body := ""
		if route.path == "/gizclaw/v1/device/actions/play-sound" {
			body = `{"sound":"chime"}`
		}
		response := f.do(t, route.method, route.path, body)
		if response.Code != http.StatusConflict || errorCode(t, response) != deviceOfflineCode {
			t.Fatalf("offline %s %s status = %d body=%s", route.method, route.path, response.Code, response.Body.String())
		}
	}

	f.control.timeout = 150 * time.Millisecond
	released := make(chan struct{})
	defer close(released)
	device := newFakeDeviceConn(func(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		select {
		case <-ctx.Done():
		case <-released:
		}
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "late"}.RPCResponse(), nil
	})
	f.manager.SetPeerUp(f.owner, device)
	started := time.Now()
	response = f.do(t, http.MethodPut, "/gizclaw/v1/device/volume", `{"level":10,"muted":false}`)
	if response.Code != http.StatusGatewayTimeout || errorCode(t, response) != deviceTimeoutCode {
		t.Fatalf("timeout PUT status = %d body=%s", response.Code, response.Body.String())
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("timeout took %v", elapsed)
	}
	status, err := f.manager.PeerRun.GetStatus(context.Background(), f.owner)
	if err != nil || status.Volume != nil {
		t.Fatalf("status after timeout = %+v, %v; want unchanged", status, err)
	}
}

func TestDeviceControlMapsDeviceErrors(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	var code rpcapi.RPCErrorCode
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		return rpcapi.Error{RequestID: req.Id, Code: code, Message: "device says no"}.RPCResponse(), nil
	})
	f.manager.SetPeerUp(f.owner, device)

	for _, tc := range []struct {
		code   rpcapi.RPCErrorCode
		status int
		public string
	}{
		{rpcapi.RPCErrorCodeInvalidParams, http.StatusBadRequest, deviceRejectedCode},
		{rpcapi.RPCErrorCodeMethodNotFound, http.StatusNotImplemented, deviceUnsupportedCode},
		{rpcapi.RPCErrorCodeInternalError, http.StatusBadGateway, deviceErrorCode},
		{rpcapi.RPCErrorCodeForbidden, http.StatusBadGateway, deviceErrorCode},
	} {
		code = tc.code
		response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/play-sound", `{"sound":"chime","duration_ms":500}`)
		if response.Code != tc.status {
			t.Fatalf("code %d status = %d body=%s", tc.code, response.Code, response.Body.String())
		}
		body := decodeJSON[apitypes.ErrorResponse](t, response)
		if body.Error.Code != tc.public || body.Error.Message == "device says no" {
			t.Fatalf("code %d body = %+v, want redacted %s", tc.code, body, tc.public)
		}
	}

	code = rpcapi.RPCErrorCodeNotFound
	response := f.do(t, http.MethodDelete, "/gizclaw/v1/device/wifi/saved/unknown", "")
	if response.Code != http.StatusNotFound || errorCode(t, response) != wifiNetworkNotFoundKey {
		t.Fatalf("forget unknown status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestDeviceControlSoundWifiAndRebootFlow(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	var soundRequest rpcapi.ClientDeviceSoundPlayRequest
	var rebootRequest rpcapi.ClientDeviceRebootRequest
	var forgetRequest rpcapi.ClientWifiSavedForgetRequest
	dispatch := func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		switch req.Method {
		case rpcapi.RPCMethodClientDeviceSoundPlay:
			params, err := req.Params.AsClientDeviceSoundPlayRequest()
			if err != nil {
				return nil, err
			}
			soundRequest = params
			return newRPCResultResponse(req.Id, rpcapi.ClientDeviceSoundPlayResponse{}, (*rpcapi.RPCPayload).FromClientDeviceSoundPlayResponse)
		case rpcapi.RPCMethodClientDeviceReboot:
			params, err := req.Params.AsClientDeviceRebootRequest()
			if err != nil {
				return nil, err
			}
			rebootRequest = params
			return newRPCResultResponse(req.Id, rpcapi.ClientDeviceRebootResponse{}, (*rpcapi.RPCPayload).FromClientDeviceRebootResponse)
		case rpcapi.RPCMethodClientWifiStatusGet:
			return newRPCResultResponse(req.Id, rpcapi.ClientWifiStatusGetResponse{Value: rpcapi.WifiStatus{
				Connected: true, Ssid: new("home"), RssiDbm: new(int64(-61)), Ip: new("192.0.2.20"), Bssid: new("aa:bb:cc:dd:ee:ff"),
			}}, (*rpcapi.RPCPayload).FromClientWifiStatusGetResponse)
		case rpcapi.RPCMethodClientWifiSavedList:
			return newRPCResultResponse(req.Id, rpcapi.ClientWifiSavedListResponse{Networks: []rpcapi.WifiSavedNetwork{{Ssid: "home"}, {Ssid: "office"}}}, (*rpcapi.RPCPayload).FromClientWifiSavedListResponse)
		case rpcapi.RPCMethodClientWifiSavedForget:
			params, err := req.Params.AsClientWifiSavedForgetRequest()
			if err != nil {
				return nil, err
			}
			forgetRequest = params
			return newRPCResultResponse(req.Id, rpcapi.ClientWifiSavedForgetResponse{}, (*rpcapi.RPCPayload).FromClientWifiSavedForgetResponse)
		default:
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: "unsupported"}.RPCResponse(), nil
		}
	}
	device := newFakeDeviceConn(dispatch)
	f.manager.SetPeerUp(f.owner, device)

	response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/play-sound", `{"sound":"chime","duration_ms":1500}`)
	if response.Code != http.StatusNoContent {
		t.Fatalf("play-sound status = %d body=%s", response.Code, response.Body.String())
	}
	if soundRequest.Sound != "chime" || soundRequest.DurationMs == nil || *soundRequest.DurationMs != 1500 {
		t.Fatalf("device sound request = %+v", soundRequest)
	}
	for _, body := range []string{`{"sound":""}`, `{"sound":"` + string(make([]byte, 33)) + `"}`, `{"sound":"chime","duration_ms":-1}`, `{"sound":"123456789012345678901234567890123"}`} {
		if response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/play-sound", body); response.Code != http.StatusBadRequest {
			t.Fatalf("play-sound %q status = %d body=%s", body, response.Code, response.Body.String())
		}
	}
	if response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/play-sound", `{"sound":"12345678901234567890123456789012"}`); response.Code != http.StatusNoContent {
		t.Fatalf("play-sound 32-byte status = %d body=%s", response.Code, response.Body.String())
	}

	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi", "")
	if response.Code != http.StatusOK {
		t.Fatalf("wifi status = %d body=%s", response.Code, response.Body.String())
	}
	wifi := decodeJSON[peerhttp.DeviceWifiStatus](t, response)
	if !wifi.Connected || wifi.Ssid == nil || *wifi.Ssid != "home" || wifi.RssiDbm == nil || *wifi.RssiDbm != -61 || wifi.Ip == nil || *wifi.Ip != "192.0.2.20" || wifi.Bssid == nil || *wifi.Bssid != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("wifi = %+v", wifi)
	}
	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi/saved", "")
	if response.Code != http.StatusOK {
		t.Fatalf("wifi saved status = %d body=%s", response.Code, response.Body.String())
	}
	saved := decodeJSON[peerhttp.DeviceWifiSavedList](t, response)
	if len(saved.Networks) != 2 || saved.Networks[1].Ssid != "office" {
		t.Fatalf("saved = %+v", saved)
	}
	if response := f.do(t, http.MethodDelete, "/gizclaw/v1/device/wifi/saved/office", ""); response.Code != http.StatusNoContent {
		t.Fatalf("forget status = %d body=%s", response.Code, response.Body.String())
	}
	if forgetRequest.Ssid != "office" {
		t.Fatalf("device forget request = %+v", forgetRequest)
	}
	if response := f.do(t, http.MethodDelete, "/gizclaw/v1/device/wifi/saved/123456789012345678901234567890123", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("forget oversized status = %d body=%s", response.Code, response.Body.String())
	}

	response = f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/reboot", `{"delay_ms":2000}`)
	if response.Code != http.StatusNoContent {
		t.Fatalf("reboot status = %d body=%s", response.Code, response.Body.String())
	}
	if rebootRequest.DelayMs == nil || *rebootRequest.DelayMs != 2000 {
		t.Fatalf("device reboot request = %+v", rebootRequest)
	}
	calls := device.calls.Load()
	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi", "")
	if response.Code != http.StatusConflict || errorCode(t, response) != deviceOfflineCode {
		t.Fatalf("control after reboot status = %d body=%s", response.Code, response.Body.String())
	}
	if device.calls.Load() != calls {
		t.Fatal("control after reboot reached the rebooting device")
	}

	// A replacement connection clears the reboot marker.
	replacement := newFakeDeviceConn(dispatch)
	f.manager.SetPeerUp(f.owner, replacement)
	if response := f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi", ""); response.Code != http.StatusOK {
		t.Fatalf("control after reconnect status = %d body=%s", response.Code, response.Body.String())
	}
	if response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/reboot", ""); response.Code != http.StatusNoContent {
		t.Fatalf("reboot without body status = %d body=%s", response.Code, response.Body.String())
	}
	f.manager.SetPeerDown(f.owner, replacement)
	if response := f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi", ""); response.Code != http.StatusConflict {
		t.Fatalf("control after disconnect status = %d", response.Code)
	}
}

func TestDeviceControlSerializesCommandsPerOwner(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	var active atomic.Int32
	var overlap atomic.Bool
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		if active.Add(1) > 1 {
			overlap.Store(true)
		}
		time.Sleep(20 * time.Millisecond)
		active.Add(-1)
		return newRPCResultResponse(req.Id, rpcapi.ClientDeviceSoundPlayResponse{}, (*rpcapi.RPCPayload).FromClientDeviceSoundPlayResponse)
	})
	f.manager.SetPeerUp(f.owner, device)
	done := make(chan int, 4)
	for range 4 {
		go func() {
			done <- f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/play-sound", `{"sound":"chime"}`).Code
		}()
	}
	for range 4 {
		if code := <-done; code != http.StatusNoContent {
			t.Fatalf("concurrent play-sound status = %d", code)
		}
	}
	if overlap.Load() {
		t.Fatal("control commands for one owner overlapped on the device")
	}
	if device.calls.Load() != 4 {
		t.Fatalf("device calls = %d, want 4", device.calls.Load())
	}
}

var _ giznet.Conn = (*fakeDeviceConn)(nil)

// rebootingDevice is a fake device whose reboot acknowledgement blocks until
// release is closed, so tests can interleave other events deterministically.
func rebootingDevice(release <-chan struct{}, entered chan<- struct{}) *fakeDeviceConn {
	return newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		switch req.Method {
		case rpcapi.RPCMethodClientDeviceReboot:
			close(entered)
			<-release
			return newRPCResultResponse(req.Id, rpcapi.ClientDeviceRebootResponse{}, (*rpcapi.RPCPayload).FromClientDeviceRebootResponse)
		case rpcapi.RPCMethodClientWifiStatusGet:
			return newRPCResultResponse(req.Id, rpcapi.ClientWifiStatusGetResponse{Value: rpcapi.WifiStatus{Connected: true}}, (*rpcapi.RPCPayload).FromClientWifiStatusGetResponse)
		default:
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: "unsupported"}.RPCResponse(), nil
		}
	})
}

// TestDeviceControlRebootMarkerHoldsOwnerLock checks that a command queued
// behind an in-flight reboot observes the marker installed under the owner
// lock and never reaches the acknowledging device.
func TestDeviceControlRebootMarkerHoldsOwnerLock(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	f.control.timeout = 5 * time.Second
	entered := make(chan struct{})
	release := make(chan struct{})
	device := rebootingDevice(release, entered)
	f.manager.SetPeerUp(f.owner, device)

	rebootDone := make(chan int, 1)
	go func() { rebootDone <- f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/reboot", "").Code }()
	<-entered
	// The reboot holds the owner lock, so this command queues behind it.
	queuedDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { queuedDone <- f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi", "") }()
	close(release)

	if code := <-rebootDone; code != http.StatusNoContent {
		t.Fatalf("reboot status = %d", code)
	}
	queued := <-queuedDone
	if queued.Code != http.StatusConflict || errorCode(t, queued) != deviceOfflineCode {
		t.Fatalf("queued command after reboot status = %d body=%s", queued.Code, queued.Body.String())
	}
	if device.calls.Load() != 1 {
		t.Fatalf("device calls = %d; the queued command must not reach the rebooting device", device.calls.Load())
	}
}

// TestDeviceControlRebootMarkerIgnoresReconnectDuringAck checks that a device
// reconnecting while its reboot acknowledgement is in flight is not treated
// as rebooting: the marker is pinned to the connection that answered.
func TestDeviceControlRebootMarkerIgnoresReconnectDuringAck(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	f.control.timeout = 5 * time.Second
	entered := make(chan struct{})
	release := make(chan struct{})
	old := rebootingDevice(release, entered)
	f.manager.SetPeerUp(f.owner, old)

	rebootDone := make(chan int, 1)
	go func() {
		rebootDone <- f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/reboot", `{"delay_ms":10}`).Code
	}()
	<-entered
	replacement := rebootingDevice(make(chan struct{}), make(chan struct{}))
	f.manager.SetPeerUp(f.owner, replacement)
	close(release)
	if code := <-rebootDone; code != http.StatusNoContent {
		t.Fatalf("reboot status = %d", code)
	}

	if response := f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi", ""); response.Code != http.StatusOK {
		t.Fatalf("control on replacement status = %d body=%s", response.Code, response.Body.String())
	}
	if old.calls.Load() != 1 || replacement.calls.Load() != 1 {
		t.Fatalf("device calls old=%d replacement=%d", old.calls.Load(), replacement.calls.Load())
	}
}

// TestDeviceControlRebootMarkerNamesAcknowledgingConnection checks that when
// the device reconnects between the connection lookup and the RPC dispatch,
// the marker still names the connection that carried the acknowledgement.
func TestDeviceControlRebootMarkerNamesAcknowledgingConnection(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	old := rebootingDevice(closedChannel(), make(chan struct{}))
	replacement := rebootingDevice(closedChannel(), make(chan struct{}))
	old.onDial = func() { f.manager.SetPeerUp(f.owner, replacement) }
	f.manager.SetPeerUp(f.owner, old)

	if response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/reboot", ""); response.Code != http.StatusNoContent {
		t.Fatalf("reboot status = %d body=%s", response.Code, response.Body.String())
	}
	if old.calls.Load() != 1 || replacement.calls.Load() != 0 {
		t.Fatalf("reboot reached old=%d replacement=%d, want the looked-up connection only", old.calls.Load(), replacement.calls.Load())
	}
	// The old connection acknowledged and is marked; the replacement is live.
	if response := f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi", ""); response.Code != http.StatusOK {
		t.Fatalf("control on replacement status = %d body=%s", response.Code, response.Body.String())
	}
	if replacement.calls.Load() != 1 {
		t.Fatalf("replacement calls = %d", replacement.calls.Load())
	}

	// The acknowledging connection is what gets marked, whichever it is: a
	// reboot answered by the replacement blocks the replacement.
	if response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/reboot", ""); response.Code != http.StatusNoContent {
		t.Fatalf("second reboot status = %d", response.Code)
	}
	if response := f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi", ""); response.Code != http.StatusConflict {
		t.Fatalf("control after replacement reboot status = %d body=%s", response.Code, response.Body.String())
	}
	if replacement.calls.Load() != 2 {
		t.Fatalf("replacement calls after marked reboot = %d", replacement.calls.Load())
	}
}

func closedChannel() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
