package gizcli

import (
	"context"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func TestMhsProviderDispatchAndDiscovery(t *testing.T) {
	device := &Client{}
	request := &rpcpb.ClientMhsV0WriteRequest{States: []*rpcpb.MhsStateValue{{DeviceId: "led.main", State: "enabled", Value: &rpcpb.MhsValue{Value: &rpcpb.MhsValue_BoolValue{BoolValue: false}}}}}
	call := func() *rpcapi.RPCResponse {
		return deviceControlDispatch(t, device, rpcapi.RPCMethodClientMhsV0Write, func(p *rpcapi.RPCPayload) error { return p.FromClientMhsV0WriteRequest(request) })
	}
	if response := call(); response.Error == nil || response.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatal(response)
	}
	calls := 0
	if err := device.HandleDeviceControl(DeviceControlHandlers{WriteMhsStates: func(_ context.Context, in *rpcpb.ClientMhsV0WriteRequest) (*rpcpb.ClientMhsV0WriteResponse, error) {
		calls++
		return &rpcpb.ClientMhsV0WriteResponse{States: in.States}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	response := call()
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	result, err := response.Result.AsClientMhsV0WriteResponse()
	if err != nil || len(result.States) != 1 || result.States[0].Value.Value == nil || result.States[0].Value.GetBoolValue() {
		t.Fatalf("%v %v", result, err)
	}
	methods := deviceControlDispatch(t, device, rpcapi.RPCMethodClientRPCMethodsGet, nil)
	list, err := methods.Result.AsClientRPCMethodsGetResponse()
	if err != nil || !slices.Contains(list.Methods, "client.mhs.v0.write") || slices.Contains(list.Methods, "client.mhs.v0.read") {
		t.Fatalf("%v %v", list, err)
	}
	request.States = append(request.States, request.States[0])
	response = call()
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInvalidArgument || calls != 1 {
		t.Fatalf("%v calls %d", response, calls)
	}
	if err := device.HandleDeviceControl(DeviceControlHandlers{ReadMhsStates: func(context.Context, *rpcpb.ClientMhsV0ReadRequest) (*rpcpb.ClientMhsV0ReadResponse, error) {
		return nil, ErrDeviceResourceNotFound
	}}); err != nil {
		t.Fatal(err)
	}
	response = deviceControlDispatch(t, device, rpcapi.RPCMethodClientMhsV0Read, func(p *rpcapi.RPCPayload) error {
		return p.FromClientMhsV0ReadRequest(&rpcpb.ClientMhsV0ReadRequest{States: []*rpcpb.MhsStateRef{{DeviceId: "led.main", State: "enabled"}}})
	})
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound {
		t.Fatal(response)
	}
}
