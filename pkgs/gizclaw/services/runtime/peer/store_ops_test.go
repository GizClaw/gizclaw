package peer

import (
	"context"
	"errors"
	"strings"
	"testing"

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

func TestStoreOpsBootstrapEdgeNodesRejectsZeroKey(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	if err := server.BootstrapEdgeNodes(context.Background(), []giznet.PublicKey{{}}); err == nil || !strings.Contains(err.Error(), "empty edge-node public key") {
		t.Fatalf("BootstrapEdgeNodes zero key error = %v", err)
	}
}

func TestStoreOpsLoadPeerMissing(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}

	_, err := server.LoadPeer(context.Background(), giznet.PublicKey{1})
	if !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("LoadPeer missing err = %v", err)
	}
}

func TestBindFirmwarePersistsReleaseLine(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	publicKey := giznet.PublicKey{1}
	if _, err := server.EnsureConnectedPeer(context.Background(), publicKey); err != nil {
		t.Fatal(err)
	}
	bound, err := server.BindFirmware(context.Background(), publicKey, " h106 ")
	if err != nil {
		t.Fatal(err)
	}
	if bound.FirmwareId == nil || *bound.FirmwareId != "h106" {
		t.Fatalf("BindFirmware() = %#v, want h106", bound)
	}
	loaded, err := server.LoadPeer(context.Background(), publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.FirmwareId == nil || *loaded.FirmwareId != "h106" {
		t.Fatalf("LoadPeer() = %#v, want h106", loaded)
	}
	projected := toAdminRegistration(loaded)
	if projected.FirmwareId == nil || *projected.FirmwareId != "h106" {
		t.Fatalf("toAdminRegistration() = %#v, want h106", projected)
	}
	if _, err := server.BindFirmware(context.Background(), publicKey, " "); err == nil {
		t.Fatal("BindFirmware(empty) error = nil")
	}
}

func TestStoreOpsSavePeerRejectsInvalidPeer(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}

	_, err := server.SavePeer(context.Background(), apitypes.Peer{})
	if err == nil || !strings.Contains(err.Error(), "empty key") {
		t.Fatalf("SavePeer invalid err = %v", err)
	}

}
