package gizcli

import (
	"context"
	"net"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestRPCClientHandleDeviceInfoMethods(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	name := "main"
	device := &Client{Device: apitypes.DeviceInfo{
		Name: new("peer-1"),
		Sn:   new("sn-1"),
		Hardware: &apitypes.HardwareInfo{
			Manufacturer: new("Acme"),
			Model:        new("M1"),
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
	if info.Name == nil || *info.Name != "peer-1" || info.Manufacturer == nil || *info.Manufacturer != "Acme" {
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
