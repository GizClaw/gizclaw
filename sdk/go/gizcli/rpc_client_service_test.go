package gizcli

import (
	"context"
	"net"
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

func TestRPCClientHandleToolInvoke(t *testing.T) {
	device := &Client{ToolInvoker: func(_ context.Context, request rpcapi.ToolInvokeRequest) (rpcapi.ToolInvokeResponse, error) {
		if request.ToolId != "peer.device.music.play" || request.Method != "music.play" || request.Args["query"] != "song" {
			t.Fatalf("ToolInvoker request = %#v", request)
		}
		return rpcapi.ToolInvokeResponse{DataJson: `{"playing":true}`}, nil
	}}
	var params rpcapi.RPCPayload
	if err := params.FromToolInvokeRequest(rpcapi.ToolInvokeRequest{CallId: "call", ToolId: "peer.device.music.play", Method: "music.play", Args: map[string]any{"query": "song"}}); err != nil {
		t.Fatalf("FromToolInvokeRequest() error = %v", err)
	}
	resp, err := (&rpcClient{peer: device}).dispatch(context.Background(), &rpcapi.RPCRequest{Id: "invoke", Method: rpcapi.RPCMethodClientToolInvoke, Params: &params})
	if err != nil || resp.Error != nil || resp.Result == nil {
		t.Fatalf("dispatch() = %#v, %v", resp, err)
	}
	result, err := resp.Result.AsToolInvokeResponse()
	if err != nil || string(result.DataJson) != `{"playing":true}` {
		t.Fatalf("tool result = %s, %v", result.DataJson, err)
	}

	resp, err = (&rpcClient{peer: &Client{}}).dispatch(context.Background(), &rpcapi.RPCRequest{Id: "invoke", Method: rpcapi.RPCMethodClientToolInvoke, Params: &params})
	if err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeMethodNotFound {
		t.Fatalf("dispatch(no handler) = %#v, %v", resp, err)
	}
}
