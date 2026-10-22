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

func TestNFCInstancesAndModemPresence(t *testing.T) {
	device := &Client{}
	left := &rpcpb.NfcHwdReadResponse{Enabled: new(true), CardPresent: new(false), LastRead: &rpcpb.NfcReadSnapshot{ReadAtUnixMs: 1000, Uid: []byte{0, 1}, Result: "failed", ErrorCode: new("READ_FAILED")}}
	right := &rpcpb.NfcHwdReadResponse{Enabled: new(false), CardPresent: new(false)}
	states := map[string]*rpcpb.NfcHwdReadResponse{"nfc.left": left, "nfc.right": right}
	if err := device.HandleDeviceControl(DeviceControlHandlers{
		ReadMhsHwd: func(_ context.Context, r *rpcpb.ClientMhsV0ReadRequest) (*rpcpb.ClientMhsV0ReadResponse, error) {
			var value proto.Message
			if r.Hwd == rpcpb.ClientHwd_CLIENT_HWD_MODEM {
				value = &rpcpb.ModemHwdReadResponse{Enabled: new(false), Registered: new(false)}
			} else {
				value = states[r.Id]
				if value == nil {
					return nil, ErrDeviceResourceNotFound
				}
			}
			data, err := proto.Marshal(value)
			return &rpcpb.ClientMhsV0ReadResponse{Payload: data}, err
		},
		WriteMhsHwd: func(_ context.Context, r *rpcpb.ClientMhsV0WriteRequest) (*rpcpb.ClientMhsV0WriteResponse, error) {
			state := states[r.Id]
			if state == nil {
				return nil, ErrDeviceResourceNotFound
			}
			var write rpcpb.NfcHwdWriteRequest
			if err := proto.Unmarshal(r.Payload, &write); err != nil {
				return nil, err
			}
			state.Enabled = write.Enabled
			data, err := proto.Marshal(&rpcpb.NfcHwdWriteResponse{Applied: state})
			return &rpcpb.ClientMhsV0WriteResponse{Payload: data}, err
		},
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"nfc.left", "nfc.right", "nfc.left"} {
		response := deviceControlDispatch(t, device, rpcapi.RPCMethodClientMhsV0Read, func(p *rpcapi.RPCPayload) error {
			return p.FromClientMhsV0ReadRequest(&rpcpb.ClientMhsV0ReadRequest{Id: id, Hwd: rpcpb.ClientHwd_CLIENT_HWD_NFC})
		})
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		outer, err := response.Result.AsClientMhsV0ReadResponse()
		if err != nil {
			t.Fatal(err)
		}
		var read rpcpb.NfcHwdReadResponse
		if err = proto.Unmarshal(outer.Payload, &read); err != nil {
			t.Fatal(err)
		}
		if read.CardPresent == nil || *read.CardPresent {
			t.Fatal("card absence lost")
		}
		if (read.LastRead != nil) != (id == "nfc.left") {
			t.Fatal("NFC instances shared cached reads")
		}
		if id == "nfc.left" && (read.LastRead.Result != "failed" || read.LastRead.ReadAtUnixMs != 1000) {
			t.Fatal("removed card lost most recent failed attempt")
		}
	}
	data, _ := proto.Marshal(&rpcpb.NfcHwdWriteRequest{Enabled: new(false)})
	result := deviceControlDispatch(t, device, rpcapi.RPCMethodClientMhsV0Write, func(p *rpcapi.RPCPayload) error {
		return p.FromClientMhsV0WriteRequest(&rpcpb.ClientMhsV0WriteRequest{Id: "nfc.left", Hwd: rpcpb.ClientHwd_CLIENT_HWD_NFC, Payload: data})
	})
	if result.Error != nil || left.GetEnabled() || right.GetEnabled() || left.LastRead == nil || right.LastRead != nil {
		t.Fatal("instance write leaked or lost cache")
	}
	modem := deviceControlDispatch(t, device, rpcapi.RPCMethodClientMhsV0Read, func(p *rpcapi.RPCPayload) error {
		return p.FromClientMhsV0ReadRequest(&rpcpb.ClientMhsV0ReadRequest{Id: "modem.main", Hwd: rpcpb.ClientHwd_CLIENT_HWD_MODEM})
	})
	out, err := modem.Result.AsClientMhsV0ReadResponse()
	if err != nil {
		t.Fatal(err)
	}
	var read rpcpb.ModemHwdReadResponse
	if err = proto.Unmarshal(out.Payload, &read); err != nil || read.Enabled == nil || *read.Enabled || read.Powered != nil {
		t.Fatal("modem configuration/power presence conflated")
	}
}
