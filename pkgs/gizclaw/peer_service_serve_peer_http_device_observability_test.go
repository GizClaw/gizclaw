package gizclaw

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
)

type controlLogCapture struct {
	mu   sync.Mutex
	last map[string]any
}

func (c *controlLogCapture) Write(p []byte) (int, error) {
	var event map[string]any
	if json.Unmarshal(p, &event) == nil && event["msg"] == observability.CompletionMessage && event["route"] == "/gizclaw/v1/device/tool/v0/invoke" {
		c.mu.Lock()
		c.last = event
		c.mu.Unlock()
	}
	return len(p), nil
}

func (c *controlLogCapture) take() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	event := c.last
	c.last = nil
	return event
}

type controlDialFailure struct{ testGiznetConn }

func (*controlDialFailure) Dial(uint64) (net.Conn, error) { return nil, net.ErrClosed }

func TestDeviceControlFailureReasonsInHTTPCompletion(t *testing.T) {
	fixture := newDeviceHTTPFixture(t)
	fixture.handler = observeHTTPHandler(fixture.handler, httpObservationOptions{surface: observability.SurfacePeerHTTP})
	capture := &controlLogCapture{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(capture, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	check := func(status int, code string, reason observability.DeviceControlFailureReason) {
		t.Helper()
		response := fixture.invoke(t, "device.status.get", `{}`)
		if response.Code != status || errorCode(t, response) != code {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body)
		}
		entry := capture.take()
		if entry["control_failure_reason"] != string(reason) || entry["error_code"] != code || entry["peer_public_key"] != fixture.owner.String() {
			t.Fatalf("completion = %v, want %s and %s on the authenticated request", entry, code, reason)
		}
	}

	check(http.StatusConflict, deviceOfflineCode, observability.ControlFailurePeerMissing)
	fixture.manager.SetPeerUp(fixture.owner, &controlDialFailure{})
	check(http.StatusConflict, deviceOfflineCode, observability.ControlFailureRPCDial)
	fixture.manager.SetPeerUp(fixture.owner, newFakeToolConn(func(_ context.Context, _ rpcpb.ClientTool, request *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		return rpcapi.Error{RequestID: request.Id, Code: rpcapi.StatusCodeUnavailable}.RPCResponse(), nil
	}))
	check(http.StatusConflict, deviceOfflineCode, observability.ControlFailureRPCUnavailable)
	fixture.manager.SetPeerUp(fixture.owner, newFakeToolConn(func(_ context.Context, _ rpcpb.ClientTool, request *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		return rpcapi.Error{RequestID: request.Id, Code: rpcapi.StatusCodeDeadlineExceeded}.RPCResponse(), nil
	}))
	check(http.StatusGatewayTimeout, deviceTimeoutCode, observability.ControlFailureRPCTimeout)

	makeDevice := func() *fakeDeviceConn {
		return newFakeToolConn(func(_ context.Context, tool rpcpb.ClientTool, request *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
			switch tool {
			case rpcpb.ClientTool_CLIENT_TOOL_WIFI_CONNECT:
				return newRPCResultResponse(request.Id, rpcapi.ClientWifiConnectResponse{}, (*rpcapi.RPCPayload).FromClientWifiConnectResponse)
			case rpcpb.ClientTool_CLIENT_TOOL_DEVICE_REBOOT:
				return newRPCResultResponse(request.Id, rpcapi.ClientDeviceRebootResponse{}, (*rpcapi.RPCPayload).FromClientDeviceRebootResponse)
			default:
				return deviceStatusResult(request.Id, rpcapi.PeerStatus{Volume: new(30)})
			}
		})
	}

	for _, transition := range []struct {
		tool   string
		args   string
		reason observability.DeviceControlFailureReason
	}{
		{"wifi.connect", `{"ssid":"fixture-network"}`, observability.ControlFailureWiFiTransition},
		{"device.reboot", `{}`, observability.ControlFailureRestartTransition},
	} {
		device := makeDevice()
		fixture.manager.SetPeerUp(fixture.owner, device)
		if response := fixture.invoke(t, transition.tool, transition.args); response.Code != http.StatusOK {
			t.Fatalf("%s acknowledgement = %d, body = %s", transition.tool, response.Code, response.Body)
		}
		check(http.StatusConflict, deviceOfflineCode, transition.reason)
		runtime := decodeJSON[apitypes.Runtime](t, fixture.do(t, http.MethodGet, "/gizclaw/v1/device/runtime", ""))
		if !runtime.Online || device.calls.Load() != 1 {
			t.Fatalf("%s runtime online = %t, device calls = %d", transition.tool, runtime.Online, device.calls.Load())
		}
		replacement := makeDevice()
		fixture.manager.SetPeerUp(fixture.owner, replacement)
		if response := fixture.invoke(t, "device.status.get", `{}`); response.Code != http.StatusOK {
			t.Fatalf("status after %s reconnect = %d, body = %s", transition.tool, response.Code, response.Body)
		}
		if entry := capture.take(); entry["control_failure_reason"] != nil || replacement.calls.Load() != 1 {
			t.Fatalf("status after %s inherited a failure reason or replayed an operation", transition.tool)
		}
	}
}
