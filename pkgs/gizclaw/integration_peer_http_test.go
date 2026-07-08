package gizclaw_test

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestIntegrationPeerHTTPAutoPeerAndReadBack(t *testing.T) {
	ts := startTestServer(t)
	device := newTestClient(t, ts)
	if device.PeerConn() == nil {
		t.Fatal("PeerConn returned nil")
	}

	publicKey := ensurePeerInfo(t, device, apitypes.DeviceInfo{
		Name: strPtr("demo-device"),
		Sn:   strPtr("sn-001"),
		Hardware: &apitypes.HardwareInfo{
			Manufacturer: strPtr("Acme"),
			Model:        strPtr("M1"),
		},
	})
	if publicKey == "" {
		t.Fatal("empty public key after auto peer setup")
	}

	info, err := getInfo(context.Background(), device)
	if err != nil {
		t.Fatalf("GetInfo error: %v", err)
	}
	if info.Name == nil || *info.Name != "demo-device" {
		t.Fatalf("device name = %+v", info.Name)
	}

	peer, err := ts.server.Manager().Peers.LoadPeer(context.Background(), device.KeyPair.Public)
	if err != nil {
		t.Fatalf("LoadPeer error: %v", err)
	}
	if peer.Role != apitypes.PeerRoleClient {
		t.Fatalf("role = %q", peer.Role)
	}

	if _, err := getServerInfo(context.Background(), device); err != nil {
		t.Fatalf("GetServerInfo error: %v", err)
	}
	if _, err := putInfo(context.Background(), device, apitypes.DeviceInfo{
		Name: strPtr("demo-device-2"),
		Sn:   strPtr("sn-002"),
	}); err != nil {
		t.Fatalf("PutInfo error: %v", err)
	}
	if _, err := getRuntime(context.Background(), device); err != nil {
		t.Fatalf("GetRuntime error: %v", err)
	}
}
