package gizclaw

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func seedMhs(t *testing.T, f *deviceHTTPFixture) {
	t.Helper()
	seedRuntimeProfile(t, f, f.owner, "mhs", apitypes.RuntimeProfileSpec{Mhs: &apitypes.RuntimeProfileMhs{V0: &apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{
		{Id: "display.main", Hwd: "display"},
		{Id: "battery.main", Hwd: "battery"},
		{Id: "led.left", Hwd: "led"},
		{Id: "led.right", Hwd: "led"},
	}}}})
}

func TestMhsManifestOfflineAndInstances(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/mhs/v0/manifest", "")
	if response.Code != 200 || strings.TrimSpace(response.Body.String()) != `{"devices":[]}` {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
	seedMhs(t, f)
	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/mhs/v0/manifest", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"id":"led.left"`) || !strings.Contains(response.Body.String(), `"id":"led.right"`) || !strings.Contains(response.Body.String(), `"hwd":"led"`) {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "resources") || strings.Contains(response.Body.String(), "states") {
		t.Fatalf("manifest leaked unrelated fields: %s", response.Body)
	}
}

func TestMhsHTTPValidationAndAppliedValues(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedMhs(t, f)
	calls := 0
	f.manager.SetPeerUp(f.owner, newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		calls++
		switch req.Method {
		case rpcapi.RPCMethodClientMhsV0Read:
			request, err := req.Params.AsClientMhsV0ReadRequest()
			if err != nil || request.Id != "display.main" || request.Hwd != rpcpb.ClientHwd_CLIENT_HWD_DISPLAY {
				t.Fatalf("read request %+v, %v", request, err)
			}
			payload, err := proto.Marshal(&rpcpb.DisplayHwdReadResponse{BrightnessPercent: proto.Uint32(40)})
			if err != nil {
				return nil, err
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientMhsV0ReadResponse{Payload: payload}, (*rpcapi.RPCPayload).FromClientMhsV0ReadResponse)
		case rpcapi.RPCMethodClientMhsV0Write:
			request, err := req.Params.AsClientMhsV0WriteRequest()
			if err != nil || request.Id != "display.main" || request.Hwd != rpcpb.ClientHwd_CLIENT_HWD_DISPLAY {
				t.Fatalf("write request %+v, %v", request, err)
			}
			var value rpcpb.DisplayHwdWriteRequest
			if err := proto.Unmarshal(request.Payload, &value); err != nil || value.BrightnessPercent == nil || *value.BrightnessPercent != 50 {
				t.Fatalf("write payload %+v, %v", &value, err)
			}
			payload, err := proto.Marshal(&rpcpb.DisplayHwdWriteResponse{Applied: &rpcpb.DisplayHwdReadResponse{BrightnessPercent: proto.Uint32(40)}})
			if err != nil {
				return nil, err
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientMhsV0WriteResponse{Payload: payload}, (*rpcapi.RPCPayload).FromClientMhsV0WriteResponse)
		default:
			t.Fatalf("unexpected %s", req.Method)
			return nil, nil
		}
	}))
	for _, body := range []string{
		``, `{}`, `{"id":"display.main","hwd":"battery","value":{"brightness_percent":50}}`,
		`{"id":"battery.main","hwd":"battery","value":{"percent":50}}`,
		`{"id":"missing","hwd":"display","value":{"brightness_percent":50}}`,
		`{"id":"display.main","hwd":"display","value":{}}`,
		`{"id":"display.main","hwd":"display","value":{"brightness_percent":101}}`,
		`{"id":"display.main","hwd":"display","value":{"brightness_percent":"50"}}`,
		`{"id":"display.main","hwd":"display","value":{"unexpected":true}}`,
	} {
		response := f.do(t, http.MethodPost, "/gizclaw/v1/device/mhs/v0/write", body)
		if response.Code != 400 {
			t.Fatalf("%s: %d %s", body, response.Code, response.Body)
		}
	}
	for _, body := range []string{`{}`, `{"id":"missing","hwd":"display"}`, `{"id":"display.main","hwd":"battery"}`} {
		response := f.do(t, http.MethodPost, "/gizclaw/v1/device/mhs/v0/read", body)
		if response.Code != 400 {
			t.Fatalf("%s: %d %s", body, response.Code, response.Body)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid requests reached device %d times", calls)
	}
	for _, tc := range []struct{ path, body string }{
		{"read", `{"id":"display.main","hwd":"display"}`},
		{"write", `{"id":"display.main","hwd":"display","value":{"brightness_percent":50}}`},
	} {
		response := f.do(t, http.MethodPost, "/gizclaw/v1/device/mhs/v0/"+tc.path, tc.body)
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"brightness_percent":40`) {
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
		{rpcapi.StatusCodeNotFound, 404, mhsHwdNotFoundCode},
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
			for _, call := range []struct{ path, body string }{
				{"read", `{"id":"display.main","hwd":"display"}`},
				{"write", `{"id":"display.main","hwd":"display","value":{"brightness_percent":50}}`},
			} {
				response := f.do(t, http.MethodPost, "/gizclaw/v1/device/mhs/v0/"+call.path, call.body)
				if response.Code != tc.http || errorCode(t, response) != tc.name || strings.Contains(response.Body.String(), "device-private-detail") {
					t.Fatalf("%d %s", response.Code, response.Body)
				}
			}
		})
	}
}

func TestMhsHTTPMalformedResponse(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedMhs(t, f)
	f.manager.SetPeerUp(f.owner, newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		if req.Method == rpcapi.RPCMethodClientMhsV0Read {
			return newRPCResultResponse(req.Id, &rpcpb.ClientMhsV0ReadResponse{Payload: []byte{0xff}}, (*rpcapi.RPCPayload).FromClientMhsV0ReadResponse)
		}
		return newRPCResultResponse(req.Id, &rpcpb.ClientMhsV0WriteResponse{}, (*rpcapi.RPCPayload).FromClientMhsV0WriteResponse)
	}))
	for _, call := range []struct{ path, body string }{
		{"read", `{"id":"display.main","hwd":"display"}`},
		{"write", `{"id":"display.main","hwd":"display","value":{"brightness_percent":50}}`},
	} {
		response := f.do(t, http.MethodPost, "/gizclaw/v1/device/mhs/v0/"+call.path, call.body)
		if response.Code != 502 || errorCode(t, response) != deviceErrorCode {
			t.Fatalf("%d %s", response.Code, response.Body)
		}
	}
}
