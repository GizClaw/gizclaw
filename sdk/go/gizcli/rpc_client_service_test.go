package gizcli

import (
	"context"
	"net"
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
	client.observeClientRPC(rpcapi.RPCMethodClientInfoGet)
	if observed != rpcapi.RPCMethodClientInfoGet {
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
		Id: "invalid", Method: rpcapi.RPCMethodClientInfoGet, Params: &params,
	})
	if err != nil || response.Error == nil || calls.Load() != 0 {
		t.Fatalf("invalid dispatch = %#v, %v; observed calls = %d", response, err, calls.Load())
	}
}
