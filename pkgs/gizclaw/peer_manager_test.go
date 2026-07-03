package gizclaw

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestManagerSetPeerDownDeletesPeer(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{1}
	conn := &testGiznetConn{}

	manager.SetPeerUp(key, conn)
	if runtime := manager.PeerRuntime(context.Background(), key); !runtime.Online {
		t.Fatalf("peer should be online before removal: %+v", runtime)
	}

	manager.SetPeerDown(key)
	if _, ok := manager.Peer(key); ok {
		t.Fatal("peer should be removed")
	}
	if runtime := manager.PeerRuntime(context.Background(), key); runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after removal = %+v", runtime)
	}
}

func TestManagerSetPeerUpReplacesConnection(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{1}
	oldConn := &testGiznetConn{}
	newConn := &testGiznetConn{}

	manager.SetPeerUp(key, oldConn)
	manager.SetPeerUp(key, newConn)

	got, ok := manager.Peer(key)
	if !ok || got != newConn {
		t.Fatalf("ActivePeer after replacement = %v, %v", got, ok)
	}
	if runtime := manager.PeerRuntime(context.Background(), key); !runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after replacement = %+v", runtime)
	}
}

func TestManagerSetPeerUpAndDownUpdatesRuntime(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{1}
	conn := &testGiznetConn{}

	manager.SetPeerUp(key, conn)
	if got, ok := manager.Peer(key); !ok || got != conn {
		t.Fatalf("active peer after set = %v, %v", got, ok)
	}
	if runtime := manager.PeerRuntime(context.Background(), key); !runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after set = %+v, want online with no peer info", runtime)
	}

	manager.SetPeerDown(key)
	if runtime := manager.PeerRuntime(context.Background(), key); runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after remove = %+v", runtime)
	}
}

func TestManagerEnsurePeerCreatesDefaultPeer(t *testing.T) {
	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	manager := NewManager(service)
	ctx := context.Background()
	key := giznet.PublicKey{1}

	created, err := manager.EnsurePeer(ctx, key)
	if err != nil {
		t.Fatalf("EnsurePeer error = %v", err)
	}
	if created.PublicKey != key.String() {
		t.Fatalf("PublicKey = %q, want %q", created.PublicKey, key.String())
	}
	if created.Role != apitypes.PeerRoleClient {
		t.Fatalf("Role = %q, want client", created.Role)
	}
	if created.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("Status = %q, want active", created.Status)
	}
	if created.AutoRegistered == nil || !*created.AutoRegistered {
		t.Fatalf("AutoRegistered = %v, want true", created.AutoRegistered)
	}

	loaded, err := service.LoadPeer(ctx, key)
	if err != nil {
		t.Fatalf("LoadPeer error = %v", err)
	}
	if loaded.Role != apitypes.PeerRoleClient || loaded.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("loaded peer = %+v", loaded)
	}
}

func TestManagerEnsurePeerPreservesExistingPeer(t *testing.T) {
	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	manager := NewManager(service)
	ctx := context.Background()
	key := giznet.PublicKey{1}
	if _, err := service.SavePeer(ctx, apitypes.Peer{
		PublicKey:     key.String(),
		Role:          apitypes.PeerRoleAdmin,
		Status:        apitypes.PeerRegistrationStatusBlocked,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}

	got, err := manager.EnsurePeer(ctx, key)
	if err != nil {
		t.Fatalf("EnsurePeer error = %v", err)
	}
	if got.Role != apitypes.PeerRoleAdmin || got.Status != apitypes.PeerRegistrationStatusBlocked {
		t.Fatalf("EnsurePeer overwrote existing peer: %+v", got)
	}
}

func TestManagerRefreshDeviceErrors(t *testing.T) {
	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	manager := NewManager(service)
	ctx := context.Background()
	missingKey := giznet.PublicKey{1}
	deviceKey := giznet.PublicKey{2}

	if _, _, err := manager.RefreshPeer(ctx, missingKey); !errors.Is(err, peer.ErrPeerNotFound) {
		t.Fatalf("RefreshPeer missing err = %v", err)
	}

	if _, err := service.SavePeer(ctx, apitypes.Peer{
		PublicKey:     deviceKey.String(),
		Role:          apitypes.PeerRoleUnspecified,
		Status:        apitypes.PeerRegistrationStatusUnspecified,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer error: %v", err)
	}

	if _, online, err := manager.RefreshPeer(ctx, deviceKey); !errors.Is(err, ErrDeviceOffline) {
		t.Fatalf("RefreshPeer offline err = %v", err)
	} else if online {
		t.Fatal("offline RefreshPeer should report online=false")
	}
}

func TestApplyPeerRefreshIdentifiersSkipsUnchangedCollections(t *testing.T) {
	name := "primary"
	sn := "sn-1"
	peer := apitypes.Peer{
		Device: apitypes.DeviceInfo{
			Sn: &sn,
			Hardware: &apitypes.HardwareInfo{
				Imeis: &[]apitypes.PeerIMEI{{
					Name:   &name,
					Tac:    "12345678",
					Serial: "0000001",
				}},
				Labels: &[]apitypes.PeerLabel{{
					Key:   "batch",
					Value: "cn-east",
				}},
			},
		},
	}
	identifiers := apitypes.RefreshIdentifiers{
		Sn: &sn,
		Imeis: &[]apitypes.PeerIMEI{{
			Name:   &name,
			Tac:    "12345678",
			Serial: "0000001",
		}},
		Labels: &[]apitypes.PeerLabel{{
			Key:   "batch",
			Value: "cn-east",
		}},
	}

	var updatedFields []string
	applyPeerRefreshIdentifiers(&peer, identifiers, &updatedFields)

	if len(updatedFields) != 0 {
		t.Fatalf("applyPeerRefreshIdentifiers() updatedFields = %v, want none", updatedFields)
	}
}

func TestApplyPeerRefreshIdentifiersUpdatesChangedCollections(t *testing.T) {
	name := "primary"
	nextName := "secondary"
	peer := apitypes.Peer{
		Device: apitypes.DeviceInfo{
			Hardware: &apitypes.HardwareInfo{
				Imeis: &[]apitypes.PeerIMEI{{
					Name:   &name,
					Tac:    "12345678",
					Serial: "0000001",
				}},
				Labels: &[]apitypes.PeerLabel{{
					Key:   "batch",
					Value: "cn-east",
				}},
			},
		},
	}
	identifiers := apitypes.RefreshIdentifiers{
		Imeis: &[]apitypes.PeerIMEI{{
			Name:   &nextName,
			Tac:    "87654321",
			Serial: "0000009",
		}},
		Labels: &[]apitypes.PeerLabel{{
			Key:   "batch",
			Value: "cn-west",
		}},
	}

	var updatedFields []string
	applyPeerRefreshIdentifiers(&peer, identifiers, &updatedFields)

	if len(updatedFields) != 2 {
		t.Fatalf("applyPeerRefreshIdentifiers() updatedFields = %v, want 2 entries", updatedFields)
	}
	if peer.Device.Hardware == nil || peer.Device.Hardware.Imeis == nil || (*peer.Device.Hardware.Imeis)[0].Tac != "87654321" {
		t.Fatalf("IMEIs not updated: %+v", peer.Device.Hardware)
	}
	if peer.Device.Hardware.Labels == nil || (*peer.Device.Hardware.Labels)[0].Value != "cn-west" {
		t.Fatalf("labels not updated: %+v", peer.Device.Hardware)
	}
}

func TestIsPeerDisconnectedError(t *testing.T) {
	t.Run("closed connection errors are offline", func(t *testing.T) {
		if !isPeerDisconnectedError(errors.New("gizhttp: read response: giznet: conn closed")) {
			t.Fatal("conn closed error should be treated as disconnected")
		}
	})

	t.Run("generic read response errors stay online", func(t *testing.T) {
		if isPeerDisconnectedError(errors.New("gizhttp: read response: malformed HTTP response")) {
			t.Fatal("generic read response error should not be treated as disconnected")
		}
	})
}
