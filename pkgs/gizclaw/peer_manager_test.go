package gizclaw

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestManagerSetPeerDownDeletesMatchingPeer(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{1}
	conn := &testGiznetConn{}

	if oldConn := manager.SetPeerUp(key, conn); oldConn != nil {
		t.Fatalf("SetPeerUp first oldConn = %v, want nil", oldConn)
	}
	if runtime := manager.PeerRuntime(context.Background(), key); !runtime.Online {
		t.Fatalf("peer should be online before removal: %+v", runtime)
	}

	manager.SetPeerDown(key, conn)
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

	if replaced := manager.SetPeerUp(key, oldConn); replaced != nil {
		t.Fatalf("first SetPeerUp replaced = %v, want nil", replaced)
	}
	if replaced := manager.SetPeerUp(key, newConn); replaced != oldConn {
		t.Fatalf("second SetPeerUp replaced = %v, want old conn", replaced)
	}

	got, ok := manager.Peer(key)
	if !ok || got != newConn {
		t.Fatalf("ActivePeer after replacement = %v, %v", got, ok)
	}
	manager.SetPeerDown(key, oldConn)
	got, ok = manager.Peer(key)
	if !ok || got != newConn {
		t.Fatalf("stale SetPeerDown removed active peer: %v, %v", got, ok)
	}
	manager.SetPeerDown(key, newConn)
	if _, ok := manager.Peer(key); ok {
		t.Fatal("matching SetPeerDown should remove active peer")
	}
	if runtime := manager.PeerRuntime(context.Background(), key); runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after matching down = %+v", runtime)
	}
}

func TestManagerSetPeerUpSameConnectionDoesNotReplace(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{1}
	conn := &testGiznetConn{}

	if replaced := manager.SetPeerUp(key, conn); replaced != nil {
		t.Fatalf("first SetPeerUp replaced = %v, want nil", replaced)
	}
	if replaced := manager.SetPeerUp(key, conn); replaced != nil {
		t.Fatalf("same-conn SetPeerUp replaced = %v, want nil", replaced)
	}
	if runtime := manager.PeerRuntime(context.Background(), key); !runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after same-conn replacement = %+v", runtime)
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

	manager.SetPeerDown(key, conn)
	if runtime := manager.PeerRuntime(context.Background(), key); runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after remove = %+v", runtime)
	}
}

func TestManagerForcePeerDownRemovesActivePeer(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{1}
	conn := &testGiznetConn{}

	manager.SetPeerUp(key, conn)
	manager.ForcePeerDown(key)
	if _, ok := manager.Peer(key); ok {
		t.Fatal("ForcePeerDown should remove active peer")
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

func TestApplyPeerRefreshIdentifiersSkipsUnchangedCollections(t *testing.T) {
	name := "primary"
	sn := "sn-1"
	peer := apitypes.Peer{
		Device: apitypes.DeviceInfo{
			Identifiers: &apitypes.DeviceIdentifiers{
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
			},
		},
	}
	identifiers := apitypes.DeviceIdentifiers{
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
			Identifiers: &apitypes.DeviceIdentifiers{
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
	identifiers := apitypes.DeviceIdentifiers{
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
	if peer.Device.Identifiers == nil || peer.Device.Identifiers.Imeis == nil || (*peer.Device.Identifiers.Imeis)[0].Tac != "87654321" {
		t.Fatalf("IMEIs not updated: %+v", peer.Device.Identifiers)
	}
	if peer.Device.Identifiers.Labels == nil || (*peer.Device.Identifiers.Labels)[0].Value != "cn-west" {
		t.Fatalf("labels not updated: %+v", peer.Device.Identifiers)
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
