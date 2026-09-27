package gizcli

import (
	"context"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestMhsProviderDispatchAndDiscovery(t *testing.T) {
	device := &Client{}
	value, err := proto.Marshal(&rpcpb.LedHwdWriteRequest{Enabled: new(false)})
	if err != nil {
		t.Fatal(err)
	}
	request := &rpcpb.ClientMhsV0WriteRequest{Id: "led.main", Hwd: rpcpb.ClientHwd_CLIENT_HWD_LED, Payload: value}
	call := func() *rpcapi.RPCResponse {
		return deviceControlDispatch(t, device, rpcapi.RPCMethodClientMhsV0Write, func(p *rpcapi.RPCPayload) error { return p.FromClientMhsV0WriteRequest(request) })
	}
	if response := call(); response.Error == nil || response.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatal(response)
	}
	calls := 0
	if err := device.HandleDeviceControl(DeviceControlHandlers{WriteMhsHwd: func(_ context.Context, in *rpcpb.ClientMhsV0WriteRequest) (*rpcpb.ClientMhsV0WriteResponse, error) {
		calls++
		var wanted rpcpb.LedHwdWriteRequest
		if err := proto.Unmarshal(in.Payload, &wanted); err != nil || wanted.Enabled == nil || *wanted.Enabled {
			t.Fatalf("lost false presence: %+v, %v", &wanted, err)
		}
		applied, err := proto.Marshal(&rpcpb.LedHwdWriteResponse{Applied: &rpcpb.LedHwdReadResponse{Enabled: new(false)}})
		if err != nil {
			return nil, err
		}
		return &rpcpb.ClientMhsV0WriteResponse{Payload: applied}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	response := call()
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	result, err := response.Result.AsClientMhsV0WriteResponse()
	if err != nil {
		t.Fatal(err)
	}
	var applied rpcpb.LedHwdWriteResponse
	if err := proto.Unmarshal(result.Payload, &applied); err != nil || applied.Applied == nil || applied.Applied.Enabled == nil || *applied.Applied.Enabled {
		t.Fatalf("write response %+v, %v", &applied, err)
	}
	methods := deviceControlDispatch(t, device, rpcapi.RPCMethodClientRPCMethodsList, nil)
	list, err := methods.Result.AsClientRpcMethodsListResponse()
	if err != nil || !slices.Contains(list.Methods, rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_WRITE) || slices.Contains(list.Methods, rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_READ) {
		t.Fatalf("%v %v", list, err)
	}
	request.Hwd = rpcpb.ClientHwd_CLIENT_HWD_BATTERY
	response = call()
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInvalidArgument || calls != 1 {
		t.Fatalf("read-only write %v calls %d", response, calls)
	}
	if err := device.HandleDeviceControl(DeviceControlHandlers{ReadMhsHwd: func(context.Context, *rpcpb.ClientMhsV0ReadRequest) (*rpcpb.ClientMhsV0ReadResponse, error) {
		return nil, ErrDeviceResourceNotFound
	}}); err != nil {
		t.Fatal(err)
	}
	response = deviceControlDispatch(t, device, rpcapi.RPCMethodClientMhsV0Read, func(p *rpcapi.RPCPayload) error {
		return p.FromClientMhsV0ReadRequest(&rpcpb.ClientMhsV0ReadRequest{Id: "led.main", Hwd: rpcpb.ClientHwd_CLIENT_HWD_LED})
	})
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound {
		t.Fatal(response)
	}
}
