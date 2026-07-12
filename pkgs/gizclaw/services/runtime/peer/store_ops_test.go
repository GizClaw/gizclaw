package peer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestStoreOpsHelpers(t *testing.T) {
	server := &Server{}
	if _, err := server.store(); err == nil {
		t.Fatal("store should fail when store is nil")
	}
	if (&Server{}).peerRuntime(context.Background(), giznet.PublicKey{1}).Online {
		t.Fatal("zero peerRuntime should be offline")
	}
	if optionalPeer(apitypes.Peer{PublicKey: giznet.PublicKey{1}.String()}, nil) == nil {
		t.Fatal("optionalPeer should keep value")
	}
	if optionalPeer(apitypes.Peer{}, errors.New("boom")) != nil {
		t.Fatal("optionalPeer should drop error case")
	}
}

func TestStoreOpsEnsureConnectedPeerValidation(t *testing.T) {
	server := &Server{
		Store: mustBadgerInMemory(t, nil),
	}

	_, err := server.EnsureConnectedPeer(context.Background(), giznet.PublicKey{})
	if err == nil || !strings.Contains(err.Error(), "empty public key") {
		t.Fatalf("empty public key err = %v", err)
	}
}

func TestStoreOpsEnsureConnectedPeer(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	ctx := context.Background()
	publicKey := giznet.PublicKey{1}

	connected, err := server.EnsureConnectedPeer(ctx, publicKey)
	if err != nil {
		t.Fatalf("EnsureConnectedPeer error = %v", err)
	}
	if connected.Role != apitypes.PeerRoleClient || connected.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("connected peer = %+v", connected)
	}
	if connected.AutoRegistered == nil || !*connected.AutoRegistered {
		t.Fatalf("connected auto_registered = %+v", connected.AutoRegistered)
	}
}

func TestPeerDisplayName(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	ctx := context.Background()
	publicKey := giznet.PublicKey{1}
	name := "  Ada  "
	if _, err := server.SavePeer(ctx, apitypes.Peer{
		PublicKey:     publicKey.String(),
		Role:          apitypes.PeerRoleClient,
		Status:        apitypes.PeerRegistrationStatusActive,
		Device:        apitypes.DeviceInfo{Name: &name},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}

	got, err := server.PeerDisplayName(ctx, publicKey.String())
	if err != nil {
		t.Fatalf("PeerDisplayName error = %v", err)
	}
	if got != "Ada" {
		t.Fatalf("PeerDisplayName = %q, want Ada", got)
	}
	if _, err := server.PeerDisplayName(ctx, "not-a-public-key"); err == nil {
		t.Fatal("PeerDisplayName invalid public key error = nil")
	}
}

func TestStoreOpsEnsureConnectedPeerPreservesExisting(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	ctx := context.Background()
	publicKey := giznet.PublicKey{1}
	if _, err := server.SavePeer(ctx, apitypes.Peer{
		PublicKey:     publicKey.String(),
		Role:          apitypes.PeerRoleAdmin,
		Status:        apitypes.PeerRegistrationStatusBlocked,
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}

	got, err := server.EnsureConnectedPeer(ctx, publicKey)
	if err != nil {
		t.Fatalf("EnsureConnectedPeer error = %v", err)
	}
	if got.Role != apitypes.PeerRoleAdmin || got.Status != apitypes.PeerRegistrationStatusBlocked {
		t.Fatalf("EnsureConnectedPeer overwrote existing peer: %+v", got)
	}
}

func TestStoreOpsBootstrapEdgeNodesCreatesAndUpdatesPeers(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	ctx := context.Background()
	existingKey := giznet.PublicKey{1}
	newKey := giznet.PublicKey{2}
	createdAt := time.Unix(100, 0).UTC()
	deviceName := "existing"
	if _, err := server.SavePeer(ctx, apitypes.Peer{
		PublicKey: existingKey.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusBlocked,
		Device: apitypes.DeviceInfo{
			Name: &deviceName,
		},
		Configuration: apitypes.Configuration{},
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}

	if err := server.BootstrapEdgeNodes(ctx, []giznet.PublicKey{existingKey, newKey}); err != nil {
		t.Fatalf("BootstrapEdgeNodes error = %v", err)
	}

	existing, err := server.LoadPeer(ctx, existingKey)
	if err != nil {
		t.Fatalf("LoadPeer(existing) error = %v", err)
	}
	if existing.Role != apitypes.PeerRoleEdgeNode || existing.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("existing peer role/status = %+v", existing)
	}
	if existing.CreatedAt != createdAt || existing.Device.Name == nil || *existing.Device.Name != deviceName {
		t.Fatalf("existing peer metadata not preserved: %+v", existing)
	}

	created, err := server.LoadPeer(ctx, newKey)
	if err != nil {
		t.Fatalf("LoadPeer(new) error = %v", err)
	}
	if created.Role != apitypes.PeerRoleEdgeNode || created.Status != apitypes.PeerRegistrationStatusActive || created.PublicKey != newKey.String() {
		t.Fatalf("new edge peer = %+v", created)
	}
}

func TestStoreOpsBootstrapEdgeNodesRejectsZeroKey(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	if err := server.BootstrapEdgeNodes(context.Background(), []giznet.PublicKey{{}}); err == nil || !strings.Contains(err.Error(), "empty edge-node public key") {
		t.Fatalf("BootstrapEdgeNodes zero key error = %v", err)
	}
}

func TestStoreOpsLoadAndSavePeer(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	publicKey := giznet.PublicKey{1}
	want := apitypes.Peer{
		PublicKey: publicKey.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusActive,
		Device: apitypes.DeviceInfo{
			Name: func() *string {
				value := "demo"
				return &value
			}(),
		},
		Configuration: apitypes.Configuration{},
	}

	got, err := server.SavePeer(context.Background(), want)
	if err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}
	if got.PublicKey != want.PublicKey {
		t.Fatalf("SavePeer public key = %q, want %q", got.PublicKey, want.PublicKey)
	}

	loaded, err := server.LoadPeer(context.Background(), publicKey)
	if err != nil {
		t.Fatalf("LoadPeer error = %v", err)
	}
	if loaded.PublicKey != want.PublicKey || loaded.Role != want.Role || loaded.Status != want.Status {
		t.Fatalf("LoadPeer = %+v", loaded)
	}
	if loaded.Device.Name == nil || *loaded.Device.Name != "demo" {
		t.Fatalf("LoadPeer device name = %+v", loaded.Device.Name)
	}
}

func TestStoreOpsLoadPeerMissing(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}

	_, err := server.LoadPeer(context.Background(), giznet.PublicKey{1})
	if !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("LoadPeer missing err = %v", err)
	}
}

func TestStoreOpsSavePeerRejectsInvalidPeer(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}

	_, err := server.SavePeer(context.Background(), apitypes.Peer{})
	if err == nil || !strings.Contains(err.Error(), "empty key") {
		t.Fatalf("SavePeer invalid err = %v", err)
	}

}

func TestStoreOpsExists(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	publicKey := giznet.PublicKey{1}

	if exists, err := server.exists(context.Background(), publicKey); err != nil || exists {
		t.Fatalf("exists(missing) = %v, %v", exists, err)
	}

	if _, err := server.SavePeer(context.Background(), apitypes.Peer{
		PublicKey:     publicKey.String(),
		Role:          apitypes.PeerRoleClient,
		Status:        apitypes.PeerRegistrationStatusActive,
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}

	if exists, err := server.exists(context.Background(), publicKey); err != nil || !exists {
		t.Fatalf("exists(peer) = %v, %v", exists, err)
	}
}
