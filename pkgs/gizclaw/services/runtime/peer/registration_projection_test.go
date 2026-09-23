package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestRegistrationProjectionFailureDoesNotCommitProfile(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	key := giznet.PublicKey{73}
	if _, err := server.EnsureConnectedPeer(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	before, err := server.Store.Get(t.Context(), peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	projectionErr := errors.New("registration lookup failed")
	server.RegistrationFirmware = func(context.Context, string) (*string, error) {
		return nil, projectionErr
	}
	if _, err := server.PutSelfInfo(t.Context(), key, apitypes.DeviceInfo{Name: new("Alice")}); !errors.Is(err, projectionErr) {
		t.Fatalf("profile update error = %v, want %v", err, projectionErr)
	}
	after, err := server.Store.Get(t.Context(), peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("profile was committed despite registration lookup failure")
	}
}

func TestRegistrationProjectionIsNotRetriedAfterCommit(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	key := giznet.PublicKey{74}
	if _, err := server.EnsureConnectedPeer(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server.RegistrationFirmware = func(context.Context, string) (*string, error) {
		calls++
		if calls > 1 {
			return nil, errors.New("registration lookup failed after commit")
		}
		return new("registered-firmware"), nil
	}
	updated, err := server.putInfo(t.Context(), key, apitypes.DeviceInfo{Name: new("Alice")})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("registration lookup calls = %d, want 1", calls)
	}
	if updated.Device.Name == nil || *updated.Device.Name != "Alice" || updated.FirmwareId == nil || *updated.FirmwareId != "registered-firmware" {
		t.Fatalf("committed profile or projection missing: %#v", updated)
	}
}

func TestRegistrationProjectionFailureDoesNotCommitBlock(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	key := giznet.PublicKey{75}
	if _, err := server.EnsureConnectedPeer(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	before, err := server.Store.Get(t.Context(), peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	projectionErr := errors.New("registration lookup failed")
	server.RegistrationFirmware = func(context.Context, string) (*string, error) {
		return nil, projectionErr
	}
	if _, err := server.block(t.Context(), key); !errors.Is(err, projectionErr) {
		t.Fatalf("block error = %v, want %v", err, projectionErr)
	}
	after, err := server.Store.Get(t.Context(), peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("block was committed despite registration lookup failure")
	}
}

func TestRegistrationProjectionFailureDoesNotCommitConnectedPeer(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	key := giznet.PublicKey{76}
	projectionErr := errors.New("registration lookup failed")
	server.RegistrationFirmware = func(context.Context, string) (*string, error) {
		return nil, projectionErr
	}
	if _, err := server.EnsureConnectedPeer(t.Context(), key); !errors.Is(err, projectionErr) {
		t.Fatalf("connected Peer creation error = %v, want %v", err, projectionErr)
	}
	if _, err := server.Store.Get(t.Context(), peerKey(key.String())); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("connected Peer was committed despite registration lookup failure: %v", err)
	}
}

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
