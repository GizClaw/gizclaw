package gizclaw

import (
	"context"
	"net"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestRPCClientHandleDeviceInfoMethods(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	name := "main"
	device := &Client{Device: apitypes.DeviceInfo{
		Name: stringPtr("gear-1"),
		Sn:   stringPtr("sn-1"),
		Hardware: &apitypes.HardwareInfo{
			Manufacturer: stringPtr("Acme"),
			Model:        stringPtr("M1"),
			Imeis: &[]apitypes.GearIMEI{{
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
	info, err := caller.GetDeviceInfo(context.Background(), serverSide, "device-info")
	if err != nil {
		t.Fatalf("GetDeviceInfo() error = %v", err)
	}
	if info.Name == nil || *info.Name != "gear-1" || info.Manufacturer == nil || *info.Manufacturer != "Acme" {
		t.Fatalf("GetDeviceInfo() = %+v", info)
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

	identifiers, err := caller.GetDeviceIdentifiers(context.Background(), serverSide, "device-identifiers")
	if err != nil {
		t.Fatalf("GetDeviceIdentifiers() error = %v", err)
	}
	if identifiers.Sn == nil || *identifiers.Sn != "sn-1" || identifiers.Imeis == nil || len(*identifiers.Imeis) != 1 {
		t.Fatalf("GetDeviceIdentifiers() = %+v", identifiers)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Handle(identifiers) error = %v", err)
	}
}
