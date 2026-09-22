package peer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestRegistrationFirmwareDoesNotConflictWithPeerUpdates(t *testing.T) {
	for _, projected := range []bool{false, true} {
		t.Run(map[bool]string{false: "unbound", true: "bound"}[projected], func(t *testing.T) {
			server := &Server{Store: mustBadgerInMemory(t, nil)}
			key := giznet.PublicKey{71}
			if _, err := server.EnsureConnectedPeer(t.Context(), key); err != nil {
				t.Fatal(err)
			}
			if projected {
				server.RegistrationFirmware = func(context.Context, string) (*string, error) {
					return new("registered-firmware"), nil
				}
			}
			if _, err := server.PutSelfInfo(t.Context(), key, apitypes.DeviceInfo{Name: new("Alice")}); err != nil {
				t.Fatalf("serial profile update: %v", err)
			}
			if _, err := server.SaveRefreshedDeviceFields(t.Context(), key,
				apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: new("device-sn")}},
				[]string{"device.identifiers.sn"}); err != nil {
				t.Fatalf("device refresh after registration: %v", err)
			}
			loaded, err := server.LoadPeer(t.Context(), key)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Device.Name == nil || *loaded.Device.Name != "Alice" || loaded.Device.Identifiers == nil || loaded.Device.Identifiers.Sn == nil || *loaded.Device.Identifiers.Sn != "device-sn" {
				t.Fatalf("profile or identifiers lost: %#v", loaded.Device)
			}
			if projected && (loaded.FirmwareId == nil || *loaded.FirmwareId != "registered-firmware") {
				t.Fatalf("registration projection lost: %#v", loaded.FirmwareId)
			}
			data, err := server.Store.Get(t.Context(), peerKey(key.String()))
			if err != nil {
				t.Fatal(err)
			}
			var stored apitypes.Peer
			if err := json.Unmarshal(data, &stored); err != nil {
				t.Fatal(err)
			}
			if stored.FirmwareId != nil {
				t.Fatalf("SQL projection written into KV: %q", *stored.FirmwareId)
			}
			matches, err := server.listBySN(t.Context(), "device-sn")
			if err != nil || len(matches) != 1 {
				t.Fatalf("SN index: %v %v", matches, err)
			}
		})
	}
}

func TestRegistrationFirmwareDeviceRefreshOrdering(t *testing.T) {
	for _, registerFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "refresh_before_registration", true: "refresh_after_registration"}[registerFirst], func(t *testing.T) {
			server := &Server{Store: mustBadgerInMemory(t, nil)}
			key := giznet.PublicKey{72}
			if _, err := server.EnsureConnectedPeer(t.Context(), key); err != nil {
				t.Fatal(err)
			}
			register := func() {
				server.RegistrationFirmware = func(context.Context, string) (*string, error) {
					return new("registered-firmware"), nil
				}
			}
			if registerFirst {
				register()
			}
			if _, err := server.SaveRefreshedDeviceFields(t.Context(), key,
				apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: new("giztest-device-sn")}},
				[]string{"device.identifiers.sn"}); err != nil {
				t.Fatalf("identifiers refresh: %v", err)
			}
			if !registerFirst {
				register()
			}
			info, err := server.GetSelfInfo(t.Context(), key)
			if err != nil {
				t.Fatal(err)
			}
			if info.Identifiers == nil || info.Identifiers.Sn == nil || *info.Identifiers.Sn != "giztest-device-sn" {
				t.Fatalf("identifiers not persisted: %#v", info.Identifiers)
			}
		})
	}
}
