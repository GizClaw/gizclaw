package gizcli

import (
	"context"
	"errors"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCClientHandleDeviceInfoMethods(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	name := "main"
	device := &Client{Device: apitypes.DeviceInfo{
		Hardware: &apitypes.HardwareInfo{
			Manufacturer: new("Acme"),
			Model:        new("M1"),
		},
		Identifiers: &apitypes.DeviceIdentifiers{
			Sn: new("sn-1"),
			Imeis: &[]apitypes.PeerIMEI{{
				Name:   &name,
				Tac:    "12345678",
				Serial: "0000001",
			}},
		},
	}}

	errCh := make(chan error, 1)
	go func() {
		errCh <- (&rpcClient{peer: device}).Handle(clientSide)
	}()

	caller := &rpcClient{}
	info, err := caller.GetClientInfo(context.Background(), serverSide, "device-info")
	if err != nil {
		t.Fatalf("GetClientInfo() error = %v", err)
	}
	if info.Manufacturer == nil || *info.Manufacturer != "Acme" {
		t.Fatalf("GetClientInfo() = %+v", info)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Handle(info) error = %v", err)
	}

	serverSide, clientSide = net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	errCh = make(chan error, 1)
	go func() {
		errCh <- (&rpcClient{peer: device}).Handle(clientSide)
	}()

	identifiers, err := caller.GetClientIdentifiers(context.Background(), serverSide, "device-identifiers")
	if err != nil {
		t.Fatalf("GetClientIdentifiers() error = %v", err)
	}
	if identifiers.Sn == nil || *identifiers.Sn != "sn-1" || identifiers.Imeis == nil || len(*identifiers.Imeis) != 1 {
		t.Fatalf("GetClientIdentifiers() = %+v", identifiers)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Handle(identifiers) error = %v", err)
	}
}

func TestObserveClientRPC(t *testing.T) {
	client := &Client{}
	var observed rpcapi.RPCMethod
	if err := client.ObserveClientRPC(func(method rpcapi.RPCMethod) { observed = method }); err != nil {
		t.Fatal(err)
	}
	client.observeClientRPC(rpcapi.RPCMethodClientToolV0Invoke)
	if observed != rpcapi.RPCMethodClientToolV0Invoke {
		t.Fatalf("observed = %q", observed)
	}
}

func TestObserveClientRPCSkipsInvalidRequest(t *testing.T) {
	client := &Client{}
	var calls atomic.Int32
	if err := client.ObserveClientRPC(func(rpcapi.RPCMethod) { calls.Add(1) }); err != nil {
		t.Fatal(err)
	}
	var params rpcapi.RPCPayload
	if err := params.FromPingRequest(rpcapi.PingRequest{ClientSendTime: 1}); err != nil {
		t.Fatal(err)
	}
	response, err := (&rpcClient{peer: client}).dispatch(t.Context(), &rpcapi.RPCRequest{
		Id: "invalid", Method: rpcapi.RPCMethodClientToolV0Invoke, Params: &params,
	})
	if err != nil || response.Error == nil || calls.Load() != 0 {
		t.Fatalf("invalid dispatch = %#v, %v; observed calls = %d", response, err, calls.Load())
	}
}

func TestRPCClientToolV0RegistrationAndDispatch(t *testing.T) {
	device := &Client{}
	for _, tool := range []rpcpb.ClientTool{0, -1, 22, 999} {
		if err := device.HandleClientTool(tool, func(context.Context, proto.Message) (proto.Message, error) { return nil, nil }); err == nil {
			t.Fatalf("unknown tool %d accepted", tool)
		}
	}
	calls := 0
	if err := device.HandleClientTool(rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY, func(_ context.Context, message proto.Message) (proto.Message, error) {
		request := message.(*rpcpb.ClientDeviceSoundPlayRequest)
		calls++
		if request.Sound != "chime" {
			t.Fatalf("sound %q", request.Sound)
		}
		return &rpcpb.ClientDeviceSoundPlayResponse{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	response := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceSoundPlayRequest(rpcapi.ClientDeviceSoundPlayRequest{Sound: "chime"})
	})
	if response.Error != nil || calls != 1 {
		t.Fatalf("response %v calls %d", response, calls)
	}
	list := deviceControlDispatch(t, device, rpcapi.RPCMethodClientToolV0List, nil)
	tools, err := list.Result.AsClientToolV0ListResponse()
	if err != nil || len(tools.Tools) != 3 || tools.Tools[2] != rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY {
		t.Fatalf("tools %v %v", tools, err)
	}
	var invalid rpcapi.RPCPayload
	if err := invalid.FromClientToolV0InvokeRequest(&rpcpb.ClientToolV0InvokeRequest{Tool: rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY, Payload: []byte{10, 128}}); err != nil {
		t.Fatal(err)
	}
	response, err = (&rpcClient{peer: device}).dispatch(t.Context(), &rpcapi.RPCRequest{Id: "bad", Method: rpcapi.RPCMethodClientToolV0Invoke, Params: &invalid})
	if err != nil || response.Error == nil || response.Error.Code != rpcapi.StatusCodeInvalidArgument || calls != 1 {
		t.Fatalf("malformed invocation reached handler: %v %v %d", response, err, calls)
	}
	response = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceSoundPlayRequest(rpcapi.ClientDeviceSoundPlayRequest{Sound: strings.Repeat("x", 33)})
	})
	if response.Error == nil || calls != 1 {
		t.Fatal("oversized sound reached handler")
	}
	if err := device.HandleClientTool(rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY, nil); err != nil {
		t.Fatal(err)
	}
	response = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceSoundPlayRequest(rpcapi.ClientDeviceSoundPlayRequest{Sound: "chime"})
	})
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatalf("uninstalled tool: %v", response)
	}
	list = deviceControlDispatch(t, device, rpcapi.RPCMethodClientToolV0List, nil)
	tools, err = list.Result.AsClientToolV0ListResponse()
	if err != nil || len(tools.Tools) != 2 {
		t.Fatalf("removed tool still advertised: %v %v", tools, err)
	}
}

func TestRPCClientToolV0HandlerErrorsAreRedacted(t *testing.T) {
	for name, handler := range map[string]ClientToolHandler{
		"error": func(context.Context, proto.Message) (proto.Message, error) {
			return nil, errors.New("secret internal failure")
		},
		"wrong response type": func(context.Context, proto.Message) (proto.Message, error) { return &rpcpb.PingResponse{}, nil },
		"nil response":        func(context.Context, proto.Message) (proto.Message, error) { return nil, nil },
	} {
		t.Run(name, func(t *testing.T) {
			device := &Client{}
			if err := device.HandleClientTool(rpcpb.ClientTool_CLIENT_TOOL_DEVICE_REBOOT, handler); err != nil {
				t.Fatal(err)
			}
			response := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_REBOOT, nil)
			if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInternal || strings.Contains(response.Error.Message, "secret") {
				t.Fatalf("response %v", response)
			}
		})
	}
}
