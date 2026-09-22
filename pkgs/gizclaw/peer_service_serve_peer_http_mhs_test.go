package gizclaw

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func seedMhs(t *testing.T, f *deviceHTTPFixture) {
	t.Helper()
	seedRuntimeProfile(t, f, f.owner, "mhs", apitypes.RuntimeProfileSpec{Mhs: &apitypes.RuntimeProfileMhs{V0: &apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{{Id: "display.main", Kind: "display", States: []apitypes.MhsV0State{
		{Name: "brightness", Type: "int", Access: "read_write", Min: new(0.0), Max: new(100.0), Step: new(5.0)},
		{Name: "enabled", Type: "bool", Access: "read_write"},
		{Name: "mode", Type: "enum", Access: "read_write", EnumValues: new([]string{"auto", "off"})},
		{Name: "label", Type: "string", Access: "read_write"},
		{Name: "voltage", Type: "double", Access: "read"},
	}}}}}})
}

func TestMhsManifestOfflineAndNoConfiguration(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/mhs/v0/manifest", "")
	if response.Code != 200 || strings.TrimSpace(response.Body.String()) != `{"devices":[]}` {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
	seedMhs(t, f)
	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/mhs/v0/manifest", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"id":"display.main"`) {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "resources") {
		t.Fatal("profile leaked")
	}
}

func TestMhsHTTPValidationAndAppliedValues(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedMhs(t, f)
	calls := 0
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		calls++
		switch req.Method {
		case rpcapi.RPCMethodClientMhsV0Read:
			request, err := req.Params.AsClientMhsV0ReadRequest()
			if err != nil {
				return nil, err
			}
			state := request.States[0]
			return newRPCResultResponse(req.Id, &rpcpb.ClientMhsV0ReadResponse{States: []*rpcpb.MhsStateValue{{DeviceId: state.DeviceId, State: state.State, Value: &rpcpb.MhsValue{Value: &rpcpb.MhsValue_IntValue{IntValue: 40}}}}}, (*rpcapi.RPCPayload).FromClientMhsV0ReadResponse)
		case rpcapi.RPCMethodClientMhsV0Write:
			request, err := req.Params.AsClientMhsV0WriteRequest()
			if err != nil {
				return nil, err
			}
			request.States[0].Value = &rpcpb.MhsValue{Value: &rpcpb.MhsValue_IntValue{IntValue: 40}}
			return newRPCResultResponse(req.Id, &rpcpb.ClientMhsV0WriteResponse{States: request.States}, (*rpcapi.RPCPayload).FromClientMhsV0WriteResponse)
		default:
			t.Fatalf("unexpected %s", req.Method)
			return nil, nil
		}
	})
	f.manager.SetPeerUp(f.owner, device)
	for _, body := range []string{
		``, `{}`, `{"states":[]}`, `{"states":null}`,
		`{"states":[{"device_id":"display.main","state":"brightness"}]}`,
		`{"states":[{"device_id":"missing","state":"brightness","value":50}]}`,
		`{"states":[{"device_id":"display.main","state":"missing","value":50}]}`,
		`{"states":[{"device_id":"display.main","state":"voltage","value":1}]}`,
		`{"states":[{"device_id":"display.main","state":"brightness","value":50},{"device_id":"display.main","state":"brightness","value":60}]}`,
		`{"states":[{"device_id":"display.main","state":"brightness","value":1.5}]}`,
		`{"states":[{"device_id":"display.main","state":"brightness","value":101}]}`,
		`{"states":[{"device_id":"display.main","state":"brightness","value":-1}]}`,
		`{"states":[{"device_id":"display.main","state":"brightness","value":52}]}`,
		`{"states":[{"device_id":"display.main","state":"brightness","value":"50"}]}`,
		`{"states":[{"device_id":"display.main","state":"enabled","value":0}]}`,
		`{"states":[{"device_id":"display.main","state":"mode","value":"bad"}]}`,
		`{"states":[{"device_id":"display.main","state":"label","value":null}]}`,
		`{"states":[{"device_id":"display.main","state":"label","value":"\u0000"}]}`,
		`{"states":[{"device_id":"display.main","state":"label","value":"` + strings.Repeat("中", 86) + `"}]}`,
		`{"states":[` + strings.TrimSuffix(strings.Repeat(`{"device_id":"display.main","state":"brightness","value":50},`, 33), ",") + `]}`,
	} {
		response := f.do(t, http.MethodPatch, "/gizclaw/v1/device/mhs/v0/states", body)
		if response.Code != 400 {
			t.Fatalf("%s: %d %s", body, response.Code, response.Body)
		}
	}
	for _, body := range []string{`{}`, `{"states":[]}`, `{"states":[{"device_id":"missing","state":"brightness"}]}`, `{"states":[{"device_id":"display.main","state":"brightness"},{"device_id":"display.main","state":"brightness"}]}`} {
		response := f.do(t, http.MethodPost, "/gizclaw/v1/device/mhs/v0/read", body)
		if response.Code != 400 {
			t.Fatalf("%d %s", response.Code, response.Body)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid requests reached device %d times", calls)
	}
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "read", `{"states":[{"device_id":"display.main","state":"brightness"}]}`},
		{http.MethodPatch, "states", `{"states":[{"device_id":"display.main","state":"brightness","value":50}]}`},
	} {
		response := f.do(t, tc.method, "/gizclaw/v1/device/mhs/v0/"+tc.path, tc.body)
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"value":40`) {
			t.Fatalf("%d %s", response.Code, response.Body)
		}
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestMhsHTTPDeviceErrors(t *testing.T) {
	for _, tc := range []struct {
		code rpcapi.StatusCode
		http int
		name string
	}{
		{rpcapi.StatusCodeNotFound, 404, mhsStateNotFoundCode},
		{rpcapi.StatusCodeInvalidArgument, 400, deviceRejectedCode},
		{rpcapi.StatusCodeFailedPrecondition, 502, deviceErrorCode},
		{rpcapi.StatusCodeUnimplemented, 501, deviceUnsupportedCode},
		{rpcapi.StatusCodeDeadlineExceeded, 504, deviceTimeoutCode},
		{rpcapi.StatusCodeUnavailable, 409, deviceOfflineCode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeviceHTTPFixture(t)
			seedMhs(t, f)
			f.manager.SetPeerUp(f.owner, newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
				return rpcapi.Error{RequestID: req.Id, Code: tc.code, Message: "device-private-detail"}.RPCResponse(), nil
			}))
			for _, verb := range []string{"read", "write"} {
				method, path, body := http.MethodPost, "read", `{"states":[{"device_id":"display.main","state":"brightness"}]}`
				if verb == "write" {
					method, path, body = http.MethodPatch, "states", `{"states":[{"device_id":"display.main","state":"brightness","value":50}]}`
				}
				response := f.do(t, method, "/gizclaw/v1/device/mhs/v0/"+path, body)
				if response.Code != tc.http || errorCode(t, response) != tc.name || strings.Contains(response.Body.String(), "device-private-detail") {
					t.Fatalf("%d %s", response.Code, response.Body)
				}
			}
		})
	}
	f := newDeviceHTTPFixture(t)
	seedMhs(t, f)
	response := f.do(t, http.MethodPost, "/gizclaw/v1/device/mhs/v0/read", `{"states":[{"device_id":"display.main","state":"brightness"}]}`)
	if response.Code != 409 {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
}

func TestMhsHTTPMalformedResponse(t *testing.T) {
	for _, states := range [][]*rpcpb.MhsStateValue{nil, {{DeviceId: "display.main", State: "brightness"}}, {{DeviceId: "display.main", State: "brightness", Value: &rpcpb.MhsValue{Value: &rpcpb.MhsValue_StringValue{StringValue: "50"}}}}} {
		f := newDeviceHTTPFixture(t)
		seedMhs(t, f)
		f.manager.SetPeerUp(f.owner, newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
			if req.Method == rpcapi.RPCMethodClientMhsV0Read {
				return newRPCResultResponse(req.Id, &rpcpb.ClientMhsV0ReadResponse{States: states}, (*rpcapi.RPCPayload).FromClientMhsV0ReadResponse)
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientMhsV0WriteResponse{States: states}, (*rpcapi.RPCPayload).FromClientMhsV0WriteResponse)
		}))
		for _, tc := range []struct{ method, path, body string }{{http.MethodPost, "read", `{"states":[{"device_id":"display.main","state":"brightness"}]}`}, {http.MethodPatch, "states", `{"states":[{"device_id":"display.main","state":"brightness","value":50}]}`}} {
			response := f.do(t, tc.method, "/gizclaw/v1/device/mhs/v0/"+tc.path, tc.body)
			if response.Code != 502 || errorCode(t, response) != deviceErrorCode {
				t.Fatalf("%d %s", response.Code, response.Body)
			}
		}
	}
}
