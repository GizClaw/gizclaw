package gizclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func TestGNSSReportingInvokeForwardsWithoutStatusWrite(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	enabled := true
	device := newFakeToolConn(func(_ context.Context, tool rpcpb.ClientTool, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		switch tool {
		case rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_GET:
			return newRPCResultResponse(req.Id, &rpcpb.ClientGnssReportingGetResponse{Enabled: new(enabled)}, (*rpcapi.RPCPayload).FromClientGnssReportingGetResponse)
		case rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_SET:
			request, err := req.Params.AsClientGnssReportingSetRequest()
			if err != nil || request.Enabled == nil {
				t.Fatalf("set request = %v, %v", request, err)
			}
			enabled = request.GetEnabled()
			return newRPCResultResponse(req.Id, &rpcpb.ClientGnssReportingSetResponse{Enabled: new(enabled)}, (*rpcapi.RPCPayload).FromClientGnssReportingSetResponse)
		default:
			t.Fatalf("unexpected tool %v", tool)
			return nil, nil
		}
	})
	f.manager.SetPeerUp(f.owner, device)
	before := f.do(t, http.MethodGet, "/gizclaw/v1/device/status", "").Body.String()
	for _, value := range []bool{true, false, false, true} {
		args, err := json.Marshal(map[string]bool{"enabled": value})
		if err != nil {
			t.Fatal(err)
		}
		response := f.invoke(t, "gnss.reporting.set", string(args))
		if response.Code != http.StatusOK {
			t.Fatalf("set = %d %s", response.Code, response.Body)
		}
		result := decodeToolResult[map[string]bool](t, response)
		if got, present := result["enabled"]; !present || got != value {
			t.Fatalf("set result = %v", result)
		}
		response = f.invoke(t, "gnss.reporting.get", `{}`)
		if response.Code != http.StatusOK {
			t.Fatalf("get = %d %s", response.Code, response.Body)
		}
		result = decodeToolResult[map[string]bool](t, response)
		if got, present := result["enabled"]; !present || got != value {
			t.Fatalf("get result = %v", result)
		}
	}
	calls := device.calls.Load()
	for _, args := range []string{`{}`, `{"enabled":null}`, `{"enabled":"false"}`, `{"enabled":0}`, `{"enabled":false,"extra":true}`} {
		response := f.invoke(t, "gnss.reporting.set", args)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid set %s = %d %s", args, response.Code, response.Body)
		}
	}
	if response := f.invoke(t, "gnss.reporting.get", `{"enabled":true}`); response.Code != http.StatusBadRequest {
		t.Fatalf("get arguments = %d %s", response.Code, response.Body)
	}
	if device.calls.Load() != calls {
		t.Fatal("invalid arguments reached the device")
	}
	if after := f.do(t, http.MethodGet, "/gizclaw/v1/device/status", "").Body.String(); after != before {
		t.Fatalf("invoke changed Server status: before=%s after=%s", before, after)
	}
}

func TestGNSSReportingInvokeRequiresDeviceValue(t *testing.T) {
	for _, name := range []string{"gnss.reporting.get", "gnss.reporting.set"} {
		t.Run(name, func(t *testing.T) {
			f := newDeviceHTTPFixture(t)
			device := newFakeToolConn(func(_ context.Context, tool rpcpb.ClientTool, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
				if tool == rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_GET {
					return newRPCResultResponse(req.Id, &rpcpb.ClientGnssReportingGetResponse{}, (*rpcapi.RPCPayload).FromClientGnssReportingGetResponse)
				}
				return newRPCResultResponse(req.Id, &rpcpb.ClientGnssReportingSetResponse{}, (*rpcapi.RPCPayload).FromClientGnssReportingSetResponse)
			})
			f.manager.SetPeerUp(f.owner, device)
			args := `{}`
			if name == "gnss.reporting.set" {
				args = `{"enabled":false}`
			}
			response := f.invoke(t, name, args)
			if response.Code != http.StatusBadGateway || errorCode(t, response) != deviceErrorCode {
				t.Fatalf("empty result = %d %s", response.Code, response.Body)
			}
		})
	}
}
