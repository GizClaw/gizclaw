package peer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
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
